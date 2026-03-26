package v1alpha1

import (
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/testutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func minimalDaemonSet(name string) *apiv1alpha1.DaemonSet {
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

func TestGenerateDaemonSetCompose_injectsWalheimLabels(t *testing.T) {
	mem := testutil.NewMemFS()
	m := minimalDaemonSet("my-service")

	if err := generateDaemonSetCompose("prod", "my-service", m, mem, "/data"); err != nil {
		t.Fatalf("generateDaemonSetCompose: %v", err)
	}

	svc := m.Spec.Compose.Services["web"]
	if svc.Labels.Values["walheim.managed"] != "true" {
		t.Error("missing walheim.managed label")
	}

	if svc.Labels.Values["walheim.namespace"] != "prod" {
		t.Errorf("walheim.namespace = %q, want %q", svc.Labels.Values["walheim.namespace"], "prod")
	}

	if svc.Labels.Values["walheim.owner"] != "my-service" {
		t.Errorf("walheim.owner = %q, want %q", svc.Labels.Values["walheim.owner"], "my-service")
	}

	if svc.Labels.Values["walheim.kind"] != "DaemonSet" {
		t.Errorf("walheim.kind = %q, want %q", svc.Labels.Values["walheim.kind"], "DaemonSet")
	}
}

func TestGenerateDaemonSetCompose_writesComposeFile(t *testing.T) {
	mem := testutil.NewMemFS()
	m := minimalDaemonSet("my-service")

	if err := generateDaemonSetCompose("prod", "my-service", m, mem, "/data"); err != nil {
		t.Fatalf("generateDaemonSetCompose: %v", err)
	}

	data, err := mem.ReadFile("/data/daemonsets/my-service/prod/docker-compose.yml")
	if err != nil {
		t.Fatalf("compose file not written: %v", err)
	}

	if !strings.Contains(string(data), "nginx:latest") {
		t.Errorf("compose file missing image: %s", data)
	}
}

func TestGenerateDaemonSetCompose_emptyServices_error(t *testing.T) {
	mem := testutil.NewMemFS()
	m := minimalDaemonSet("my-service")
	m.Spec.Compose.Services = nil

	if err := generateDaemonSetCompose("prod", "my-service", m, mem, "/data"); err == nil {
		t.Error("expected error for empty services")
	}
}

func TestGenerateDaemonSetCompose_envFromSecret(t *testing.T) {
	mem := testutil.NewMemFS()
	writeSecret(t, mem, "prod", "db-creds", map[string]string{"DB_PASS": "secret"})

	m := minimalDaemonSet("my-service")
	m.Spec.EnvFrom = []apiv1alpha1.EnvFromEntry{
		{SecretRef: &apiv1alpha1.NamedRef{Name: "db-creds"}},
	}

	if err := generateDaemonSetCompose("prod", "my-service", m, mem, "/data"); err != nil {
		t.Fatalf("generateDaemonSetCompose: %v", err)
	}

	svc := m.Spec.Compose.Services["web"]
	if svc.Environment.Values["DB_PASS"] != "secret" {
		t.Errorf("DB_PASS = %q, want %q", svc.Environment.Values["DB_PASS"], "secret")
	}
}

func TestGenerateDaemonSetCompose_envOverridesEnvFrom(t *testing.T) {
	mem := testutil.NewMemFS()
	writeSecret(t, mem, "prod", "cfg", map[string]string{"LOG_LEVEL": "info"})

	m := minimalDaemonSet("my-service")
	m.Spec.EnvFrom = []apiv1alpha1.EnvFromEntry{
		{SecretRef: &apiv1alpha1.NamedRef{Name: "cfg"}},
	}
	m.Spec.Env = []apiv1alpha1.EnvEntry{
		{Name: "LOG_LEVEL", Value: "debug"},
	}

	if err := generateDaemonSetCompose("prod", "my-service", m, mem, "/data"); err != nil {
		t.Fatalf("generateDaemonSetCompose: %v", err)
	}

	svc := m.Spec.Compose.Services["web"]
	if svc.Environment.Values["LOG_LEVEL"] != "debug" {
		t.Errorf("LOG_LEVEL = %q, want env to override envFrom", svc.Environment.Values["LOG_LEVEL"])
	}
}
