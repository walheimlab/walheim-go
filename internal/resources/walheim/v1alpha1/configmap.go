package v1alpha1

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/doctor"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/output"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

// ── KindInfo ──────────────────────────────────────────────────────────────────

var configMapKind = resource.KindInfo{
	Group:   "",
	Version: "v1",
	Kind:    "ConfigMap",
	Plural:  "configmaps",
	Aliases: []string{"cm"},
}

// ── Handler ───────────────────────────────────────────────────────────────────

// ConfigMap is the handler for the ConfigMap resource kind.
// ConfigMaps are purely local — they are never synced to any host.
// They are loaded by generateCompose when building the docker-compose.yml for an App.
type ConfigMap struct {
	resource.NamespacedBase
}

func newConfigMap(dataDir string, filesystem fs.FS) *ConfigMap {
	return &ConfigMap{
		NamespacedBase: resource.NamespacedBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             configMapKind,
			ManifestFilename: ".configmap.yaml",
		},
	}
}

func (c *ConfigMap) KindInfo() resource.KindInfo { return configMapKind }

// ── Typed read/list helpers ───────────────────────────────────────────────────

func (c *ConfigMap) parseManifest(namespace, name string) (*ConfigMapManifest, error) {
	data, err := c.ReadBytes(namespace, name)
	if err != nil {
		return nil, err
	}

	var m ConfigMapManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse configmap %q in namespace %q: %w", name, namespace, err)
	}

	return &m, nil
}

// configMapKeys returns a sorted, comma-joined list of data keys.
func configMapKeys(m *ConfigMapManifest) string {
	keys := make([]string, 0, len(m.Data))
	for k := range m.Data {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return strings.Join(keys, ", ")
}

func configMapToMeta(namespace, name string, m *ConfigMapManifest) resource.ResourceMeta {
	return resource.ResourceMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    m.Metadata.Labels,
		Summary: map[string]string{
			"KEYS": configMapKeys(m),
		},
		Raw: m,
	}
}

func (c *ConfigMap) getOne(namespace, name string) (resource.ResourceMeta, error) {
	exists, err := c.Exists(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, err
	}

	if !exists {
		return resource.ResourceMeta{},
			exitcode.New(exitcode.NotFound, fmt.Errorf("configmap %q not found in namespace %q", name, namespace))
	}

	m, err := c.parseManifest(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, err
	}

	return configMapToMeta(namespace, name, m), nil
}

func (c *ConfigMap) listNamespace(namespace string) ([]resource.ResourceMeta, error) {
	names, err := c.ListNames(namespace)
	if err != nil {
		return nil, err
	}

	items := make([]resource.ResourceMeta, 0, len(names))
	for _, name := range names {
		m, err := c.parseManifest(namespace, name)
		if err != nil {
			output.Warnf("skipping configmap %q in namespace %q: %v", name, namespace, err)
			continue
		}

		items = append(items, configMapToMeta(namespace, name, m))
	}

	return items, nil
}

// ── Validation ────────────────────────────────────────────────────────────────

func validateConfigMapManifest(m *ConfigMapManifest, namespace, name string) error {
	if m.APIVersion != configMapKind.APIVersion() {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", configMapKind.APIVersion(), m.APIVersion)
	}

	if m.Kind != configMapKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", configMapKind.Kind, m.Kind)
	}

	if m.Metadata.Name != name {
		return fmt.Errorf("metadata.name %q does not match argument %q", m.Metadata.Name, name)
	}

	if m.Metadata.Namespace != namespace {
		return fmt.Errorf("metadata.namespace %q does not match -n %q", m.Metadata.Namespace, namespace)
	}

	return nil
}

// ── Operations ────────────────────────────────────────────────────────────────

func (c *ConfigMap) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name != "" {
		meta, err := c.getOne(opts.Namespace, opts.Name)
		if err != nil {
			output.Errorf(jsonMode, "NotFound",
				fmt.Sprintf("configmap %q not found in namespace %q", opts.Name, opts.Namespace), "", nil, false)

			return err
		}

		return output.PrintOne(meta, opts.Output)
	}

	if opts.AllNamespaces {
		namespaces, err := c.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		var items []resource.ResourceMeta

		for _, ns := range namespaces {
			nsItems, err := c.listNamespace(ns)
			if err != nil {
				return exitErr(exitcode.Failure, err)
			}

			items = append(items, nsItems...)
		}

		if len(items) == 0 {
			output.PrintEmpty("", configMapKind, opts.Output, opts.Quiet)
			return nil
		}

		return output.PrintList(items, []string{"NAMESPACE", "NAME", "KEYS"}, configMapKind, opts.Output, opts.Quiet)
	}

	items, err := c.listNamespace(opts.Namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(items) == 0 {
		output.PrintEmpty(opts.Namespace, configMapKind, opts.Output, opts.Quiet)
		return nil
	}

	return output.PrintList(items, []string{"NAME", "KEYS"}, configMapKind, opts.Output, opts.Quiet)
}

func (c *ConfigMap) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	filePath := opts.String("file")
	if filePath == "" {
		msg := "--file (-f) is required for 'apply configmap'"
		output.Errorf(jsonMode, "UsageError", msg,
			"whctl apply configmap <name> -n <namespace> -f <path>", nil, false)

		return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
	}

	data, err := readInput(filePath, opts.FS)
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
	}

	var m ConfigMapManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateConfigMapManifest(&m, namespace, name); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	exists, err := c.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}

		fmt.Printf("Would %s configmap %q in namespace %q\n", verb, name, namespace)

		return nil
	}

	if !exists {
		if err := c.EnsureDir(namespace, name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := c.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Created configmap %q in namespace %q\n", name, namespace)
	} else {
		if err := c.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Updated configmap %q in namespace %q\n", name, namespace)
	}

	return nil
}

func (c *ConfigMap) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	exists, err := c.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("configmap %q not found in namespace %q", name, namespace)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would delete configmap %q in namespace %q\n", name, namespace)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete configmap %q in namespace %q?", name, namespace)); err != nil {
		return err
	}

	if err := c.RemoveDir(namespace, name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted configmap %q in namespace %q\n", name, namespace)

	return nil
}

// ── doctor ────────────────────────────────────────────────────────────────────

func (c *ConfigMap) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	if opts.Name != "" {
		namespace := opts.Namespace

		exists, err := c.Exists(namespace, opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("configmap %q not found in namespace %q", opts.Name, namespace)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		c.doctorConfigMap(rep, namespace, opts.Name)
	} else if opts.AllNamespaces {
		namespaces, err := c.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		for _, ns := range namespaces {
			names, err := c.ListNames(ns)
			if err != nil {
				rep.Warnf("namespace/"+ns, "list-configmaps", "cannot list configmaps: %v", err)
				continue
			}

			for _, name := range names {
				c.doctorConfigMap(rep, ns, name)
			}
		}
	} else {
		namespace := opts.Namespace

		names, err := c.ListNames(namespace)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		for _, name := range names {
			c.doctorConfigMap(rep, namespace, name)
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

func (c *ConfigMap) doctorConfigMap(rep *doctor.Report, namespace, name string) {
	resourceID := "configmap/" + namespace + "/" + name

	data, err := c.ReadBytes(namespace, name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m ConfigMapManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, configMapKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, configMapKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Metadata.Name)
	doctor.CheckNamespaceFieldMatchesDir(rep, resourceID, m.Metadata.Namespace, namespace)

	if len(m.Data) == 0 {
		rep.Warnf(resourceID, "empty-configmap", "configmap has no data keys")
	}
}

// ── Registration ──────────────────────────────────────────────────────────────

func registerConfigMap() {
	fileFlag := []registry.FlagDef{
		{Name: "file", Short: "f", Type: "string", Usage: "Manifest file path, or - for stdin"},
	}

	registry.Register(registry.Registration{
		Info:  configMapKind,
		Scope: registry.NamespaceScoped,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List configmaps in a namespace, or get a single configmap by name",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl get configmaps -n production",
					"whctl get configmap app-config -n production",
					"whctl get cm -A",
					"whctl get configmaps -n production -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*ConfigMap).runGet(opts)
				},
			},
			{
				Verb:         "apply",
				Short:        "Create or update a configmap from a manifest file",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags:        fileFlag,
				Examples: []string{
					"whctl apply configmap app-config -n production -f app-config.yaml",
					"whctl apply cm app-config -n production -f app-config.yaml --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*ConfigMap).runApply(opts)
				},
			},
			{
				Verb:         "delete",
				Short:        "Delete a configmap and remove its local manifest",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl delete configmap app-config -n production --yes",
					"whctl delete cm app-config -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*ConfigMap).runDelete(opts)
				},
			},
			{
				Verb:         "doctor",
				Short:        "Check configmap manifests for issues",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl doctor configmaps -n production",
					"whctl doctor configmap app-config -n production",
					"whctl doctor configmaps -A",
					"whctl doctor configmaps -A -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*ConfigMap).runDoctor(opts)
				},
			},
		},
		SummaryColumns: []string{"NAME", "KEYS"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newConfigMap(dataDir, filesystem)
		},
	})
}
