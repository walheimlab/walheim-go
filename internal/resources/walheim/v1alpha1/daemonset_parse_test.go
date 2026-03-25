package v1alpha1

import (
	"strings"
	"testing"

	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

// ── validateDaemonSetManifest ─────────────────────────────────────────────────

func validDaemonSet(name string) *apiv1alpha1.DaemonSet {
	m := &apiv1alpha1.DaemonSet{}
	m.APIVersion = "walheim/v1alpha1"
	m.Kind = "DaemonSet"
	m.Name = name
	m.Spec = apiv1alpha1.DaemonSetSpec{
		Compose: apiv1alpha1.ComposeSpec{
			Services: map[string]apiv1alpha1.ComposeService{
				"web": {Image: "nginx:latest"},
			},
		},
	}

	return m
}

func TestValidateDaemonSetManifest_valid(t *testing.T) {
	m := validDaemonSet("my-service")
	if err := validateDaemonSetManifest(m, "my-service"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDaemonSetManifest_wrongAPIVersion(t *testing.T) {
	m := validDaemonSet("my-service")
	m.APIVersion = "wrong/v1"

	if err := validateDaemonSetManifest(m, "my-service"); err == nil {
		t.Error("expected error for wrong apiVersion")
	}
}

func TestValidateDaemonSetManifest_wrongKind(t *testing.T) {
	m := validDaemonSet("my-service")
	m.Kind = "App"

	if err := validateDaemonSetManifest(m, "my-service"); err == nil {
		t.Error("expected error for wrong kind")
	}
}

func TestValidateDaemonSetManifest_nameMismatch(t *testing.T) {
	m := validDaemonSet("my-service")

	if err := validateDaemonSetManifest(m, "other"); err == nil {
		t.Error("expected error for name mismatch")
	}
}

func TestValidateDaemonSetManifest_emptyServices(t *testing.T) {
	m := validDaemonSet("my-service")
	m.Spec.Compose.Services = nil

	if err := validateDaemonSetManifest(m, "my-service"); err == nil {
		t.Error("expected error for empty services")
	}
}

// ── daemonSetToMeta ───────────────────────────────────────────────────────────

func TestDaemonSetToMeta_basic(t *testing.T) {
	m := validDaemonSet("my-service")
	meta := daemonSetToMeta("my-service", m, []string{"prod", "staging"})

	if meta.Name != "my-service" {
		t.Errorf("Name = %q, want %q", meta.Name, "my-service")
	}

	if meta.Summary["IMAGE"] != "nginx:latest" {
		t.Errorf("IMAGE = %q, want %q", meta.Summary["IMAGE"], "nginx:latest")
	}

	if meta.Summary["SELECTOR"] != "(all)" {
		t.Errorf("SELECTOR = %q, want %q", meta.Summary["SELECTOR"], "(all)")
	}

	ns := meta.Summary["NAMESPACES"]
	if !strings.Contains(ns, "prod") || !strings.Contains(ns, "staging") {
		t.Errorf("NAMESPACES = %q, want prod and staging", ns)
	}
}

func TestDaemonSetToMeta_withSelector(t *testing.T) {
	m := validDaemonSet("my-service")
	m.Spec.NamespaceSelector = &apiv1alpha1.LabelSelector{
		MatchLabels: map[string]string{"env": "prod"},
	}

	meta := daemonSetToMeta("my-service", m, []string{"prod"})
	if meta.Summary["SELECTOR"] != "env=prod" {
		t.Errorf("SELECTOR = %q, want %q", meta.Summary["SELECTOR"], "env=prod")
	}
}

func TestDaemonSetToMeta_noNamespaces(t *testing.T) {
	m := validDaemonSet("my-service")
	meta := daemonSetToMeta("my-service", m, nil)

	if meta.Summary["NAMESPACES"] != "0" {
		t.Errorf("NAMESPACES = %q, want %q", meta.Summary["NAMESPACES"], "0")
	}
}
