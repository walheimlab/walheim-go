package v1alpha1

import (
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

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
				Verb:         "describe",
				Short:        "Show daemonset details with live per-namespace status",
				NSHandling:   registry.NSNone,
				RequiresName: true,
				Examples: []string{
					"whctl describe daemonset my-service",
					"whctl describe daemonset my-service -o json",
					"whctl describe daemonset my-service -o yaml",
				},
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*DaemonSet).runDescribe(opts)
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
