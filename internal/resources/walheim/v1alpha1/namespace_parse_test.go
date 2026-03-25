package v1alpha1

import (
	"testing"

	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

// ── validateNamespaceManifest ─────────────────────────────────────────────────

func validNamespace(name string) *apiv1alpha1.Namespace {
	m := &apiv1alpha1.Namespace{}
	m.APIVersion = "walheim/v1alpha1"
	m.Kind = "Namespace"
	m.Name = name
	m.Spec = apiv1alpha1.NamespaceSpec{Hostname: "host.example.com"}

	return m
}

func TestValidateNamespaceManifest_valid(t *testing.T) {
	m := validNamespace("prod")
	if err := validateNamespaceManifest(m); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateNamespaceManifest_wrongAPIVersion(t *testing.T) {
	m := validNamespace("prod")
	m.APIVersion = "wrong/v1"

	if err := validateNamespaceManifest(m); err == nil {
		t.Error("expected error for wrong apiVersion")
	}
}

func TestValidateNamespaceManifest_wrongKind(t *testing.T) {
	m := validNamespace("prod")
	m.Kind = "App"

	if err := validateNamespaceManifest(m); err == nil {
		t.Error("expected error for wrong kind")
	}
}

func TestValidateNamespaceManifest_missingHostname(t *testing.T) {
	m := validNamespace("prod")
	m.Spec.Hostname = ""

	if err := validateNamespaceManifest(m); err == nil {
		t.Error("expected error for missing hostname")
	}
}

// ── namespaceToMeta ───────────────────────────────────────────────────────────

func TestNamespaceToMeta_basic(t *testing.T) {
	m := validNamespace("prod")
	meta := namespaceToMeta("prod", m)

	if meta.Name != "prod" {
		t.Errorf("Name = %q, want %q", meta.Name, "prod")
	}

	if meta.Summary["HOSTNAME"] != "host.example.com" {
		t.Errorf("HOSTNAME = %q, want %q", meta.Summary["HOSTNAME"], "host.example.com")
	}
}

func TestNamespaceToMeta_emptyHostname(t *testing.T) {
	m := validNamespace("prod")
	m.Spec.Hostname = ""

	meta := namespaceToMeta("prod", m)
	if meta.Summary["HOSTNAME"] != "N/A" {
		t.Errorf("HOSTNAME = %q, want %q", meta.Summary["HOSTNAME"], "N/A")
	}
}
