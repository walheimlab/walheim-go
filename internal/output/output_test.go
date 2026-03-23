package output

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/resource"
)

// ── flattenMeta ───────────────────────────────────────────────────────────────

func TestFlattenMeta_basic(t *testing.T) {
	item := resource.ResourceMeta{
		Namespace: "prod",
		Name:      "myapp",
		Summary:   map[string]string{"STATUS": "Running"},
		Labels:    map[string]string{"env": "prod"},
	}
	got := flattenMeta(item)

	if got["namespace"] != "prod" {
		t.Errorf("namespace = %q, want %q", got["namespace"], "prod")
	}

	if got["name"] != "myapp" {
		t.Errorf("name = %q, want %q", got["name"], "myapp")
	}
	// Summary keys are lowercased
	if got["status"] != "Running" {
		t.Errorf("status = %q, want %q", got["status"], "Running")
	}

	labels, ok := got["labels"].(map[string]string)
	if !ok {
		t.Fatalf("labels not present or wrong type: %T", got["labels"])
	}

	if labels["env"] != "prod" {
		t.Errorf("labels[env] = %q, want %q", labels["env"], "prod")
	}
}

func TestFlattenMeta_noNamespace(t *testing.T) {
	item := resource.ResourceMeta{Name: "mything"}

	got := flattenMeta(item)
	if _, ok := got["namespace"]; ok {
		t.Error("namespace key should be absent for cluster-scoped resources")
	}

	if got["name"] != "mything" {
		t.Errorf("name = %q, want %q", got["name"], "mything")
	}
}

func TestFlattenMeta_emptyLabelsOmitted(t *testing.T) {
	item := resource.ResourceMeta{Name: "x", Labels: map[string]string{}}

	got := flattenMeta(item)
	if _, ok := got["labels"]; ok {
		t.Error("labels key should be absent when empty")
	}
}

func TestFlattenMeta_nilLabelsOmitted(t *testing.T) {
	item := resource.ResourceMeta{Name: "x"}

	got := flattenMeta(item)
	if _, ok := got["labels"]; ok {
		t.Error("labels key should be absent when nil")
	}
}

func TestFlattenMeta_summaryKeysLowercased(t *testing.T) {
	item := resource.ResourceMeta{
		Name:    "x",
		Summary: map[string]string{"MyKey": "val", "UPPER": "u"},
	}

	got := flattenMeta(item)
	if got["mykey"] != "val" {
		t.Errorf("mykey = %q, want %q", got["mykey"], "val")
	}

	if got["upper"] != "u" {
		t.Errorf("upper = %q, want %q", got["upper"], "u")
	}
}

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
