package ssh

import (
	"testing"
)

// Run, RunOutput, Exec, TestConnection require a live SSH server and are
// covered by the integration tests in internal/integration.

func TestNewClient_defaults(t *testing.T) {
	c := NewClient("myhost.local")
	if c.RemoteHost != "myhost.local" {
		t.Errorf("RemoteHost = %q, want %q", c.RemoteHost, "myhost.local")
	}

	if c.ConnectTimeout != 5 {
		t.Errorf("ConnectTimeout = %d, want 5", c.ConnectTimeout)
	}
}

func TestParseRemote_withUser(t *testing.T) {
	u, h := parseRemote("alice@example.com")
	if u != "alice" {
		t.Errorf("user = %q, want %q", u, "alice")
	}

	if h != "example.com" {
		t.Errorf("host = %q, want %q", h, "example.com")
	}
}

func TestParseRemote_noUser_fallsBackToOSUser(t *testing.T) {
	u, h := parseRemote("example.com")
	if u == "" {
		t.Error("expected non-empty user from OS fallback")
	}

	if h != "example.com" {
		t.Errorf("host = %q, want %q", h, "example.com")
	}
}

func TestParseRemote_atInHost(t *testing.T) {
	// Only the last '@' separates user from host.
	u, h := parseRemote("user@host@example.com")
	if u != "user@host" {
		t.Errorf("user = %q, want %q", u, "user@host")
	}

	if h != "example.com" {
		t.Errorf("host = %q, want %q", h, "example.com")
	}
}
