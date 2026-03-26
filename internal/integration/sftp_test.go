//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/rsync"
	"github.com/walheimlab/walheim-go/internal/testutil"
	internalssh "github.com/walheimlab/walheim-go/internal/ssh"
)

func newSyncer(tgt sshTarget) *rsync.Syncer {
	return &rsync.Syncer{
		Port:         tgt.Port,
		IdentityFile: tgt.IdentityFile,
	}
}

// remoteContains runs "cat <path>" on the target and returns the output.
func remoteContains(t *testing.T, tgt sshTarget, path, want string) {
	t.Helper()

	c := &internalssh.Client{
		RemoteHost:     tgt.Remote(),
		ConnectTimeout: 10,
		Port:           tgt.Port,
		IdentityFile:   tgt.IdentityFile,
	}

	out, err := c.RunOutput("cat " + path)
	if err != nil {
		t.Fatalf("cat %s: %v", path, err)
	}

	if !strings.Contains(out, want) {
		t.Errorf("remote file %s: got %q, want it to contain %q", path, out, want)
	}
}

// ── basic sync ────────────────────────────────────────────────────────────────

func TestSFTP_Sync(t *testing.T) {
	fs := testutil.NewMemFS()

	if err := fs.WriteFile("/app/compose.yml", []byte("version: '3'\n")); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := fs.WriteFile("/app/config/env", []byte("KEY=value\n")); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := newSyncer(target1).Sync(fs, "/app", target1.Remote(), "/tmp/walheim-sftp-basic"); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	remoteContains(t, target1, "/tmp/walheim-sftp-basic/compose.yml", "version: '3'")
}

// ── nested directory structure ────────────────────────────────────────────────

func TestSFTP_Sync_nested(t *testing.T) {
	fs := testutil.NewMemFS()

	files := map[string]string{
		"/src/a/b/c/deep.txt": "deep content",
		"/src/a/b/mid.txt":    "mid content",
		"/src/a/top.txt":      "top content",
		"/src/root.txt":       "root content",
	}

	for path, content := range files {
		if err := fs.WriteFile(path, []byte(content)); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if err := newSyncer(target1).Sync(fs, "/src", target1.Remote(), "/tmp/walheim-sftp-nested"); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	remoteContains(t, target1, "/tmp/walheim-sftp-nested/root.txt", "root content")
	remoteContains(t, target1, "/tmp/walheim-sftp-nested/a/top.txt", "top content")
	remoteContains(t, target1, "/tmp/walheim-sftp-nested/a/b/mid.txt", "mid content")
	remoteContains(t, target1, "/tmp/walheim-sftp-nested/a/b/c/deep.txt", "deep content")
}

// ── overwrite existing files ──────────────────────────────────────────────────

func TestSFTP_Sync_overwrite(t *testing.T) {
	fs := testutil.NewMemFS()
	dest := "/tmp/walheim-sftp-overwrite"

	if err := fs.WriteFile("/data/file.txt", []byte("original\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := newSyncer(target1).Sync(fs, "/data", target1.Remote(), dest); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	remoteContains(t, target1, dest+"/file.txt", "original")

	if err := fs.WriteFile("/data/file.txt", []byte("updated\n")); err != nil {
		t.Fatalf("write updated: %v", err)
	}

	if err := newSyncer(target1).Sync(fs, "/data", target1.Remote(), dest); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	remoteContains(t, target1, dest+"/file.txt", "updated")
}

// ── empty source ──────────────────────────────────────────────────────────────

func TestSFTP_Sync_emptySource(t *testing.T) {
	fs := testutil.NewMemFS()

	if err := fs.MkdirAll("/empty"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := newSyncer(target1).Sync(fs, "/empty", target1.Remote(), "/tmp/walheim-sftp-empty"); err != nil {
		t.Fatalf("Sync empty source: %v", err)
	}
}
