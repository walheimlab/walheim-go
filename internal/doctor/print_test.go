package doctor_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/doctor"
)

// captureStdout redirects os.Stdout during f(), returns captured output.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	orig := os.Stdout
	os.Stdout = w

	f()

	_ = w.Close()

	os.Stdout = orig

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}

	return buf.String()
}

// ── PrintHuman ────────────────────────────────────────────────────────────────

func TestPrintHuman_noFindings_prints_noIssues(t *testing.T) {
	var r doctor.Report

	out := captureStdout(t, func() { r.PrintHuman(false) })

	if !strings.Contains(out, "No issues found.") {
		t.Errorf("expected 'No issues found.', got: %q", out)
	}
}

func TestPrintHuman_noFindings_quiet_printsNothing(t *testing.T) {
	var r doctor.Report

	out := captureStdout(t, func() { r.PrintHuman(true) })

	if out != "" {
		t.Errorf("expected empty output in quiet mode, got: %q", out)
	}
}

func TestPrintHuman_withFindings_showsTable(t *testing.T) {
	var r doctor.Report
	r.Errorf("ns/app", "missing-file", "file not found")
	r.Warnf("ns/app", "stale-config", "config outdated")

	out := captureStdout(t, func() { r.PrintHuman(false) })

	if !strings.Contains(out, "SEVERITY") {
		t.Errorf("expected header row, got: %q", out)
	}

	if !strings.Contains(out, "ERROR") {
		t.Errorf("expected ERROR row, got: %q", out)
	}

	if !strings.Contains(out, "WARNING") {
		t.Errorf("expected WARNING row, got: %q", out)
	}

	if !strings.Contains(out, "missing-file") {
		t.Errorf("expected check name, got: %q", out)
	}

	if !strings.Contains(out, "Summary:") {
		t.Errorf("expected Summary line, got: %q", out)
	}
}

func TestPrintHuman_quiet_withFindings_showsRowsNoHeader(t *testing.T) {
	var r doctor.Report
	r.Errorf("res", "chk", "error msg")

	out := captureStdout(t, func() { r.PrintHuman(true) })

	if strings.Contains(out, "SEVERITY") {
		t.Errorf("quiet mode should not show header, got: %q", out)
	}

	if !strings.Contains(out, "ERROR") {
		t.Errorf("expected ERROR row in quiet mode, got: %q", out)
	}

	if strings.Contains(out, "Summary:") {
		t.Errorf("quiet mode should not show summary, got: %q", out)
	}
}

// ── PrintJSON ─────────────────────────────────────────────────────────────────

func TestPrintJSON_emptyReport(t *testing.T) {
	var r doctor.Report

	out := captureStdout(t, func() {
		if err := r.PrintJSON(); err != nil {
			t.Errorf("PrintJSON: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %q", err, out)
	}

	findings, ok := result["findings"].([]any)
	if !ok {
		t.Fatalf("findings not []any: %T", result["findings"])
	}

	if len(findings) != 0 {
		t.Errorf("expected empty findings, got %v", findings)
	}

	summary, ok := result["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary not map: %T", result["summary"])
	}

	if summary["errors"] != float64(0) {
		t.Errorf("errors = %v, want 0", summary["errors"])
	}
}

func TestPrintJSON_withFindings(t *testing.T) {
	var r doctor.Report
	r.Errorf("ns/app", "missing-file", "file not found")
	r.Warnf("ns/app", "stale", "config outdated")
	r.Infof("ns/app", "info-check", "all good")

	out := captureStdout(t, func() {
		if err := r.PrintJSON(); err != nil {
			t.Errorf("PrintJSON: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %q", err, out)
	}

	findings, ok := result["findings"].([]any)
	if !ok {
		t.Fatalf("findings not []any: %T", result["findings"])
	}

	if len(findings) != 3 {
		t.Errorf("expected 3 findings, got %d", len(findings))
	}

	summary, ok := result["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary not map: %T", result["summary"])
	}

	if summary["errors"] != float64(1) {
		t.Errorf("errors = %v, want 1", summary["errors"])
	}

	if summary["warnings"] != float64(1) {
		t.Errorf("warnings = %v, want 1", summary["warnings"])
	}

	if summary["infos"] != float64(1) {
		t.Errorf("infos = %v, want 1", summary["infos"])
	}
}
