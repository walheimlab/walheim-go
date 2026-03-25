package v1alpha1

import (
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

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
