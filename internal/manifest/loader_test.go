package manifest_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/walheimlab/walheim-go/internal/manifest"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

// ── fixture manifests ─────────────────────────────────────────────────────────

const nsYAML = `apiVersion: walheim/v1alpha1
kind: Namespace
metadata:
  name: production
spec:
  hostname: server.example.com
`

const appYAML = `apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: memos
  namespace: production
spec:
  compose:
    services:
      web:
        image: neosmemo/memos:latest
`

const secretYAML = `apiVersion: v1
kind: Secret
metadata:
  name: db-creds
  namespace: production
stringData:
  password: s3cr3t
`

const appJSON = `{
  "apiVersion": "walheim/v1alpha1",
  "kind": "App",
  "metadata": {"name": "memos", "namespace": "production"},
  "spec": {"compose": {"services": {"web": {"image": "neosmemo/memos:latest"}}}}
}`

const kindListYAML = `apiVersion: v1
kind: List
items:
  - apiVersion: walheim/v1alpha1
    kind: Namespace
    metadata:
      name: production
    spec:
      hostname: server.example.com
  - apiVersion: walheim/v1alpha1
    kind: App
    metadata:
      name: memos
      namespace: production
    spec:
      compose:
        services:
          web:
            image: neosmemo/memos:latest
`

// assertEnvelope checks the GVK and metadata fields of a single Envelope.
func assertEnvelope(t *testing.T, got manifest.Envelope, wantKind, wantName, wantNS string) {
	t.Helper()

	if got.Kind != wantKind {
		t.Errorf("Kind = %q, want %q", got.Kind, wantKind)
	}

	if got.Name != wantName {
		t.Errorf("Name = %q, want %q", got.Name, wantName)
	}

	if got.Namespace != wantNS {
		t.Errorf("Namespace = %q, want %q", got.Namespace, wantNS)
	}

	if len(got.Raw) == 0 {
		t.Error("Raw is empty")
	}
}

// ── single document ───────────────────────────────────────────────────────────

func TestLoadSources_singleDocument(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/manifests/ns.yaml", []byte(nsYAML))

	envs, err := manifest.LoadSources([]string{"/manifests/ns.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(envs))
	}

	assertEnvelope(t, envs[0], "Namespace", "production", "")
}

func TestLoadSources_envelopeAPIVersion(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/ns.yaml", []byte(nsYAML))

	envs, err := manifest.LoadSources([]string{"/ns.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	if envs[0].APIVersion != "walheim/v1alpha1" {
		t.Errorf("APIVersion = %q, want %q", envs[0].APIVersion, "walheim/v1alpha1")
	}
}

func TestLoadSources_rawBytesAreValidYAML(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/app.yaml", []byte(appYAML))

	envs, err := manifest.LoadSources([]string{"/app.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	var m map[string]any

	if err := yaml.Unmarshal(envs[0].Raw, &m); err != nil {
		t.Errorf("Raw bytes are not valid YAML: %v", err)
	}

	if m["kind"] != "App" {
		t.Errorf("kind in Raw = %v, want App", m["kind"])
	}
}

// ── multi-document YAML ───────────────────────────────────────────────────────

func TestLoadSources_multiDocYAML(t *testing.T) {
	multi := nsYAML + "---\n" + appYAML
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/manifests/multi.yaml", []byte(multi))

	envs, err := manifest.LoadSources([]string{"/manifests/multi.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes, got %d", len(envs))
	}

	assertEnvelope(t, envs[0], "Namespace", "production", "")
	assertEnvelope(t, envs[1], "App", "memos", "production")
}

func TestLoadSources_emptyDocumentsSkipped(t *testing.T) {
	// A blank and a comment-only document between two real ones.
	input := nsYAML + "---\n\n---\n# comment only\n---\n" + appYAML
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/manifests/mixed.yaml", []byte(input))

	envs, err := manifest.LoadSources([]string{"/manifests/mixed.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes (empty docs skipped), got %d", len(envs))
	}
}

// ── kind:List ─────────────────────────────────────────────────────────────────

func TestLoadSources_kindList_expanded(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/manifests/list.yaml", []byte(kindListYAML))

	envs, err := manifest.LoadSources([]string{"/manifests/list.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes from List, got %d", len(envs))
	}

	assertEnvelope(t, envs[0], "Namespace", "production", "")
	assertEnvelope(t, envs[1], "App", "memos", "production")
}

func TestLoadSources_kindList_itemRawIsUnmarshalable(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/list.yaml", []byte(kindListYAML))

	envs, err := manifest.LoadSources([]string{"/list.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	for i, env := range envs {
		var m map[string]any

		if err := yaml.Unmarshal(env.Raw, &m); err != nil {
			t.Errorf("item %d Raw not valid YAML: %v", i, err)
		}
	}
}

// ── JSON ──────────────────────────────────────────────────────────────────────

func TestLoadSources_jsonFile(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/manifests/app.json", []byte(appJSON))

	envs, err := manifest.LoadSources([]string{"/manifests/app.json"}, mem)
	if err != nil {
		t.Fatalf("LoadSources JSON: %v", err)
	}

	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(envs))
	}

	assertEnvelope(t, envs[0], "App", "memos", "production")
}

// ── directory ─────────────────────────────────────────────────────────────────

func TestLoadSources_directory_processesYAMLYMLAndJSON(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/dir/ns.yaml", []byte(nsYAML))
	_ = mem.WriteFile("/dir/app.yml", []byte(appYAML))
	_ = mem.WriteFile("/dir/secret.json", []byte(appJSON))
	_ = mem.WriteFile("/dir/readme.txt", []byte("not a manifest"))
	_ = mem.WriteFile("/dir/notes.md", []byte("also not a manifest"))

	envs, err := manifest.LoadSources([]string{"/dir"}, mem)
	if err != nil {
		t.Fatalf("LoadSources dir: %v", err)
	}

	if len(envs) != 3 {
		t.Fatalf("expected 3 envelopes (.yaml+.yml+.json), got %d", len(envs))
	}
}

func TestLoadSources_directory_skipsNonManifestExtensions(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/dir/data.csv", []byte("col1,col2"))
	_ = mem.WriteFile("/dir/script.sh", []byte("#!/bin/bash"))
	_ = mem.WriteFile("/dir/app.yaml", []byte(appYAML))

	envs, err := manifest.LoadSources([]string{"/dir"}, mem)
	if err != nil {
		t.Fatalf("LoadSources dir: %v", err)
	}

	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope (.yaml only), got %d", len(envs))
	}

	assertEnvelope(t, envs[0], "App", "memos", "production")
}

func TestLoadSources_directory_extensionCheckIsCaseInsensitive(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/dir/app.YAML", []byte(appYAML))
	_ = mem.WriteFile("/dir/ns.JSON", []byte(appJSON))

	envs, err := manifest.LoadSources([]string{"/dir"}, mem)
	if err != nil {
		t.Fatalf("LoadSources dir: %v", err)
	}

	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes (.YAML + .JSON), got %d", len(envs))
	}
}

func TestLoadSources_directory_nestedPath(t *testing.T) {
	mem := testutil.NewMemFS()
	dir := filepath.Join("/some", "deep", "dir")
	_ = mem.WriteFile(filepath.Join(dir, "ns.yaml"), []byte(nsYAML))

	envs, err := manifest.LoadSources([]string{dir}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(envs))
	}

	assertEnvelope(t, envs[0], "Namespace", "production", "")
}

// ── multiple sources ──────────────────────────────────────────────────────────

func TestLoadSources_multipleFiles(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/a/ns.yaml", []byte(nsYAML))
	_ = mem.WriteFile("/b/app.yaml", []byte(appYAML))

	envs, err := manifest.LoadSources([]string{"/a/ns.yaml", "/b/app.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes, got %d", len(envs))
	}

	assertEnvelope(t, envs[0], "Namespace", "production", "")
	assertEnvelope(t, envs[1], "App", "memos", "production")
}

func TestLoadSources_mixedFileAndDirectory(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/single/ns.yaml", []byte(nsYAML))
	_ = mem.WriteFile("/dir/app.yaml", []byte(appYAML))
	_ = mem.WriteFile("/dir/secret.yaml", []byte(secretYAML))

	envs, err := manifest.LoadSources([]string{"/single/ns.yaml", "/dir"}, mem)
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	if len(envs) != 3 {
		t.Fatalf("expected 3 envelopes (1 file + 2 dir), got %d", len(envs))
	}
}

// ── error cases ───────────────────────────────────────────────────────────────

func TestLoadSources_missingFile_error(t *testing.T) {
	mem := testutil.NewMemFS()

	_, err := manifest.LoadSources([]string{"/missing.yaml"}, mem)
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadSources_invalidYAML_error(t *testing.T) {
	mem := testutil.NewMemFS()
	_ = mem.WriteFile("/bad.yaml", []byte(":\t: invalid: [yaml"))

	_, err := manifest.LoadSources([]string{"/bad.yaml"}, mem)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// ── URL ───────────────────────────────────────────────────────────────────────

func TestLoadSources_URL_ok(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(nsYAML))
	}))
	defer srv.Close()

	mem := testutil.NewMemFS()

	envs, err := manifest.LoadSources([]string{srv.URL + "/ns.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources URL: %v", err)
	}

	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(envs))
	}

	assertEnvelope(t, envs[0], "Namespace", "production", "")
}

func TestLoadSources_URL_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	mem := testutil.NewMemFS()

	_, err := manifest.LoadSources([]string{srv.URL + "/missing.yaml"}, mem)
	if err == nil {
		t.Error("expected error for HTTP 404, got nil")
	}
}

func TestLoadSources_URL_multiDoc(t *testing.T) {
	multi := nsYAML + "---\n" + appYAML

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(multi))
	}))
	defer srv.Close()

	mem := testutil.NewMemFS()

	envs, err := manifest.LoadSources([]string{srv.URL + "/multi.yaml"}, mem)
	if err != nil {
		t.Fatalf("LoadSources URL multi-doc: %v", err)
	}

	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes from URL, got %d", len(envs))
	}
}
