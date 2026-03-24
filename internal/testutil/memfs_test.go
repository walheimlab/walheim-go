package testutil_test

import (
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/testutil"
)

// ── ReadFile / WriteFile ───────────────────────────────────────────────────────

func TestMemFS_ReadFile_notFound(t *testing.T) {
	m := testutil.NewMemFS()

	_, err := m.ReadFile("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMemFS_WriteFile_then_ReadFile(t *testing.T) {
	m := testutil.NewMemFS()

	if err := m.WriteFile("/a/b/file.txt", []byte("hello")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := m.ReadFile("/a/b/file.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(data) != "hello" {
		t.Errorf("ReadFile = %q, want hello", data)
	}
}

func TestMemFS_WriteFile_returnsCopy(t *testing.T) {
	m := testutil.NewMemFS()

	original := []byte("hello")
	_ = m.WriteFile("/f", original)

	// Mutate original; stored data should be unaffected.
	original[0] = 'X'

	data, _ := m.ReadFile("/f")
	if data[0] == 'X' {
		t.Error("WriteFile stored a live reference, not a copy")
	}
}

func TestMemFS_ReadFile_returnsCopy(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.WriteFile("/f", []byte("hello"))

	data1, _ := m.ReadFile("/f")
	data1[0] = 'X'

	data2, _ := m.ReadFile("/f")
	if data2[0] == 'X' {
		t.Error("ReadFile returned a live reference, not a copy")
	}
}

// ── MkdirAll ──────────────────────────────────────────────────────────────────

func TestMemFS_MkdirAll(t *testing.T) {
	m := testutil.NewMemFS()

	if err := m.MkdirAll("/a/b/c"); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ok, err := m.IsDir("/a/b/c")
	if err != nil {
		t.Fatalf("IsDir: %v", err)
	}

	if !ok {
		t.Error("MkdirAll did not create directory")
	}
}

func TestMemFS_MkdirAll_createsParents(t *testing.T) {
	m := testutil.NewMemFS()

	_ = m.MkdirAll("/a/b/c")

	ok, _ := m.IsDir("/a/b")
	if !ok {
		t.Error("MkdirAll did not create parent /a/b")
	}

	ok, _ = m.IsDir("/a")
	if !ok {
		t.Error("MkdirAll did not create parent /a")
	}
}

// ── RemoveAll ─────────────────────────────────────────────────────────────────

func TestMemFS_RemoveAll_removesFiles(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.WriteFile("/mydir/file.txt", []byte("x"))
	_ = m.WriteFile("/mydir/sub/other.txt", []byte("y"))

	if err := m.RemoveAll("/mydir"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	ok, _ := m.Exists("/mydir/file.txt")
	if ok {
		t.Error("file still exists after RemoveAll")
	}

	ok, _ = m.Exists("/mydir/sub/other.txt")
	if ok {
		t.Error("nested file still exists after RemoveAll")
	}
}

func TestMemFS_RemoveAll_removesDirs(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.MkdirAll("/mydir/sub")

	_ = m.RemoveAll("/mydir")

	ok, _ := m.IsDir("/mydir/sub")
	if ok {
		t.Error("subdir still present after RemoveAll")
	}
}

func TestMemFS_RemoveAll_nonexistent_isNoop(t *testing.T) {
	m := testutil.NewMemFS()

	if err := m.RemoveAll("/nonexistent"); err != nil {
		t.Errorf("RemoveAll nonexistent: %v", err)
	}
}

// ── Exists ────────────────────────────────────────────────────────────────────

func TestMemFS_Exists_file(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.WriteFile("/f", []byte("x"))

	ok, err := m.Exists("/f")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !ok {
		t.Error("Exists = false for existing file")
	}
}

func TestMemFS_Exists_dir(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.MkdirAll("/d")

	ok, _ := m.Exists("/d")
	if !ok {
		t.Error("Exists = false for existing dir")
	}
}

func TestMemFS_Exists_false(t *testing.T) {
	m := testutil.NewMemFS()

	ok, _ := m.Exists("/nothing")
	if ok {
		t.Error("Exists = true for nonexistent path")
	}
}

// ── IsDir ─────────────────────────────────────────────────────────────────────

func TestMemFS_IsDir_true(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.MkdirAll("/d")

	ok, err := m.IsDir("/d")
	if err != nil {
		t.Fatalf("IsDir: %v", err)
	}

	if !ok {
		t.Error("IsDir = false for directory")
	}
}

func TestMemFS_IsDir_false_for_file(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.WriteFile("/f", []byte("x"))

	ok, _ := m.IsDir("/f")
	if ok {
		t.Error("IsDir = true for a file")
	}
}

func TestMemFS_IsDir_false_for_missing(t *testing.T) {
	m := testutil.NewMemFS()

	ok, _ := m.IsDir("/nothing")
	if ok {
		t.Error("IsDir = true for nonexistent path")
	}
}

// ── ReadDir ───────────────────────────────────────────────────────────────────

func TestMemFS_ReadDir_returnsChildren(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.WriteFile("/root/alpha/file.txt", []byte("a"))
	_ = m.WriteFile("/root/beta/file.txt", []byte("b"))

	entries, err := m.ReadDir("/root")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %v", entries)
	}

	entrySet := map[string]bool{entries[0]: true, entries[1]: true}
	if !entrySet["alpha"] || !entrySet["beta"] {
		t.Errorf("ReadDir() = %v, want [alpha beta]", entries)
	}
}

func TestMemFS_ReadDir_isSorted(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.WriteFile("/root/z/f", []byte(""))
	_ = m.WriteFile("/root/a/f", []byte(""))
	_ = m.WriteFile("/root/m/f", []byte(""))

	entries, err := m.ReadDir("/root")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	for i := 1; i < len(entries); i++ {
		if entries[i] < entries[i-1] {
			t.Errorf("ReadDir not sorted: %v", entries)
		}
	}
}

func TestMemFS_ReadDir_skipsHiddenFiles(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.WriteFile("/root/visible.txt", []byte("x"))
	_ = m.WriteFile("/root/.hidden", []byte("x"))

	entries, err := m.ReadDir("/root")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	for _, e := range entries {
		if strings.HasPrefix(e, ".") {
			t.Errorf("ReadDir returned hidden entry %q", e)
		}
	}
}

func TestMemFS_ReadDir_nonexistentDir_returnsError(t *testing.T) {
	m := testutil.NewMemFS()

	_, err := m.ReadDir("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestMemFS_ReadDir_emptyDir_returnsNil(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.MkdirAll("/empty")

	entries, err := m.ReadDir("/empty")
	if err != nil {
		t.Fatalf("ReadDir empty dir: %v", err)
	}

	if entries != nil {
		t.Errorf("expected nil for empty dir, got %v", entries)
	}
}

// ── WriteFile creates parent dirs ─────────────────────────────────────────────

func TestMemFS_WriteFile_createsParentDirs(t *testing.T) {
	m := testutil.NewMemFS()
	_ = m.WriteFile("/a/b/c/file.txt", []byte("x"))

	ok, _ := m.IsDir("/a/b/c")
	if !ok {
		t.Error("WriteFile did not create parent directory /a/b/c")
	}
}
