package v1alpha1

import (
	"testing"

	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

// ── validateAppManifest ───────────────────────────────────────────────────────

func validApp(namespace, name string) *apiv1alpha1.App {
	m := &apiv1alpha1.App{}
	m.APIVersion = "walheim/v1alpha1"
	m.Kind = "App"
	m.Name = name
	m.Namespace = namespace
	m.Spec = apiv1alpha1.AppSpec{
		Compose: apiv1alpha1.ComposeSpec{
			Services: map[string]apiv1alpha1.ComposeService{
				"web": {Image: "nginx:latest"},
			},
		},
	}

	return m
}

func TestValidateAppManifest_valid(t *testing.T) {
	m := validApp("prod", "myapp")
	if err := validateAppManifest(m, "prod", "myapp"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAppManifest_wrongAPIVersion(t *testing.T) {
	m := validApp("prod", "myapp")
	m.APIVersion = "wrong/v1"

	if err := validateAppManifest(m, "prod", "myapp"); err == nil {
		t.Error("expected error for wrong apiVersion")
	}
}

func TestValidateAppManifest_wrongKind(t *testing.T) {
	m := validApp("prod", "myapp")
	m.Kind = "Job"

	if err := validateAppManifest(m, "prod", "myapp"); err == nil {
		t.Error("expected error for wrong kind")
	}
}

func TestValidateAppManifest_nameMismatch(t *testing.T) {
	m := validApp("prod", "myapp")

	if err := validateAppManifest(m, "prod", "other"); err == nil {
		t.Error("expected error for name mismatch")
	}
}

func TestValidateAppManifest_namespaceMismatch(t *testing.T) {
	m := validApp("prod", "myapp")

	if err := validateAppManifest(m, "staging", "myapp"); err == nil {
		t.Error("expected error for namespace mismatch")
	}
}

func TestValidateAppManifest_emptyServices(t *testing.T) {
	m := validApp("prod", "myapp")
	m.Spec.Compose.Services = nil

	if err := validateAppManifest(m, "prod", "myapp"); err == nil {
		t.Error("expected error for empty services")
	}
}

// ── appToMeta ─────────────────────────────────────────────────────────────────

func TestAppToMeta_basic(t *testing.T) {
	m := validApp("prod", "myapp")
	meta := appToMeta("prod", "myapp", m, "Running", "2/2")

	if meta.Namespace != "prod" {
		t.Errorf("Namespace = %q, want %q", meta.Namespace, "prod")
	}

	if meta.Name != "myapp" {
		t.Errorf("Name = %q, want %q", meta.Name, "myapp")
	}

	if meta.Summary["IMAGE"] != "nginx:latest" {
		t.Errorf("IMAGE = %q, want %q", meta.Summary["IMAGE"], "nginx:latest")
	}

	if meta.Summary["STATUS"] != "Running" {
		t.Errorf("STATUS = %q, want %q", meta.Summary["STATUS"], "Running")
	}

	if meta.Summary["READY"] != "2/2" {
		t.Errorf("READY = %q, want %q", meta.Summary["READY"], "2/2")
	}
}

func TestAppToMeta_noImage(t *testing.T) {
	m := validApp("prod", "myapp")
	m.Spec.Compose.Services = map[string]apiv1alpha1.ComposeService{
		"web": {Image: ""},
	}

	meta := appToMeta("prod", "myapp", m, "Stopped", "0/1")
	if meta.Summary["IMAGE"] != "N/A" {
		t.Errorf("IMAGE = %q, want %q", meta.Summary["IMAGE"], "N/A")
	}
}
