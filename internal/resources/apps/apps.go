// Package apps registers the App namespace-scoped resource.
// Full implementation is in plan-04. This stub ensures the package exists
// and the blank-import in main.go compiles.
package apps

import (
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

var kindInfo = resource.KindInfo{
	Plural:   "apps",
	Singular: "app",
	Aliases:  []string{},
}

func init() {
	registry.Register(registry.Registration{
		Info:  kindInfo,
		Scope: registry.NamespaceScoped,
		Operations: []registry.OperationDef{
			{
				Verb:         "get",
				Short:        "List or retrieve apps",
				NSHandling:   registry.NSOptionalAll,
				RequiresName: false,
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Apps).runGet(opts)
				},
			},
		},
		SummaryColumns: []string{"NAMESPACE", "NAME", "STATUS"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return NewApps(dataDir, filesystem)
		},
	})
}

// Apps is the handler for the App resource kind.
type Apps struct {
	base resource.NamespacedBase
}

// NewApps creates a new Apps handler.
func NewApps(dataDir string, filesystem fs.FS) *Apps {
	return &Apps{
		base: resource.NamespacedBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             kindInfo,
			ManifestFilename: ".app.yaml",
			Fields: map[string]resource.SummaryField{
				"STATUS": func(m resource.Manifest) string {
					return "Unknown"
				},
			},
		},
	}
}

// KindInfo implements resource.Handler.
func (a *Apps) KindInfo() resource.KindInfo {
	return kindInfo
}

func (a *Apps) runGet(opts registry.OperationOpts) error {
	// Stub: plan-04 will implement this
	return nil
}
