package v1alpha1

import (
	"errors"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

func TestJob_KindInfo(t *testing.T) {
	fs := testutil.NewMemFS()
	j := newJob("/data", fs)

	info := j.KindInfo()
	if info.Kind != "Job" {
		t.Errorf("expected Job, got %q", info.Kind)
	}
}

func TestJob_RunGet(t *testing.T) {
	fs := testutil.NewMemFS()
	j := newJob("/data", fs)

	optsList := registry.OperationOpts{
		Namespace: "prod",
		Output:    "human",
		FS:        fs,
	}

	restore := captureStdout(t)
	err := j.runGet(optsList)
	out := restore()

	if err != nil {
		t.Fatalf("runGet empty failed: %v", err)
	}

	if !strings.Contains(out, "No jobs found") {
		t.Errorf("unexpected output: %q", out)
	}

	optsGetMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		Output:    "human",
		FS:        fs,
	}

	restore = captureStdout(t)
	err = j.runGet(optsGetMissing)
	_ = restore()

	if err == nil {
		t.Fatal("expected error getting nonexistent job, got nil")
	}

	var exitErr *exitcode.Error

	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.NotFound {
		t.Errorf("expected exitcode.NotFound, got %v", err)
	}

	_ = fs.MkdirAll("/data/namespaces/prod/jobs/myjob")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))
	_ = fs.WriteFile("/data/namespaces/prod/jobs/myjob/.job.yaml", []byte(`
apiVersion: walheim/v1alpha1
kind: Job
metadata:
  name: myjob
  namespace: prod
spec:
  image: alpine:latest
`))

	restore = captureStdout(t)
	err = j.runGet(optsList)
	out = restore()

	if err != nil {
		t.Fatalf("runGet list failed: %v", err)
	}

	if !strings.Contains(out, "myjob") || !strings.Contains(out, "alpine:latest") {
		t.Errorf("unexpected output: %q", out)
	}

	optsGetOne := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		Output:    "human",
		FS:        fs,
	}

	restore = captureStdout(t)
	err = j.runGet(optsGetOne)
	out = restore()

	if err != nil {
		t.Fatalf("runGet single human failed: %v", err)
	}

	if !strings.Contains(out, "myjob") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/namespaces/dev/jobs/devjob")
	_ = fs.WriteFile("/data/namespaces/dev/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: dev\nspec:\n  hostname: 127.0.0.1\n"))
	_ = fs.WriteFile("/data/namespaces/dev/jobs/devjob/.job.yaml", []byte(`
apiVersion: walheim/v1alpha1
kind: Job
metadata:
  name: devjob
  namespace: dev
spec:
  image: busybox:latest
`))

	optsAll := registry.OperationOpts{
		AllNamespaces: true,
		Output:        "human",
		FS:            fs,
	}

	restore = captureStdout(t)
	err = j.runGet(optsAll)
	out = restore()

	if err != nil {
		t.Fatalf("runGet AllNamespaces failed: %v", err)
	}

	if !strings.Contains(out, "devjob") || !strings.Contains(out, "myjob") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/namespaces/dev/jobs/badjob")
	_ = fs.WriteFile("/data/namespaces/dev/jobs/badjob/.job.yaml", []byte("invalid yaml{"))

	restore = captureStdout(t)
	err = j.runGet(optsAll)
	out = restore()

	if err != nil {
		t.Fatalf("runGet with malformed job failed: %v", err)
	}

	if !strings.Contains(out, "skipping job \"badjob\" in namespace \"dev\"") {
		t.Errorf("expected warning snippet about badjob, got: %q", out)
	}
}

func TestJob_RunApply(t *testing.T) {
	fs := testutil.NewMemFS()
	j := newJob("/data", fs)

	optsNoFile := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		FS:        fs,
	}

	restore := captureStdout(t)
	err := j.runApply(optsNoFile)
	out := restore()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(out, "--file (-f) is required") {
		t.Errorf("unexpected output: %q", out)
	}

	optsBadFile := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		FS:        fs,
		Flags: map[string]any{
			"file": "nonexistent.yaml",
		},
	}

	err = j.runApply(optsBadFile)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}

	_ = fs.WriteFile("bad.yaml", []byte("invalid yaml {"))
	optsBadYAML := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		FS:        fs,
		Flags: map[string]any{
			"file": "bad.yaml",
		},
	}

	err = j.runApply(optsBadYAML)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}

	optsWrongAPI := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myjob",
		FS:          fs,
		RawManifest: []byte("apiVersion: wrong/v1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: prod\n"),
	}

	err = j.runApply(optsWrongAPI)
	if err == nil {
		t.Fatal("expected invalid apiVersion error, got nil")
	}

	optsWrongKind := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myjob",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: WrongKind\nmetadata:\n  name: myjob\n  namespace: prod\n"),
	}

	err = j.runApply(optsWrongKind)
	if err == nil {
		t.Fatal("expected invalid kind error, got nil")
	}

	optsWrongName := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myjob",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: other-name\n  namespace: prod\n"),
	}

	err = j.runApply(optsWrongName)
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}

	optsWrongNS := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myjob",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: other-ns\n"),
	}

	err = j.runApply(optsWrongNS)
	if err == nil {
		t.Fatal("expected namespace mismatch error, got nil")
	}

	optsMissingImage := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myjob",
		FS:          fs,
		RawManifest: []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: prod\nspec:\n  image: \"\"\n"),
	}

	err = j.runApply(optsMissingImage)
	if err == nil {
		t.Fatal("expected missing image error, got nil")
	}

	validYAML := []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: prod\nspec:\n  image: alpine\n")
	optsDryCreate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myjob",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}

	restore = captureStdout(t)
	err = j.runApply(optsDryCreate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply create failed: %v", err)
	}

	if !strings.Contains(out, "Would create job \"myjob\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/namespaces/prod/jobs/myjob")
	_ = fs.WriteFile("/data/namespaces/prod/jobs/myjob/.job.yaml", validYAML)

	optsDryUpdate := registry.OperationOpts{
		Namespace:   "prod",
		Name:        "myjob",
		DryRun:      true,
		FS:          fs,
		RawManifest: validYAML,
	}

	restore = captureStdout(t)
	err = j.runApply(optsDryUpdate)
	out = restore()

	if err != nil {
		t.Fatalf("dry-run apply update failed: %v", err)
	}

	if !strings.Contains(out, "Would update job \"myjob\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestJob_RunDelete(t *testing.T) {
	fs := testutil.NewMemFS()
	j := newJob("/data", fs)

	optsDelMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		FS:        fs,
	}

	restore := captureStdout(t)
	err := j.runDelete(optsDelMissing)
	out := restore()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(out, "job \"nonexistent\" not found in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	_ = fs.MkdirAll("/data/namespaces/prod/jobs/myjob")
	_ = fs.WriteFile("/data/namespaces/prod/jobs/myjob/.job.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: prod\nspec:\n  image: alpine\n"))

	optsDelNoTTY := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		Yes:       false,
		FS:        fs,
	}

	err = j.runDelete(optsDelNoTTY)
	if err == nil {
		t.Fatal("expected confirm abort error, got nil")
	}

	optsDelDry := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		DryRun:    true,
		FS:        fs,
	}

	restore = captureStdout(t)
	err = j.runDelete(optsDelDry)
	out = restore()

	if err != nil {
		t.Fatalf("delete dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would delete job \"myjob\" in namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestJob_RunDoctor(t *testing.T) {
	fs := testutil.NewMemFS()
	j := newJob("/data", fs)

	optsDocMissing := registry.OperationOpts{
		Namespace: "prod",
		Name:      "nonexistent",
		FS:        fs,
	}

	err := j.runDoctor(optsDocMissing)
	if err == nil {
		t.Fatal("expected doctor nonexistent error, got nil")
	}

	_ = fs.MkdirAll("/data/namespaces/prod/jobs/myjob")
	_ = fs.WriteFile("/data/namespaces/prod/jobs/myjob/.job.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: prod\nspec:\n  image: alpine\n"))

	optsDocSingle := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		FS:        fs,
	}

	restore := captureStdout(t)
	err = j.runDoctor(optsDocSingle)
	out := restore()

	if err != nil {
		t.Fatalf("doctor single failed: %v", err)
	}

	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected no issues, got: %q", out)
	}

	_ = fs.WriteFile("/data/namespaces/prod/jobs/myjob/.job.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: prod\nspec:\n  image: \"\"\n"))

	restore = captureStdout(t)
	err = j.runDoctor(optsDocSingle)
	out = restore()

	if err == nil {
		t.Fatal("expected doctor errors, got nil")
	}

	if !strings.Contains(out, "spec.image is required but not set") {
		t.Errorf("expected error details, got: %q", out)
	}
}

func TestJob_RunRun(t *testing.T) {
	fs := testutil.NewMemFS()
	j := newJob("/data", fs)

	_ = fs.MkdirAll("/data/namespaces/prod/jobs/myjob")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))
	_ = fs.WriteFile("/data/namespaces/prod/jobs/myjob/.job.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: prod\nspec:\n  image: alpine\n"))

	optsDryRun := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		DryRun:    true,
		FS:        fs,
	}

	restore := captureStdout(t)
	err := j.runRun(optsDryRun)
	out := restore()

	if err != nil {
		t.Fatalf("run dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would rsync and run on 127.0.0.1: cd /data/walheim/jobs/myjob && docker compose --progress plain run --rm job") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestJob_RunLogs(t *testing.T) {
	fs := testutil.NewMemFS()
	j := newJob("/data", fs)

	_ = fs.MkdirAll("/data/namespaces/prod/jobs/myjob")
	_ = fs.WriteFile("/data/namespaces/prod/.namespace.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"))
	_ = fs.WriteFile("/data/namespaces/prod/jobs/myjob/.job.yaml", []byte("apiVersion: walheim/v1alpha1\nkind: Job\nmetadata:\n  name: myjob\n  namespace: prod\nspec:\n  image: alpine\n"))

	// 1. Basic logs with default tail (-1)
	optsDryLogs := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		DryRun:    true,
		FS:        fs,
		Flags: map[string]any{
			"tail": -1,
		},
	}

	restore := captureStdout(t)
	err := j.runLogs(optsDryLogs)
	out := restore()

	if err != nil {
		t.Fatalf("logs dry-run failed: %v", err)
	}

	if !strings.Contains(out, "Would run on 127.0.0.1: cd /data/walheim/jobs/myjob && docker compose logs job") {
		t.Errorf("unexpected output: %q", out)
	}

	// 2. Logs with tail and follow
	optsFlags := registry.OperationOpts{
		Namespace: "prod",
		Name:      "myjob",
		DryRun:    true,
		FS:        fs,
		Flags: map[string]any{
			"tail":   50,
			"follow": true,
		},
	}

	restore = captureStdout(t)
	err = j.runLogs(optsFlags)
	out = restore()

	if err != nil {
		t.Fatalf("logs dry-run with flags failed: %v", err)
	}

	if !strings.Contains(out, "docker compose logs --follow --tail 50 job") {
		t.Errorf("unexpected output: %q", out)
	}
}
