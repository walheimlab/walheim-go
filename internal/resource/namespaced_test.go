package resource_test

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/resource"
)

// ── Pure path helpers ──────────────────────────────────────────────────────────

func TestNamespacedBase_ResourceDir(t *testing.T) {
	b := resource.NamespacedBase{
		DataDir: "/data/walheim",
		Info:    resource.KindInfo{Plural: "apps"},
	}
	got := b.ResourceDir("myhome", "memos")

	want := "/data/walheim/namespaces/myhome/apps/memos"
	if got != want {
		t.Errorf("ResourceDir() = %q, want %q", got, want)
	}
}

func TestNamespacedBase_ManifestPath(t *testing.T) {
	b := resource.NamespacedBase{
		DataDir:          "/data/walheim",
		Info:             resource.KindInfo{Plural: "apps"},
		ManifestFilename: ".app.yaml",
	}
	got := b.ManifestPath("myhome", "memos")

	want := "/data/walheim/namespaces/myhome/apps/memos/.app.yaml"
	if got != want {
		t.Errorf("ManifestPath() = %q, want %q", got, want)
	}
}

func TestNamespacedBase_ResourceDir_secrets(t *testing.T) {
	b := resource.NamespacedBase{
		DataDir:          "/srv/data",
		Info:             resource.KindInfo{Plural: "secrets"},
		ManifestFilename: ".secret.yaml",
	}
	got := b.ResourceDir("prod", "db-creds")

	want := "/srv/data/namespaces/prod/secrets/db-creds"
	if got != want {
		t.Errorf("ResourceDir() = %q, want %q", got, want)
	}
}
