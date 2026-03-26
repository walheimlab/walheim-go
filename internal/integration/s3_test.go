//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/fs"
)

// newS3FS creates an S3FS pointed at the MinIO test container.
// prefix scopes the instance to an isolated sub-tree within the shared bucket.
func newS3FS(t *testing.T, prefix string) *fs.S3FS {
	t.Helper()

	sfs, err := fs.NewS3FS(fs.S3FSConfig{
		Endpoint:        minioEndpoint,
		Region:          "us-east-1",
		Bucket:          minioBucket,
		Prefix:          prefix,
		AccessKeyID:     minioUser,
		SecretAccessKey: minioPassword,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	return sfs
}

// testRoot returns a unique prefix for the current test and registers a
// cleanup that removes all objects written under that prefix.
func testRoot(t *testing.T) string {
	t.Helper()

	root := "inttest/" + strings.ReplaceAll(t.Name(), "/", "_")

	t.Cleanup(func() {
		// Best-effort cleanup; ignore errors so test failures stay readable.
		_ = newS3FS(t, "").RemoveAll(root)
	})

	return root
}

// ── Ping ──────────────────────────────────────────────────────────────────────

func TestS3FS_Ping(t *testing.T) {
	sfs := newS3FS(t, "")

	if err := sfs.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestS3FS_Ping_wrongBucket(t *testing.T) {
	sfs, err := fs.NewS3FS(fs.S3FSConfig{
		Endpoint:        minioEndpoint,
		Region:          "us-east-1",
		Bucket:          "does-not-exist",
		AccessKeyID:     minioUser,
		SecretAccessKey: minioPassword,
	})
	if err != nil {
		t.Fatalf("NewS3FS: %v", err)
	}

	if pingErr := sfs.Ping(); pingErr == nil {
		t.Fatal("expected error pinging non-existent bucket, got nil")
	}
}

// ── WriteFile / ReadFile ──────────────────────────────────────────────────────

func TestS3FS_WriteRead(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	path := root + "/hello.txt"
	content := []byte("hello walheim\n")

	if err := sfs.WriteFile(path, content); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := sfs.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("ReadFile = %q, want %q", got, content)
	}
}

func TestS3FS_ReadFile_missing(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	_, err := sfs.ReadFile(root + "/nonexistent.txt")
	if err == nil {
		t.Fatal("expected error reading missing key, got nil")
	}
}

func TestS3FS_WriteRead_withPrefix(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, root+"/prefix")

	if err := sfs.WriteFile("data.txt", []byte("prefixed")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := sfs.ReadFile("data.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != "prefixed" {
		t.Errorf("ReadFile = %q, want %q", got, "prefixed")
	}
}

// ── MkdirAll ──────────────────────────────────────────────────────────────────

func TestS3FS_MkdirAll_isNoOp(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	// S3 hierarchies are implicit; MkdirAll must always succeed.
	if err := sfs.MkdirAll(root + "/a/b/c"); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
}

// ── Exists ────────────────────────────────────────────────────────────────────

func TestS3FS_Exists_file(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	path := root + "/exists.txt"

	if err := sfs.WriteFile(path, []byte("yes")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ok, err := sfs.Exists(path)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !ok {
		t.Error("Exists = false, want true")
	}
}

func TestS3FS_Exists_prefix(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	if err := sfs.WriteFile(root+"/dir/child.txt", []byte("data")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ok, err := sfs.Exists(root + "/dir")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !ok {
		t.Error("Exists for prefix = false, want true")
	}
}

func TestS3FS_Exists_missing(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	ok, err := sfs.Exists(root + "/ghost.txt")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if ok {
		t.Error("Exists = true for missing key, want false")
	}
}

// ── IsDir ─────────────────────────────────────────────────────────────────────

func TestS3FS_IsDir_prefix(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	if err := sfs.WriteFile(root+"/subdir/file.txt", []byte("data")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ok, err := sfs.IsDir(root + "/subdir")
	if err != nil {
		t.Fatalf("IsDir: %v", err)
	}

	if !ok {
		t.Error("IsDir for prefix with children = false, want true")
	}
}

func TestS3FS_IsDir_file(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	path := root + "/plain.txt"

	if err := sfs.WriteFile(path, []byte("not a dir")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ok, err := sfs.IsDir(path)
	if err != nil {
		t.Fatalf("IsDir: %v", err)
	}

	if ok {
		t.Error("IsDir for plain file = true, want false")
	}
}

func TestS3FS_IsDir_missing(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	ok, err := sfs.IsDir(root + "/absent")
	if err != nil {
		t.Fatalf("IsDir: %v", err)
	}

	if ok {
		t.Error("IsDir for missing path = true, want false")
	}
}

// ── ReadDir ───────────────────────────────────────────────────────────────────

func TestS3FS_ReadDir(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	// Write a mix of files and a sub-prefix.
	writes := []string{
		root + "/dir/alpha.txt",
		root + "/dir/beta.txt",
		root + "/dir/gamma.txt",
		root + "/dir/sub/nested.txt", // should appear as "sub", not "sub/nested.txt"
	}

	for _, p := range writes {
		if err := sfs.WriteFile(p, []byte("data")); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
	}

	entries, err := sfs.ReadDir(root + "/dir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	want := map[string]bool{"alpha.txt": true, "beta.txt": true, "gamma.txt": true, "sub": true}

	if len(entries) != len(want) {
		t.Fatalf("ReadDir returned %d entries, want %d: %v", len(entries), len(want), entries)
	}

	for _, e := range entries {
		if !want[e] {
			t.Errorf("unexpected entry %q in ReadDir result", e)
		}
	}
}

func TestS3FS_ReadDir_hiddenEntriesSkipped(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	if err := sfs.WriteFile(root+"/d/visible.txt", []byte("yes")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := sfs.WriteFile(root+"/d/.hidden", []byte("no")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entries, err := sfs.ReadDir(root + "/d")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	for _, e := range entries {
		if strings.HasPrefix(e, ".") {
			t.Errorf("ReadDir returned hidden entry %q", e)
		}
	}

	if len(entries) != 1 || entries[0] != "visible.txt" {
		t.Errorf("ReadDir = %v, want [visible.txt]", entries)
	}
}

func TestS3FS_ReadDir_sorted(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	for _, name := range []string{"z.txt", "a.txt", "m.txt"} {
		if err := sfs.WriteFile(root+"/sort/"+name, []byte("x")); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	entries, err := sfs.ReadDir(root + "/sort")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	want := []string{"a.txt", "m.txt", "z.txt"}

	if len(entries) != len(want) {
		t.Fatalf("ReadDir = %v, want %v", entries, want)
	}

	for i, e := range entries {
		if e != want[i] {
			t.Errorf("entries[%d] = %q, want %q", i, e, want[i])
		}
	}
}

// ── RemoveAll ─────────────────────────────────────────────────────────────────

func TestS3FS_RemoveAll_prefix(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	for _, p := range []string{
		root + "/rm/a.txt",
		root + "/rm/b.txt",
		root + "/rm/sub/c.txt",
	} {
		if err := sfs.WriteFile(p, []byte("data")); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
	}

	if err := sfs.RemoveAll(root + "/rm"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	ok, err := sfs.Exists(root + "/rm")
	if err != nil {
		t.Fatalf("Exists after RemoveAll: %v", err)
	}

	if ok {
		t.Error("Exists = true after RemoveAll, want false")
	}
}

func TestS3FS_RemoveAll_singleFile(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	path := root + "/single.txt"

	if err := sfs.WriteFile(path, []byte("gone")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := sfs.RemoveAll(path); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	ok, err := sfs.Exists(path)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if ok {
		t.Error("Exists = true after RemoveAll of single file, want false")
	}
}

func TestS3FS_RemoveAll_nonExistent_noError(t *testing.T) {
	root := testRoot(t)
	sfs := newS3FS(t, "")

	// RemoveAll on a path that doesn't exist should not error.
	if err := sfs.RemoveAll(root + "/nothing/here"); err != nil {
		t.Fatalf("RemoveAll on non-existent path: %v", err)
	}
}
