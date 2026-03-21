package v1alpha1

// Full implementation in plan-04.

import (
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

var appKind = resource.KindInfo{
	Group:   "walheimlab.github.io",
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

func (a *App) runGet(opts registry.OperationOpts) error {
	// Stub: plan-04 will implement this.
	return nil
}

func init() {
	registry.Register(registry.Registration{
		Info:  appKind,
		Scope: registry.NamespaceScoped,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List or retrieve apps",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*App).runGet(opts)
				},
			},
		},
		SummaryColumns: []string{"NAMESPACE", "NAME", "STATUS"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return newApp(dataDir, filesystem)
		},
	})
}
