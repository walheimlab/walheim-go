package v1alpha1

import (
	"encoding/base64"
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

var secretKind = resource.KindInfo{
	Group:   "",
	Version: "v1",
	Kind:    "Secret",
	Plural:  "secrets",
	Aliases: []string{},
}

// ── Handler ───────────────────────────────────────────────────────────────────

// Secret is the handler for the Secret resource kind.
// Secrets are purely local — they are never synced to any host.
// They are loaded by generateCompose when building the docker-compose.yml for an App.
type Secret struct {
	resource.NamespacedBase
}

func newSecret(dataDir string, filesystem fs.FS) *Secret {
	return &Secret{
		NamespacedBase: resource.NamespacedBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             secretKind,
			ManifestFilename: ".secret.yaml",
		},
	}
}

func (s *Secret) KindInfo() resource.KindInfo { return secretKind }

// ── Typed read/list helpers ───────────────────────────────────────────────────

func (s *Secret) parseManifest(namespace, name string) (*SecretManifest, error) {
	data, err := s.ReadBytes(namespace, name)
	if err != nil {
		return nil, err
	}

	var m SecretManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse secret %q in namespace %q: %w", name, namespace, err)
	}

	return &m, nil
}

// secretKeys returns a sorted, comma-joined union of keys from data and stringData.
func secretKeys(m *SecretManifest) string {
	seen := make(map[string]struct{})
	for k := range m.Data {
		seen[k] = struct{}{}
	}

	for k := range m.StringData {
		seen[k] = struct{}{}
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return strings.Join(keys, ", ")
}

func secretToMeta(namespace, name string, m *SecretManifest) resource.ResourceMeta {
	return resource.ResourceMeta{
		Namespace: namespace,
		Name:      name,
		Labels:    m.Metadata.Labels,
		Summary: map[string]string{
			"TYPE": "Opaque",
			"KEYS": secretKeys(m),
		},
		Raw: m,
	}
}

func (s *Secret) getOne(namespace, name string) (resource.ResourceMeta, error) {
	exists, err := s.Exists(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, err
	}

	if !exists {
		return resource.ResourceMeta{},
			exitcode.New(exitcode.NotFound, fmt.Errorf("secret %q not found in namespace %q", name, namespace))
	}

	m, err := s.parseManifest(namespace, name)
	if err != nil {
		return resource.ResourceMeta{}, err
	}

	return secretToMeta(namespace, name, m), nil
}

func (s *Secret) listNamespace(namespace string) ([]resource.ResourceMeta, error) {
	names, err := s.ListNames(namespace)
	if err != nil {
		return nil, err
	}

	items := make([]resource.ResourceMeta, 0, len(names))
	for _, name := range names {
		m, err := s.parseManifest(namespace, name)
		if err != nil {
			output.Warnf("skipping secret %q in namespace %q: %v", name, namespace, err)
			continue
		}

		items = append(items, secretToMeta(namespace, name, m))
	}

	return items, nil
}

// ── Validation ────────────────────────────────────────────────────────────────

func validateSecretManifest(m *SecretManifest, namespace, name string) error {
	if m.APIVersion != secretKind.APIVersion() {
		return fmt.Errorf("invalid apiVersion: expected %q, got %q", secretKind.APIVersion(), m.APIVersion)
	}

	if m.Kind != secretKind.Kind {
		return fmt.Errorf("invalid kind: expected %q, got %q", secretKind.Kind, m.Kind)
	}

	if m.Metadata.Name != name {
		return fmt.Errorf("metadata.name %q does not match argument %q", m.Metadata.Name, name)
	}

	if m.Metadata.Namespace != namespace {
		return fmt.Errorf("metadata.namespace %q does not match -n %q", m.Metadata.Namespace, namespace)
	}

	for k, v := range m.Data {
		if _, err := base64.StdEncoding.DecodeString(v); err != nil {
			return fmt.Errorf("data[%q] is not valid base64: %w", k, err)
		}
	}

	return nil
}

// ── Operations ────────────────────────────────────────────────────────────────

func (s *Secret) runGet(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"

	if opts.Name != "" {
		meta, err := s.getOne(opts.Namespace, opts.Name)
		if err != nil {
			output.Errorf(jsonMode, "NotFound",
				fmt.Sprintf("secret %q not found in namespace %q", opts.Name, opts.Namespace), "", nil, false)

			return err
		}

		return output.PrintOne(meta, jsonMode)
	}

	if opts.AllNamespaces {
		namespaces, err := s.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		var items []resource.ResourceMeta

		for _, ns := range namespaces {
			nsItems, err := s.listNamespace(ns)
			if err != nil {
				return exitErr(exitcode.Failure, err)
			}

			items = append(items, nsItems...)
		}

		if len(items) == 0 {
			output.PrintEmpty("secrets", "", jsonMode, opts.Quiet)
			return nil
		}

		return output.PrintList(items, []string{"NAMESPACE", "NAME", "TYPE", "KEYS"}, jsonMode, opts.Quiet)
	}

	items, err := s.listNamespace(opts.Namespace)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if len(items) == 0 {
		output.PrintEmpty("secrets", opts.Namespace, jsonMode, opts.Quiet)
		return nil
	}

	return output.PrintList(items, []string{"NAME", "TYPE", "KEYS"}, jsonMode, opts.Quiet)
}

func (s *Secret) runApply(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	filePath := opts.String("file")
	if filePath == "" {
		msg := "--file (-f) is required for 'apply secret'"
		output.Errorf(jsonMode, "UsageError", msg,
			"whctl apply secret <name> -n <namespace> -f <path>", nil, false)

		return exitErr(exitcode.UsageError, fmt.Errorf("%s", msg))
	}

	data, err := readInput(filePath, opts.FS)
	if err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("read %q: %w", filePath, err))
	}

	var m SecretManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return exitErr(exitcode.Failure, fmt.Errorf("parse manifest: %w", err))
	}

	if err := validateSecretManifest(&m, namespace, name); err != nil {
		output.Errorf(jsonMode, "ValidationError", err.Error(), "", nil, false)
		return exitErr(exitcode.UsageError, err)
	}

	exists, err := s.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if opts.DryRun {
		verb := "create"
		if exists {
			verb = "update"
		}

		fmt.Printf("Would %s secret %q in namespace %q\n", verb, name, namespace)

		return nil
	}

	if !exists {
		if err := s.EnsureDir(namespace, name); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if err := s.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Created secret %q in namespace %q\n", name, namespace)
	} else {
		if err := s.WriteManifest(namespace, name, &m); err != nil {
			return exitErr(exitcode.Failure, err)
		}

		fmt.Printf("Updated secret %q in namespace %q\n", name, namespace)
	}

	return nil
}

func (s *Secret) runDelete(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	namespace := opts.Namespace
	name := opts.Name

	exists, err := s.Exists(namespace, name)
	if err != nil {
		return exitErr(exitcode.Failure, err)
	}

	if !exists {
		msg := fmt.Sprintf("secret %q not found in namespace %q", name, namespace)
		output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

		return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
	}

	if opts.DryRun {
		fmt.Printf("Would delete secret %q in namespace %q\n", name, namespace)
		return nil
	}

	if err := promptConfirm(opts.Yes,
		fmt.Sprintf("Delete secret %q in namespace %q?", name, namespace)); err != nil {
		return err
	}

	if err := s.RemoveDir(namespace, name); err != nil {
		return exitErr(exitcode.Failure, err)
	}

	fmt.Printf("Deleted secret %q in namespace %q\n", name, namespace)

	return nil
}

// ── doctor ────────────────────────────────────────────────────────────────────

func (s *Secret) runDoctor(opts registry.OperationOpts) error {
	jsonMode := opts.Output == "json"
	rep := &doctor.Report{}

	if opts.Name != "" {
		namespace := opts.Namespace

		exists, err := s.Exists(namespace, opts.Name)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		if !exists {
			msg := fmt.Sprintf("secret %q not found in namespace %q", opts.Name, namespace)
			output.Errorf(jsonMode, "NotFound", msg, "", nil, false)

			return exitErr(exitcode.NotFound, fmt.Errorf("%s", msg))
		}

		s.doctorSecret(rep, namespace, opts.Name)
	} else if opts.AllNamespaces {
		namespaces, err := s.ValidNamespaces()
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		for _, ns := range namespaces {
			names, err := s.ListNames(ns)
			if err != nil {
				rep.Warnf("namespace/"+ns, "list-secrets", "cannot list secrets: %v", err)
				continue
			}

			for _, name := range names {
				s.doctorSecret(rep, ns, name)
			}
		}
	} else {
		namespace := opts.Namespace

		names, err := s.ListNames(namespace)
		if err != nil {
			return exitErr(exitcode.Failure, err)
		}

		for _, name := range names {
			s.doctorSecret(rep, namespace, name)
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

func (s *Secret) doctorSecret(rep *doctor.Report, namespace, name string) {
	resourceID := "secret/" + namespace + "/" + name

	data, err := s.ReadBytes(namespace, name)
	if err != nil {
		rep.Errorf(resourceID, "manifest-unreadable", "cannot read manifest: %v", err)
		return
	}

	var m SecretManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		rep.Errorf(resourceID, "manifest-parse", "manifest YAML is invalid: %v", err)
		return
	}

	doctor.CheckAPIVersion(rep, resourceID, m.APIVersion, secretKind.APIVersion())
	doctor.CheckKind(rep, resourceID, m.Kind, secretKind.Kind)
	doctor.CheckDirNameMatchesMetadataName(rep, resourceID, name, m.Metadata.Name)
	doctor.CheckNamespaceFieldMatchesDir(rep, resourceID, m.Metadata.Namespace, namespace)

	for k, v := range m.Data {
		if _, err := base64.StdEncoding.DecodeString(v); err != nil {
			rep.Errorf(resourceID, "invalid-base64",
				"data[%q] is not valid base64: %v", k, err)
		}
	}

	if len(m.Data) == 0 && len(m.StringData) == 0 {
		rep.Warnf(resourceID, "empty-secret", "secret has no data or stringData keys")
	}
}

// ── Registration ──────────────────────────────────────────────────────────────

func registerSecret() {
	fileFlag := []registry.FlagDef{
		{Name: "file", Short: "f", Type: "string", Usage: "Manifest file path, or - for stdin"},
	}

	registry.Register(registry.Registration{
		Info:  secretKind,
		Scope: registry.NamespaceScoped,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List secrets in a namespace, or get a single secret by name",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl get secrets -n production",
					"whctl get secret db-creds -n production",
					"whctl get secrets -A",
					"whctl get secrets -n production -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Secret).runGet(opts)
				},
			},
			{
				Verb:         "apply",
				Short:        "Create or update a secret from a manifest file",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Flags:        fileFlag,
				Examples: []string{
					"whctl apply secret db-creds -n production -f db-creds.yaml",
					"whctl apply secret db-creds -n production -f db-creds.yaml --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Secret).runApply(opts)
				},
			},
			{
				Verb:         "delete",
				Short:        "Delete a secret and remove its local manifest",
				NSHandling:   registry.NSRequired,
				RequiresName: true,
				Examples: []string{
					"whctl delete secret db-creds -n production --yes",
					"whctl delete secret db-creds -n production --dry-run",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Secret).runDelete(opts)
				},
			},
			{
				Verb:         "doctor",
				Short:        "Check secret manifests for issues (invalid base64, missing fields)",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Examples: []string{
					"whctl doctor secrets -n production",
					"whctl doctor secret db-creds -n production",
					"whctl doctor secrets -A",
					"whctl doctor secrets -A -o json",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Secret).runDoctor(opts)
				},
			},
		},
		SummaryColumns: []string{"NAME", "TYPE", "KEYS"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newSecret(dataDir, filesystem)
		},
	})
}
