package v1alpha1

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/doctor"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/yamlutil"
	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/rsync"
	"github.com/walheimlab/walheim-go/internal/ssh"
)

// ── KindInfo & validation ─────────────────────────────────────────────────────

var daemonSetKind = resource.KindInfo{
	Group:   "walheim",
	Version: "v1alpha1",
	Kind:    "DaemonSet",
	Plural:  "daemonsets",
	Aliases: []string{"ds"},
}

func validateDaemonSetManifest(m *DaemonSetManifest, name string) error {
	if m.APIVersion != daemonSetKind.APIVersion() {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", daemonSetKind.APIVersion(), m.APIVersion)
	}

	if m.Kind != daemonSetKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", daemonSetKind.Kind, m.Kind)
	}

	if m.Metadata.Name != name {
		return fmt.Errorf("metadata.name %q does not match argument %q", m.Metadata.Name, name)
	}

	if len(m.Spec.Compose.Services) == 0 {
		return fmt.Errorf("spec.compose.services must define at least one service")
	}

	return nil
}

// ── Handler ───────────────────────────────────────────────────────────────────

// DaemonSet is the handler for the DaemonSet resource kind.
// It is cluster-scoped: stored at <dataDir>/daemonsets/<name>/.daemonset.yaml
// and deployed to every namespace whose labels match spec.namespaceSelector.
type DaemonSet struct {
	resource.ClusterBase
}

func newDaemonSet(dataDir string, filesystem fs.FS) *DaemonSet {
	return &DaemonSet{
		ClusterBase: resource.ClusterBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             daemonSetKind,
			ManifestFilename: ".daemonset.yaml",
		},
	}
}

func (d *DaemonSet) KindInfo() resource.KindInfo { return daemonSetKind }

// ── Namespace selector helpers ────────────────────────────────────────────────

// matchingNamespaces returns all namespace manifests whose labels match the
// given selector, along with their names, ordered alphabetically.
func matchingNamespaces(selector *LabelSelector, filesystem fs.FS, dataDir string) ([]*NamespaceManifest, []string, error) {
	baseDir := filepath.Join(dataDir, "namespaces")

	entries, err := filesystem.ReadDir(baseDir)
	if err != nil {
		exists, existsErr := filesystem.Exists(baseDir)
		if existsErr != nil {
			return nil, nil, existsErr
		}

		if !exists {
			return nil, nil, nil
		}

		return nil, nil, fmt.Errorf("read namespaces dir: %w", err)
	}

	var (
		manifests []*NamespaceManifest
		names     []string
	)

	for _, entry := range entries {
		manifestPath := filepath.Join(baseDir, entry, ".namespace.yaml")

		ok, err := filesystem.Exists(manifestPath)
		if err != nil || !ok {
			continue
		}

		data, err := filesystem.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var m NamespaceManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			continue
		}

		if selector.Matches(m.Metadata.Labels) {
			manifests = append(manifests, &m)
			names = append(names, entry)
		}
	}

	return manifests, names, nil
}

// ── Typed read/list ───────────────────────────────────────────────────────────

func (d *DaemonSet) parseManifest(name string) (*DaemonSetManifest, error) {
	data, err := d.ReadBytes(name)
	if err != nil {
		return nil, err
	}

	var m DaemonSetManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse daemonset %q: %w", name, err)
	}

	return &m, nil
}

// copyDaemonSetManifest returns a deep copy of m via YAML round-trip.
// generateDaemonSetCompose mutates its manifest argument, so each parallel
// goroutine must work on its own copy.
func copyDaemonSetManifest(m *DaemonSetManifest) (*DaemonSetManifest, error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}

	var cp DaemonSetManifest
	if err := yaml.Unmarshal(data, &cp); err != nil {
		return nil, err
	}

	return &cp, nil
}

func daemonSetToMeta(name string, m *DaemonSetManifest, matchedNS []string) resource.ResourceMeta {
	img := "N/A"

	for _, svc := range m.Spec.Compose.Services {
		if svc.Image != "" {
			img = svc.Image
		}

		break
	}

	nsDisplay := fmt.Sprintf("%d", len(matchedNS))
	if len(matchedNS) > 0 {
		nsDisplay = strings.Join(matchedNS, ",")
	}

	selector := "(all)"

	if m.Spec.NamespaceSelector != nil && len(m.Spec.NamespaceSelector.MatchLabels) != 0 {
		parts := make([]string, 0, len(m.Spec.NamespaceSelector.MatchLabels))
		for k, v := range m.Spec.NamespaceSelector.MatchLabels {
			parts = append(parts, k+"="+v)
		}

		sort.Strings(parts)
		selector = strings.Join(parts, ",")
	}

	return resource.ResourceMeta{
		Name:   name,
		Labels: m.Metadata.Labels,
		Summary: map[string]string{
			"IMAGE":      img,
			"SELECTOR":   selector,
			"NAMESPACES": nsDisplay,
		},
		Raw: m,
	}
}

func (d *DaemonSet) getOne(name string) (resource.ResourceMeta, *DaemonSetManifest, error) {
	exists, err := d.Exists(name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	if !exists {
		return resource.ResourceMeta{}, nil,
			exitcode.New(exitcode.NotFound, fmt.Errorf("daemonset %q not found", name))
	}

	m, err := d.parseManifest(name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	_, nsNames, _ := matchingNamespaces(m.Spec.NamespaceSelector, d.FS, d.DataDir)

	return daemonSetToMeta(name, m, nsNames), m, nil
}

func (d *DaemonSet) listAll() ([]resource.ResourceMeta, error) {
	names, err := d.ListNames()
	if err != nil {
		return nil, err
	}

	items := make([]resource.ResourceMeta, 0, len(names))
	for _, name := range names {
		m, err := d.parseManifest(name)
		if err != nil {
			output.Warnf("skipping daemonset %q: %v", name, err)
			continue
		}

		_, nsNames, _ := matchingNamespaces(m.Spec.NamespaceSelector, d.FS, d.DataDir)
		items = append(items, daemonSetToMeta(name, m, nsNames))
	}

	return items, nil
}

// ── Deployment tracking & shared stop logic ───────────────────────────────────

// deployedNamespaces returns the names of namespaces that have a local compose
// directory under <dataDir>/daemonsets/<dsName>/, indicating a prior deployment.
func (d *DaemonSet) deployedNamespaces(dsName string) ([]string, error) {
	dsDir := d.ResourceDir(dsName)

	entries, err := d.FS.ReadDir(dsDir)
	if err != nil {
		// Dir may not exist yet for a brand-new daemonset.
		if ok, _ := d.FS.Exists(dsDir); !ok {
			return nil, nil
		}

		return nil, fmt.Errorf("read daemonset dir: %w", err)
	}

	var names []string

	for _, entry := range entries {
		composePath := filepath.Join(dsDir, entry, "docker-compose.yml")
		if ok, _ := d.FS.Exists(composePath); ok {
			names = append(names, entry)
		}
	}

	return names, nil
}

// loadNamespace reads a namespace manifest by name without any selector
// filtering. Used during cleanup to reach namespaces that no longer match.
func (d *DaemonSet) loadNamespace(ns string) (*NamespaceManifest, error) {
	path := filepath.Join(d.DataDir, "namespaces", ns, ".namespace.yaml")

	data, err := d.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("namespace %q not found", ns)
	}

	var m NamespaceManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

// stopInNamespace runs docker compose down, removes remote files, and removes
// the local compose directory for a daemonset in a specific namespace.
// If the namespace manifest cannot be read, it logs a warning and skips SSH
// work while still cleaning up local files.
func (d *DaemonSet) stopInNamespace(dsName, ns string) error {
	nsMeta, err := d.loadNamespace(ns)
	if err != nil {
		output.Warnf("cannot load namespace %q for cleanup: %v — skipping remote cleanup", ns, err)
	} else {
		target := nsMeta.Spec.sshTarget()
		remoteDir := nsMeta.Spec.remoteBaseDir() + "/daemonsets/" + dsName

		sshClient := ssh.NewClient(target)
		if _, checkErr := sshClient.RunOutput("test -d " + remoteDir); checkErr == nil {
			if err := sshClient.Run("cd " + remoteDir + " && docker compose --progress plain down"); err != nil {
				return exitErr(exitcode.Failure, fmt.Errorf("docker compose down in %q: %w", ns, err))
			}

			if err := sshClient.Run("rm -rf " + remoteDir); err != nil {
				return exitErr(exitcode.Failure, fmt.Errorf("remove remote files in %q: %w", ns, err))
			}
		}
	}

	// Always clean up the local compose directory.
	localDir := filepath.Join(d.DataDir, "daemonsets", dsName, ns)
	if err := d.FS.RemoveAll(localDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("remove local compose dir for %q: %w", ns, err))
	}

	return nil
}

// ── Compose generation ────────────────────────────────────────────────────────

// generateDaemonSetCompose builds docker-compose.yml for a daemonset in a
// specific namespace, written to:
//
//	<dataDir>/daemonsets/<dsName>/<namespace>/docker-compose.yml
//
// NOTE: This function modifies m.Spec.Compose.Services in-place.
func generateDaemonSetCompose(namespace, dsName string, m *DaemonSetManifest, filesystem fs.FS, dataDir string) error {
	services := m.Spec.Compose.Services
	if services == nil {
		return fmt.Errorf("spec.compose.services is empty")
	}

	// Ensure all service environment and label maps are initialized.
	for svcName, svc := range services {
		if svc.Environment.Values == nil {
			svc.Environment.Values = make(map[string]string)
		}

		if svc.Labels.Values == nil {
			svc.Labels.Values = make(map[string]string)
		}

		services[svcName] = svc
	}

	// Inject Walheim labels into every service.
	for svcName, svc := range services {
		for k := range svc.Labels.Values {
			if strings.HasPrefix(k, "walheim.") {
				delete(svc.Labels.Values, k)
			}
		}

		svc.Labels.Values["walheim.managed"] = "true"
		svc.Labels.Values["walheim.namespace"] = namespace
		svc.Labels.Values["walheim.daemonset"] = dsName
		services[svcName] = svc
	}

	// Load and inject envFrom (lower precedence — only if key not present).
	for _, entry := range m.Spec.EnvFrom {
		var (
			kvMap map[string]string
			err   error
		)

		if entry.SecretRef != nil {
			kvMap, err = loadSecret(namespace, entry.SecretRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("envFrom: %w", err)
			}
		} else if entry.ConfigMapRef != nil {
			kvMap, err = loadConfigMap(namespace, entry.ConfigMapRef.Name, filesystem, dataDir)
			if err != nil {
				return fmt.Errorf("envFrom: %w", err)
			}
		} else {
			continue
		}

		targets := targetServices(services, entry.ServiceNames)
		for _, svcName := range targets {
			svc := services[svcName]
			for k, v := range kvMap {
				if _, exists := svc.Environment.Values[k]; !exists {
					svc.Environment.Values[k] = v
				}
			}

			services[svcName] = svc
		}
	}

	// Inject env entries (higher precedence — always overwrite).
	for _, entry := range m.Spec.Env {
		targets := targetServices(services, entry.ServiceNames)
		for _, svcName := range targets {
			svc := services[svcName]
			value := substituteVars(entry.Value, svc.Environment.Values)
			svc.Environment.Values[entry.Name] = value
			services[svcName] = svc
		}
	}

	m.Spec.Compose.Services = services

	resourceDir := filepath.Join(dataDir, "daemonsets", dsName, namespace)
	if err := filesystem.MkdirAll(resourceDir); err != nil {
		return fmt.Errorf("create compose dir: %w", err)
	}

	if err := injectComposeMounts(resourceDir, services, m.Spec.Mounts, namespace, filesystem, dataDir); err != nil {
		return err
	}

	encoded, err := yamlutil.Marshal(m.Spec.Compose)
	if err != nil {
		return fmt.Errorf("marshal docker-compose: %w", err)
	}

	composePath := filepath.Join(resourceDir, "docker-compose.yml")
	if err := filesystem.WriteFile(composePath, encoded); err != nil {
		return fmt.Errorf("write docker-compose.yml: %w", err)
	}

	return nil
}

// ── Operations ────────────────────────────────────────────────────────────────

func (d *DaemonSet) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name != "" {
		meta, _, err := d.getOne(opts.Name)
		if err != nil {
			output.Errorf(jsonMode, "NotFound",
				fmt.Sprintf("daemonset %q not found", opts.Name), "", nil, false)

			return err
		}

		return output.PrintOne(meta, opts.Output)
	}

	items, err := d.listAll()
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(items) == 0 {
		output.PrintEmpty("", daemonSetKind, opts.Output, opts.Quiet)
		return nil
	}

	return output.PrintList(items, []string{"NAME", "IMAGE", "SELECTOR", "NAMESPACES"}, daemonSetKind, opts.Output, opts.Quiet)
}

func (d *DaemonSet) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	var data []byte
	if len(opts.RawManifest) > 0 {
		data = opts.RawManifest
	} else {
		filePath := opts.String("file")
		if filePath == "" {
			msg := "--file (-f) is required for 'apply daemonset'"
			output.Errorf(jsonMode, "UsageError", msg,
				"whctl apply daemonset <name> -f <path>", nil, false)

			return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
		}

		var err error

		data, err = readInput(filePath, opts.FS)
		if err != nil {
			return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
		}
	}

	var m DaemonSetManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateDaemonSetManifest(&m, name); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	exists, err := d.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}

		fmt.Printf("Would %s daemonset %q\n", verb, name)

		return nil
	}

	if !exists {
		if err := d.EnsureDir(name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := d.WriteManifest(name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Created daemonset %q\n", name)
	} else {
		if err := d.WriteManifest(name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Updated daemonset %q\n", name)
	}

	return d.runStart(opts)
}

func (d *DaemonSet) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	exists, err := d.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("daemonset %q not found", name)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would stop and delete daemonset %q\n", name)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete daemonset %q (stops containers on all matching namespaces)?", name)); err != nil {
		return err
	}

	if err := d.runStop(opts); err != nil {
		return err
	}

	if err := d.RemoveDir(name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted daemonset %q\n", name)

	return nil
}

func (d *DaemonSet) runStart(opts registry.OperationOpts) error {
	name := opts.Name

	_, m, err := d.getOne(name)
	if err != nil {
		return err
	}

	nsMetas, nsNames, err := matchingNamespaces(m.Spec.NamespaceSelector, d.FS, d.DataDir)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	// ── Reconcile: remove from namespaces that no longer match the selector ──
	deployed, err := d.deployedNamespaces(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	desired := make(map[string]bool, len(nsNames))
	for _, ns := range nsNames {
		desired[ns] = true
	}

	// ── Reconcile: remove from namespaces that no longer match (parallel) ───
	var toRemove []string

	for _, ns := range deployed {
		if !desired[ns] {
			toRemove = append(toRemove, ns)
		}
	}

	if opts.DryRun {
		for _, ns := range toRemove {
			fmt.Printf("Would remove daemonset %q from namespace %q (no longer selected)\n", name, ns)
		}
	} else if len(toRemove) > 0 {
		removeErrs := make([]error, len(toRemove))

		var removeWg sync.WaitGroup

		for i, ns := range toRemove {
			removeWg.Add(1)

			go func(i int, ns string) {
				defer removeWg.Done()

				removeErrs[i] = d.stopInNamespace(name, ns)
			}(i, ns)
		}

		removeWg.Wait()

		for i, err := range removeErrs {
			if err != nil {
				return err
			}

			fmt.Printf("Removed daemonset %q from namespace %q (no longer selected)\n", name, toRemove[i])
		}
	}

	if len(nsNames) == 0 {
		fmt.Printf("Daemonset %q: no matching namespaces\n", name)
		return nil
	}

	if opts.DryRun {
		fmt.Printf("Would deploy daemonset %q to namespaces: %s\n", name, strings.Join(nsNames, ", "))
		return nil
	}

	// ── Deploy to all currently matching namespaces (parallel) ──────────────
	deployErrs := make([]error, len(nsNames))

	var deployWg sync.WaitGroup

	for i, ns := range nsNames {
		deployWg.Add(1)

		go func(i int, ns string, nsMeta *NamespaceManifest) {
			defer deployWg.Done()

			mc, err := copyDaemonSetManifest(m)
			if err != nil {
				deployErrs[i] = exitErr(exitcode.Failure, fmt.Errorf("copy manifest for namespace %q: %w", ns, err))
				return
			}

			if err := generateDaemonSetCompose(ns, name, mc, d.FS, d.DataDir); err != nil {
				deployErrs[i] = exitErr(exitcode.Failure, fmt.Errorf("generate compose for namespace %q: %w", ns, err))
				return
			}

			target := nsMeta.Spec.sshTarget()
			localDir := filepath.Join(d.DataDir, "daemonsets", name, ns)
			remoteDir := nsMeta.Spec.remoteBaseDir() + "/daemonsets/" + name

			if err := rsync.NewSyncer().Sync(d.FS, localDir, target, remoteDir); err != nil {
				deployErrs[i] = exitErr(exitcode.Failure, fmt.Errorf("rsync to %q: %w", ns, err))
				return
			}

			sshClient := ssh.NewClient(target)

			cmd := "cd " + remoteDir + " && docker compose --progress plain up -d --remove-orphans"
			if err := sshClient.Run(cmd); err != nil {
				deployErrs[i] = exitErr(exitcode.Failure, fmt.Errorf("docker compose up in %q: %w", ns, err))
			}
		}(i, ns, nsMetas[i])
	}

	deployWg.Wait()

	for i, err := range deployErrs {
		if err != nil {
			return err
		}

		fmt.Printf("Started daemonset %q in namespace %q\n", name, nsNames[i])
	}

	return nil
}

func (d *DaemonSet) runStop(opts registry.OperationOpts) error {
	name := opts.Name

	// Use deployed namespaces (from local compose dirs), not the current
	// selector — the selector may have already changed before stop was called.
	deployed, err := d.deployedNamespaces(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(deployed) == 0 {
		fmt.Printf("Daemonset %q is not deployed anywhere\n", name)
		return nil
	}

	if opts.DryRun {
		fmt.Printf("Would stop daemonset %q on namespaces: %s\n", name, strings.Join(deployed, ", "))
		return nil
	}

	stopErrs := make([]error, len(deployed))

	var wg sync.WaitGroup

	for i, ns := range deployed {
		wg.Add(1)

		go func(i int, ns string) {
			defer wg.Done()

			stopErrs[i] = d.stopInNamespace(name, ns)
		}(i, ns)
	}

	wg.Wait()

	for i, err := range stopErrs {
		if err != nil {
			return err
		}

		fmt.Printf("Stopped daemonset %q in namespace %q\n", name, deployed[i])
	}

	return nil
}

func (d *DaemonSet) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	names, err := d.ListNames()
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.Name != "" {
		exists, err := d.Exists(opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("daemonset %q not found", opts.Name)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		names = []string{opts.Name}
	}

	for _, name := range names {
		d.doctorDaemonSet(rep, name)
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

func (d *DaemonSet) doctorDaemonSet(rep *doctor.Report, name string) {
	resourceID := "daemonset/" + name

	data, err := d.ReadBytes(name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m DaemonSetManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, daemonSetKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, daemonSetKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Metadata.Name)

	if len(m.Spec.Compose.Services) == 0 {
		rep.Errorf(resourceID, "missing-services", "spec.compose.services must define at least one service")
	}
}

// ── Registration ─────────────────────────────────────────────────────────────

func registerDaemonSet() {
	registry.Register(registry.Registration{
		Info:       daemonSetKind,
		Scope:      registry.ClusterScoped,
		ApplyOrder: registry.ApplyOrderClusterWorkload,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List daemonsets or get a single daemonset by name",
				NSHandling:   registry.NSNone,
				RequiresName: false,
				Examples: []string{
					"whctl get daemonsets",
					"whctl get daemonset my-service",
					"whctl get ds -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*DaemonSet).runGet(opts)
				},
			},
			{
				Verb:         "apply",
				Short:        "Create or update a daemonset from a manifest file and deploy it",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "file", Short: "f", Type: "string", Usage: "Manifest file path, or - for stdin"},
				},
				Examples: []string{
					"whctl apply daemonset my-service -f daemonset.yaml",
					"cat daemonset.yaml | whctl apply daemonset my-service -f -",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*DaemonSet).runApply(opts)
				},
			},
			{
				Verb:         "delete",
				Short:        "Stop and delete a daemonset from all matching namespaces",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Examples: []string{
					"whctl delete daemonset my-service --yes",
					"whctl delete daemonset my-service --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*DaemonSet).runDelete(opts)
				},
			},
			{
				Verb:         "start",
				Short:        "Deploy a daemonset to all matching namespaces",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Examples: []string{
					"whctl start daemonset my-service",
					"whctl start daemonset my-service --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*DaemonSet).runStart(opts)
				},
			},
			{
				Verb:         "stop",
				Short:        "Stop and remove a daemonset from all matching namespaces",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Examples: []string{
					"whctl stop daemonset my-service",
					"whctl stop daemonset my-service --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*DaemonSet).runStop(opts)
				},
			},
			{
				Verb:         "doctor",
				Short:        "Check daemonset manifests for issues",
				NSHandling:   registry.NSNone,
				RequiresName: false,
				Examples: []string{
					"whctl doctor daemonsets",
					"whctl doctor daemonset my-service",
					"whctl doctor daemonsets -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*DaemonSet).runDoctor(opts)
				},
			},
		},
		SummaryColumns: []string{"NAME", "IMAGE", "SELECTOR", "NAMESPACES"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newDaemonSet(dataDir, filesystem)
		},
	})
}
