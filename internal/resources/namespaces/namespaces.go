// Package namespaces registers the Namespace cluster-scoped resource.
package namespaces

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/ssh"
)

// ── Typed manifest structs ─────────────────────────────────────────────────────

// Manifest is the typed representation of a .namespace.yaml file.
type Manifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   ManifestMetadata `yaml:"metadata"`
	Spec       NamespaceSpec    `yaml:"spec"`
}

// ManifestMetadata holds the metadata block.
type ManifestMetadata struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

// NamespaceSpec holds the spec block.
type NamespaceSpec struct {
	Hostname string `yaml:"hostname"`
	Username string `yaml:"username,omitempty"`
}

// sshTarget returns "user@host" or "host" depending on whether Username is set.
func (s NamespaceSpec) sshTarget() string {
	if s.Username != "" {
		return s.Username + "@" + s.Hostname
	}
	return s.Hostname
}

// usernameDisplay returns the username for human display.
func (s NamespaceSpec) usernameDisplay() string {
	if s.Username != "" {
		return s.Username
	}
	return "(from SSH config)"
}

// ── Validation ────────────────────────────────────────────────────────────────

func validateManifest(m *Manifest) error {
	if m.APIVersion != "walheim/v1alpha1" {
		return fmt.Errorf("invalid apiVersion: expected walheim/v1alpha1, got %q", m.APIVersion)
	}
	if m.Kind != "Namespace" {
		return fmt.Errorf("invalid kind: expected Namespace, got %q", m.Kind)
	}
	if m.Spec.Hostname == "" {
		return fmt.Errorf("spec.hostname is required")
	}
	return nil
}

// ── Handler ───────────────────────────────────────────────────────────────────

var kindInfo = resource.KindInfo{
	Plural:   "namespaces",
	Singular: "namespace",
	Aliases:  []string{"ns"},
}

// Namespaces is the handler for the Namespace resource kind.
type Namespaces struct {
	resource.ClusterBase
}

// NewNamespaces creates a new Namespaces handler.
func NewNamespaces(dataDir string, filesystem fs.FS) *Namespaces {
	return &Namespaces{
		ClusterBase: resource.ClusterBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             kindInfo,
			ManifestFilename: ".namespace.yaml",
		},
	}
}

// KindInfo implements resource.Handler.
func (n *Namespaces) KindInfo() resource.KindInfo { return kindInfo }

// ── Typed read/list ───────────────────────────────────────────────────────────

// parseManifest reads and parses the manifest for name.
func (n *Namespaces) parseManifest(name string) (*Manifest, error) {
	data, err := n.ReadBytes(name)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest for namespace %q: %w", name, err)
	}
	return &m, nil
}

// toResourceMeta converts a typed Manifest into a ResourceMeta.
func toResourceMeta(name string, m *Manifest) resource.ResourceMeta {
	return resource.ResourceMeta{
		Name:   name,
		Labels: m.Metadata.Labels,
		Summary: map[string]string{
			"HOSTNAME": hostnameDisplay(m.Spec.Hostname),
			"USERNAME": m.Spec.usernameDisplay(),
		},
		Raw: m,
	}
}

func hostnameDisplay(h string) string {
	if h == "" {
		return "N/A"
	}
	return h
}

// getOne fetches a single namespace by name. Returns exitcode.Error on not-found.
func (n *Namespaces) getOne(name string) (resource.ResourceMeta, *Manifest, error) {
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
	return toResourceMeta(name, m), m, nil
}

// listAll returns ResourceMeta for every namespace with a valid manifest.
func (n *Namespaces) listAll() ([]resource.ResourceMeta, error) {
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
		items = append(items, toResourceMeta(name, m))
	}
	return items, nil
}

// ── Subdirectory helpers ──────────────────────────────────────────────────────

// createSubdirs creates apps/, secrets/, and configmaps/ inside the namespace dir.
func (n *Namespaces) createSubdirs(name string) error {
	nsDir := n.ResourceDir(name)
	for _, sub := range []string{"apps", "secrets", "configmaps"} {
		if err := n.FS.MkdirAll(filepath.Join(nsDir, sub)); err != nil {
			return fmt.Errorf("create %s subdir: %w", sub, err)
		}
	}
	return nil
}

// countLocalResources counts non-hidden subdirs in apps/, secrets/, configmaps/.
func (n *Namespaces) countLocalResources(nsName string) resourceCounts {
	nsDir := n.ResourceDir(nsName)
	count := func(sub string) int {
		entries, err := n.FS.ReadDir(filepath.Join(nsDir, sub))
		if err != nil {
			return 0
		}
		return len(entries)
	}
	return resourceCounts{
		Apps:       count("apps"),
		Secrets:    count("secrets"),
		ConfigMaps: count("configmaps"),
	}
}

// ── Operations ────────────────────────────────────────────────────────────────

func (n *Namespaces) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name == "" {
		items, err := n.listAll()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}
		if len(items) == 0 {
			output.PrintEmpty("namespaces", "", jsonMode, opts.Quiet)
			return nil
		}
		return output.PrintList(items, []string{"NAME", "HOSTNAME", "USERNAME"}, jsonMode, opts.Quiet)
	}

	meta, _, err := n.getOne(opts.Name)
	if err != nil {
		output.Errorf(jsonMode, "NotFound",
			fmt.Sprintf("namespace %q not found", opts.Name), "", nil, false)
		return err
	}
	return output.PrintOne(meta, jsonMode)
}

func (n *Namespaces) runCreate(opts registry.OperationOpts) error {
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

	m := &Manifest{
		APIVersion: "walheim/v1alpha1",
		Kind:       "Namespace",
		Metadata:   ManifestMetadata{Name: name},
		Spec:       NamespaceSpec{Hostname: hostname, Username: username},
	}

	if opts.DryRun {
		fmt.Printf("Would create namespace %q (hostname: %s, username: %s)\n",
			name, hostname, m.Spec.usernameDisplay())
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

	fmt.Printf("Created namespace %q (hostname: %s, username: %s)\n",
		name, hostname, m.Spec.usernameDisplay())
	return nil
}

func (n *Namespaces) runApply(opts registry.OperationOpts) error {
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

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateManifest(&m); err != nil {
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

func (n *Namespaces) runDelete(opts registry.OperationOpts) error {
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

// describeResult is the full describe output used in JSON mode.
type describeResult struct {
	Name         string         `json:"name"`
	Hostname     string         `json:"hostname"`
	Username     string         `json:"username,omitempty"`
	SSH          string         `json:"ssh"`
	Connection   string         `json:"connection"`
	Docker       *string        `json:"docker"`
	DeployedApps []deployedApp  `json:"deployed_apps,omitempty"`
	Resources    resourceCounts `json:"resources"`
	Usage        *usageInfo     `json:"usage,omitempty"`
}

type deployedApp struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status"`
}

type resourceCounts struct {
	Apps       int `json:"apps"`
	Secrets    int `json:"secrets"`
	ConfigMaps int `json:"configmaps"`
}

type usageInfo struct {
	Disk       string `json:"disk"`
	Containers string `json:"containers"`
}

func (n *Namespaces) runDescribe(opts registry.OperationOpts) error {
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

func (n *Namespaces) describeJSON(name string, m *Manifest, target string) error {
	result := describeResult{
		Name:     name,
		Hostname: m.Spec.Hostname,
		Username: m.Spec.Username,
		SSH:      target,
	}

	client := ssh.NewClient(target)
	connected := client.TestConnection()
	if connected {
		result.Connection = "Connected"
	} else {
		result.Connection = "Failed"
	}

	if connected {
		v := describeDockerVersion(client)
		result.Docker = &v
		result.DeployedApps = describeContainers(client, name)
		result.Usage = describeUsage(client)
	}

	result.Resources = n.countLocalResources(name)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func (n *Namespaces) describeHuman(name string, m *Manifest, target string) error {
	fmt.Printf("Name:      %s\n", name)
	fmt.Printf("Hostname:  %s\n", m.Spec.Hostname)
	fmt.Printf("Username:  %s\n", m.Spec.usernameDisplay())
	fmt.Printf("SSH:       %s\n", target)
	fmt.Println()

	client := ssh.NewClient(target)
	connected := client.TestConnection()

	if connected {
		fmt.Println("Connection:  Connected")
	} else {
		fmt.Println("Connection:  Failed")
		fmt.Println()
		fmt.Println("Unable to connect. Check SSH configuration.")
		return nil
	}

	fmt.Printf("Docker:      %s\n", describeDockerVersion(client))
	fmt.Println()

	if apps := describeContainers(client, name); len(apps) > 0 {
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

	if u := describeUsage(client); u != nil {
		fmt.Println("Usage:")
		fmt.Printf("  Disk:        %s\n", u.Disk)
		fmt.Printf("  Containers:  %s\n", u.Containers)
	}

	return nil
}

// ── SSH probe helpers ─────────────────────────────────────────────────────────

func describeDockerVersion(client *ssh.Client) string {
	out, err := client.RunOutput("docker --version 2>/dev/null")
	if err != nil || strings.TrimSpace(out) == "" {
		return "Not available"
	}
	// "Docker version 24.0.7, build afdd53b4e3"
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 3 {
		ver := strings.TrimSuffix(parts[2], ",")
		return fmt.Sprintf("Available (v%s)", ver)
	}
	return fmt.Sprintf("Available (%s)", strings.TrimSpace(out))
}

func describeContainers(client *ssh.Client, nsName string) []deployedApp {
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

	result := make([]deployedApp, 0, len(order))
	for _, appName := range order {
		a := aggMap[appName]
		result = append(result, deployedApp{
			Name:   appName,
			State:  appState(a.state, a.running, a.total),
			Status: strconv.Itoa(a.running) + "/" + strconv.Itoa(a.total),
		})
	}
	return result
}

func appState(rawState string, running, total int) string {
	switch {
	case running == total && total > 0:
		return "Running"
	case running > 0:
		return "Degraded"
	case strings.ToLower(rawState) == "paused":
		return "Paused"
	case strings.ToLower(rawState) == "exited",
		strings.ToLower(rawState) == "dead",
		strings.ToLower(rawState) == "stopped":
		return "Stopped"
	default:
		return "Unknown"
	}
}

func describeUsage(client *ssh.Client) *usageInfo {
	diskOut, _ := client.RunOutput("df -h /data 2>/dev/null | tail -1")
	diskStr := "N/A"
	if line := strings.TrimSpace(diskOut); line != "" {
		// df -h: Filesystem  Size  Used  Avail  Use%  Mounted
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
	return &usageInfo{Disk: diskStr, Containers: ctStr}
}

// ── Package-local helpers ─────────────────────────────────────────────────────

// readInput reads bytes from a file path or "-" for stdin.
func readInput(filePath string, filesystem fs.FS) ([]byte, error) {
	if filePath == "-" {
		return readStdin()
	}
	return filesystem.ReadFile(filePath)
}

func readStdin() ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 512)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

func exitErr(code int, err error) error {
	return exitcode.New(code, err)
}

func promptConfirm(yes bool, prompt string) error {
	if yes {
		return nil
	}
	fi, err := os.Stdin.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return exitcode.New(exitcode.UsageError,
			fmt.Errorf("stdin is not a TTY; pass --yes to confirm destructive operations"))
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	var answer string
	fmt.Fscan(os.Stdin, &answer)
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		return fmt.Errorf("aborted")
	}
	return nil
}

// ── init() registration ───────────────────────────────────────────────────────

var operations = []registry.OperationDef{
	{
		Verb:         "get",
		Short:        "List or retrieve namespaces",
		NSHandling:   registry.NSNone,
		RequiresName: false,
		Examples: []string{
			"whctl get namespaces",
			"whctl get namespace production",
			"whctl get ns -o json",
		},
		Run: func(h resource.Handler, opts registry.OperationOpts) error {
			return h.(*Namespaces).runGet(opts)
		},
	},
	{
		Verb:         "create",
		Short:        "Create a namespace",
		NSHandling:   registry.NSNone,
		RequiresName: true,
		Flags: []registry.FlagDef{
			{Name: "hostname", Type: "string", Usage: "SSH hostname or IP (defaults to name if omitted)"},
			{Name: "username", Type: "string", Usage: "SSH username (uses SSH config if omitted)"},
		},
		Examples: []string{
			"whctl create namespace production --hostname prod.example.com --username admin",
			"whctl create namespace staging --hostname 192.168.1.20",
		},
		Run: func(h resource.Handler, opts registry.OperationOpts) error {
			return h.(*Namespaces).runCreate(opts)
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
			return h.(*Namespaces).runApply(opts)
		},
	},
	{
		Verb:         "delete",
		Short:        "Delete a namespace and all its local contents",
		NSHandling:   registry.NSNone,
		RequiresName: true,
		Examples: []string{
			"whctl delete namespace staging --yes",
		},
		Run: func(h resource.Handler, opts registry.OperationOpts) error {
			return h.(*Namespaces).runDelete(opts)
		},
	},
	{
		Verb:         "describe",
		Short:        "Show detailed namespace status including live SSH probe",
		NSHandling:   registry.NSNone,
		RequiresName: true,
		Examples: []string{
			"whctl describe namespace production",
		},
		Run: func(h resource.Handler, opts registry.OperationOpts) error {
			return h.(*Namespaces).runDescribe(opts)
		},
	},
}

func init() {
	registry.Register(registry.Registration{
		Info:           kindInfo,
		Scope:          registry.ClusterScoped,
		Operations:     operations,
		SummaryColumns: []string{"NAME", "HOSTNAME", "USERNAME"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return NewNamespaces(dataDir, filesystem)
		},
	})
}
