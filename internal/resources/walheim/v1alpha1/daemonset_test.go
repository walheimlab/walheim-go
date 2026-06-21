package v1alpha1

import (
	"errors"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

func TestDaemonSet_KindInfo(t *testing.T) {
	fs := testutil.NewMemFS()
	d := newDaemonSet("/data", fs)

	info := d.KindInfo()
	if info.Kind != "DaemonSet" {
		t.Errorf("expected DaemonSet, got %q", info.Kind)
	}
}

func TestDaemonSet_RunGet(t *testing.T) {
	fs := testutil.NewMemFS()
	d := newDaemonSet("/data", fs)

	optsList := registry.OperationOpts{
		Output: "human",
		FS:     fs,
	}

	restore := captureStdout(t)
	err := d.runGet(optsList)
	out := restore()

	if err != nil {
		t.Fatalf("runGet empty failed: %v", err)
	}

	if !strings.Contains(out, "No daemonsets found") {
		t.Errorf("unexpected output: %q", out)
	}

	optsGetMissing := registry.OperationOpts{
		Name:   "nonexistent",
		Output: "human",
		FS:     fs,
	}

	restore = captureStdout(t)
	err = d.runGet(optsGetMissing)
	_ = restore()

	if err == nil {
		t.Fatal("expected error getting nonexistent daemonset, got nil")
	}

	var exitErr *exitcode.Error

	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.NotFound {
		t.Errorf("expected exitcode.NotFound, got %v", err)
	}

	_ = fs.MkdirAll("/data/daemonsets/my-ds")
	_ = fs.WriteFile("/data/daemonsets/my-ds/.daemonset.yaml", []byte(`
apiVersion: walheim/v1alpha1
kind: DaemonSet
metadata:
  name: my-ds
spec:
  compose:
    services:
      web:
        image: nginx:latest
`))

	restore = captureStdout(t)
	err = d.runGet(optsList)
	out = restore()

	if err != nil {
		t.Fatalf("runGet list failed: %v", err)
	}

	if !strings.Contains(out, "my-ds") || !strings.Contains(out, "nginx:latest") {
		t.Errorf("unexpected output: %q", out)
	}

	optsGetOne := registry.OperationOpts{
		Name:   "my-ds",
		Output: "human",
		FS:     fs,
	}

	restore = captureStdout(t)
	err = d.runGet(optsGetOne)
	out = restore()

	if err != nil {
		t.Fatalf("runGet single human failed: %v", err)
	}

	if !strings.Contains(out, "my-ds") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/daemonsets/bad-ds")
	_ = fs.WriteFile("/data/daemonsets/bad-ds/.daemonset.yaml", []byte("invalid yaml{"))

	restore = captureStdout(t)
	err = d.runGet(optsList)
	out = restore()

	if err != nil {
		t.Fatalf("runGet with malformed daemonset failed: %v", err)
	}

	if !strings.Contains(out, "skipping daemonset \"bad-ds\"") {
		t.Errorf("expected warning snippet about bad-ds, got: %q", out)
	}
}

func TestDaemonSet_RunApply(t *testing.T) {
	fs := testutil.NewMemFS()
	d := newDaemonSet("/data", fs)

	optsNoFile := registry.OperationOpts{
		Name: "my-ds",
		FS:   fs,
	}

	restore := captureStdout(t)
	err := d.runApply(optsNoFile)
	out := restore()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(out, "--file (-f) is required") {
		t.Errorf("unexpected output: %q", out)
	}

	optsBadFile := registry.OperationOpts{
		Name: "my-ds",
		FS:   fs,
		Flags: map[string]any{
			"file": "nonexistent.yaml",
		},
	}

	err = d.runApply(optsBadFile)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}

	_ = fs.WriteFile("bad.yaml", []byte("invalid yaml {"))
	optsBadYAML := registry.OperationOpts{
		Name: "my-ds",
		FS:   fs,
		Flags: map[string]any{
			"file": "bad.yaml",
		},
	}

	err = d.runApply(optsBadYAML)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}

	optsWrongAPI := registry.OperationOpts{
		Name:        "my-ds",
		FS:          fs,
		RawManifest: []byte("apiVersion: wrong/v1\nkind: DaemonSet\nmetadata:\n  name: my-ds\n"),
	}

	err = d.runApply(optsWrongAPI)
	if err == nil {
		t.Fatal("expected invalid apiVersion error, got nil")
	}

	optsWrongKind := registry.OperationOpts{
		Name:        "my-ds",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: WrongKind\nmetadata:\n  name: my-ds\n"),
	}

	err = d.runApply(optsWrongKind)
	if err == nil {
		t.Fatal("expected invalid kind error, got nil")
	}

	optsWrongName := registry.OperationOpts{
		Name:        "my-ds",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: DaemonSet\nmetadata:\n  name: other-name\n"),
	}

	err = d.runApply(optsWrongName)
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}

	optsEmptyServices := registry.OperationOpts{
		Name:        "my-ds",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: DaemonSet\nmetadata:\n  name: my-ds\nspec:\n  compose:\n    services: {}\n"),
	}

	err = d.runApply(optsEmptyServices)
	if err == nil {
		t.Fatal("expected empty services error, got nil")
	}

	validYAML := []byte("apiVersion: walheim/v1alpha1\nkind: DaemonSet\nmetadata:\n  name: my-ds\nspec:\n  compose:\n    services:\n      web:\n        image: nginx\n")
	optsDryCreate := registry.OperationOpts{
		Name:        "my-ds",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}

	restore = captureStdout(t)
	err = d.runApply(optsDryCreate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply create failed: %v", err)
	}

	if !strings.Contains(out, "Would create daemonset \"my-ds\"") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/daemonsets/my-ds")
	_ = fs.WriteFile("/data/daemonsets/my-ds/.daemonset.yaml", validYAML)

	optsDryUpdate := registry.OperationOpts{
		Name:        "my-ds",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}

	restore = captureStdout(t)
	err = d.runApply(optsDryUpdate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply update failed: %v", err)
	}

	if !strings.Contains(out, "Would update daemonset \"my-ds\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestDaemonSet_RunDelete(t *testing.T) {
	fs := testutil.NewMemFS()
	d := newDaemonSet("/data", fs)

	optsDelMissing := registry.OperationOpts{
		Name: "nonexistent",
		FS:   fs,
	}

	restore := captureStdout(t)
	err := d.runDelete(optsDelMissing)
	out := restore()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(out, "daemonset \"nonexistent\" not found") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/daemonsets/my-ds")
	_ = fs.WriteFile("/data/daemonsets/my-ds/.daemonset.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: DaemonSet\nmetadata:\n  name: my-ds\nspec:\n  compose:\n    services:\n      web:\n        image: nginx\n"))

	optsDelNoTTY := registry.OperationOpts{
		Name: "my-ds",
		Yes:  false,
		FS:   fs,
	}

	err = d.runDelete(optsDelNoTTY)
	if err == nil {
		t.Fatal("expected confirm abort error, got nil")
	}

	optsDelDry := registry.OperationOpts{
		Name:   "my-ds",
		DryRun: true,
		FS:     fs,
	}

	restore = captureStdout(t)
	err = d.runDelete(optsDelDry)
	out = restore()

	if err != nil {
		t.Fatalf("delete dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would stop and delete daemonset \"my-ds\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestDaemonSet_RunDoctor(t *testing.T) {
	fs := testutil.NewMemFS()
	d := newDaemonSet("/data", fs)

	optsDocMissing := registry.OperationOpts{
		Name: "nonexistent",
		FS:   fs,
	}

	err := d.runDoctor(optsDocMissing)
	if err == nil {
		t.Fatal("expected doctor nonexistent error, got nil")
	}

	_ = fs.MkdirAll("/data/daemonsets/my-ds")
	_ = fs.WriteFile("/data/daemonsets/my-ds/.daemonset.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: DaemonSet\nmetadata:\n  name: my-ds\nspec:\n  compose:\n    services:\n      web:\n        image: nginx\n"))

	optsDocSingle := registry.OperationOpts{
		Name: "my-ds",
		FS:   fs,
	}

	restore := captureStdout(t)
	err = d.runDoctor(optsDocSingle)
	out := restore()

	if err != nil {
		t.Fatalf("doctor single failed: %v", err)
	}

	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected no issues, got: %q", out)
	}

	_ = fs.WriteFile("/data/daemonsets/my-ds/.daemonset.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: DaemonSet\nmetadata:\n  name: my-ds\nspec:\n  compose:\n    services: {}\n"))

	restore = captureStdout(t)
	err = d.runDoctor(optsDocSingle)
	out = restore()

	if err == nil {
		t.Fatal("expected doctor errors, got nil")
	}

	if !strings.Contains(out, "spec.compose.services must define at least one service") {
		t.Errorf("expected error details, got: %q", out)
	}
}

func TestDaemonSet_RunStartAndStop(t *testing.T) {
	fs := testutil.NewMemFS()
	d := newDaemonSet("/data", fs)

	_ = fs.MkdirAll("/data/daemonsets/my-ds")
	_ = fs.WriteFile("/data/daemonsets/my-ds/.daemonset.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: DaemonSet\nmetadata:\n  name: my-ds\nspec:\n  compose:\n    services:\n      web:\n        image: nginx\n"))

	optsDryStart := registry.OperationOpts{
		Name:   "my-ds",
		DryRun: true,
		FS:     fs,
	}

	restore := captureStdout(t)
	err := d.runStart(optsDryStart)
	out := restore()

	if err != nil {
		t.Fatalf("start dry-run failed: %v", err)
	}

	if !strings.Contains(out, "no matching namespaces") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/namespaces/prod")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))

	restore = captureStdout(t)
	err = d.runStart(optsDryStart)
	out = restore()

	if err != nil {
		t.Fatalf("start dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would deploy daemonset \"my-ds\" to namespaces: prod") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/daemonsets/my-ds/prod")
	_ = fs.WriteFile("/data/daemonsets/my-ds/prod/docker-compose.yml", []byte("version: '3.8'\n"))

	optsDryStop := registry.OperationOpts{
		Name:   "my-ds",
		DryRun: true,
		FS:     fs,
	}

	restore = captureStdout(t)
	err = d.runStop(optsDryStop)
	out = restore()

	if err != nil {
		t.Fatalf("stop dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would stop daemonset \"my-ds\" on namespaces: prod") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestDaemonSet_RunDescribe(t *testing.T) {
	fs := testutil.NewMemFS()
	d := newDaemonSet("/data", fs)

	optsDescMissing := registry.OperationOpts{
		Name: "nonexistent",
		FS:   fs,
	}

	err := d.runDescribe(optsDescMissing)
	if err == nil {
		t.Fatal("expected describe nonexistent error, got nil")
	}

	_ = fs.MkdirAll("/data/daemonsets/my-ds")
	_ = fs.WriteFile("/data/daemonsets/my-ds/.daemonset.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: DaemonSet\nmetadata:\n  name: my-ds\nspec:\n  compose:\n    services:\n      web:\n        image: nginx\n"))

	optsDescSingle := registry.OperationOpts{
		Name: "my-ds",
		FS:   fs,
	}

	restore := captureStdout(t)
	err = d.runDescribe(optsDescSingle)
	out := restore()

	if err != nil {
		t.Fatalf("describe single failed: %v", err)
	}

	if !strings.Contains(out, "Name:      my-ds") {
		t.Errorf("unexpected output: %q", out)
	}

	optsDescJSON := registry.OperationOpts{
		Name:   "my-ds",
		Output: "json",
		FS:     fs,
	}

	restore = captureStdout(t)
	err = d.runDescribe(optsDescJSON)
	out = restore()

	if err != nil {
		t.Fatalf("describe json failed: %v", err)
	}

	if !strings.Contains(out, `"kind": "DaemonSet"`) || !strings.Contains(out, `"name": "my-ds"`) {
		t.Errorf("unexpected json output: %q", out)
	}
}
