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
	"github.com/walheimlab/walheim-go/internal/yamlutil"
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

func (n *Namespace) countLocalResources(nsName string) NamespaceResourceCounts {
	nsDir := n.ResourceDir(nsName)
	count := func(sub string) int {
		entries, err := n.FS.ReadDir(filepath.Join(nsDir, sub))
		if err != nil {
			return 0
		}

		return len(entries)
	}

	return NamespaceResourceCounts{
		Apps:       count("apps"),
		Secrets:    count("secrets"),
		ConfigMaps: count("configmaps"),
	}
}

// localAppNames returns the set of app names in the local context for nsName.
func (n *Namespace) localAppNames(nsName string) map[string]struct{} {
	nsDir := n.ResourceDir(nsName)

	entries, err := n.FS.ReadDir(filepath.Join(nsDir, "apps"))
	if err != nil {
		return nil
	}

	set := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		set[entry] = struct{}{}
	}

	return set
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

	meta, m, err := n.getOne(opts.Name)
	if err != nil {
		output.Errorf(jsonMode, "NotFound",
			fmt.Sprintf("namespace %q not found", opts.Name), "", nil, false)

		return err
	}

	// For structured output, populate runtime status via SSH and emit a full view.
	if opts.Output == "json" || opts.Output == "yaml" {
		return n.getWithStatus(opts.Name, m, opts.Output)
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

	var data []byte
	if len(opts.RawManifest) > 0 {
		data = opts.RawManifest
	} else {
		filePath := opts.String("file")
		if filePath == "" {
			msg := "--file (-f) is required for 'apply namespace'"
			output.Errorf(jsonMode, "UsageError", msg,
				"whctl apply namespace <name> -f <path>", nil, false)

			return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
		}

		var err error

		data, err = readInput(filePath, opts.FS)
		if err != nil {
			return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
		}
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
	APIVersion string                `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                `json:"kind" yaml:"kind"`
	Metadata   namespaceDescribeMeta `json:"metadata" yaml:"metadata"`
	Spec       namespaceDescribeSpec `json:"spec" yaml:"spec"`
	Status     *NamespaceStatus      `json:"status,omitempty" yaml:"status,omitempty"`
}

type namespaceDescribeMeta struct {
	Name string `json:"name" yaml:"name"`
}

type namespaceDescribeSpec struct {
	Hostname string `json:"hostname" yaml:"hostname"`
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	BaseDir  string `json:"baseDir" yaml:"baseDir"`
}

func (n *Namespace) runDescribe(opts registry.OperationOpts) error {
	name := opts.Name

	_, m, err := n.getOne(name)
	if err != nil {
		output.Errorf(false, "NotFound",
			fmt.Sprintf("namespace %q not found", name), "", nil, false)

		return err
	}

	return n.describeHuman(name, m, m.Spec.sshTarget())
}

// buildDescribeStatus connects to the namespace host and collects runtime status.
func (n *Namespace) buildDescribeStatus(name, target string) *NamespaceStatus {
	status := &NamespaceStatus{
		Resources: n.countLocalResources(name),
	}

	client := ssh.NewClient(target)
	if client.TestConnection() {
		status.Connection = "Connected"
		status.Docker = namespaceDockerStatus(client)
		info := namespaceCollectStatus(client, name, n.localAppNames(name))
		status.DeployedApps = info.DeployedApps
		status.Containers = info.Containers
		status.Usage = namespaceUsageInfo(client)
	} else {
		status.Connection = "Failed"
	}

	return status
}

func (n *Namespace) getWithStatus(name string, m *NamespaceManifest, format string) error {
	target := m.Spec.sshTarget()

	result := namespaceDescribeResult{
		APIVersion: m.APIVersion,
		Kind:       m.Kind,
		Metadata:   namespaceDescribeMeta{Name: name},
		Spec: namespaceDescribeSpec{
			Hostname: m.Spec.Hostname,
			Username: m.Spec.Username,
			BaseDir:  m.Spec.remoteBaseDir(),
		},
		Status: n.buildDescribeStatus(name, target),
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(result)
	}

	// yaml
	data, err := yamlutil.Marshal(result)
	if err != nil {
		return err
	}

	fmt.Print(string(data))

	return nil
}

func (n *Namespace) describeHuman(name string, m *NamespaceManifest, target string) error {
	fmt.Printf("Name:      %s\n", name)
	fmt.Printf("Hostname:  %s\n", m.Spec.Hostname)
	fmt.Printf("Username:  %s\n", m.Spec.usernameDisplay())
	fmt.Printf("Base Dir:  %s\n", m.Spec.baseDirDisplay())
	fmt.Printf("SSH:       %s\n", target)
	fmt.Println()

	fmt.Println("Status:")

	status := n.buildDescribeStatus(name, target)

	fmt.Printf("  Connection:  %s\n", status.Connection)

	if status.Connection == "Failed" {
		fmt.Println()
		fmt.Println("  Unable to connect. Check SSH configuration.")

		return nil
	}

	if d := status.Docker; d != nil {
		if d.Available {
			fmt.Printf("  Docker:      Available (v%s)\n", d.Version)
		} else {
			fmt.Println("  Docker:      Not available")
		}
	}

	if len(status.DeployedApps) > 0 {
		fmt.Println()
		fmt.Println("  Deployed Apps:")

		for _, a := range status.DeployedApps {
			fmt.Printf("    %-20s %-12s %d/%d\n", a.Name, a.State, a.Running, a.Total)
		}
	}

	if len(status.Containers) > 0 {
		fmt.Println()
		fmt.Println("  Containers:")
		fmt.Printf("    %-30s %-20s %-10s %-25s %s\n", "NAME", "APP", "STATE", "STATUS", "WALHEIM")

		for _, c := range status.Containers {
			fmt.Printf("    %-30s %-20s %-10s %-25s %s\n", c.Name, c.App, c.State, c.DockerStatus, c.Management)
		}
	}

	fmt.Println()
	fmt.Println("  Resources:")
	fmt.Printf("    Apps:       %d\n", status.Resources.Apps)
	fmt.Printf("    Secrets:    %d\n", status.Resources.Secrets)
	fmt.Printf("    ConfigMaps: %d\n", status.Resources.ConfigMaps)

	if u := status.Usage; u != nil {
		fmt.Println()
		fmt.Println("  Usage:")

		if d := u.Disk; d != nil {
			fmt.Printf("    Disk:        %s used of %s\n", d.Used, d.Total)
		}

		if c := u.Containers; c != nil {
			fmt.Printf("    Containers:  %d running, %d stopped\n", c.Running, c.Stopped)
		}
	}

	return nil
}

func namespaceDockerStatus(client *ssh.Client) *NamespaceDockerStatus {
	out, err := client.RunOutput("docker --version 2>/dev/null")
	if err != nil || strings.TrimSpace(out) == "" {
		return &NamespaceDockerStatus{Available: false}
	}

	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 3 {
		return &NamespaceDockerStatus{
			Available: true,
			Version:   strings.TrimSuffix(parts[2], ","),
		}
	}

	return &NamespaceDockerStatus{Available: true}
}

type namespaceStatusInfo struct {
	DeployedApps []NamespaceDeployedApp
	Containers   []NamespaceContainerStatus
}

// namespaceCollectStatus fetches all containers on the host in a single
// docker ps call and returns both the deployed-app summary (walheim-managed,
// aggregated by app) and the full per-container list with management status.
func namespaceCollectStatus(client *ssh.Client, nsName string, localApps map[string]struct{}) namespaceStatusInfo {
	cmd := `docker ps -a --format '{{.Names}}|{{.Label "walheim.namespace"}}|{{.Label "walheim.app"}}|{{.State}}|{{.Status}}' 2>/dev/null`

	out, err := client.RunOutput(cmd)
	if err != nil || strings.TrimSpace(out) == "" {
		return namespaceStatusInfo{}
	}

	type appAgg struct {
		state   string
		running int
		total   int
	}

	appMap := make(map[string]*appAgg)

	var appOrder []string

	var containers []NamespaceContainerStatus

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		containerName, labelNs, labelApp, state, dockerStatus := parts[0], parts[1], parts[2], parts[3], parts[4]

		management := containerManagement(labelNs, labelApp, nsName, localApps)

		containers = append(containers, NamespaceContainerStatus{
			Name:         containerName,
			App:          labelApp,
			State:        state,
			DockerStatus: dockerStatus,
			Management:   management,
		})

		// Aggregate deployed-app summary for walheim-owned containers in this namespace.
		if labelNs == nsName && labelApp != "" {
			if _, ok := appMap[labelApp]; !ok {
				appMap[labelApp] = &appAgg{}
				appOrder = append(appOrder, labelApp)
			}

			a := appMap[labelApp]
			a.total++

			if strings.ToLower(state) == "running" {
				a.running++
			}

			if a.state == "" || strings.ToLower(state) != "running" {
				a.state = state
			}
		}
	}

	apps := make([]NamespaceDeployedApp, 0, len(appOrder))
	for _, appName := range appOrder {
		a := appMap[appName]
		apps = append(apps, NamespaceDeployedApp{
			Name:    appName,
			State:   namespaceAppState(a.state, a.running, a.total),
			Running: a.running,
			Total:   a.total,
		})
	}

	return namespaceStatusInfo{DeployedApps: apps, Containers: containers}
}

func containerManagement(labelNs, labelApp, nsName string, localApps map[string]struct{}) string {
	if labelNs == nsName && labelApp != "" {
		if localApps != nil {
			if _, ok := localApps[labelApp]; ok {
				return "managed"
			}
		}

		return "orphan"
	}

	return "unmanaged"
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

func namespaceUsageInfo(client *ssh.Client) *NamespaceUsage {
	usage := &NamespaceUsage{}
	hasData := false

	diskOut, _ := client.RunOutput("df -h /data 2>/dev/null | tail -1")
	if line := strings.TrimSpace(diskOut); line != "" {
		if parts := strings.Fields(line); len(parts) >= 3 {
			usage.Disk = &NamespaceDiskUsage{Used: parts[2], Total: parts[1]}
			hasData = true
		}
	}

	ctOut, _ := client.RunOutput("docker ps -q 2>/dev/null | wc -l; docker ps -aq 2>/dev/null | wc -l")
	if lines := strings.Split(strings.TrimSpace(ctOut), "\n"); len(lines) >= 2 {
		run, err1 := strconv.Atoi(strings.TrimSpace(lines[0]))
		total, err2 := strconv.Atoi(strings.TrimSpace(lines[1]))

		if err1 == nil && err2 == nil {
			usage.Containers = &NamespaceContainerCounts{Running: run, Stopped: total - run}
			hasData = true
		}
	}

	if !hasData {
		return nil
	}

	return usage
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
		Info:       namespaceKind,
		Scope:      registry.ClusterScoped,
		ApplyOrder: registry.ApplyOrderNamespace,
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
