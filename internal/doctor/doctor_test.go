package doctor_test

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/doctor"
)

// ── Report accumulation ───────────────────────────────────────────────────────

func TestReport_Add(t *testing.T) {
	var r doctor.Report
	r.Add(doctor.SeverityError, "ns/app", "check-one", "something broke")

	findings := r.Findings()
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	f := findings[0]
	if f.Severity != doctor.SeverityError {
		t.Errorf("Severity = %q, want %q", f.Severity, doctor.SeverityError)
	}

	if f.Resource != "ns/app" {
		t.Errorf("Resource = %q, want %q", f.Resource, "ns/app")
	}

	if f.Check != "check-one" {
		t.Errorf("Check = %q, want %q", f.Check, "check-one")
	}

	if f.Message != "something broke" {
		t.Errorf("Message = %q, want %q", f.Message, "something broke")
	}
}

func TestReport_Errorf(t *testing.T) {
	var r doctor.Report
	r.Errorf("res", "chk", "value is %d", 42)

	f := r.Findings()[0]
	if f.Severity != doctor.SeverityError {
		t.Errorf("Severity = %q, want error", f.Severity)
	}

	if f.Message != "value is 42" {
		t.Errorf("Message = %q, want %q", f.Message, "value is 42")
	}
}

func TestReport_Warnf(t *testing.T) {
	var r doctor.Report
	r.Warnf("res", "chk", "warn %s", "msg")

	f := r.Findings()[0]
	if f.Severity != doctor.SeverityWarning {
		t.Errorf("Severity = %q, want warning", f.Severity)
	}
}

func TestReport_Infof(t *testing.T) {
	var r doctor.Report
	r.Infof("res", "chk", "info %s", "msg")

	f := r.Findings()[0]
	if f.Severity != doctor.SeverityInfo {
		t.Errorf("Severity = %q, want info", f.Severity)
	}
}

func TestReport_Findings_returnsCopy(t *testing.T) {
	var r doctor.Report
	r.Add(doctor.SeverityInfo, "r", "c", "m")
	f1 := r.Findings()
	f1[0].Message = "mutated"

	f2 := r.Findings()
	if f2[0].Message == "mutated" {
		t.Error("Findings() returned a live reference, not a copy")
	}
}

// ── HasErrors / Counts ────────────────────────────────────────────────────────

func TestReport_HasErrors_false_when_empty(t *testing.T) {
	var r doctor.Report
	if r.HasErrors() {
		t.Error("HasErrors() = true on empty report")
	}
}

func TestReport_HasErrors_false_with_only_warnings(t *testing.T) {
	var r doctor.Report
	r.Warnf("r", "c", "warn")

	if r.HasErrors() {
		t.Error("HasErrors() = true with only warnings")
	}
}

func TestReport_HasErrors_true(t *testing.T) {
	var r doctor.Report
	r.Errorf("r", "c", "err")

	if !r.HasErrors() {
		t.Error("HasErrors() = false but an error was added")
	}
}

func TestReport_Counts(t *testing.T) {
	var r doctor.Report
	r.Errorf("r", "c", "e1")
	r.Errorf("r", "c", "e2")
	r.Warnf("r", "c", "w1")
	r.Infof("r", "c", "i1")
	r.Infof("r", "c", "i2")
	r.Infof("r", "c", "i3")

	errors, warnings, infos := r.Counts()
	if errors != 2 {
		t.Errorf("errors = %d, want 2", errors)
	}

	if warnings != 1 {
		t.Errorf("warnings = %d, want 1", warnings)
	}

	if infos != 3 {
		t.Errorf("infos = %d, want 3", infos)
	}
}

func TestReport_Counts_empty(t *testing.T) {
	var r doctor.Report

	e, w, i := r.Counts()
	if e != 0 || w != 0 || i != 0 {
		t.Errorf("Counts() = (%d,%d,%d), want (0,0,0)", e, w, i)
	}
}

// ── Common checks ─────────────────────────────────────────────────────────────

func TestCheckDirNameMatchesMetadataName_match(t *testing.T) {
	var r doctor.Report
	doctor.CheckDirNameMatchesMetadataName(&r, "ns/app", "myapp", "myapp")

	if r.HasErrors() {
		t.Errorf("unexpected error for matching names: %v", r.Findings())
	}
}

func TestCheckDirNameMatchesMetadataName_mismatch(t *testing.T) {
	var r doctor.Report
	doctor.CheckDirNameMatchesMetadataName(&r, "ns/app", "myapp", "different")

	if !r.HasErrors() {
		t.Error("expected error for mismatched names")
	}

	f := r.Findings()[0]
	if f.Check != "metadata-name-mismatch" {
		t.Errorf("Check = %q, want %q", f.Check, "metadata-name-mismatch")
	}
}

func TestCheckNamespaceFieldMatchesDir_match(t *testing.T) {
	var r doctor.Report
	doctor.CheckNamespaceFieldMatchesDir(&r, "ns/app", "prod", "prod")

	if r.HasErrors() {
		t.Errorf("unexpected error: %v", r.Findings())
	}
}

func TestCheckNamespaceFieldMatchesDir_mismatch(t *testing.T) {
	var r doctor.Report
	doctor.CheckNamespaceFieldMatchesDir(&r, "ns/app", "staging", "prod")

	if !r.HasErrors() {
		t.Error("expected error for namespace mismatch")
	}

	if r.Findings()[0].Check != "metadata-namespace-mismatch" {
		t.Errorf("Check = %q", r.Findings()[0].Check)
	}
}

func TestCheckAPIVersion_match(t *testing.T) {
	var r doctor.Report
	doctor.CheckAPIVersion(&r, "res", "walheim/v1alpha1", "walheim/v1alpha1")

	if r.HasErrors() {
		t.Errorf("unexpected error: %v", r.Findings())
	}
}

func TestCheckAPIVersion_mismatch(t *testing.T) {
	var r doctor.Report
	doctor.CheckAPIVersion(&r, "res", "walheim/v1beta1", "walheim/v1alpha1")

	if !r.HasErrors() {
		t.Error("expected error for apiVersion mismatch")
	}

	if r.Findings()[0].Check != "apiversion-mismatch" {
		t.Errorf("Check = %q", r.Findings()[0].Check)
	}
}

func TestCheckKind_match(t *testing.T) {
	var r doctor.Report
	doctor.CheckKind(&r, "res", "App", "App")

	if r.HasErrors() {
		t.Errorf("unexpected error: %v", r.Findings())
	}
}

func TestCheckKind_mismatch(t *testing.T) {
	var r doctor.Report
	doctor.CheckKind(&r, "res", "Deployment", "App")

	if !r.HasErrors() {
		t.Error("expected error for kind mismatch")
	}

	if r.Findings()[0].Check != "kind-mismatch" {
		t.Errorf("Check = %q", r.Findings()[0].Check)
	}
}
