package v1alpha1

import (
	"errors"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

func TestApp_KindInfo(t *testing.T) {
	fs := testutil.NewMemFS()
	a := newApp("/data", fs)

	info := a.KindInfo()
	if info.Kind != "App" {
		t.Errorf("expected App, got %q", info.Kind)
	}
}

func TestApp_RunGet(t *testing.T) {
	fs := testutil.NewMemFS()
	a := newApp("/data", fs)

	optsList := registry.OperationOpts{
		Namespace: "prod",
		Output:    "human",
		FS:        fs,
	}

	// 1. Get empty list
	restore := captureStdout(t)
	err := a.runGet(optsList)
	out := restore()

	if err != nil {
		t.Fatalf("runGet empty failed: %v", err)
	}

	if !strings.Contains(out, "No apps found") {
		t.Errorf("unexpected output: %q", out)
	}

	// 2. Get missing single app
	optsGetMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		Output:    "human",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = a.runGet(optsGetMissing)
	_ = restore()

	if err == nil {
		t.Fatal("expected error getting nonexistent app, got nil")
	}

	var exitErr *exitcode.Error

	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.NotFound {
		t.Errorf("expected exitcode.NotFound, got %v", err)
	}

	// 3. Setup namespace and apps
	_ = fs.MkdirAll("/data/namespaces/prod/apps/myapp")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))
	_ = fs.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", []byte(`
apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: myapp
  namespace: prod
spec:
  compose:
    services:
      web:
        image: nginx:latest
`))

	// 4. Get list with results
	restore = captureStdout(t)
	err = a.runGet(optsList)
	out = restore()

	if err != nil {
		t.Fatalf("runGet list failed: %v", err)
	}

	if !strings.Contains(out, "myapp") || !strings.Contains(out, "nginx:latest") {
		t.Errorf("unexpected output: %q", out)
	}

	// 5. Get single (human)
	optsGetOne := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myapp",
		Output:    "human",
		FS:        fs,
	}
	restore = captureStdout(t)
	err = a.runGet(optsGetOne)
	out = restore()

	if err != nil {
		t.Fatalf("runGet single human failed: %v", err)
	}

	if !strings.Contains(out, "myapp") {
		t.Errorf("unexpected output: %q", out)
	}

	// 6. Get All Namespaces (-A)
	_ = fs.MkdirAll("/data/namespaces/dev/apps/devapp")
	_ = fs.WriteFile("/data/namespaces/dev/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: dev\nspec:\n  hostname: 127.0.0.1\n"))
	_ = fs.WriteFile("/data/namespaces/dev/apps/devapp/.app.yaml", []byte(`
apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: devapp
  namespace: dev
spec:
  compose:
    services:
      db:
        image: mysql:latest
`))

	optsAll := registry.OperationOpts{
		AllNamespaces: true,
		Output:        "human",
		FS:            fs,
	}
	restore = captureStdout(t)
	err = a.runGet(optsAll)
	out = restore()

	if err != nil {
		t.Fatalf("runGet AllNamespaces failed: %v", err)
	}

	if !strings.Contains(out, "devapp") || !strings.Contains(out, "myapp") {
		t.Errorf("unexpected output: %q", out)
	}

	// 7. Skip bad app in list
	_ = fs.MkdirAll("/data/namespaces/dev/apps/badapp")
	_ = fs.WriteFile("/data/namespaces/dev/apps/badapp/.app.yaml", []byte("invalid yaml{"))

	restore = captureStdout(t)
	err = a.runGet(optsAll)
	out = restore()

	if err != nil {
		t.Fatalf("runGet with malformed app failed: %v", err)
	}

	if !strings.Contains(out, "skipping app \"badapp\"") {
		t.Errorf("expected warning snippet about badapp, got: %q", out)
	}

	// 8. All namespaces empty
	fsEmpty := testutil.NewMemFS()
	aEmpty := newApp("/data", fsEmpty)
	optsAllEmpty := registry.OperationOpts{
		AllNamespaces: true,
		Output:        "human",
		FS:            fsEmpty,
	}
	restore = captureStdout(t)
	err = aEmpty.runGet(optsAllEmpty)
	out = restore()

	if err != nil {
		t.Fatalf("runGet empty AllNamespaces failed: %v", err)
	}

	if !strings.Contains(out, "No apps found") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestApp_RunApply(t *testing.T) {
	fs := testutil.NewMemFS()
	a := newApp("/data", fs)

	// 1. Missing file flag
	optsNoFile := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myapp",
		FS:        fs,
	}
	restore := captureStdout(t)
	err := a.runApply(optsNoFile)
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
		Name:      "myapp",
		FS:        fs,
		Flags: map[string]any{
			"file": "nonexistent.yaml",
		},
	}

	err = a.runApply(optsBadFile)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}

	// 3. Parse manifest error (invalid yaml)
	_ = fs.WriteFile("bad.yaml", []byte("invalid yaml {"))
	optsBadYAML := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myapp",
		FS:        fs,
		Flags: map[string]any{
			"file": "bad.yaml",
		},
	}

	err = a.runApply(optsBadYAML)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}

	// 4. Validation errors
	// 4a. Wrong API Version
	optsWrongAPI := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myapp",
		FS:          fs,
		RawManifest: []byte("apiVersion: wrong/v1\nkind: App\nmetadata:\n  name: myapp\n  namespace: prod\n"),
	}

	err = a.runApply(optsWrongAPI)
	if err == nil {
		t.Fatal("expected invalid apiVersion error, got nil")
	}

	// 4b. Wrong Kind
	optsWrongKind := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myapp",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: WrongKind\nmetadata:\n  name: myapp\n  namespace: prod\n"),
	}

	err = a.runApply(optsWrongKind)
	if err == nil {
		t.Fatal("expected invalid kind error, got nil")
	}

	// 4c. Name mismatch
	optsWrongName := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myapp",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: App\nmetadata:\n  name: other-name\n  namespace: prod\n"),
	}

	err = a.runApply(optsWrongName)
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}

	// 4d. Namespace mismatch
	optsWrongNS := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myapp",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: App\nmetadata:\n  name: myapp\n  namespace: other-ns\n"),
	}

	err = a.runApply(optsWrongNS)
	if err == nil {
		t.Fatal("expected namespace mismatch error, got nil")
	}

	// 4e. Empty services
	optsEmptyServices := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myapp",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: App\nmetadata:\n  name: myapp\n  namespace: prod\nspec:\n  compose:\n    services: {}\n"),
	}

	err = a.runApply(optsEmptyServices)
	if err == nil {
		t.Fatal("expected empty services validation error, got nil")
	}

	// Setup namespace manifest
	_ = fs.MkdirAll("/data/namespaces/prod")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))

	// 5. Success create dry-run
	validYAML := []byte("apiVersion: walheim/v1alpha1\nkind: App\nmetadata:\n  name: myapp\n  namespace: prod\nspec:\n  compose:\n    services:\n      web:\n        image: nginx\n")
	optsDryCreate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myapp",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}
	restore = captureStdout(t)
	err = a.runApply(optsDryCreate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply create failed: %v", err)
	}

	if !strings.Contains(out, "Would create app \"myapp\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// 6. Success create and update (real write, but dry-run deployment start)
	// We first write it directly to the filesystem to simulate it already exists for update check.
	// Real write to filesystem
	_ = fs.MkdirAll("/data/namespaces/prod/apps/myapp")
	_ = fs.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", validYAML)

	optsDryUpdate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myapp",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}
	restore = captureStdout(t)
	err = a.runApply(optsDryUpdate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply update failed: %v", err)
	}

	if !strings.Contains(out, "Would update app \"myapp\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestApp_RunDelete(t *testing.T) {
	fs := testutil.NewMemFS()
	a := newApp("/data", fs)

	// 1. Delete nonexistent
	optsDelMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		FS:        fs,
	}
	restore := captureStdout(t)
	err := a.runDelete(optsDelMissing)
	out := restore()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(out, "app \"nonexistent\" not found in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Setup app and namespace
	_ = fs.MkdirAll("/data/namespaces/prod/apps/myapp")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))
	_ = fs.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: App\nmetadata:\n  name: myapp\n  namespace: prod\nspec:\n  compose:\n    services:\n      web:\n        image: nginx\n"))

	// 2. Delete non-TTY abort
	optsDelNoTTY := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myapp",
		Yes:       false,
		FS:        fs,
	}

	err = a.runDelete(optsDelNoTTY)
	if err == nil {
		t.Fatal("expected confirm abort error, got nil")
	}

	// 3. Delete dry-run
	optsDelDry := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myapp",
		DryRun:    true,
		FS:        fs,
	}
	restore = captureStdout(t)
	err = a.runDelete(optsDelDry)
	out = restore()

	if err != nil {
		t.Fatalf("delete dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would stop and delete app \"myapp\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestApp_RunDoctor(t *testing.T) {
	fs := testutil.NewMemFS()
	a := newApp("/data", fs)

	// 1. Single doctor missing
	optsDocMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		FS:        fs,
	}

	err := a.runDoctor(optsDocMissing)
	if err == nil {
		t.Fatal("expected doctor nonexistent error, got nil")
	}

	// 2. Setup namespace and app
	_ = fs.MkdirAll("/data/namespaces/prod/apps/myapp")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))

	// Create app manifest with warnings/errors
	// (No services error, missing secretRef/configMapRef warnings)
	_ = fs.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", []byte(`
apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: myapp
  namespace: prod
spec:
  envFrom:
    - secretRef:
        name: missing-secret
    - configMapRef:
        name: missing-cm
  compose:
    services: {}
`))

	optsDocSingle := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myapp",
		FS:        fs,
	}
	restore := captureStdout(t)
	err = a.runDoctor(optsDocSingle)
	out := restore()

	if err == nil {
		t.Fatal("expected doctor errors, got nil")
	}

	if !strings.Contains(out, "must define at least one service") {
		t.Errorf("expected services error, got: %q", out)
	}

	if !strings.Contains(out, "secretRef \"missing-secret\" does not exist") {
		t.Errorf("expected secret warning, got: %q", out)
	}

	// 3. Fix errors and run again
	_ = fs.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", []byte(`
apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: myapp
  namespace: prod
spec:
  compose:
    services:
      web:
        image: nginx
`))
	restore = captureStdout(t)
	err = a.runDoctor(optsDocSingle)
	out = restore()

	if err != nil {
		t.Fatalf("doctor single failed: %v", err)
	}

	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected no issues, got: %q", out)
	}

	// 4. List / All namespaces doctor
	optsDocAll := registry.OperationOpts{
		AllNamespaces: true,
		FS:            fs,
	}
	restore = captureStdout(t)
	err = a.runDoctor(optsDocAll)
	out = restore()

	if err != nil {
		t.Fatalf("doctor all failed: %v", err)
	}

	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected no issues, got: %q", out)
	}
}

func TestApp_RunPauseAndPull(t *testing.T) {
	fs := testutil.NewMemFS()
	a := newApp("/data", fs)

	// Setup namespace and app
	_ = fs.MkdirAll("/data/namespaces/prod/apps/myapp")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))
	_ = fs.WriteFile("/data/namespaces/prod/apps/myapp/.app.yaml", []byte(`
apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: myapp
  namespace: prod
spec:
  compose:
    services:
      web:
        image: nginx
`))

	optsDryPause := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myapp",
		DryRun:    true,
		FS:        fs,
	}
	restore := captureStdout(t)
	err := a.runPause(optsDryPause)
	out := restore()

	if err != nil {
		t.Fatalf("pause dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would run docker compose down for app \"myapp\"") {
		t.Errorf("unexpected output: %q", out)
	}

	optsDryPull := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myapp",
		DryRun:    true,
		FS:        fs,
	}
	restore = captureStdout(t)
	err = a.runPull(optsDryPull)
	out = restore()

	if err != nil {
		t.Fatalf("pull dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would run docker compose pull for app \"myapp\"") {
		t.Errorf("unexpected output: %q", out)
	}
}
