package v1alpha1

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/doctor"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/rsync"
	"github.com/walheimlab/walheim-go/internal/ssh"
)

// ── KindInfo & validation ─────────────────────────────────────────────────────

var jobKind = resource.KindInfo{
	Group:   "walheim",
	Version: "v1alpha1",
	Kind:    "Job",
	Plural:  "jobs",
	Aliases: []string{},
}

func validateJobManifest(m *JobManifest, namespace, name string) error {
	if m.APIVersion != jobKind.APIVersion() {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", jobKind.APIVersion(), m.APIVersion)
	}
	if m.Kind != jobKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", jobKind.Kind, m.Kind)
	}
	if m.Metadata.Name != name {
		return fmt.Errorf("metadata.name %q does not match argument %q", m.Metadata.Name, name)
	}
	if m.Metadata.Namespace != namespace {
		return fmt.Errorf("metadata.namespace %q does not match -n %q", m.Metadata.Namespace, namespace)
	}
	if m.Spec.Image == "" {
		return fmt.Errorf("spec.image is required")
	}
	return nil
}

// ── Handler ───────────────────────────────────────────────────────────────────

// Job is the handler for the Job resource kind.
type Job struct {
	resource.NamespacedBase
}

func newJob(dataDir string, filesystem fs.FS) *Job {
	return &Job{
		NamespacedBase: resource.NamespacedBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             jobKind,
			ManifestFilename: ".job.yaml",
		},
	}
}

func (j *Job) KindInfo() resource.KindInfo { return jobKind }

func (j *Job) loadNamespaceManifest(namespace string) (*NamespaceManifest, error) {
	path := filepath.Join(j.DataDir, "namespaces", namespace, ".namespace.yaml")
	data, err := j.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("namespace %q not found", namespace)
	}
	var m NamespaceManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ── Status querying ───────────────────────────────────────────────────────────

type jobRunInfo struct {
	queried    bool
	state      string // "running", "exited", "created", …
	statusText string // e.g. "Exited (0) 2 hours ago", "Up 3 minutes"
	createdAt  string // e.g. "2024-01-15 10:30:00 +0000 UTC"
}

// prefetchJobStatus queries each unique SSH host once (concurrently) and
// returns a map keyed by "namespace/name" → jobRunInfo (most recent run).
// A sentinel key "_ns_<namespace>" records whether the host was reachable.
func (j *Job) prefetchJobStatus(namespaces []string) map[string]jobRunInfo {
	type nsHost struct{ ns, host string }
	var pairs []nsHost
	seen := map[string]bool{}
	for _, ns := range namespaces {
		m, err := j.loadNamespaceManifest(ns)
		if err != nil {
			continue
		}
		host := m.Spec.sshTarget()
		if !seen[host] {
			seen[host] = true
			pairs = append(pairs, nsHost{ns, host})
		}
	}

	mu := sync.Mutex{}
	results := map[string]jobRunInfo{}
	var wg sync.WaitGroup

	for _, p := range pairs {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := ssh.NewClient(p.host)
			out, err := client.RunOutput(
				`docker ps -a` +
					` --filter "label=walheim.managed=true"` +
					` --filter "label=walheim.job"` +
					` --format '{{.Label "walheim.namespace"}}|{{.Label "walheim.job"}}|{{.State}}|{{.Status}}|{{.CreatedAt}}'`)
			mu.Lock()
			defer mu.Unlock()
			results["_ns_"+p.ns] = jobRunInfo{queried: err == nil}
			if err != nil {
				return
			}
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// SplitN 5: ns|job|state|statusText|createdAt
				// (statusText and createdAt may contain spaces)
				parts := strings.SplitN(line, "|", 5)
				if len(parts) < 5 {
					continue
				}
				ns, name := parts[0], parts[1]
				key := ns + "/" + name
				// docker ps -a returns newest-first; only record the first (most recent).
				if _, exists := results[key]; !exists {
					results[key] = jobRunInfo{
						queried:    true,
						state:      parts[2],
						statusText: parts[3],
						createdAt:  parts[4],
					}
				}
			}
		}()
	}
	wg.Wait()
	return results
}

// aggregateJobStatus derives STATUS and LAST RUN from the prefetched map.
func aggregateJobStatus(results map[string]jobRunInfo, namespace, name string) (status, lastRun string) {
	if !results["_ns_"+namespace].queried {
		return "Unknown", "-"
	}
	info, ok := results[namespace+"/"+name]
	if !ok || info.state == "" {
		return "Never", "-"
	}

	switch info.state {
	case "running":
		status = "Running"
	case "exited":
		if strings.Contains(info.statusText, "(0)") {
			status = "Succeeded"
		} else {
			status = "Failed"
		}
	case "created":
		status = "Pending"
	default:
		status = "Unknown"
	}

	// Show date + time only (drop timezone suffix).
	parts := strings.Fields(info.createdAt)
	if len(parts) >= 2 {
		lastRun = parts[0] + " " + parts[1]
	} else {
		lastRun = info.createdAt
	}
	return
}

// ── Compose file generation ───────────────────────────────────────────────────

// generateJobCompose builds a docker-compose.yml for the job and writes it to
// <localResourceDir>/docker-compose.yml. Mount files are also written under
// <localResourceDir>/mounts/ so they can be rsynced together.
func generateJobCompose(localResourceDir, ns, name string, spec JobSpec, filesystem fs.FS, dataDir string) error {
	// Resolve env (envFrom + env overrides).
	env := make(map[string]string)
	for _, entry := range spec.EnvFrom {
		var kvMap map[string]string
		var err error
		if entry.SecretRef != nil {
			kvMap, err = loadSecret(ns, entry.SecretRef.Name, filesystem, dataDir)
		} else if entry.ConfigMapRef != nil {
			kvMap, err = loadConfigMap(ns, entry.ConfigMapRef.Name, filesystem, dataDir)
		} else {
			continue
		}
		if err != nil {
			return fmt.Errorf("envFrom: %w", err)
		}
		for k, v := range kvMap {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}
	}
	for _, entry := range spec.Env {
		env[entry.Name] = substituteVars(entry.Value, env)
	}

	svc := ComposeService{
		Image: spec.Image,
		Environment: ServiceEnv{Values: env},
		Labels: ServiceLabels{Values: map[string]string{
			"walheim.managed":   "true",
			"walheim.namespace": ns,
			"walheim.job":       name,
		}},
		Extra: map[string]any{
			"restart": "no",
		},
	}

	// Write mount files and add volumes.
	for _, entry := range spec.Mounts {
		var kvMap map[string]string
		var sourceType, sourceName string
		var err error
		if entry.SecretRef != nil {
			kvMap, err = loadSecret(ns, entry.SecretRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("mounts: %w", err)
			}
			sourceType, sourceName = "secrets", entry.SecretRef.Name
		} else if entry.ConfigMapRef != nil {
			kvMap, err = loadConfigMap(ns, entry.ConfigMapRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("mounts: %w", err)
			}
			sourceType, sourceName = "configmaps", entry.ConfigMapRef.Name
		} else {
			continue
		}
		if err := writeMountFiles(localResourceDir, sourceType, sourceName, kvMap, filesystem); err != nil {
			return err
		}
		existing, _ := svc.Extra["volumes"].([]any)
		svc.Extra["volumes"] = append(existing, fmt.Sprintf("./mounts/%s/%s:%s:ro", sourceType, sourceName, entry.MountPath))
	}

	if len(spec.Command) > 0 {
		svc.Extra["command"] = spec.Command
	}

	compose := ComposeSpec{
		Services: map[string]ComposeService{"job": svc},
	}
	encoded, err := yaml.Marshal(compose)
	if err != nil {
		return fmt.Errorf("marshal docker-compose: %w", err)
	}
	composePath := filepath.Join(localResourceDir, "docker-compose.yml")
	return filesystem.WriteFile(composePath, encoded)
}

// ── Typed read/list helpers ───────────────────────────────────────────────────

func (j *Job) parseManifest(namespace, name string) (*JobManifest, error) {
	data, err := j.ReadBytes(namespace, name)
	if err != nil {
		return nil, err
	}
	var m JobManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse job %q in namespace %q: %w", name, namespace, err)
	}
	return &m, nil
}

func jobToMeta(namespace, name string, m *JobManifest, status, lastRun string) resource.ResourceMeta {
	return resource.ResourceMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    m.Metadata.Labels,
		Summary: map[string]string{
			"IMAGE":    m.Spec.Image,
			"STATUS":   status,
			"LAST RUN": lastRun,
		},
		Raw: m,
	}
}

func (j *Job) getOne(namespace, name string) (resource.ResourceMeta, *JobManifest, error) {
	exists, err := j.Exists(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}
	if !exists {
		return resource.ResourceMeta{}, nil,
			exitcode.New(exitcode.NotFound, fmt.Errorf("job %q not found in namespace %q", name, namespace))
	}
	m, err := j.parseManifest(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}
	return jobToMeta(namespace, name, m, "Configured", "-"), m, nil
}

func (j *Job) listNamespace(namespace string) ([]*JobManifest, []string, error) {
	names, err := j.ListNames(namespace)
	if err != nil {
		return nil, nil, err
	}
	var manifests []*JobManifest
	var validNames []string
	for _, name := range names {
		m, err := j.parseManifest(namespace, name)
		if err != nil {
			output.Warnf("skipping job %q in namespace %q: %v", name, namespace, err)
			continue
		}
		manifests = append(manifests, m)
		validNames = append(validNames, name)
	}
	return manifests, validNames, nil
}

// ── Operations ────────────────────────────────────────────────────────────────

func (j *Job) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name != "" {
		namespace := opts.Namespace
		_, m, err := j.getOne(namespace, opts.Name)
		if err != nil {
			output.Errorf(jsonMode, "NotFound",
				fmt.Sprintf("job %q not found in namespace %q", opts.Name, namespace), "", nil, false)
			return err
		}
		statusMap := j.prefetchJobStatus([]string{namespace})
		status, lastRun := aggregateJobStatus(statusMap, namespace, opts.Name)
		return output.PrintOne(jobToMeta(namespace, opts.Name, m, status, lastRun), jsonMode)
	}

	if opts.AllNamespaces {
		namespaces, err := j.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}
		statusMap := j.prefetchJobStatus(namespaces)
		var items []resource.ResourceMeta
		for _, ns := range namespaces {
			manifests, names, err := j.listNamespace(ns)
			if err != nil {
				return exitErr(exitcode.Failure, err)
			}
			for i, m := range manifests {
				status, lastRun := aggregateJobStatus(statusMap, ns, names[i])
				items = append(items, jobToMeta(ns, names[i], m, status, lastRun))
			}
		}
		if len(items) == 0 {
			output.PrintEmpty("jobs", "", jsonMode, opts.Quiet)
			return nil
		}
		return output.PrintList(items, []string{"NAMESPACE", "NAME", "IMAGE", "STATUS", "LAST RUN"}, jsonMode, opts.Quiet)
	}

	namespace := opts.Namespace
	statusMap := j.prefetchJobStatus([]string{namespace})
	manifests, names, err := j.listNamespace(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}
	if len(manifests) == 0 {
		output.PrintEmpty("jobs", namespace, jsonMode, opts.Quiet)
		return nil
	}
	items := make([]resource.ResourceMeta, len(manifests))
	for i, m := range manifests {
		status, lastRun := aggregateJobStatus(statusMap, namespace, names[i])
		items[i] = jobToMeta(namespace, names[i], m, status, lastRun)
	}
	return output.PrintList(items, []string{"NAME", "IMAGE", "STATUS", "LAST RUN"}, jsonMode, opts.Quiet)
}

func (j *Job) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	filePath := opts.String("file")
	if filePath == "" {
		msg := "--file (-f) is required for 'apply job'"
		output.Errorf(jsonMode, "UsageError", msg,
			"whctl apply job <name> -n <namespace> -f <path>", nil, false)
		return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
	}

	data, err := readInput(filePath, opts.FS)
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
	}

	var m JobManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateJobManifest(&m, namespace, name); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	exists, err := j.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}
		fmt.Printf("Would %s job %q in namespace %q\n", verb, name, namespace)
		return nil
	}

	if !exists {
		if err := j.EnsureDir(namespace, name); err != nil {
			return exitErr(exitcode.Failure, err)
		}
		if err := j.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}
		fmt.Printf("Created job %q in namespace %q\n", name, namespace)
	} else {
		if err := j.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}
		fmt.Printf("Updated job %q in namespace %q\n", name, namespace)
	}
	return nil
}

func (j *Job) runRun(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	_, m, err := j.getOne(namespace, name)
	if err != nil {
		return err
	}

	nsMeta, err := j.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.sshTarget()
	localResourceDir := j.ResourceDir(namespace, name)
	remoteResourceDir := nsMeta.Spec.remoteBaseDir() + "/jobs/" + name

	if err := generateJobCompose(localResourceDir, namespace, name, m.Spec, j.FS, j.DataDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("generate docker-compose: %w", err))
	}

	detach := opts.Bool("detach")

	var cmdParts []string
	cmdParts = append(cmdParts, "cd "+remoteResourceDir+" && docker compose run --rm")
	if detach {
		cmdParts = append(cmdParts, "--detach")
	}
	cmdParts = append(cmdParts, "job")
	cmd := strings.Join(cmdParts, " ")

	if opts.DryRun {
		fmt.Printf("Would rsync and run on %s: %s\n", target, cmd)
		return nil
	}

	if err := rsync.NewSyncer().Sync(localResourceDir, target, remoteResourceDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("rsync: %w", err))
	}

	sshClient := ssh.NewClient(target)
	if err := sshClient.Run(cmd); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("job %q failed: %w", name, err))
	}
	return nil
}

func (j *Job) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	exists, err := j.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}
	if !exists {
		msg := fmt.Sprintf("job %q not found in namespace %q", name, namespace)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)
		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would delete job %q in namespace %q\n", name, namespace)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete job %q in namespace %q? (remote containers are not removed)", name, namespace)); err != nil {
		return err
	}

	if err := j.RemoveDir(namespace, name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted job %q\n", name)
	return nil
}

func (j *Job) runLogs(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	nsMeta, err := j.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	follow := opts.Bool("follow")
	tail := opts.Int("tail")

	remoteResourceDir := nsMeta.Spec.remoteBaseDir() + "/jobs/" + name
	var cmdParts []string
	cmdParts = append(cmdParts, "cd "+remoteResourceDir+" && docker compose logs")
	if follow {
		cmdParts = append(cmdParts, "--follow")
	}
	if tail != -1 {
		cmdParts = append(cmdParts, fmt.Sprintf("--tail %d", tail))
	}
	cmdParts = append(cmdParts, "job")
	cmd := strings.Join(cmdParts, " ")

	if opts.DryRun {
		fmt.Printf("Would run on %s: %s\n", nsMeta.Spec.sshTarget(), cmd)
		return nil
	}

	sshClient := ssh.NewClient(nsMeta.Spec.sshTarget())
	if follow {
		return sshClient.Exec(cmd, false)
	}
	return sshClient.Run(cmd)
}

func (j *Job) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	names, err := j.ListNames(opts.Namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.Name != "" {
		exists, err := j.Exists(opts.Namespace, opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}
		if !exists {
			msg := fmt.Sprintf("job %q not found in namespace %q", opts.Name, opts.Namespace)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)
			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}
		names = []string{opts.Name}
	}

	for _, name := range names {
		j.doctorJob(rep, opts.Namespace, name)
	}

	if jsonMode {
		return rep.PrintJSON()
	}
	rep.PrintHuman(opts.Quiet)
	if rep.HasErrors() {
		return exitErr(exitcode.Failure, fmt.Errorf("doctor found errors"))
	}
	return nil
}

func (j *Job) doctorJob(rep *doctor.Report, namespace, name string) {
	resourceID := fmt.Sprintf("job/%s/%s", namespace, name)

	data, err := j.ReadBytes(namespace, name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m JobManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, jobKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, jobKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Metadata.Name)

	if m.Spec.Image == "" {
		rep.Errorf(resourceID, "missing-image", "spec.image is required but not set")
	}
}

// ── Registration ─────────────────────────────────────────────────────────────

func registerJob() {
	registry.Register(registry.Registration{
		Info:  jobKind,
		Scope: registry.NamespaceScoped,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List jobs or get a single job with its last run status",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl get jobs -n production",
					"whctl get job db-backup -n production",
					"whctl get jobs -A",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runGet(opts)
				},
			},
			{
				Verb:         "apply",
				Short:        "Create or update a job manifest (does not run it)",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "file", Short: "f", Type: "string", Usage: "Manifest file path, or - for stdin"},
				},
				Examples: []string{
					"whctl apply job db-backup -n production -f job.yaml",
					"cat job.yaml | whctl apply job db-backup -n production -f -",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runApply(opts)
				},
			},
			{
				Verb:         "run",
				Short:        "Execute a job on its namespace's host and stream output",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "detach", Short: "d", Type: "bool", Usage: "Run in the background; return immediately"},
				},
				Examples: []string{
					"whctl run job db-backup -n production",
					"whctl run job db-backup -n production --detach",
					"whctl run job db-backup -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runRun(opts)
				},
			},
			{
				Verb:         "delete",
				Short:        "Delete a job manifest (remote containers are not removed)",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl delete job db-backup -n production",
					"whctl delete job db-backup -n production --yes",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runDelete(opts)
				},
			},
			{
				Verb:         "logs",
				Short:        "Fetch logs from the most recent container run of a job",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "follow", Short: "f", Type: "bool", Usage: "Follow log output"},
					{Name: "tail", Type: "int", Default: -1, Usage: "Number of lines to show from the end (-1 for all)"},
				},
				Examples: []string{
					"whctl logs job db-backup -n production",
					"whctl logs job db-backup -n production --follow",
					"whctl logs job db-backup -n production --tail 50",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runLogs(opts)
				},
			},
			{
				Verb:         "doctor",
				Short:        "Check job manifests for issues",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl doctor jobs -n production",
					"whctl doctor job db-backup -n production",
					"whctl doctor jobs -A",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Job).runDoctor(opts)
				},
			},
		},
		SummaryColumns: []string{"NAME", "IMAGE", "STATUS", "LAST RUN"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newJob(dataDir, filesystem)
		},
	})
}
