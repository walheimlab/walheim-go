package resource_test

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

func testBase(mem *testutil.MemFS) resource.NamespacedBase {
	return resource.NamespacedBase{
		DataDir:          "/data",
		FS:               mem,
		Info:             resource.KindInfo{Plural: "apps"},
		ManifestFilename: ".app.yaml",
	}
}

// ── Exists ────────────────────────────────────────────────────────────────────

func TestNamespacedBase_Exists_false(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)

	ok, err := b.Exists("prod", "myapp")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if ok {
		t.Error("Exists() = true for nonexistent resource")
	}
}

func TestNamespacedBase_Exists_true(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)
	_ = mem.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", []byte("content"))

	ok, err := b.Exists("prod", "myapp")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !ok {
		t.Error("Exists() = false for existing resource")
	}
}

// ── ReadBytes ─────────────────────────────────────────────────────────────────

func TestNamespacedBase_ReadBytes(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)
	_ = mem.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", []byte("hello"))

	data, err := b.ReadBytes("prod", "myapp")
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}

	if string(data) != "hello" {
		t.Errorf("ReadBytes() = %q, want %q", data, "hello")
	}
}

func TestNamespacedBase_ReadBytes_notFound(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)

	_, err := b.ReadBytes("prod", "missing")
	if err == nil {
		t.Error("expected error reading nonexistent resource")
	}
}

// ── EnsureDir / RemoveDir ─────────────────────────────────────────────────────

func TestNamespacedBase_EnsureDir(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)

	if err := b.EnsureDir("prod", "myapp"); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	ok, _ := mem.IsDir("/data/namespaces/prod/apps/myapp")
	if !ok {
		t.Error("directory not created by EnsureDir")
	}
}

func TestNamespacedBase_RemoveDir(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)
	_ = mem.MkdirAll("/data/namespaces/prod/apps/myapp")
	_ = mem.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", []byte("x"))

	if err := b.RemoveDir("prod", "myapp"); err != nil {
		t.Fatalf("RemoveDir: %v", err)
	}

	ok, _ := mem.Exists("/data/namespaces/prod/apps/myapp")
	if ok {
		t.Error("directory still exists after RemoveDir")
	}
}

// ── ListNames ─────────────────────────────────────────────────────────────────

func TestNamespacedBase_ListNames_empty(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)

	// No namespace dir at all → should return nil, not error.
	names, err := b.ListNames("prod")
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}

	if len(names) != 0 {
		t.Errorf("expected 0 names, got %v", names)
	}
}

func TestNamespacedBase_ListNames_returnsOnlyWithManifest(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)

	// "memos" has a manifest; "broken" does not.
	_ = mem.WriteFile("/data/namespaces/prod/apps/memos/.app.yaml", []byte("x"))
	_ = mem.MkdirAll("/data/namespaces/prod/apps/broken")

	names, err := b.ListNames("prod")
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}

	if len(names) != 1 || names[0] != "memos" {
		t.Errorf("ListNames() = %v, want [memos]", names)
	}
}

// ── ValidNamespaces ───────────────────────────────────────────────────────────

func TestNamespacedBase_ValidNamespaces(t *testing.T) {
	mem := testutil.NewMemFS()
	b := testBase(mem)

	_ = mem.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("x"))
	_ = mem.WriteFile("/data/namespaces/staging/.namespace.yaml", []byte("x"))
	_ = mem.MkdirAll("/data/namespaces/ghost") // no .namespace.yaml

	ns, err := b.ValidNamespaces()
	if err != nil {
		t.Fatalf("ValidNamespaces: %v", err)
	}

	if len(ns) != 2 {
		t.Fatalf("expected 2 namespaces, got %v", ns)
	}

	set := map[string]bool{ns[0]: true, ns[1]: true}
	if !set["prod"] || !set["staging"] {
		t.Errorf("ValidNamespaces() = %v, want [prod staging]", ns)
	}
}
