package v1alpha1

import (
	"errors"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

func TestConfigMap_KindInfo(t *testing.T) {
	fs := testutil.NewMemFS()
	c := newConfigMap("/data", fs)

	info := c.KindInfo()
	if info.Kind != "ConfigMap" {
		t.Errorf("expected ConfigMap, got %q", info.Kind)
	}
}

func TestConfigMap_RunGet(t *testing.T) {
	fs := testutil.NewMemFS()
	c := newConfigMap("/data", fs)

	optsList := registry.OperationOpts{
		Namespace: "prod",
		Output:    "human",
		FS:        fs,
	}

	// 1. Get empty list
	restore := captureStdout(t)
	err := c.runGet(optsList)
	out := restore()

	if err != nil {
		t.Fatalf("runGet empty failed: %v", err)
	}

	if !strings.Contains(out, "No configmaps found") {
		t.Errorf("unexpected output: %q", out)
	}

	// 2. Get missing single configmap
	optsGetMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		Output:    "human",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = c.runGet(optsGetMissing)
	_ = restore()

	if err == nil {
		t.Fatal("expected error getting nonexistent configmap, got nil")
	}

	var exitErr *exitcode.Error

	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.NotFound {
		t.Errorf("expected exitcode.NotFound, got %v", err)
	}

	// 3. Setup namespace and configmaps
	_ = fs.MkdirAll("/data/namespaces/prod/configmaps/app-config")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\n"))
	_ = fs.WriteFile("/data/namespaces/prod/configmaps/app-config/.configmap.yaml", []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: prod
data:
  key1: val1
  key2: val2
`))

	// 4. Get list with results
	restore = captureStdout(t)
	err = c.runGet(optsList)
	out = restore()

	if err != nil {
		t.Fatalf("runGet list failed: %v", err)
	}

	if !strings.Contains(out, "app-config") || !strings.Contains(out, "key1, key2") {
		t.Errorf("unexpected output: %q", out)
	}

	// 5. Get single (human)
	optsGetOne := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		Output:    "human",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = c.runGet(optsGetOne)
	out = restore()

	if err != nil {
		t.Fatalf("runGet single human failed: %v", err)
	}

	if !strings.Contains(out, "app-config") {
		t.Errorf("unexpected output: %q", out)
	}

	// 6. Get single (yaml)
	optsGetOneYAML := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		Output:    "yaml",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = c.runGet(optsGetOneYAML)
	out = restore()

	if err != nil {
		t.Fatalf("runGet single yaml failed: %v", err)
	}

	if !strings.Contains(out, "key1: val1") {
		t.Errorf("unexpected output: %q", out)
	}

	// 7. Get All Namespaces (-A)
	_ = fs.MkdirAll("/data/namespaces/dev/configmaps/dev-config")
	_ = fs.WriteFile("/data/namespaces/dev/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: dev\n"))
	_ = fs.WriteFile("/data/namespaces/dev/configmaps/dev-config/.configmap.yaml", []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: dev-config
  namespace: dev
data:
  database: sqlite
`))

	optsAll := registry.OperationOpts{
		AllNamespaces: true,
		Output:        "human",
		FS:            fs,
	}
	restore = captureStdout(t)
	err = c.runGet(optsAll)
	out = restore()

	if err != nil {
		t.Fatalf("runGet AllNamespaces failed: %v", err)
	}

	if !strings.Contains(out, "dev-config") || !strings.Contains(out, "app-config") {
		t.Errorf("unexpected output: %q", out)
	}

	// 8. Skip bad configmap in list
	_ = fs.MkdirAll("/data/namespaces/dev/configmaps/bad-config")
	_ = fs.WriteFile("/data/namespaces/dev/configmaps/bad-config/.configmap.yaml", []byte("invalid yaml{"))

	restore = captureStdout(t)
	err = c.runGet(optsAll)
	out = restore()

	if err != nil {
		t.Fatalf("runGet with malformed configmap failed: %v", err)
	}

	if !strings.Contains(out, "skipping configmap \"bad-config\"") {
		t.Errorf("expected warning snippet about bad-config, got: %q", out)
	}

	// 9. All namespaces empty
	fsEmpty := testutil.NewMemFS()
	cEmpty := newConfigMap("/data", fsEmpty)
	optsAllEmpty := registry.OperationOpts{
		AllNamespaces: true,
		Output:        "human",
		FS:            fsEmpty,
	}
	restore = captureStdout(t)
	err = cEmpty.runGet(optsAllEmpty)
	out = restore()

	if err != nil {
		t.Fatalf("runGet empty AllNamespaces failed: %v", err)
	}

	if !strings.Contains(out, "No configmaps found") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestConfigMap_RunApply(t *testing.T) {
	fs := testutil.NewMemFS()
	c := newConfigMap("/data", fs)

	// 1. Missing file flag
	optsNoFile := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		FS:        fs,
	}
	restore := captureStdout(t)
	err := c.runApply(optsNoFile)
	out := restore()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(out, "--file (-f) is required") {
		t.Errorf("unexpected output: %q", out)
	}

	// 2. Read input file error
	optsBadFile := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		FS:        fs,
		Flags: map[string]any{
			"file": "nonexistent.yaml",
		},
	}

	err = c.runApply(optsBadFile)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}

	// 3. Parse manifest error (invalid yaml)
	_ = fs.WriteFile("bad.yaml", []byte("invalid yaml {"))
	optsBadYAML := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		FS:        fs,
		Flags: map[string]any{
			"file": "bad.yaml",
		},
	}

	err = c.runApply(optsBadYAML)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}

	// 4. Validation errors (using RawManifest to simplify)
	// 4a. Wrong API Version
	optsWrongAPI := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "app-config",
		FS:          fs,
		RawManifest: []byte("apiVersion: wrong/v1\nkind: ConfigMap\nmetadata:\n  name: app-config\n  namespace: prod\n"),
	}

	err = c.runApply(optsWrongAPI)
	if err == nil {
		t.Fatal("expected invalid apiVersion error, got nil")
	}

	// 4b. Wrong Kind
	optsWrongKind := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "app-config",
		FS:          fs,
		RawManifest: []byte("apiVersion: v1\nkind: WrongKind\nmetadata:\n  name: app-config\n  namespace: prod\n"),
	}

	err = c.runApply(optsWrongKind)
	if err == nil {
		t.Fatal("expected invalid kind error, got nil")
	}

	// 4c. Name mismatch
	optsWrongName := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "app-config",
		FS:          fs,
		RawManifest: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: other-name\n  namespace: prod\n"),
	}

	err = c.runApply(optsWrongName)
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}

	// 4d. Namespace mismatch
	optsWrongNS := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "app-config",
		FS:          fs,
		RawManifest: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app-config\n  namespace: other-ns\n"),
	}

	err = c.runApply(optsWrongNS)
	if err == nil {
		t.Fatal("expected namespace mismatch error, got nil")
	}

	// 5. Success create (dry-run)
	validYAML := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app-config\n  namespace: prod\n")
	optsDryCreate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "app-config",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}
	restore = captureStdout(t)
	err = c.runApply(optsDryCreate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply failed: %v", err)
	}

	if !strings.Contains(out, "Would create configmap \"app-config\" in namespace \"prod\"") {
		t.Errorf("unexpected dry-run output: %q", out)
	}

	// 6. Success create
	optsRealCreate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "app-config",
		FS:          fs,
		RawManifest: validYAML,
	}
	restore = captureStdout(t)
	err = c.runApply(optsRealCreate)
	out = restore()

	if err != nil {
		t.Fatalf("real apply create failed: %v", err)
	}

	if !strings.Contains(out, "Created configmap \"app-config\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Verify manifest exists
	exists, _ := fs.Exists("/data/namespaces/prod/configmaps/app-config/.configmap.yaml")
	if !exists {
		t.Error("manifest file was not created")
	}

	// 7. Success update (dry-run)
	optsDryUpdate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "app-config",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}
	restore = captureStdout(t)
	err = c.runApply(optsDryUpdate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply update failed: %v", err)
	}

	if !strings.Contains(out, "Would update configmap \"app-config\" in namespace \"prod\"") {
		t.Errorf("unexpected dry-run output: %q", out)
	}

	// 8. Success update
	restore = captureStdout(t)
	err = c.runApply(optsRealCreate)
	out = restore()

	if err != nil {
		t.Fatalf("real apply update failed: %v", err)
	}

	if !strings.Contains(out, "Updated configmap \"app-config\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestConfigMap_RunDelete(t *testing.T) {
	fs := testutil.NewMemFS()
	c := newConfigMap("/data", fs)

	// 1. Delete non-existent
	optsDelMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		FS:        fs,
	}
	restore := captureStdout(t)
	err := c.runDelete(optsDelMissing)
	out := restore()

	if err == nil {
		t.Fatal("expected error deleting nonexistent, got nil")
	}

	if !strings.Contains(out, "configmap \"nonexistent\" not found in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Setup configmap
	_ = fs.MkdirAll("/data/namespaces/prod/configmaps/app-config")
	_ = fs.WriteFile("/data/namespaces/prod/configmaps/app-config/.configmap.yaml", []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app-config\n  namespace: prod\n"))

	// 2. Delete non-TTY abort (Yes = false)
	optsDelNoTTY := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		Yes:       false,
		FS:        fs,
	}

	err = c.runDelete(optsDelNoTTY)
	if err == nil {
		t.Fatal("expected non-TTY confirm error, got nil")
	}

	// 3. Delete dry-run
	optsDelDry := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		DryRun:    true,
		FS:        fs,
	}
	restore = captureStdout(t)
	err = c.runDelete(optsDelDry)
	out = restore()

	if err != nil {
		t.Fatalf("delete dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would delete configmap \"app-config\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// 4. Delete success
	optsDelReal := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		Yes:       true,
		FS:        fs,
	}
	restore = captureStdout(t)
	err = c.runDelete(optsDelReal)
	out = restore()

	if err != nil {
		t.Fatalf("delete real failed: %v", err)
	}

	if !strings.Contains(out, "Deleted configmap \"app-config\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Verify directory deleted
	exists, _ := fs.Exists("/data/namespaces/prod/configmaps/app-config")
	if exists {
		t.Error("configmap directory was not deleted")
	}
}

func TestConfigMap_RunDoctor(t *testing.T) {
	fs := testutil.NewMemFS()
	c := newConfigMap("/data", fs)

	// 1. Single doctor missing
	optsDocMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		FS:        fs,
	}

	err := c.runDoctor(optsDocMissing)
	if err == nil {
		t.Fatal("expected doctor nonexistent error, got nil")
	}

	// 2. Doctor warnings and validation checks
	// Create a namespace
	_ = fs.MkdirAll("/data/namespaces/prod/configmaps/app-config")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\n"))

	// Empty configmap keys warning
	_ = fs.WriteFile("/data/namespaces/prod/configmaps/app-config/.configmap.yaml", []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app-config\n  namespace: prod\n"))

	optsDocSingle := registry.OperationOpts{
		Namespace: "prod",
		Name:      "app-config",
		FS:        fs,
	}
	restore := captureStdout(t)
	err = c.runDoctor(optsDocSingle)
	out := restore()

	if err != nil {
		t.Fatalf("doctor single warning failed: %v", err)
	}

	if !strings.Contains(out, "configmap has no data keys") {
		t.Errorf("expected empty warning, got: %q", out)
	}

	// 3. Unreadable manifest error
	_ = fs.RemoveAll("/data/namespaces/prod/configmaps/app-config/.configmap.yaml")
	// Make it a directory to make ReadBytes fail
	_ = fs.MkdirAll("/data/namespaces/prod/configmaps/app-config/.configmap.yaml")

	restore = captureStdout(t)
	err = c.runDoctor(optsDocSingle)
	out = restore()

	if err == nil {
		t.Fatal("expected doctor error for unreadable manifest, got nil")
	}

	if !strings.Contains(out, "cannot read manifest") {
		t.Errorf("expected unreadable error, got: %q", out)
	}

	// Clean it up
	_ = fs.RemoveAll("/data/namespaces/prod/configmaps/app-config/.configmap.yaml")

	// 4. Invalid YAML
	_ = fs.WriteFile("/data/namespaces/prod/configmaps/app-config/.configmap.yaml", []byte("invalid yaml{["))
	restore = captureStdout(t)
	err = c.runDoctor(optsDocSingle)
	out = restore()

	if err == nil {
		t.Fatal("expected doctor error for invalid YAML, got nil")
	}

	if !strings.Contains(out, "manifest YAML is invalid") {
		t.Errorf("expected parse error, got: %q", out)
	}

	// 5. Doctor list for namespace
	_ = fs.WriteFile("/data/namespaces/prod/configmaps/app-config/.configmap.yaml", []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app-config\n  namespace: prod\ndata:\n  key: value\n"))
	optsDocNS := registry.OperationOpts{
		Namespace: "prod",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = c.runDoctor(optsDocNS)
	out = restore()

	if err != nil {
		t.Fatalf("doctor namespace list failed: %v", err)
	}

	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected no issues, got: %q", out)
	}

	// 6. Doctor all namespaces
	optsDocAll := registry.OperationOpts{
		AllNamespaces: true,
		FS:            fs,
	}
	restore = captureStdout(t)
	err = c.runDoctor(optsDocAll)
	out = restore()

	if err != nil {
		t.Fatalf("doctor all namespaces failed: %v", err)
	}

	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected no issues, got: %q", out)
	}
}
