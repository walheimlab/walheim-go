package resource_test

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/resource"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

func clusterBase(mem *testutil.MemFS) resource.ClusterBase {
	return resource.ClusterBase{
		DataDir:          "/data",
		FS:               mem,
		Info:             resource.KindInfo{Plural: "namespaces", Kind: "Namespace"},
		ManifestFilename: ".namespace.yaml",
	}
}

// ── Path helpers ──────────────────────────────────────────────────────────────

func TestClusterBase_ResourceDir(t *testing.T) {
	b := clusterBase(testutil.NewMemFS())

	got := b.ResourceDir("myhome")
	want := "/data/namespaces/myhome"

	if got != want {
		t.Errorf("ResourceDir() = %q, want %q", got, want)
	}
}

func TestClusterBase_ManifestPath(t *testing.T) {
	b := clusterBase(testutil.NewMemFS())

	got := b.ManifestPath("myhome")
	want := "/data/namespaces/myhome/.namespace.yaml"

	if got != want {
		t.Errorf("ManifestPath() = %q, want %q", got, want)
	}
}

// ── Exists ────────────────────────────────────────────────────────────────────

func TestClusterBase_Exists_false(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)

	ok, err := b.Exists("myhome")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if ok {
		t.Error("Exists() = true for nonexistent resource")
	}
}

func TestClusterBase_Exists_true(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)
	_ = mem.WriteFile("/data/namespaces/myhome/.namespace.yaml", []byte("x"))

	ok, err := b.Exists("myhome")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !ok {
		t.Error("Exists() = false for existing resource")
	}
}

// ── ReadBytes ─────────────────────────────────────────────────────────────────

func TestClusterBase_ReadBytes(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)
	_ = mem.WriteFile("/data/namespaces/myhome/.namespace.yaml", []byte("hello"))

	data, err := b.ReadBytes("myhome")
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}

	if string(data) != "hello" {
		t.Errorf("ReadBytes() = %q, want hello", data)
	}
}

func TestClusterBase_ReadBytes_notFound(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)

	_, err := b.ReadBytes("missing")
	if err == nil {
		t.Error("expected error reading nonexistent resource")
	}
}

// ── WriteManifest ─────────────────────────────────────────────────────────────

func TestClusterBase_WriteManifest(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)

	type simple struct {
		Name string `yaml:"name"`
	}

	if err := b.WriteManifest("myhome", simple{Name: "myhome"}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	data, err := mem.ReadFile("/data/namespaces/myhome/.namespace.yaml")
	if err != nil {
		t.Fatalf("ReadFile after WriteManifest: %v", err)
	}

	if string(data) == "" {
		t.Error("written file is empty")
	}
}

// ── EnsureDir / RemoveDir ─────────────────────────────────────────────────────

func TestClusterBase_EnsureDir(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)

	if err := b.EnsureDir("myhome"); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	ok, _ := mem.IsDir("/data/namespaces/myhome")
	if !ok {
		t.Error("directory not created by EnsureDir")
	}
}

func TestClusterBase_RemoveDir(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)
	_ = mem.MkdirAll("/data/namespaces/myhome")
	_ = mem.WriteFile("/data/namespaces/myhome/.namespace.yaml", []byte("x"))

	if err := b.RemoveDir("myhome"); err != nil {
		t.Fatalf("RemoveDir: %v", err)
	}

	ok, _ := mem.Exists("/data/namespaces/myhome")
	if ok {
		t.Error("directory still exists after RemoveDir")
	}
}

// ── ListNames ─────────────────────────────────────────────────────────────────

func TestClusterBase_ListNames_empty(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)

	names, err := b.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}

	if len(names) != 0 {
		t.Errorf("expected 0 names, got %v", names)
	}
}

func TestClusterBase_ListNames_returnsOnlyWithManifest(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)

	// "myhome" has a manifest; "ghost" does not.
	_ = mem.WriteFile("/data/namespaces/myhome/.namespace.yaml", []byte("x"))
	_ = mem.MkdirAll("/data/namespaces/ghost")

	names, err := b.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}

	if len(names) != 1 || names[0] != "myhome" {
		t.Errorf("ListNames() = %v, want [myhome]", names)
	}
}

func TestClusterBase_ListNames_multiple(t *testing.T) {
	mem := testutil.NewMemFS()
	b := clusterBase(mem)

	_ = mem.WriteFile("/data/namespaces/alpha/.namespace.yaml", []byte("x"))
	_ = mem.WriteFile("/data/namespaces/beta/.namespace.yaml", []byte("x"))

	names, err := b.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %v", names)
	}

	nameSet := map[string]bool{names[0]: true, names[1]: true}
	if !nameSet["alpha"] || !nameSet["beta"] {
		t.Errorf("ListNames() = %v, want [alpha beta]", names)
	}
}
