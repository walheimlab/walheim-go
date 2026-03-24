package v1alpha1

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

var appKind = resource.KindInfo{
	Group:   "walheim",
	Version: "v1alpha1",
	Kind:    "App",
	Plural:  "apps",
	Aliases: []string{},
}

// App is the handler for the App resource kind.
type App struct {
	resource.NamespacedBase
}

func newApp(dataDir string, filesystem fs.FS) *App {
	return &App{
		NamespacedBase: resource.NamespacedBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             appKind,
			ManifestFilename: ".app.yaml",
		},
	}
}

func (a *App) KindInfo() resource.KindInfo { return appKind }

// ── Namespace config helper ───────────────────────────────────────────────────

func (a *App) loadNamespaceManifest(namespace string) (*NamespaceManifest, error) {
	path := filepath.Join(a.DataDir, "namespaces", namespace, ".namespace.yaml")

	data, err := a.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("namespace %q not found", namespace)
	}

	var m NamespaceManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

// ── Validation ────────────────────────────────────────────────────────────────

func validateAppManifest(m *AppManifest, namespace, name string) error {
	if m.APIVersion != appKind.APIVersion() {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", appKind.APIVersion(), m.APIVersion)
	}

	if m.Kind != appKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", appKind.Kind, m.Kind)
	}

	if m.Metadata.Name != name {
		return fmt.Errorf("metadata.name %q does not match argument %q", m.Metadata.Name, name)
	}

	if m.Metadata.Namespace != namespace {
		return fmt.Errorf("metadata.namespace %q does not match -n %q", m.Metadata.Namespace, namespace)
	}

	if len(m.Spec.Compose.Services) == 0 {
		return fmt.Errorf("spec.compose.services must define at least one service")
	}

	return nil
}

// ── Typed read/list helpers ───────────────────────────────────────────────────

func (a *App) parseManifest(namespace, name string) (*AppManifest, error) {
	data, err := a.ReadBytes(namespace, name)
	if err != nil {
		return nil, err
	}

	var m AppManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse app %q in namespace %q: %w", name, namespace, err)
	}

	return &m, nil
}

func appToMeta(namespace, name string, m *AppManifest, status, ready string) resource.ResourceMeta {
	// Pick image from first service in iteration order.
	img := "N/A"

	for _, svc := range m.Spec.Compose.Services {
		if svc.Image != "" {
			img = svc.Image
		}

		break
	}

	return resource.ResourceMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    m.Metadata.Labels,
		Summary: map[string]string{
			"IMAGE":  img,
			"READY":  ready,
			"STATUS": status,
		},
		Raw: m,
	}
}

func (a *App) getOne(namespace, name string) (resource.ResourceMeta, *AppManifest, error) {
	exists, err := a.Exists(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	if !exists {
		return resource.ResourceMeta{}, nil,
			exitcode.New(exitcode.NotFound, fmt.Errorf("app %q not found in namespace %q", name, namespace))
	}

	m, err := a.parseManifest(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	return appToMeta(namespace, name, m, "Configured", "-"), m, nil
}

func (a *App) listNamespace(namespace string) ([]*AppManifest, []string, error) {
	names, err := a.ListNames(namespace)
	if err != nil {
		return nil, nil, err
	}

	manifests := make([]*AppManifest, 0, len(names))

	validNames := make([]string, 0, len(names))
	for _, name := range names {
		m, err := a.parseManifest(namespace, name)
		if err != nil {
			output.Warnf("skipping app %q in namespace %q: %v", name, namespace, err)
			continue
		}

		manifests = append(manifests, m)
		validNames = append(validNames, name)
	}

	return manifests, validNames, nil
}

// ── Operations ────────────────────────────────────────────────────────────────

func (a *App) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	// Single resource by name
	if opts.Name != "" {
		namespace := opts.Namespace

		meta, _, err := a.getOne(namespace, opts.Name)
		if err != nil {
			output.Errorf(jsonMode, "NotFound",
				fmt.Sprintf("app %q not found in namespace %q", opts.Name, namespace), "", nil, false)

			return err
		}

		return output.PrintOne(meta, opts.Output)
	}

	// List with status
	if opts.AllNamespaces {
		// List all namespaces
		namespaces, err := a.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		statusMap := a.prefetchStatus(namespaces)

		var items []resource.ResourceMeta

		for _, ns := range namespaces {
			manifests, names, err := a.listNamespace(ns)
			if err != nil {
				return exitErr(exitcode.Failure, err)
			}

			for i, m := range manifests {
				status, ready := aggregateStatus(statusMap, ns, names[i])
				items = append(items, appToMeta(ns, names[i], m, status, ready))
			}
		}

		if len(items) == 0 {
			output.PrintEmpty("", appKind, opts.Output, opts.Quiet)
			return nil
		}

		return output.PrintList(items, []string{"NAMESPACE", "NAME", "IMAGE", "READY", "STATUS"}, appKind, opts.Output, opts.Quiet)
	}

	// Single namespace list
	namespace := opts.Namespace
	statusMap := a.prefetchStatus([]string{namespace})

	manifests, names, err := a.listNamespace(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(manifests) == 0 {
		output.PrintEmpty(namespace, appKind, opts.Output, opts.Quiet)
		return nil
	}

	items := make([]resource.ResourceMeta, len(manifests))
	for i, m := range manifests {
		status, ready := aggregateStatus(statusMap, namespace, names[i])
		items[i] = appToMeta(namespace, names[i], m, status, ready)
	}

	return output.PrintList(items, []string{"NAME", "IMAGE", "READY", "STATUS"}, appKind, opts.Output, opts.Quiet)
}

func (a *App) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	var data []byte
	if len(opts.RawManifest) > 0 {
		data = opts.RawManifest
	} else {
		filePath := opts.String("file")
		if filePath == "" {
			msg := "--file (-f) is required for 'apply app'"
			output.Errorf(jsonMode, "UsageError", msg,
				"whctl apply app <name> -n <namespace> -f <path>", nil, false)

			return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
		}

		var err error

		data, err = readInput(filePath, opts.FS)
		if err != nil {
			return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
		}
	}

	var m AppManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateAppManifest(&m, namespace, name); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	exists, err := a.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}

		fmt.Printf("Would %s app %q in namespace %q\n", verb, name, namespace)

		return nil
	}

	if !exists {
		if err := a.EnsureDir(namespace, name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := a.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Created app %q in namespace %q\n", name, namespace)
	} else {
		if err := a.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Updated app %q in namespace %q\n", name, namespace)
	}

	// apply auto-starts (post-create and post-update hook)
	return a.runStart(opts)
}

func (a *App) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	exists, err := a.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("app %q not found in namespace %q", name, namespace)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would stop and delete app %q in namespace %q\n", name, namespace)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete app %q in namespace %q (stops containers and removes remote files)?", name, namespace)); err != nil {
		return err
	}

	// Pre-delete hook: stop (pause + remote rm)
	if err := a.runStop(opts); err != nil {
		return err
	}

	if err := a.RemoveDir(namespace, name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted app %q\n", name)

	return nil
}

func (a *App) runDescribe(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	meta, m, err := a.getOne(namespace, name)
	if err != nil {
		output.Errorf(jsonMode, "NotFound",
			fmt.Sprintf("app %q not found in namespace %q", name, namespace), "", nil, false)

		return err
	}

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.sshTarget()

	client := ssh.NewClient(target)

	// Check remote dir
	remoteExists := false

	remoteAppDir := nsMeta.Spec.remoteBaseDir() + "/apps/" + name
	if _, checkErr := client.RunOutput("test -d " + remoteAppDir + " && echo yes"); checkErr == nil {
		remoteExists = true
	}

	// Get docker compose ps
	composePS := ""
	if remoteExists {
		composePS, _ = client.RunOutput("cd " + remoteAppDir + " && docker compose ps 2>/dev/null")
	}

	if jsonMode {
		result := map[string]any{
			"name":            name,
			"namespace":       namespace,
			"image":           meta.Summary["IMAGE"],
			"status":          meta.Summary["STATUS"],
			"ready":           meta.Summary["READY"],
			"ssh":             target,
			"remote_deployed": remoteExists,
			"compose_ps":      strings.TrimSpace(composePS),
			"services":        serviceNames(m),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(result)
	}

	fmt.Printf("Name:       %s\n", name)
	fmt.Printf("Namespace:  %s\n", namespace)
	fmt.Printf("SSH Target: %s\n", target)
	fmt.Println()

	fmt.Println("Services:")

	for _, svcName := range serviceNames(m) {
		svc := m.Spec.Compose.Services[svcName]

		img := svc.Image
		if img == "" {
			img = "(no image)"
		}

		fmt.Printf("  %-20s %s\n", svcName, img)
	}

	fmt.Println()

	if remoteExists {
		fmt.Println("Remote: deployed")

		if ps := strings.TrimSpace(composePS); ps != "" {
			fmt.Println()
			fmt.Println("docker compose ps:")

			for _, line := range strings.Split(ps, "\n") {
				fmt.Printf("  %s\n", line)
			}
		}
	} else {
		fmt.Println("Remote: not deployed")
	}

	return nil
}

// serviceNames returns sorted service names from an AppManifest.
func serviceNames(m *AppManifest) []string {
	names := make([]string, 0, len(m.Spec.Compose.Services))
	for n := range m.Spec.Compose.Services {
		names = append(names, n)
	}

	sortStrings(names)

	return names
}

func sortStrings(s []string) {
	// Use sort from standard library via compose.go's import — but since we can't
	// import sort here without adding it, we inline a simple sort.
	// Actually we can just add sort to imports.
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

func (a *App) runImport(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	filePath := opts.String("file")
	if filePath == "" {
		msg := "--file (-f) is required for 'import app'"
		output.Errorf(jsonMode, "UsageError", msg,
			"whctl import app <name> -n <namespace> -f <path>", nil, false)

		return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
	}

	data, err := readInput(filePath, opts.FS)
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
	}

	var composeSpec ComposeSpec
	if err := yaml.Unmarshal(data, &composeSpec); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse compose file: %w", err))
	}

	m := &AppManifest{
		APIVersion: appKind.APIVersion(),
		Kind:       appKind.Kind,
		Metadata:   ResourceMetadata{Name: name, Namespace: namespace},
		Spec:       AppSpec{Compose: composeSpec},
	}

	if opts.DryRun {
		encoded, err := yaml.Marshal(m)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Print(string(encoded))

		return nil
	}

	exists, err := a.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		if err := a.EnsureDir(namespace, name); err != nil {
			return exitErr(exitcode.Failure, err)
		}
	}

	if err := a.WriteManifest(namespace, name, m); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Imported app %q (no deploy — run 'whctl start app %s -n %s')\n", name, name, namespace)

	return nil
}

func (a *App) runStart(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	_, m, err := a.getOne(namespace, name)
	if err != nil {
		return err
	}

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.sshTarget()

	// Generate compose file locally
	if err := generateCompose(namespace, name, m, a.FS, a.DataDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("generate compose: %w", err))
	}

	if opts.DryRun {
		fmt.Printf("Would rsync and docker compose up for app %q in namespace %q\n", name, namespace)
		return nil
	}

	localDir := a.ResourceDir(namespace, name)
	remoteDir := nsMeta.Spec.remoteBaseDir() + "/apps/" + name

	if err := rsync.NewSyncer().Sync(a.FS, localDir, target, remoteDir); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("rsync: %w", err))
	}

	sshClient := ssh.NewClient(target)

	cmd := "cd " + remoteDir + " && docker compose up -d --remove-orphans"
	if err := sshClient.Run(cmd); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("docker compose up: %w", err))
	}

	fmt.Printf("Started app %q\n", name)

	return nil
}

func (a *App) runPause(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.sshTarget()

	if opts.DryRun {
		fmt.Printf("Would run docker compose down for app %q in namespace %q\n", name, namespace)
		return nil
	}

	// Check if remote dir exists — idempotent
	remoteAppDir := nsMeta.Spec.remoteBaseDir() + "/apps/" + name
	sshClient := ssh.NewClient(target)

	_, checkErr := sshClient.RunOutput("test -d " + remoteAppDir)
	if checkErr != nil {
		fmt.Printf("App %q is not deployed\n", name)
		return nil
	}

	if err := sshClient.Run("cd " + remoteAppDir + " && docker compose down"); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("docker compose down: %w", err))
	}

	fmt.Printf("Paused app %q\n", name)

	return nil
}

func (a *App) runStop(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	// First pause (docker compose down)
	if err := a.runPause(opts); err != nil {
		return err
	}

	// If dry-run, runPause already printed and returned — we just return too.
	if opts.DryRun {
		fmt.Printf("Would remove remote files for app %q\n", name)
		return nil
	}

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.sshTarget()

	sshClient := ssh.NewClient(target)
	if err := sshClient.Run("rm -rf " + nsMeta.Spec.remoteBaseDir() + "/apps/" + name); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("remove remote files: %w", err))
	}

	fmt.Printf("Stopped app %q\n", name)

	return nil
}

func (a *App) runPull(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.sshTarget()

	if opts.DryRun {
		fmt.Printf("Would run docker compose pull for app %q in namespace %q\n", name, namespace)
		return nil
	}

	// Check remote dir
	remoteAppDir := nsMeta.Spec.remoteBaseDir() + "/apps/" + name
	sshClient := ssh.NewClient(target)

	_, checkErr := sshClient.RunOutput("test -d " + remoteAppDir)
	if checkErr != nil {
		msg := fmt.Sprintf("app %q is not deployed in namespace %q", name, namespace)
		output.Errorf(opts.Output == "json", "NotFound", msg, "Run 'whctl start app "+name+" -n "+namespace+"' to deploy it first.", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if err := sshClient.Run("cd " + remoteAppDir + " && docker compose pull"); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("docker compose pull: %w", err))
	}

	fmt.Printf("Run 'whctl start app %s -n %s' to apply pulled images\n", name, namespace)

	return nil
}

func (a *App) runLogs(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.sshTarget()

	follow := opts.Bool("follow")
	tail := opts.Int("tail")
	timestamps := opts.Bool("timestamps")
	service := opts.String("service")

	// Build remote command
	var cmdParts []string

	cmdParts = append(cmdParts, "cd "+nsMeta.Spec.remoteBaseDir()+"/apps/"+name+" && docker compose logs")
	if follow {
		cmdParts = append(cmdParts, "--follow")
	}

	if tail != -1 {
		cmdParts = append(cmdParts, fmt.Sprintf("--tail %d", tail))
	}

	if timestamps {
		cmdParts = append(cmdParts, "--timestamps")
	}

	if service != "" {
		cmdParts = append(cmdParts, service)
	}

	cmd := strings.Join(cmdParts, " ")

	if opts.DryRun {
		fmt.Printf("Would run: ssh %s %q\n", target, cmd)
		return nil
	}

	sshClient := ssh.NewClient(target)
	if follow {
		// Replace process via syscall.Exec for proper Ctrl+C handling
		return sshClient.Exec(cmd, false)
	}

	return sshClient.Run(cmd)
}

func (a *App) runExec(opts registry.OperationOpts) error {
	namespace := opts.Namespace
	name := opts.Name

	_, m, err := a.getOne(namespace, name)
	if err != nil {
		return err
	}

	nsMeta, err := a.loadNamespaceManifest(namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	target := nsMeta.Spec.sshTarget()

	service := opts.String("service")
	if service == "" {
		// Use first service
		for svcName := range m.Spec.Compose.Services {
			service = svcName
			break
		}
	}

	if service == "" {
		return exitErr(exitcode.UsageError, fmt.Errorf("no services defined in app %q", name))
	}

	tty := opts.Bool("tty")

	execCmd := opts.String("cmd")
	if execCmd == "" {
		execCmd = "sh"
	}

	// Build remote command
	var cmdParts []string

	cmdParts = append(cmdParts, "cd "+nsMeta.Spec.remoteBaseDir()+"/apps/"+name+" && docker compose exec")
	if !tty {
		cmdParts = append(cmdParts, "--no-TTY")
	}

	cmdParts = append(cmdParts, service, execCmd)
	cmd := strings.Join(cmdParts, " ")

	if opts.DryRun {
		fmt.Printf("Would run: ssh %s %q\n", target, cmd)
		return nil
	}

	// Always replace process via syscall.Exec
	sshClient := ssh.NewClient(target)

	return sshClient.Exec(cmd, tty)
}

// ── doctor ────────────────────────────────────────────────────────────────────

func (a *App) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	if opts.Name != "" {
		// Single app requested — namespace is required (NSRequired on this op).
		namespace := opts.Namespace

		exists, err := a.Exists(namespace, opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("app %q not found in namespace %q", opts.Name, namespace)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		a.doctorApp(rep, namespace, opts.Name)
	} else if opts.AllNamespaces {
		namespaces, err := a.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		for _, ns := range namespaces {
			names, err := a.ListNames(ns)
			if err != nil {
				rep.Warnf("namespace/"+ns, "list-apps", "cannot list apps: %v", err)
				continue
			}

			for _, name := range names {
				a.doctorApp(rep, ns, name)
			}
		}
	} else {
		// Single namespace.
		namespace := opts.Namespace

		names, err := a.ListNames(namespace)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		for _, name := range names {
			a.doctorApp(rep, namespace, name)
		}
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

// doctorApp runs all checks for a single app and adds findings to rep.
func (a *App) doctorApp(rep *doctor.Report, namespace, name string) {
	resourceID := "app/" + namespace + "/" + name

	// ── YAML parse ────────────────────────────────────────────────────────
	data, err := a.ReadBytes(namespace, name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m AppManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	// ── Common structural checks ──────────────────────────────────────────
	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, appKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, appKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Metadata.Name)
	doctor.CheckNamespaceFieldMatchesDir(rep, resourceID, m.Metadata.Namespace, namespace)

	// ── App-specific field checks ─────────────────────────────────────────
	if len(m.Spec.Compose.Services) == 0 {
		rep.Errorf(resourceID, "no-services", "spec.compose.services must define at least one service")
	}

	// ── envFrom reference checks ──────────────────────────────────────────
	for i, entry := range m.Spec.EnvFrom {
		switch {
		case entry.SecretRef != nil:
			secretPath := filepath.Join(a.DataDir, "namespaces", namespace, "secrets",
				entry.SecretRef.Name, ".secret.yaml")

			exists, err := a.FS.Exists(secretPath)
			if err != nil {
				rep.Warnf(resourceID, "envfrom-secret-check",
					"envFrom[%d]: cannot check secret %q: %v", i, entry.SecretRef.Name, err)
			} else if !exists {
				rep.Errorf(resourceID, "envfrom-secret-missing",
					"envFrom[%d]: secretRef %q does not exist in namespace %q",
					i, entry.SecretRef.Name, namespace)
			}

			// Check serviceNames reference valid service names
			for _, sn := range entry.ServiceNames {
				if _, ok := m.Spec.Compose.Services[sn]; !ok {
					rep.Warnf(resourceID, "envfrom-unknown-service",
						"envFrom[%d] secretRef %q: serviceNames references unknown service %q",
						i, entry.SecretRef.Name, sn)
				}
			}

		case entry.ConfigMapRef != nil:
			cmPath := filepath.Join(a.DataDir, "namespaces", namespace, "configmaps",
				entry.ConfigMapRef.Name, ".configmap.yaml")

			exists, err := a.FS.Exists(cmPath)
			if err != nil {
				rep.Warnf(resourceID, "envfrom-configmap-check",
					"envFrom[%d]: cannot check configmap %q: %v", i, entry.ConfigMapRef.Name, err)
			} else if !exists {
				rep.Errorf(resourceID, "envfrom-configmap-missing",
					"envFrom[%d]: configMapRef %q does not exist in namespace %q",
					i, entry.ConfigMapRef.Name, namespace)
			}

			for _, sn := range entry.ServiceNames {
				if _, ok := m.Spec.Compose.Services[sn]; !ok {
					rep.Warnf(resourceID, "envfrom-unknown-service",
						"envFrom[%d] configMapRef %q: serviceNames references unknown service %q",
						i, entry.ConfigMapRef.Name, sn)
				}
			}

		default:
			rep.Warnf(resourceID, "envfrom-no-ref",
				"envFrom[%d]: entry has neither secretRef nor configMapRef", i)
		}
	}

	// ── env serviceNames reference checks ────────────────────────────────
	for i, entry := range m.Spec.Env {
		for _, sn := range entry.ServiceNames {
			if _, ok := m.Spec.Compose.Services[sn]; !ok {
				rep.Warnf(resourceID, "env-unknown-service",
					"env[%d] %q: serviceNames references unknown service %q",
					i, entry.Name, sn)
			}
		}
	}
}

// ── Registration ──────────────────────────────────────────────────────────────

func registerApp() {
	fileFlag := []registry.FlagDef{
		{Name: "file", Short: "f", Type: "string", Usage: "Manifest file path, or - for stdin"},
	}

	registry.Register(registry.Registration{
		Info:  appKind,
		Scope: registry.NamespaceScoped,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List apps in a namespace, or get a single app by name",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl get apps -n production",
					"whctl get app myapp -n production",
					"whctl get apps -A",
					"whctl get apps -n production -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runGet(opts)
				},
			},
			{
				Verb:         "apply",
				Short:        "Create or update an app from a manifest file, then start it",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags:        fileFlag,
				Examples: []string{
					"whctl apply app myapp -n production -f myapp.yaml",
					"whctl apply app myapp -n production -f myapp.yaml --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runApply(opts)
				},
			},
			{
				Verb:         "delete",
				Short:        "Stop an app and delete its local manifest and remote files",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl delete app myapp -n production --yes",
					"whctl delete app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runDelete(opts)
				},
			},
			{
				Verb:         "describe",
				Short:        "Show app details with live docker compose ps output",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl describe app myapp -n production",
					"whctl describe app myapp -n production -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runDescribe(opts)
				},
			},
			{
				Verb:         "import",
				Short:        "Wrap an existing docker-compose.yml into an app manifest without deploying",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags:        fileFlag,
				Examples: []string{
					"whctl import app myapp -n production -f docker-compose.yml",
					"whctl import app myapp -n production -f docker-compose.yml --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runImport(opts)
				},
			},
			{
				Verb:         "start",
				Short:        "Generate compose file, sync to remote, and run docker compose up",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl start app myapp -n production",
					"whctl start app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runStart(opts)
				},
			},
			{
				Verb:         "pause",
				Short:        "Run docker compose down on the remote, keeping remote files in place",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl pause app myapp -n production",
					"whctl pause app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runPause(opts)
				},
			},
			{
				Verb:         "stop",
				Short:        "Run docker compose down and remove all remote files for an app",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl stop app myapp -n production",
					"whctl stop app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runStop(opts)
				},
			},
			{
				Verb:         "pull",
				Short:        "Pull the latest images on the remote without restarting containers",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl pull app myapp -n production",
					"whctl pull app myapp -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runPull(opts)
				},
			},
			{
				Verb:         "logs",
				Short:        "Print or stream logs from a running app's containers",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "follow", Short: "f", Type: "bool", Usage: "Follow log output"},
					{Name: "tail", Type: "int", Default: -1, Usage: "Number of lines from end (-1 = all)"},
					{Name: "timestamps", Short: "T", Type: "bool", Usage: "Show timestamps"},
					{Name: "service", Short: "s", Type: "string", Usage: "Limit to one service"},
				},
				Examples: []string{
					"whctl logs app myapp -n production",
					"whctl logs app myapp -n production --follow",
					"whctl logs app myapp -n production --tail 100 --timestamps",
					"whctl logs app myapp -n production --service worker",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runLogs(opts)
				},
			},
			{
				Verb:         "exec",
				Short:        "Open an interactive shell or run a command in a running container",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "service", Short: "s", Type: "string", Usage: "Target service (default: first service)"},
					{Name: "tty", Short: "t", Type: "bool", Default: true, Usage: "Allocate a TTY"},
					{Name: "cmd", Short: "c", Type: "string", Usage: "Command to run in the container"},
				},
				Examples: []string{
					"whctl exec app myapp -n production",
					"whctl exec app myapp -n production --service worker",
					"whctl exec app myapp -n production --cmd bash",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runExec(opts)
				},
			},
			{
				Verb:         "doctor",
				Short:        "Check app manifests and envFrom references for issues",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl doctor apps -n production",
					"whctl doctor app myapp -n production",
					"whctl doctor apps -A",
					"whctl doctor apps -A -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runDoctor(opts)
				},
			},
		},
		SummaryColumns: []string{"NAMESPACE", "NAME", "IMAGE", "READY", "STATUS"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newApp(dataDir, filesystem)
		},
	})
}
