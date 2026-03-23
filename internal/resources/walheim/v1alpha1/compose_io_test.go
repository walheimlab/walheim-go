package v1alpha1

import (
	"encoding/base64"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/testutil"
)

// ── loadSecret ────────────────────────────────────────────────────────────────

func TestLoadSecret_stringData(t *testing.T) {
	mem := testutil.NewMemFS()
	sm := SecretManifest{
		APIVersion: "walheim/v1alpha1",
		Kind:       "Secret",
		Metadata:   ResourceMetadata{Name: "db", Namespace: "prod"},
		StringData: map[string]string{"PASSWORD": "s3cr3t"},
	}
	data, _ := yaml.Marshal(sm)
	_ = mem.WriteFile("/data/namespaces/prod/secrets/db/.secret.yaml", data)

	kv, err := loadSecret("prod", "db", mem, "/data")
	if err != nil {
		t.Fatalf("loadSecret: %v", err)
	}

	if kv["PASSWORD"] != "s3cr3t" {
		t.Errorf("PASSWORD = %q, want %q", kv["PASSWORD"], "s3cr3t")
	}
}

func TestLoadSecret_base64Data(t *testing.T) {
	mem := testutil.NewMemFS()
	encoded := base64.StdEncoding.EncodeToString([]byte("mypassword"))
	sm := SecretManifest{
		APIVersion: "walheim/v1alpha1",
		Kind:       "Secret",
		Metadata:   ResourceMetadata{Name: "db", Namespace: "prod"},
		Data:       map[string]string{"PASSWORD": encoded},
	}
	data, _ := yaml.Marshal(sm)
	_ = mem.WriteFile("/data/namespaces/prod/secrets/db/.secret.yaml", data)

	kv, err := loadSecret("prod", "db", mem, "/data")
	if err != nil {
		t.Fatalf("loadSecret: %v", err)
	}

	if kv["PASSWORD"] != "mypassword" {
		t.Errorf("PASSWORD = %q, want %q", kv["PASSWORD"], "mypassword")
	}
}

func TestLoadSecret_stringDataWinsOverData(t *testing.T) {
	mem := testutil.NewMemFS()
	encoded := base64.StdEncoding.EncodeToString([]byte("from-data"))
	sm := SecretManifest{
		APIVersion: "walheim/v1alpha1",
		Kind:       "Secret",
		Metadata:   ResourceMetadata{Name: "db", Namespace: "prod"},
		Data:       map[string]string{"KEY": encoded},
		StringData: map[string]string{"KEY": "from-stringdata"},
	}
	data, _ := yaml.Marshal(sm)
	_ = mem.WriteFile("/data/namespaces/prod/secrets/db/.secret.yaml", data)

	kv, err := loadSecret("prod", "db", mem, "/data")
	if err != nil {
		t.Fatalf("loadSecret: %v", err)
	}

	if kv["KEY"] != "from-stringdata" {
		t.Errorf("KEY = %q, want stringData to win", kv["KEY"])
	}
}

func TestLoadSecret_notFound(t *testing.T) {
	mem := testutil.NewMemFS()

	_, err := loadSecret("prod", "missing", mem, "/data")
	if err == nil {
		t.Error("expected error for missing secret")
	}
}

// ── loadConfigMap ─────────────────────────────────────────────────────────────

func TestLoadConfigMap_basic(t *testing.T) {
	mem := testutil.NewMemFS()
	cm := ConfigMapManifest{
		APIVersion: "walheim/v1alpha1",
		Kind:       "ConfigMap",
		Metadata:   ResourceMetadata{Name: "app-cfg", Namespace: "prod"},
		Data:       map[string]string{"LOG_LEVEL": "debug", "PORT": "8080"},
	}
	data, _ := yaml.Marshal(cm)
	_ = mem.WriteFile("/data/namespaces/prod/configmaps/app-cfg/.configmap.yaml", data)

	kv, err := loadConfigMap("prod", "app-cfg", mem, "/data")
	if err != nil {
		t.Fatalf("loadConfigMap: %v", err)
	}

	if kv["LOG_LEVEL"] != "debug" {
		t.Errorf("LOG_LEVEL = %q, want %q", kv["LOG_LEVEL"], "debug")
	}

	if kv["PORT"] != "8080" {
		t.Errorf("PORT = %q, want %q", kv["PORT"], "8080")
	}
}

func TestLoadConfigMap_notFound(t *testing.T) {
	mem := testutil.NewMemFS()

	_, err := loadConfigMap("prod", "missing", mem, "/data")
	if err == nil {
		t.Error("expected error for missing configmap")
	}
}

// ── generateCompose (envFrom + env injection) ─────────────────────────────────

func writeSecret(t *testing.T, mem *testutil.MemFS, namespace, name string, kv map[string]string) {
	t.Helper()

	sm := SecretManifest{
		APIVersion: "walheim/v1alpha1",
		Kind:       "Secret",
		Metadata:   ResourceMetadata{Name: name, Namespace: namespace},
		StringData: kv,
	}
	data, _ := yaml.Marshal(sm)
	_ = mem.WriteFile("/data/namespaces/"+namespace+"/secrets/"+name+"/.secret.yaml", data)
}

func minimalApp(namespace, name string) *AppManifest {
	return &AppManifest{
		APIVersion: "walheim/v1alpha1",
		Kind:       "App",
		Metadata:   ResourceMetadata{Name: name, Namespace: namespace},
		Spec: AppSpec{
			Compose: ComposeSpec{
				Services: map[string]ComposeService{
					"web": {Image: "nginx:latest"},
				},
			},
		},
	}
}

func TestGenerateCompose_injectsWalheimLabels(t *testing.T) {
	mem := testutil.NewMemFS()
	m := minimalApp("prod", "myapp")

	if err := generateCompose("prod", "myapp", m, mem, "/data"); err != nil {
		t.Fatalf("generateCompose: %v", err)
	}

	svc := m.Spec.Compose.Services["web"]
	if svc.Labels.Values["walheim.managed"] != "true" {
		t.Error("missing walheim.managed label")
	}

	if svc.Labels.Values["walheim.namespace"] != "prod" {
		t.Errorf("walheim.namespace = %q, want %q", svc.Labels.Values["walheim.namespace"], "prod")
	}

	if svc.Labels.Values["walheim.app"] != "myapp" {
		t.Errorf("walheim.app = %q, want %q", svc.Labels.Values["walheim.app"], "myapp")
	}
}

func TestGenerateCompose_envFromSecret(t *testing.T) {
	mem := testutil.NewMemFS()
	writeSecret(t, mem, "prod", "db-creds", map[string]string{"DB_PASS": "secret"})

	m := minimalApp("prod", "myapp")
	m.Spec.EnvFrom = []EnvFromEntry{
		{SecretRef: &NamedRef{Name: "db-creds"}},
	}

	if err := generateCompose("prod", "myapp", m, mem, "/data"); err != nil {
		t.Fatalf("generateCompose: %v", err)
	}

	svc := m.Spec.Compose.Services["web"]
	if svc.Environment.Values["DB_PASS"] != "secret" {
		t.Errorf("DB_PASS = %q, want %q", svc.Environment.Values["DB_PASS"], "secret")
	}
}

func TestGenerateCompose_envOverridesEnvFrom(t *testing.T) {
	mem := testutil.NewMemFS()
	writeSecret(t, mem, "prod", "db-creds", map[string]string{"LOG_LEVEL": "info"})

	m := minimalApp("prod", "myapp")
	m.Spec.EnvFrom = []EnvFromEntry{
		{SecretRef: &NamedRef{Name: "db-creds"}},
	}
	m.Spec.Env = []EnvEntry{
		{Name: "LOG_LEVEL", Value: "debug"},
	}

	if err := generateCompose("prod", "myapp", m, mem, "/data"); err != nil {
		t.Fatalf("generateCompose: %v", err)
	}

	svc := m.Spec.Compose.Services["web"]
	if svc.Environment.Values["LOG_LEVEL"] != "debug" {
		t.Errorf("LOG_LEVEL = %q, want env to override envFrom", svc.Environment.Values["LOG_LEVEL"])
	}
}

func TestGenerateCompose_writesComposeFile(t *testing.T) {
	mem := testutil.NewMemFS()
	m := minimalApp("prod", "myapp")

	if err := generateCompose("prod", "myapp", m, mem, "/data"); err != nil {
		t.Fatalf("generateCompose: %v", err)
	}

	data, err := mem.ReadFile("/data/namespaces/prod/apps/myapp/docker-compose.yml")
	if err != nil {
		t.Fatalf("compose file not written: %v", err)
	}

	if !strings.Contains(string(data), "nginx:latest") {
		t.Errorf("compose file missing image: %s", data)
	}
}

func TestGenerateCompose_emptyServices_error(t *testing.T) {
	mem := testutil.NewMemFS()
	m := minimalApp("prod", "myapp")
	m.Spec.Compose.Services = nil

	err := generateCompose("prod", "myapp", m, mem, "/data")
	if err == nil {
		t.Error("expected error for empty services")
	}
}
