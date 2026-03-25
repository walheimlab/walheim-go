package v1alpha1

import (
	"testing"

	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

// ── validateJobManifest ───────────────────────────────────────────────────────

func validJob(namespace, name string) *apiv1alpha1.Job {
	m := &apiv1alpha1.Job{}
	m.APIVersion = "walheim/v1alpha1"
	m.Kind = "Job"
	m.Name = name
	m.Namespace = namespace
	m.Spec = apiv1alpha1.JobSpec{Image: "alpine:latest"}

	return m
}

func TestValidateJobManifest_valid(t *testing.T) {
	m := validJob("prod", "db-backup")
	if err := validateJobManifest(m, "prod", "db-backup"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateJobManifest_wrongAPIVersion(t *testing.T) {
	m := validJob("prod", "db-backup")
	m.APIVersion = "wrong/v1"

	if err := validateJobManifest(m, "prod", "db-backup"); err == nil {
		t.Error("expected error for wrong apiVersion")
	}
}

func TestValidateJobManifest_wrongKind(t *testing.T) {
	m := validJob("prod", "db-backup")
	m.Kind = "App"

	if err := validateJobManifest(m, "prod", "db-backup"); err == nil {
		t.Error("expected error for wrong kind")
	}
}

func TestValidateJobManifest_nameMismatch(t *testing.T) {
	m := validJob("prod", "db-backup")

	if err := validateJobManifest(m, "prod", "other"); err == nil {
		t.Error("expected error for name mismatch")
	}
}

func TestValidateJobManifest_namespaceMismatch(t *testing.T) {
	m := validJob("prod", "db-backup")

	if err := validateJobManifest(m, "staging", "db-backup"); err == nil {
		t.Error("expected error for namespace mismatch")
	}
}

func TestValidateJobManifest_missingImage(t *testing.T) {
	m := validJob("prod", "db-backup")
	m.Spec.Image = ""

	if err := validateJobManifest(m, "prod", "db-backup"); err == nil {
		t.Error("expected error for missing image")
	}
}

// ── jobToMeta ─────────────────────────────────────────────────────────────────

func TestJobToMeta_basic(t *testing.T) {
	m := validJob("prod", "db-backup")
	meta := jobToMeta("prod", "db-backup", m, "Succeeded", "2024-01-15 10:30")

	if meta.Namespace != "prod" {
		t.Errorf("Namespace = %q, want %q", meta.Namespace, "prod")
	}

	if meta.Name != "db-backup" {
		t.Errorf("Name = %q, want %q", meta.Name, "db-backup")
	}

	if meta.Summary["IMAGE"] != "alpine:latest" {
		t.Errorf("IMAGE = %q, want %q", meta.Summary["IMAGE"], "alpine:latest")
	}

	if meta.Summary["STATUS"] != "Succeeded" {
		t.Errorf("STATUS = %q, want %q", meta.Summary["STATUS"], "Succeeded")
	}

	if meta.Summary["LAST RUN"] != "2024-01-15 10:30" {
		t.Errorf("LAST RUN = %q, want %q", meta.Summary["LAST RUN"], "2024-01-15 10:30")
	}
}
