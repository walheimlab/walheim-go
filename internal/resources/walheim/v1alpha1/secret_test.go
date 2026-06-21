package v1alpha1

import (
	"errors"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

func TestSecret_KindInfo(t *testing.T) {
	fs := testutil.NewMemFS()
	s := newSecret("/data", fs)

	info := s.KindInfo()
	if info.Kind != "Secret" {
		t.Errorf("expected Secret, got %q", info.Kind)
	}
}

func TestSecret_RunGet(t *testing.T) {
	fs := testutil.NewMemFS()
	s := newSecret("/data", fs)

	optsList := registry.OperationOpts{
		Namespace: "prod",
		Output:    "human",
		FS:        fs,
	}

	// 1. Get empty list
	restore := captureStdout(t)
	err := s.runGet(optsList)
	out := restore()

	if err != nil {
		t.Fatalf("runGet empty failed: %v", err)
	}

	if !strings.Contains(out, "No secrets found") {
		t.Errorf("unexpected output: %q", out)
	}

	// 2. Get missing single secret
	optsGetMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		Output:    "human",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = s.runGet(optsGetMissing)
	_ = restore()

	if err == nil {
		t.Fatal("expected error getting nonexistent secret, got nil")
	}

	var exitErr *exitcode.Error

	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.NotFound {
		t.Errorf("expected exitcode.NotFound, got %v", err)
	}

	// 3. Setup namespace and secrets
	_ = fs.MkdirAll("/data/namespaces/prod/secrets/db-creds")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\n"))
	_ = fs.WriteFile("/data/namespaces/prod/secrets/db-creds/.secret.yaml", []byte(`
apiVersion: v1
kind: Secret
metadata:
  name: db-creds
  namespace: prod
data:
  password: cGFzc3dvcmQxMjM=
stringData:
  username: postgres
`))

	// 4. Get list with results
	restore = captureStdout(t)
	err = s.runGet(optsList)
	out = restore()

	if err != nil {
		t.Fatalf("runGet list failed: %v", err)
	}

	if !strings.Contains(out, "db-creds") || !strings.Contains(out, "password, username") {
		t.Errorf("unexpected output: %q", out)
	}

	// 5. Get single (human)
	optsGetOne := registry.OperationOpts{
		Namespace: "prod",
		Name:      "db-creds",
		Output:    "human",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = s.runGet(optsGetOne)
	out = restore()

	if err != nil {
		t.Fatalf("runGet single human failed: %v", err)
	}

	if !strings.Contains(out, "db-creds") {
		t.Errorf("unexpected output: %q", out)
	}

	// 6. Get single (yaml)
	optsGetOneYAML := registry.OperationOpts{
		Namespace: "prod",
		Name:      "db-creds",
		Output:    "yaml",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = s.runGet(optsGetOneYAML)
	out = restore()

	if err != nil {
		t.Fatalf("runGet single yaml failed: %v", err)
	}

	if !strings.Contains(out, "password: cGFzc3dvcmQxMjM=") {
		t.Errorf("unexpected output: %q", out)
	}

	// 7. Get All Namespaces (-A)
	_ = fs.MkdirAll("/data/namespaces/dev/secrets/dev-creds")
	_ = fs.WriteFile("/data/namespaces/dev/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: dev\n"))
	_ = fs.WriteFile("/data/namespaces/dev/secrets/dev-creds/.secret.yaml", []byte(`
apiVersion: v1
kind: Secret
metadata:
  name: dev-creds
  namespace: dev
data:
  token: c2VjcmV0dG9rZW4=
`))

	optsAll := registry.OperationOpts{
		AllNamespaces: true,
		Output:        "human",
		FS:            fs,
	}
	restore = captureStdout(t)
	err = s.runGet(optsAll)
	out = restore()

	if err != nil {
		t.Fatalf("runGet AllNamespaces failed: %v", err)
	}

	if !strings.Contains(out, "dev-creds") || !strings.Contains(out, "db-creds") {
		t.Errorf("unexpected output: %q", out)
	}

	// 8. Skip bad secret in list
	_ = fs.MkdirAll("/data/namespaces/dev/secrets/bad-secret")
	_ = fs.WriteFile("/data/namespaces/dev/secrets/bad-secret/.secret.yaml", []byte("invalid yaml{"))

	restore = captureStdout(t)
	err = s.runGet(optsAll)
	out = restore()

	if err != nil {
		t.Fatalf("runGet with malformed secret failed: %v", err)
	}

	if !strings.Contains(out, "skipping secret \"bad-secret\"") {
		t.Errorf("expected warning snippet about bad-secret, got: %q", out)
	}

	// 9. All namespaces empty
	fsEmpty := testutil.NewMemFS()
	sEmpty := newSecret("/data", fsEmpty)
	optsAllEmpty := registry.OperationOpts{
		AllNamespaces: true,
		Output:        "human",
		FS:            fsEmpty,
	}
	restore = captureStdout(t)
	err = sEmpty.runGet(optsAllEmpty)
	out = restore()

	if err != nil {
		t.Fatalf("runGet empty AllNamespaces failed: %v", err)
	}

	if !strings.Contains(out, "No secrets found") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestSecret_RunApply(t *testing.T) {
	fs := testutil.NewMemFS()
	s := newSecret("/data", fs)

	// 1. Missing file flag
	optsNoFile := registry.OperationOpts{
		Namespace: "prod",
		Name:      "db-creds",
		FS:        fs,
	}
	restore := captureStdout(t)
	err := s.runApply(optsNoFile)
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
		Name:      "db-creds",
		FS:        fs,
		Flags: map[string]any{
			"file": "nonexistent.yaml",
		},
	}

	err = s.runApply(optsBadFile)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}

	// 3. Parse manifest error (invalid yaml)
	_ = fs.WriteFile("bad.yaml", []byte("invalid yaml {"))
	optsBadYAML := registry.OperationOpts{
		Namespace: "prod",
		Name:      "db-creds",
		FS:        fs,
		Flags: map[string]any{
			"file": "bad.yaml",
		},
	}

	err = s.runApply(optsBadYAML)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}

	// 4. Validation errors
	// 4a. Wrong API Version
	optsWrongAPI := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "db-creds",
		FS:          fs,
		RawManifest: []byte("apiVersion: wrong/v1\nkind: Secret\nmetadata:\n  name: db-creds\n  namespace: prod\n"),
	}

	err = s.runApply(optsWrongAPI)
	if err == nil {
		t.Fatal("expected invalid apiVersion error, got nil")
	}

	// 4b. Wrong Kind
	optsWrongKind := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "db-creds",
		FS:          fs,
		RawManifest: []byte("apiVersion: v1\nkind: WrongKind\nmetadata:\n  name: db-creds\n  namespace: prod\n"),
	}

	err = s.runApply(optsWrongKind)
	if err == nil {
		t.Fatal("expected invalid kind error, got nil")
	}

	// 4c. Name mismatch
	optsWrongName := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "db-creds",
		FS:          fs,
		RawManifest: []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: other-name\n  namespace: prod\n"),
	}

	err = s.runApply(optsWrongName)
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}

	// 4d. Namespace mismatch
	optsWrongNS := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "db-creds",
		FS:          fs,
		RawManifest: []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db-creds\n  namespace: other-ns\n"),
	}

	err = s.runApply(optsWrongNS)
	if err == nil {
		t.Fatal("expected namespace mismatch error, got nil")
	}

	// 4e. Invalid base64 in data
	optsWrongBase64 := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "db-creds",
		FS:          fs,
		RawManifest: []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db-creds\n  namespace: prod\ndata:\n  key: notbase64!!!\n"),
	}

	err = s.runApply(optsWrongBase64)
	if err == nil {
		t.Fatal("expected base64 validation error, got nil")
	}

	// 5. Success create (dry-run)
	validYAML := []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db-creds\n  namespace: prod\ndata:\n  key: dGVzdA==\n")
	optsDryCreate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "db-creds",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}
	restore = captureStdout(t)
	err = s.runApply(optsDryCreate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply failed: %v", err)
	}

	if !strings.Contains(out, "Would create secret \"db-creds\" in namespace \"prod\"") {
		t.Errorf("unexpected dry-run output: %q", out)
	}

	// 6. Success create
	optsRealCreate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "db-creds",
		FS:          fs,
		RawManifest: validYAML,
	}
	restore = captureStdout(t)
	err = s.runApply(optsRealCreate)
	out = restore()

	if err != nil {
		t.Fatalf("real apply create failed: %v", err)
	}

	if !strings.Contains(out, "Created secret \"db-creds\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Verify manifest exists
	exists, _ := fs.Exists("/data/namespaces/prod/secrets/db-creds/.secret.yaml")
	if !exists {
		t.Error("manifest file was not created")
	}

	// 7. Success update (dry-run)
	optsDryUpdate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "db-creds",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}
	restore = captureStdout(t)
	err = s.runApply(optsDryUpdate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply update failed: %v", err)
	}

	if !strings.Contains(out, "Would update secret \"db-creds\" in namespace \"prod\"") {
		t.Errorf("unexpected dry-run output: %q", out)
	}

	// 8. Success update
	restore = captureStdout(t)
	err = s.runApply(optsRealCreate)
	out = restore()

	if err != nil {
		t.Fatalf("real apply update failed: %v", err)
	}

	if !strings.Contains(out, "Updated secret \"db-creds\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestSecret_RunDelete(t *testing.T) {
	fs := testutil.NewMemFS()
	s := newSecret("/data", fs)

	// 1. Delete non-existent
	optsDelMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		FS:        fs,
	}
	restore := captureStdout(t)
	err := s.runDelete(optsDelMissing)
	out := restore()

	if err == nil {
		t.Fatal("expected error deleting nonexistent, got nil")
	}

	if !strings.Contains(out, "secret \"nonexistent\" not found in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Setup secret
	_ = fs.MkdirAll("/data/namespaces/prod/secrets/db-creds")
	_ = fs.WriteFile("/data/namespaces/prod/secrets/db-creds/.secret.yaml", []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db-creds\n  namespace: prod\n"))

	// 2. Delete non-TTY abort (Yes = false)
	optsDelNoTTY := registry.OperationOpts{
		Namespace: "prod",
		Name:      "db-creds",
		Yes:       false,
		FS:        fs,
	}

	err = s.runDelete(optsDelNoTTY)
	if err == nil {
		t.Fatal("expected non-TTY confirm error, got nil")
	}

	// 3. Delete dry-run
	optsDelDry := registry.OperationOpts{
		Namespace: "prod",
		Name:      "db-creds",
		DryRun:    true,
		FS:        fs,
	}
	restore = captureStdout(t)
	err = s.runDelete(optsDelDry)
	out = restore()

	if err != nil {
		t.Fatalf("delete dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would delete secret \"db-creds\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// 4. Delete success
	optsDelReal := registry.OperationOpts{
		Namespace: "prod",
		Name:      "db-creds",
		Yes:       true,
		FS:        fs,
	}
	restore = captureStdout(t)
	err = s.runDelete(optsDelReal)
	out = restore()

	if err != nil {
		t.Fatalf("delete real failed: %v", err)
	}

	if !strings.Contains(out, "Deleted secret \"db-creds\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Verify directory deleted
	exists, _ := fs.Exists("/data/namespaces/prod/secrets/db-creds")
	if exists {
		t.Error("secret directory was not deleted")
	}
}

func TestSecret_RunDoctor(t *testing.T) {
	fs := testutil.NewMemFS()
	s := newSecret("/data", fs)

	// 1. Single doctor missing
	optsDocMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		FS:        fs,
	}

	err := s.runDoctor(optsDocMissing)
	if err == nil {
		t.Fatal("expected doctor nonexistent error, got nil")
	}

	// 2. Doctor warnings and validation checks
	// Create a namespace
	_ = fs.MkdirAll("/data/namespaces/prod/secrets/db-creds")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\n"))

	// Empty secret warning
	_ = fs.WriteFile("/data/namespaces/prod/secrets/db-creds/.secret.yaml", []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db-creds\n  namespace: prod\n"))

	optsDocSingle := registry.OperationOpts{
		Namespace: "prod",
		Name:      "db-creds",
		FS:        fs,
	}
	restore := captureStdout(t)
	err = s.runDoctor(optsDocSingle)
	out := restore()

	if err != nil {
		t.Fatalf("doctor single warning failed: %v", err)
	}

	if !strings.Contains(out, "secret has no data or stringData keys") {
		t.Errorf("expected empty warning, got: %q", out)
	}

	// Invalid base64 in data
	_ = fs.WriteFile("/data/namespaces/prod/secrets/db-creds/.secret.yaml", []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db-creds\n  namespace: prod\ndata:\n  key: invalidbase64!!!\n"))
	restore = captureStdout(t)
	err = s.runDoctor(optsDocSingle)
	out = restore()

	if err == nil {
		t.Fatal("expected doctor error for invalid base64, got nil")
	}

	if !strings.Contains(out, "is not valid base64") {
		t.Errorf("expected invalid base64 warning, got: %q", out)
	}

	// 3. Unreadable manifest error
	_ = fs.RemoveAll("/data/namespaces/prod/secrets/db-creds/.secret.yaml")
	// Make it a directory to make ReadBytes fail
	_ = fs.MkdirAll("/data/namespaces/prod/secrets/db-creds/.secret.yaml")

	restore = captureStdout(t)
	err = s.runDoctor(optsDocSingle)
	out = restore()

	if err == nil {
		t.Fatal("expected doctor error for unreadable manifest, got nil")
	}

	if !strings.Contains(out, "cannot read manifest") {
		t.Errorf("expected unreadable error, got: %q", out)
	}

	// Clean it up
	_ = fs.RemoveAll("/data/namespaces/prod/secrets/db-creds/.secret.yaml")

	// 4. Invalid YAML
	_ = fs.WriteFile("/data/namespaces/prod/secrets/db-creds/.secret.yaml", []byte("invalid yaml{["))
	restore = captureStdout(t)
	err = s.runDoctor(optsDocSingle)
	out = restore()

	if err == nil {
		t.Fatal("expected doctor error for invalid YAML, got nil")
	}

	if !strings.Contains(out, "manifest YAML is invalid") {
		t.Errorf("expected parse error, got: %q", out)
	}

	// 5. Doctor list for namespace
	_ = fs.WriteFile("/data/namespaces/prod/secrets/db-creds/.secret.yaml", []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db-creds\n  namespace: prod\nstringData:\n  key: value\n"))
	optsDocNS := registry.OperationOpts{
		Namespace: "prod",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = s.runDoctor(optsDocNS)
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
	err = s.runDoctor(optsDocAll)
	out = restore()

	if err != nil {
		t.Fatalf("doctor all namespaces failed: %v", err)
	}

	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected no issues, got: %q", out)
	}
}
