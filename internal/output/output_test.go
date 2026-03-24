package output

import (
	"testing"
)

// ── lookupSummary ─────────────────────────────────────────────────────────────

func TestLookupSummary_exactMatch(t *testing.T) {
	summary := map[string]string{"STATUS": "Running"}

	got := lookupSummary(summary, "STATUS")
	if got != "Running" {
		t.Errorf("lookupSummary() = %q, want %q", got, "Running")
	}
}

func TestLookupSummary_caseInsensitive(t *testing.T) {
	summary := map[string]string{"STATUS": "Running"}

	got := lookupSummary(summary, "status")
	if got != "Running" {
		t.Errorf("lookupSummary() = %q, want %q", got, "Running")
	}
}

func TestLookupSummary_missingKey(t *testing.T) {
	summary := map[string]string{"STATUS": "Running"}

	got := lookupSummary(summary, "READY")
	if got != "" {
		t.Errorf("lookupSummary() = %q, want empty", got)
	}
}

func TestLookupSummary_nilSummary(t *testing.T) {
	got := lookupSummary(nil, "anything")
	if got != "" {
		t.Errorf("lookupSummary(nil) = %q, want empty", got)
	}
}

// ── rawToAny ──────────────────────────────────────────────────────────────────

func TestRawToAny_basicStruct(t *testing.T) {
	type manifest struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}

	m := manifest{APIVersion: "walheim/v1alpha1", Kind: "Namespace"}

	obj, err := rawToAny(m)
	if err != nil {
		t.Fatalf("rawToAny() error: %v", err)
	}

	mp, ok := obj.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", obj)
	}

	if mp["apiVersion"] != "walheim/v1alpha1" {
		t.Errorf("apiVersion = %q, want %q", mp["apiVersion"], "walheim/v1alpha1")
	}

	if mp["kind"] != "Namespace" {
		t.Errorf("kind = %q, want %q", mp["kind"], "Namespace")
	}
}

// ── buildEmptyList ────────────────────────────────────────────────────────────

func TestBuildEmptyList(t *testing.T) {
	list := buildEmptyList()

	if list["apiVersion"] != "v1" {
		t.Errorf("apiVersion = %q, want %q", list["apiVersion"], "v1")
	}

	if list["kind"] != "List" {
		t.Errorf("kind = %q, want %q", list["kind"], "List")
	}

	items, ok := list["items"].([]any)
	if !ok {
		t.Fatalf("items not []any, got %T", list["items"])
	}

	if len(items) != 0 {
		t.Errorf("items len = %d, want 0", len(items))
	}
}
