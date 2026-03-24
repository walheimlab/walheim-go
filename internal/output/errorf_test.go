package output_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/output"
)

// captureStderr redirects os.Stderr during f(), returns captured output.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	orig := os.Stderr
	os.Stderr = w

	f()

	_ = w.Close()

	os.Stderr = orig

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}

	return buf.String()
}

// ── Errorf human mode ─────────────────────────────────────────────────────────

func TestErrorf_humanMode_writesToStderr(t *testing.T) {
	out := captureStderr(t, func() {
		output.Errorf(false, "not-found", "resource not found", "", nil, false)
	})

	if !strings.Contains(out, "resource not found") {
		t.Errorf("expected error message, got: %q", out)
	}
}

func TestErrorf_humanMode_withSuggestion(t *testing.T) {
	out := captureStderr(t, func() {
		output.Errorf(false, "not-found", "resource not found", "try --all-namespaces", nil, false)
	})

	if !strings.Contains(out, "try --all-namespaces") {
		t.Errorf("expected suggestion, got: %q", out)
	}
}

func TestErrorf_humanMode_noSuggestion_nohint(t *testing.T) {
	out := captureStderr(t, func() {
		output.Errorf(false, "not-found", "resource not found", "", nil, false)
	})

	if strings.Contains(out, "Hint:") {
		t.Errorf("should not show hint when suggestion is empty, got: %q", out)
	}
}

// ── Errorf JSON mode ──────────────────────────────────────────────────────────

func TestErrorf_jsonMode_writesToStdout(t *testing.T) {
	out := captureStdout(t, func() {
		output.Errorf(true, "not-found", "resource not found", "try again", map[string]string{"name": "foo"}, true)
	})

	var result output.ErrorPayload
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal JSON: %v\noutput: %q", err, out)
	}

	if result.Error != "not-found" {
		t.Errorf("Error = %q, want not-found", result.Error)
	}

	if result.Message != "resource not found" {
		t.Errorf("Message = %q, want 'resource not found'", result.Message)
	}

	if result.Suggestion != "try again" {
		t.Errorf("Suggestion = %q, want 'try again'", result.Suggestion)
	}

	if !result.Retryable {
		t.Error("Retryable should be true")
	}

	if result.Input["name"] != "foo" {
		t.Errorf("Input[name] = %q, want foo", result.Input["name"])
	}
}

// ── Warnf ─────────────────────────────────────────────────────────────────────

func TestWarnf_writesToStderr(t *testing.T) {
	out := captureStderr(t, func() {
		output.Warnf("disk usage is %d%%", 90)
	})

	if !strings.Contains(out, "Warning:") {
		t.Errorf("expected 'Warning:' prefix, got: %q", out)
	}

	if !strings.Contains(out, "disk usage is 90%") {
		t.Errorf("expected formatted message, got: %q", out)
	}
}
