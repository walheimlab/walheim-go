//go:build integration

package integration_test

import (
	"strings"
	"testing"

	internalssh "github.com/walheimlab/walheim-go/internal/ssh"
)

func newSSHClient(t *testing.T, tgt sshTarget) *internalssh.Client {
	t.Helper()

	return &internalssh.Client{
		RemoteHost:     tgt.Remote(),
		ConnectTimeout: 10,
		Port:           tgt.Port,
		IdentityFile:   tgt.IdentityFile,
	}
}

// ── TestConnection ────────────────────────────────────────────────────────────

func TestSSH_TestConnection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target sshTarget
	}{
		{"target1", target1},
		{"target2", target2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newSSHClient(t, tc.target)

			if !c.TestConnection() {
				t.Fatalf("TestConnection failed for %s", tc.target.Remote())
			}
		})
	}
}

func TestSSH_TestConnection_wrongKey_fails(t *testing.T) {
	// Clone target1 but with a non-existent identity file — should not connect.
	c := &internalssh.Client{
		RemoteHost:     target1.Remote(),
		ConnectTimeout: 3,
		Port:           target1.Port,
		IdentityFile:   "/nonexistent/key",
	}

	if c.TestConnection() {
		t.Fatal("expected TestConnection to fail with wrong key, but it succeeded")
	}
}

// ── RunOutput ─────────────────────────────────────────────────────────────────

func TestSSH_RunOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target sshTarget
	}{
		{"target1", target1},
		{"target2", target2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newSSHClient(t, tc.target)

			out, err := c.RunOutput("echo hello-walheim")
			if err != nil {
				t.Fatalf("RunOutput: %v", err)
			}

			if !strings.Contains(out, "hello-walheim") {
				t.Fatalf("unexpected output: %q", out)
			}
		})
	}
}

func TestSSH_RunOutput_multiLine(t *testing.T) {
	c := newSSHClient(t, target1)

	out, err := c.RunOutput("printf 'line1\nline2\nline3\n'")
	if err != nil {
		t.Fatalf("RunOutput: %v", err)
	}

	for _, want := range []string{"line1", "line2", "line3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func TestSSH_RunOutput_nonZeroExit_returnsError(t *testing.T) {
	c := newSSHClient(t, target1)

	_, err := c.RunOutput("exit 42")
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
}

// ── Run ───────────────────────────────────────────────────────────────────────

func TestSSH_Run(t *testing.T) {
	c := newSSHClient(t, target1)

	if err := c.Run("true"); err != nil {
		t.Fatalf("Run(true): %v", err)
	}
}

func TestSSH_Run_nonZeroExit_returnsError(t *testing.T) {
	c := newSSHClient(t, target1)

	if err := c.Run("false"); err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
}

func TestSSH_Run_bothTargetsIndependent(t *testing.T) {
	// Verifies both containers are fully independent processes.
	c1 := newSSHClient(t, target1)
	c2 := newSSHClient(t, target2)

	out1, err := c1.RunOutput("hostname")
	if err != nil {
		t.Fatalf("target1 hostname: %v", err)
	}

	out2, err := c2.RunOutput("hostname")
	if err != nil {
		t.Fatalf("target2 hostname: %v", err)
	}

	if strings.TrimSpace(out1) == strings.TrimSpace(out2) {
		t.Errorf("both targets returned same hostname %q; expected independent containers", out1)
	}
}
