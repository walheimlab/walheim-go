package fs_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/walheimlab/walheim-go/internal/fs"
)

func newLocalFS(t *testing.T) (*fs.LocalFS, string) {
	t.Helper()
	return fs.NewLocalFS(), t.TempDir()
}

// ── WriteFile / ReadFile ───────────────────────────────────────────────────────

func TestLocalFS_WriteRead(t *testing.T) {
	lfs, dir := newLocalFS(t)
	path := filepath.Join(dir, "file.txt")
	data := []byte("hello walheim")

	if err := lfs.WriteFile(path, data); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := lfs.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Errorf("ReadFile() = %q, want %q", got, data)
	}
}

func TestLocalFS_WriteFile_createsParentDirs(t *testing.T) {
	lfs, dir := newLocalFS(t)
	path := filepath.Join(dir, "a", "b", "c", "file.txt")

	if err := lfs.WriteFile(path, []byte("data")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	exists, err := lfs.Exists(path)
	if err != nil || !exists {
		t.Errorf("file not found after write: exists=%v err=%v", exists, err)
	}
}

func TestLocalFS_WriteFile_atomic_overwrites(t *testing.T) {
	lfs, dir := newLocalFS(t)
	path := filepath.Join(dir, "file.txt")

	_ = lfs.WriteFile(path, []byte("first"))
	_ = lfs.WriteFile(path, []byte("second"))

	got, _ := lfs.ReadFile(path)
	if string(got) != "second" {
		t.Errorf("after overwrite: got %q, want %q", got, "second")
	}
}

func TestLocalFS_ReadFile_notFound(t *testing.T) {
	lfs, dir := newLocalFS(t)

	_, err := lfs.ReadFile(filepath.Join(dir, "missing.txt"))
	if err == nil {
		t.Error("expected error reading nonexistent file")
	}
}

// ── Exists / IsDir ─────────────────────────────────────────────────────────────

func TestLocalFS_Exists_file(t *testing.T) {
	lfs, dir := newLocalFS(t)
	path := filepath.Join(dir, "f.txt")
	_ = lfs.WriteFile(path, []byte("x"))

	ok, err := lfs.Exists(path)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !ok {
		t.Error("Exists() = false for existing file")
	}
}

func TestLocalFS_Exists_missing(t *testing.T) {
	lfs, dir := newLocalFS(t)

	ok, err := lfs.Exists(filepath.Join(dir, "nope"))
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if ok {
		t.Error("Exists() = true for nonexistent path")
	}
}

func TestLocalFS_IsDir(t *testing.T) {
	lfs, dir := newLocalFS(t)
	sub := filepath.Join(dir, "sub")
	_ = lfs.MkdirAll(sub)

	ok, err := lfs.IsDir(sub)
	if err != nil || !ok {
		t.Errorf("IsDir(dir) = %v, %v, want true, nil", ok, err)
	}

	file := filepath.Join(dir, "f.txt")
	_ = lfs.WriteFile(file, []byte("x"))

	ok, err = lfs.IsDir(file)
	if err != nil || ok {
		t.Errorf("IsDir(file) = %v, %v, want false, nil", ok, err)
	}
}

// ── MkdirAll / RemoveAll ───────────────────────────────────────────────────────

func TestLocalFS_MkdirAll(t *testing.T) {
	lfs, dir := newLocalFS(t)

	path := filepath.Join(dir, "a", "b", "c")
	if err := lfs.MkdirAll(path); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ok, _ := lfs.IsDir(path)
	if !ok {
		t.Error("directory not created by MkdirAll")
	}
}

func TestLocalFS_RemoveAll(t *testing.T) {
	lfs, dir := newLocalFS(t)
	sub := filepath.Join(dir, "sub")
	_ = lfs.MkdirAll(sub)
	_ = lfs.WriteFile(filepath.Join(sub, "file.txt"), []byte("x"))

	if err := lfs.RemoveAll(sub); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	ok, _ := lfs.Exists(sub)
	if ok {
		t.Error("directory still exists after RemoveAll")
	}
}

// ── ReadDir ────────────────────────────────────────────────────────────────────

func TestLocalFS_ReadDir_sorted(t *testing.T) {
	lfs, dir := newLocalFS(t)
	for _, name := range []string{"z", "a", "m"} {
		_ = lfs.WriteFile(filepath.Join(dir, name), []byte("x"))
	}

	entries, err := lfs.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(entries), entries)
	}

	if entries[0] != "a" || entries[1] != "m" || entries[2] != "z" {
		t.Errorf("entries not sorted: %v", entries)
	}
}

func TestLocalFS_ReadDir_hiddenFilesExcluded(t *testing.T) {
	lfs, dir := newLocalFS(t)
	_ = lfs.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"))
	_ = lfs.WriteFile(filepath.Join(dir, ".app.yaml"), []byte("x"))
	_ = lfs.WriteFile(filepath.Join(dir, "visible"), []byte("x"))

	entries, err := lfs.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	for _, e := range entries {
		if e[0] == '.' {
			t.Errorf("hidden entry %q should be excluded from ReadDir", e)
		}
	}

	if len(entries) != 1 || entries[0] != "visible" {
		t.Errorf("entries = %v, want [visible]", entries)
	}
}

func TestLocalFS_ReadDir_empty(t *testing.T) {
	lfs, dir := newLocalFS(t)

	entries, err := lfs.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir on empty dir: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected empty, got %v", entries)
	}
}
