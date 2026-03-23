package ssh

import (
	"testing"
)

// Run, RunOutput, Exec, TestConnection are not tested here:
// they require a live SSH server. Integration tests would be needed.

func TestNewClient_defaults(t *testing.T) {
	c := NewClient("myhost.local")
	if c.RemoteHost != "myhost.local" {
		t.Errorf("RemoteHost = %q, want %q", c.RemoteHost, "myhost.local")
	}

	if c.ConnectTimeout != 5 {
		t.Errorf("ConnectTimeout = %d, want 5", c.ConnectTimeout)
	}

	if !c.BatchMode {
		t.Error("BatchMode should default to true")
	}
}

func TestBuildArgs_basic(t *testing.T) {
	c := &Client{
		RemoteHost:     "user@host",
		ConnectTimeout: 5,
		BatchMode:      true,
	}
	args := c.buildArgs("docker ps", false)

	// Must contain timeout option
	mustContainPair(t, args, "-o", "ConnectTimeout=5")
	// Must contain batch mode
	mustContainPair(t, args, "-o", "BatchMode=yes")
	// Must contain strict host key option
	mustContainPair(t, args, "-o", "StrictHostKeyChecking=accept-new")
	// Must end with host then command
	if args[len(args)-2] != "user@host" {
		t.Errorf("second-to-last arg = %q, want host", args[len(args)-2])
	}

	if args[len(args)-1] != "docker ps" {
		t.Errorf("last arg = %q, want command", args[len(args)-1])
	}
}

func TestBuildArgs_noBatchMode(t *testing.T) {
	c := &Client{
		RemoteHost:     "host",
		ConnectTimeout: 10,
		BatchMode:      false,
	}

	args := c.buildArgs("true", false)
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" && args[i+1] == "BatchMode=yes" {
			t.Error("BatchMode=yes should not be present when BatchMode=false")
		}
	}
}

func TestBuildArgs_tty(t *testing.T) {
	c := &Client{RemoteHost: "host", ConnectTimeout: 5, BatchMode: true}
	with := c.buildArgs("bash", true)
	without := c.buildArgs("bash", false)

	hasTTY := func(args []string) bool {
		for _, a := range args {
			if a == "-t" {
				return true
			}
		}

		return false
	}

	if !hasTTY(with) {
		t.Error("expected -t flag when tty=true")
	}

	if hasTTY(without) {
		t.Error("unexpected -t flag when tty=false")
	}
}

func TestBuildArgs_customTimeout(t *testing.T) {
	c := &Client{RemoteHost: "host", ConnectTimeout: 30, BatchMode: true}
	args := c.buildArgs("true", false)
	mustContainPair(t, args, "-o", "ConnectTimeout=30")
}

// mustContainPair asserts that args contains consecutive elements [key, val].
func mustContainPair(t *testing.T, args []string, key, val string) {
	t.Helper()

	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == val {
			return
		}
	}

	t.Errorf("args %v does not contain pair [%q %q]", args, key, val)
}
