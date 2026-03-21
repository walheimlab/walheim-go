// Package namespaces registers the Namespace cluster-scoped resource.
// Full implementation is in plan-03. This stub ensures the package exists
// and the blank-import in main.go compiles.
package namespaces

import (
	"github.com/walheimlab/walheim-go/internal/fs"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/resource"
)

var kindInfo = resource.KindInfo{
	Plural:   "namespaces",
	Singular: "namespace",
	Aliases:  []string{"ns"},
}

func init() {
	registry.Register(registry.Registration{
		Info:  kindInfo,
		Scope: registry.ClusterScoped,
		Operations: []registry.OperationDef{
			{
				Verb:  "get",
				Short: "List or retrieve namespaces",
				Run: func(h resource.Handler, opts registry.OperationOpts) error {
					return h.(*Namespaces).runGet(opts)
				},
			},
		},
		SummaryColumns: []string{"NAME", "HOSTNAME", "USERNAME"},
		Factory: func(dataDir string, filesystem fs.FS) resource.Handler {
			return NewNamespaces(dataDir, filesystem)
		},
	})
}

// Namespaces is the handler for the Namespace resource kind.
type Namespaces struct {
	base resource.ClusterBase
}

// NewNamespaces creates a new Namespaces handler.
func NewNamespaces(dataDir string, filesystem fs.FS) *Namespaces {
	return &Namespaces{
		base: resource.ClusterBase{
			DataDir:          dataDir,
			FS:               filesystem,
			Info:             kindInfo,
			ManifestFilename: ".namespace.yaml",
			Fields: map[string]resource.SummaryField{
				"HOSTNAME": func(m resource.Manifest) string {
					spec, _ := m["spec"].(map[string]any)
					if spec == nil {
						return ""
					}
					v, _ := spec["hostname"].(string)
					return v
				},
				"USERNAME": func(m resource.Manifest) string {
					spec, _ := m["spec"].(map[string]any)
					if spec == nil {
						return ""
					}
					v, _ := spec["username"].(string)
					return v
				},
			},
		},
	}
}

// KindInfo implements resource.Handler.
func (n *Namespaces) KindInfo() resource.KindInfo {
	return kindInfo
}

func (n *Namespaces) runGet(opts registry.OperationOpts) error {
	// Stub: plan-03 will implement this
	return nil
}
