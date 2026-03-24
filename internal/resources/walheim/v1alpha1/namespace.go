package v1alpha1

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/doctor"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/ssh"
)

// ── KindInfo & validation ─────────────────────────────────────────────────────

var namespaceKind = resource.KindInfo{
	Group:   "walheim",
	Version: "v1alpha1",
	Kind:    "Namespace",
	Plural:  "namespaces",
	Aliases: []string{"ns"},
}

func validateNamespaceManifest(m *NamespaceManifest) error {
	if want := namespaceKind.APIVersion(); m.APIVersion != want {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", want, m.APIVersion)
	}

	if m.Kind != namespaceKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", namespaceKind.Kind, m.Kind)
	}

	if m.Spec.Hostname == "" {
		return fmt.Errorf("spec.hostname is required")
	}

	return nil
}

// ── Handler ───────────────────────────────────────────────────────────────────

// Namespace is the handler for the Namespace resource kind.
type Namespace struct {
	resource.ClusterBase
}

func newNamespace(dataDir string, filesystem fs.FS) *Namespace {
	return &Namespace{
		ClusterBase: resource.ClusterBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             namespaceKind,
			ManifestFilename: ".namespace.yaml",
		},
	}
}

func (n *Namespace) KindInfo() resource.KindInfo { return namespaceKind }

// ── Typed read/list ───────────────────────────────────────────────────────────

func (n *Namespace) parseManifest(name string) (*NamespaceManifest, error) {
	data, err := n.ReadBytes(name)
	if err != nil {
		return nil, err
	}

	var m NamespaceManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse namespace %q: %w", name, err)
	}

	return &m, nil
}

func namespaceToMeta(name string, m *NamespaceManifest) resource.ResourceMeta {
	h := m.Spec.Hostname
	if h == "" {
		h = "N/A"
	}

	return resource.ResourceMeta{
		Name:   name,
		Labels: m.Metadata.Labels,
		Summary: map[string]string{
			"HOSTNAME": h,
			"USERNAME": m.Spec.usernameDisplay(),
			"BASE DIR": m.Spec.baseDirDisplay(),
		},
		Raw: m,
	}
}

func (n *Namespace) getOne(name string) (resource.ResourceMeta, *NamespaceManifest, error) {
	exists, err := n.Exists(name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	if !exists {
		return resource.ResourceMeta{}, nil,
			exitcode.New(exitcode.NotFound, fmt.Errorf("namespace %q not found", name))
	}

	m, err := n.parseManifest(name)
	if err != nil {
		return resource.ResourceMeta{}, nil, err
	}

	return namespaceToMeta(name, m), m, nil
}

func (n *Namespace) listAll() ([]resource.ResourceMeta, error) {
	names, err := n.ListNames()
	if err != nil {
		return nil, err
	}

	items := make([]resource.ResourceMeta, 0, len(names))
	for _, name := range names {
		m, err := n.parseManifest(name)
		if err != nil {
			return nil, err
		}

		items = append(items, namespaceToMeta(name, m))
	}

	return items, nil
}

// ── Subdirectory helpers ──────────────────────────────────────────────────────

func (n *Namespace) createSubdirs(name string) error {
	nsDir := n.ResourceDir(name)
	for _, sub := range []string{"apps", "secrets", "configmaps"} {
		if err := n.FS.MkdirAll(filepath.Join(nsDir, sub)); err != nil {
			return fmt.Errorf("create %s subdir: %w", sub, err)
		}
	}

	return nil
}

func (n *Namespace) countLocalResources(nsName string) namespaceResourceCounts {
	nsDir := n.ResourceDir(nsName)
	count := func(sub string) int {
		entries, err := n.FS.ReadDir(filepath.Join(nsDir, sub))
		if err != nil {
			return 0
		}

		return len(entries)
	}

	return namespaceResourceCounts{
		Apps:       count("apps"),
		Secrets:    count("secrets"),
		ConfigMaps: count("configmaps"),
	}
}

// ── Operations ────────────────────────────────────────────────────────────────

func (n *Namespace) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name == "" {
		items, err := n.listAll()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if len(items) == 0 {
			output.PrintEmpty("", namespaceKind, opts.Output, opts.Quiet)
			return nil
		}

		return output.PrintList(items, []string{"NAME", "HOSTNAME", "USERNAME"}, namespaceKind, opts.Output, opts.Quiet)
	}

	meta, _, err := n.getOne(opts.Name)
	if err != nil {
		output.Errorf(jsonMode, "NotFound",
			fmt.Sprintf("namespace %q not found", opts.Name), "", nil, false)

		return err
	}

	return output.PrintOne(meta, opts.Output)
}

func (n *Namespace) runCreate(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	exists, err := n.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if exists {
		msg := fmt.Sprintf("namespace %q already exists", name)
		output.Errorf(jsonMode, "Conflict", msg,
			"Use 'whctl apply' to update an existing namespace.", nil, false)

		return exitErr(exitcode.Conflict, fmt.Errorf("%s", msg))
	}

	hostname := opts.String("hostname")
	if hostname == "" {
		hostname = name
	}

	username := opts.String("username")
	baseDir := opts.String("base-dir")

	m := &NamespaceManifest{
		APIVersion: namespaceKind.APIVersion(),
		Kind:       namespaceKind.Kind,
		Metadata:   ResourceMetadata{Name: name},
		Spec:       NamespaceSpec{Hostname: hostname, Username: username, BaseDir: baseDir},
	}

	if opts.DryRun {
		fmt.Printf("Would create namespace %q (hostname: %s, username: %s, base-dir: %s)\n",
			name, hostname, m.Spec.usernameDisplay(), m.Spec.baseDirDisplay())

		return nil
	}

	if err := n.EnsureDir(name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if err := n.createSubdirs(name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if err := n.WriteManifest(name, m); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Created namespace %q (hostname: %s, username: %s, base-dir: %s)\n",
		name, hostname, m.Spec.usernameDisplay(), m.Spec.baseDirDisplay())

	return nil
}

func (n *Namespace) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	filePath := opts.String("file")
	if filePath == "" {
		msg := "--file (-f) is required for 'apply namespace'"
		output.Errorf(jsonMode, "UsageError", msg,
			"whctl apply namespace <name> -f <path>", nil, false)

		return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
	}

	data, err := readInput(filePath, opts.FS)
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
	}

	var m NamespaceManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateNamespaceManifest(&m); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	if m.Metadata.Name != name {
		msg := fmt.Sprintf("manifest metadata.name %q does not match argument %q",
			m.Metadata.Name, name)
		output.Errorf(jsonMode, "ValidationError", msg, "", nil, false)

		return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
	}

	exists, err := n.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}

		fmt.Printf("Would %s namespace %q\n", verb, name)

		return nil
	}

	if !exists {
		if err := n.EnsureDir(name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := n.createSubdirs(name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := n.WriteManifest(name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Created namespace %q\n", name)
	} else {
		if err := n.WriteManifest(name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Updated namespace %q\n", name)
	}

	return nil
}

func (n *Namespace) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	exists, err := n.Exists(name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("namespace %q not found", name)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would delete namespace %q and all its contents\n", name)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete namespace %q and all its local contents?", name)); err != nil {
		return err
	}

	if err := n.RemoveDir(name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted namespace %q\n", name)

	return nil
}

// ── describe ──────────────────────────────────────────────────────────────────

type namespaceDescribeResult struct {
	Name         string                  `json:"name"`
	Hostname     string                  `json:"hostname"`
	Username     string                  `json:"username,omitempty"`
	BaseDir      string                  `json:"base_dir"`
	SSH          string                  `json:"ssh"`
	Connection   string                  `json:"connection"`
	Docker       *string                 `json:"docker"`
	DeployedApps []namespaceDeployedApp  `json:"deployed_apps,omitempty"`
	Resources    namespaceResourceCounts `json:"resources"`
	Usage        *namespaceUsage         `json:"usage,omitempty"`
}

type namespaceDeployedApp struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status"`
}

type namespaceResourceCounts struct {
	Apps       int `json:"apps"`
	Secrets    int `json:"secrets"`
	ConfigMaps int `json:"configmaps"`
}

type namespaceUsage struct {
	Disk       string `json:"disk"`
	Containers string `json:"containers"`
}

func (n *Namespace) runDescribe(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	name := opts.Name

	_, m, err := n.getOne(name)
	if err != nil {
		output.Errorf(jsonMode, "NotFound",
			fmt.Sprintf("namespace %q not found", name), "", nil, false)

		return err
	}

	target := m.Spec.sshTarget()
	if jsonMode {
		return n.describeJSON(name, m, target)
	}

	return n.describeHuman(name, m, target)
}

func (n *Namespace) describeJSON(name string, m *NamespaceManifest, target string) error {
	result := namespaceDescribeResult{
		Name:     name,
		Hostname: m.Spec.Hostname,
		Username: m.Spec.Username,
		BaseDir:  m.Spec.remoteBaseDir(),
		SSH:      target,
	}

	client := ssh.NewClient(target)
	if client.TestConnection() {
		result.Connection = "Connected"
		v := namespaceDockerVersion(client)
		result.Docker = &v
		result.DeployedApps = namespaceContainers(client, name)
		result.Usage = namespaceUsageInfo(client)
	} else {
		result.Connection = "Failed"
	}

	result.Resources = n.countLocalResources(name)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	return enc.Encode(result)
}

func (n *Namespace) describeHuman(name string, m *NamespaceManifest, target string) error {
	fmt.Printf("Name:      %s\n", name)
	fmt.Printf("Hostname:  %s\n", m.Spec.Hostname)
	fmt.Printf("Username:  %s\n", m.Spec.usernameDisplay())
	fmt.Printf("Base Dir:  %s\n", m.Spec.baseDirDisplay())
	fmt.Printf("SSH:       %s\n", target)
	fmt.Println()

	client := ssh.NewClient(target)
	if !client.TestConnection() {
		fmt.Println("Connection:  Failed")
		fmt.Println()
		fmt.Println("Unable to connect. Check SSH configuration.")

		return nil
	}

	fmt.Println("Connection:  Connected")
	fmt.Printf("Docker:      %s\n", namespaceDockerVersion(client))
	fmt.Println()

	if apps := namespaceContainers(client, name); len(apps) > 0 {
		fmt.Println("Deployed Apps:")

		for _, a := range apps {
			fmt.Printf("  %-20s %-12s %s\n", a.Name, a.State, a.Status)
		}

		fmt.Println()
	}

	counts := n.countLocalResources(name)

	fmt.Println("Resources:")
	fmt.Printf("  Apps:       %d\n", counts.Apps)
	fmt.Printf("  Secrets:    %d\n", counts.Secrets)
	fmt.Printf("  ConfigMaps: %d\n", counts.ConfigMaps)
	fmt.Println()

	if u := namespaceUsageInfo(client); u != nil {
		fmt.Println("Usage:")
		fmt.Printf("  Disk:        %s\n", u.Disk)
		fmt.Printf("  Containers:  %s\n", u.Containers)
	}

	return nil
}

func namespaceDockerVersion(client *ssh.Client) string {
	out, err := client.RunOutput("docker --version 2>/dev/null")
	if err != nil || strings.TrimSpace(out) == "" {
		return "Not available"
	}

	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 3 {
		return fmt.Sprintf("Available (v%s)", strings.TrimSuffix(parts[2], ","))
	}

	return fmt.Sprintf("Available (%s)", strings.TrimSpace(out))
}

func namespaceContainers(client *ssh.Client, nsName string) []namespaceDeployedApp {
	cmd := `docker ps -a --filter label=walheim.namespace=` + nsName +
		` --format "{{.Label "walheim.app"}}|{{.State}}|{{.Status}}" 2>/dev/null`

	out, err := client.RunOutput(cmd)
	if err != nil || strings.TrimSpace(out) == "" {
		return nil
	}

	type agg struct {
		state   string
		running int
		total   int
	}

	aggMap := make(map[string]*agg)

	var order []string

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}

		appName, state := parts[0], parts[1]
		if _, ok := aggMap[appName]; !ok {
			aggMap[appName] = &agg{}
			order = append(order, appName)
		}

		a := aggMap[appName]

		a.total++
		if strings.ToLower(state) == "running" {
			a.running++
		}

		if a.state == "" || strings.ToLower(state) != "running" {
			a.state = state
		}
	}

	result := make([]namespaceDeployedApp, 0, len(order))
	for _, appName := range order {
		a := aggMap[appName]
		result = append(result, namespaceDeployedApp{
			Name:   appName,
			State:  namespaceAppState(a.state, a.running, a.total),
			Status: strconv.Itoa(a.running) + "/" + strconv.Itoa(a.total),
		})
	}

	return result
}

func namespaceAppState(rawState string, running, total int) string {
	l := strings.ToLower(rawState)
	switch {
	case running == total && total > 0:
		return "Running"
	case running > 0:
		return "Degraded"
	case l == "paused":
		return "Paused"
	case l == "exited", l == "dead", l == "stopped":
		return "Stopped"
	default:
		return "Unknown"
	}
}

func namespaceUsageInfo(client *ssh.Client) *namespaceUsage {
	diskOut, _ := client.RunOutput("df -h /data 2>/dev/null | tail -1")
	diskStr := "N/A"

	if line := strings.TrimSpace(diskOut); line != "" {
		if parts := strings.Fields(line); len(parts) >= 3 {
			diskStr = parts[2] + " / " + parts[1]
		}
	}

	ctOut, _ := client.RunOutput("docker ps -q 2>/dev/null | wc -l; docker ps -aq 2>/dev/null | wc -l")
	ctStr := "N/A"

	if lines := strings.Split(strings.TrimSpace(ctOut), "\n"); len(lines) >= 2 {
		run, err1 := strconv.Atoi(strings.TrimSpace(lines[0]))

		total, err2 := strconv.Atoi(strings.TrimSpace(lines[1]))
		if err1 == nil && err2 == nil {
			ctStr = fmt.Sprintf("%d running, %d stopped", run, total-run)
		}
	}

	if diskStr == "N/A" && ctStr == "N/A" {
		return nil
	}

	return &namespaceUsage{Disk: diskStr, Containers: ctStr}
}

// ── doctor ────────────────────────────────────────────────────────────────────

func (n *Namespace) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	names, err := n.ListNames()
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	// If a specific name was requested, narrow down to that one.
	if opts.Name != "" {
		exists, err := n.Exists(opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("namespace %q not found", opts.Name)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		names = []string{opts.Name}
	}

	for _, name := range names {
		n.doctorNamespace(rep, name)
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

// doctorNamespace runs all checks for a single namespace and adds findings to rep.
func (n *Namespace) doctorNamespace(rep *doctor.Report, name string) {
	resourceID := "namespace/" + name

	// ── YAML parse ────────────────────────────────────────────────────────
	data, err := n.ReadBytes(name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return // nothing more can be checked without the manifest
	}

	var m NamespaceManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	// ── Common structural checks ──────────────────────────────────────────
	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, namespaceKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, namespaceKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Metadata.Name)

	// ── Namespace-specific field checks ───────────────────────────────────
	if m.Spec.Hostname == "" {
		rep.Errorf(resourceID, "missing-hostname", "spec.hostname is required but not set")
	}

	// ── Subdirectory checks ───────────────────────────────────────────────
	nsDir := n.ResourceDir(name)
	for _, sub := range []string{"apps", "secrets", "configmaps"} {
		subPath := filepath.Join(nsDir, sub)

		exists, err := n.FS.Exists(subPath)
		if err != nil {
			rep.Warnf(resourceID, "subdir-check", "cannot check %s/ directory: %v", sub, err)
			continue
		}

		if !exists {
			rep.Warnf(resourceID, "missing-subdir",
				"%s/ subdirectory is missing (run 'whctl create namespace %s' to repair)", sub, name)
		}
	}
}

// ── Registration ─────────────────────────────────────────────────────────────

func registerNamespace() {
	registry.Register(registry.Registration{
		Info:  namespaceKind,
		Scope: registry.ClusterScoped,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List namespaces or get a single namespace by name",
				NSHandling:   registry.NSNone,
				RequiresName: false,
				Examples: []string{
					"whctl get namespaces",
					"whctl get namespace production",
					"whctl get ns -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Namespace).runGet(opts)
				},
			},
			{
				Verb:         "create",
				Short:        "Create a new namespace with an SSH hostname",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "hostname", Type: "string", Usage: "SSH hostname or IP (defaults to name if omitted)"},
					{Name: "username", Type: "string", Usage: "SSH username (uses SSH config if omitted)"},
					{Name: "base-dir", Type: "string", Usage: "Remote base directory for Walheim data (default: /data/walheim)"},
				},
				Examples: []string{
					"whctl create namespace production --hostname prod.example.com --username admin",
					"whctl create namespace staging --hostname 192.168.1.20",
					"whctl create namespace homelab --hostname 192.168.1.5 --base-dir /opt/walheim",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Namespace).runCreate(opts)
				},
			},
			{
				Verb:         "apply",
				Short:        "Create or update a namespace from a manifest file",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Flags: []registry.FlagDef{
					{Name: "file", Short: "f", Type: "string", Usage: "Manifest file path, or - for stdin"},
				},
				Examples: []string{
					"whctl apply namespace production -f namespace.yaml",
					"cat namespace.yaml | whctl apply namespace production -f -",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Namespace).runApply(opts)
				},
			},
			{
				Verb:         "delete",
				Short:        "Delete a namespace and all its local contents",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Examples: []string{
					"whctl delete namespace staging --yes",
					"whctl delete namespace staging --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Namespace).runDelete(opts)
				},
			},
			{
				Verb:         "describe",
				Short:        "Show namespace details with a live SSH and Docker probe",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Examples: []string{
					"whctl describe namespace production",
					"whctl describe namespace production -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Namespace).runDescribe(opts)
				},
			},
			{
				Verb:         "doctor",
				Short:        "Check namespace manifests and directory structure for issues",
				NSHandling:   registry.NSNone,
				RequiresName: false,
				Examples: []string{
					"whctl doctor namespaces",
					"whctl doctor namespace production",
					"whctl doctor namespaces -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Namespace).runDoctor(opts)
				},
			},
		},
		SummaryColumns: []string{"NAME", "HOSTNAME", "USERNAME", "BASE DIR"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newNamespace(dataDir, filesystem)
		},
	})
}
