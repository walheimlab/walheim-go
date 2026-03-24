package resource_test

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/testutil"
)

// ── WriteManifest ─────────────────────────────────────────────────────────────

func TestNamespacedBase_WriteManifest(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)

	type simple struct {
		Name string `yaml:"name"`
	}

	if err := b.WriteManifest("prod", "myapp", simple{Name: "myapp"}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	data, err := mem.ReadFile("/data/namespaces/prod/apps/myapp/.app.yaml")
	if err != nil {
		t.Fatalf("ReadFile after WriteManifest: %v", err)
	}

	if len(data) == 0 {
		t.Error("written file is empty")
	}
}

// ── ValidNamespaces edge cases ─────────────────────────────────────────────────

func TestNamespacedBase_ValidNamespaces_noDir(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)

	// No namespace dir → should return nil without error.
	ns, err := b.ValidNamespaces()
	if err != nil {
		t.Fatalf("ValidNamespaces: %v", err)
	}

	if ns != nil {
		t.Errorf("expected nil for missing namespaces dir, got %v", ns)
	}
}
