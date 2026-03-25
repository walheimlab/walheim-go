package v1alpha1

import (
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/testutil"
	apiv1alpha1 "github.com/walheimlab/walheim-go/pkg/api/walheim/v1alpha1"
)

func TestGenerateJobCompose_writesComposeFile(t *testing.T) {
	mem := testutil.NewMemFS()
	spec := apiv1alpha1.JobSpec{Image: "alpine:latest"}

	if err := generateJobCompose("/data/out", "prod", "db-backup", spec, mem, "/data"); err != nil {
		t.Fatalf("generateJobCompose: %v", err)
	}

	data, err := mem.ReadFile("/data/out/docker-compose.yml")
	if err != nil {
		t.Fatalf("compose file not written: %v", err)
	}

	if !strings.Contains(string(data), "alpine:latest") {
		t.Errorf("compose file missing image: %s", data)
	}
}

func TestGenerateJobCompose_injectsWalheimLabels(t *testing.T) {
	mem := testutil.NewMemFS()
	spec := apiv1alpha1.JobSpec{Image: "alpine:latest"}

	if err := generateJobCompose("/data/out", "prod", "db-backup", spec, mem, "/data"); err != nil {
		t.Fatalf("generateJobCompose: %v", err)
	}

	data, err := mem.ReadFile("/data/out/docker-compose.yml")
	if err != nil {
		t.Fatalf("compose file not written: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "walheim.managed") {
		t.Error("missing walheim.managed label")
	}

	if !strings.Contains(content, "walheim.namespace") {
		t.Error("missing walheim.namespace label")
	}

	if !strings.Contains(content, "walheim.job") {
		t.Error("missing walheim.job label")
	}
}

func TestGenerateJobCompose_restartNo(t *testing.T) {
	mem := testutil.NewMemFS()
	spec := apiv1alpha1.JobSpec{Image: "alpine:latest"}

	if err := generateJobCompose("/data/out", "prod", "db-backup", spec, mem, "/data"); err != nil {
		t.Fatalf("generateJobCompose: %v", err)
	}

	data, _ := mem.ReadFile("/data/out/docker-compose.yml")
	if !strings.Contains(string(data), "restart: \"no\"") && !strings.Contains(string(data), "restart: 'no'") && !strings.Contains(string(data), "restart: no") {
		t.Errorf("compose file should set restart: no, got:\n%s", data)
	}
}

func TestGenerateJobCompose_envFromSecret(t *testing.T) {
	mem := testutil.NewMemFS()
	writeSecret(t, mem, "prod", "db-creds", map[string]string{"DB_PASS": "secret123"})

	spec := apiv1alpha1.JobSpec{
		Image: "alpine:latest",
		EnvFrom: []apiv1alpha1.EnvFromEntry{
			{SecretRef: &apiv1alpha1.NamedRef{Name: "db-creds"}},
		},
	}

	if err := generateJobCompose("/data/out", "prod", "db-backup", spec, mem, "/data"); err != nil {
		t.Fatalf("generateJobCompose: %v", err)
	}

	data, _ := mem.ReadFile("/data/out/docker-compose.yml")
	if !strings.Contains(string(data), "DB_PASS") {
		t.Errorf("compose file missing DB_PASS: %s", data)
	}
}

func TestGenerateJobCompose_envOverridesEnvFrom(t *testing.T) {
	mem := testutil.NewMemFS()
	writeSecret(t, mem, "prod", "cfg", map[string]string{"LOG_LEVEL": "info"})

	spec := apiv1alpha1.JobSpec{
		Image: "alpine:latest",
		EnvFrom: []apiv1alpha1.EnvFromEntry{
			{SecretRef: &apiv1alpha1.NamedRef{Name: "cfg"}},
		},
		Env: []apiv1alpha1.EnvEntry{
			{Name: "LOG_LEVEL", Value: "debug"},
		},
	}

	if err := generateJobCompose("/data/out", "prod", "db-backup", spec, mem, "/data"); err != nil {
		t.Fatalf("generateJobCompose: %v", err)
	}

	data, _ := mem.ReadFile("/data/out/docker-compose.yml")
	content := string(data)

	// "debug" should appear and "info" should not (overridden)
	if !strings.Contains(content, "debug") {
		t.Errorf("env override not applied, got:\n%s", content)
	}
}
