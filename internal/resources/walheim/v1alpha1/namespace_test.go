package v1alpha1

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/registry"
	"github.com/walheimlab/walheim-go/internal/testutil"
)

// captureStdout redirects os.Stdout and os.Stderr and returns a function to restore them
// and read the captured output.
func captureStdout(t *testing.T) func() string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = w
	os.Stderr = w

	return func() string {
		_ = w.Close()

		os.Stdout = oldStdout
		os.Stderr = oldStderr

		var buf bytes.Buffer

		_, _ = io.Copy(&buf, r)

		return buf.String()
	}
}

func TestNamespace_RunCreate(t *testing.T) {
	fs := testutil.NewMemFS()
	n := newNamespace("/data", fs)

	// Create success
	opts := registry.OperationOpts{
		Name:   "prod",
		Output: "human",
		Flags: map[string]any{
			"hostname": "127.0.0.1",
			"username": "admin",
			"base-dir": "/data/walheim",
		},
	}

	restore := captureStdout(t)
	err := n.runCreate(opts)
	out := restore()

	if err != nil {
		t.Fatalf("runCreate failed: %v", err)
	}

	if !strings.Contains(out, "Created namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Verify namespace files and metadata directories were created
	exists, _ := fs.Exists("/data/namespaces/prod/.namespace.yaml")
	if !exists {
		t.Error("manifest file was not created")
	}

	exists, _ = fs.Exists("/data/namespaces/prod/apps")
	if !exists {
		t.Error("apps subdirectory was not created")
	}

	// Create dry-run
	optsDry := registry.OperationOpts{
		Name:   "dry",
		DryRun: true,
		Flags: map[string]any{
			"hostname": "127.0.0.1",
		},
	}
	restore = captureStdout(t)
	err = n.runCreate(optsDry)
	out = restore()

	if err != nil {
		t.Fatalf("runCreate failed: %v", err)
	}

	if !strings.Contains(out, "Would create namespace \"dry\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Create conflict
	restore = captureStdout(t)
	err = n.runCreate(opts)
	_ = restore()

	if err == nil {
		t.Fatal("expected error creating existing namespace, got nil")
	}

	var exitErr *exitcode.Error

	if !errors.As(err, &exitErr) || exitErr.Code != exitcode.Conflict {
		t.Errorf("expected exitcode.Conflict, got %v", err)
	}
}

func TestNamespace_RunGet(t *testing.T) {
	fs := testutil.NewMemFS()
	n := newNamespace("/data", fs)

	// Get empty list
	optsList := registry.OperationOpts{
		Output: "human",
	}
	restore := captureStdout(t)
	err := n.runGet(optsList)
	out := restore()

	if err != nil {
		t.Fatalf("runGet failed: %v", err)
	}

	if !strings.Contains(out, "No namespaces found") {
		t.Errorf("expected empty message, got %q", out)
	}

	// Create namespace
	optsCreate := registry.OperationOpts{
		Name: "prod",
		Flags: map[string]any{
			"hostname": "127.0.0.1",
		},
	}
	_ = n.runCreate(optsCreate)

	// Get list
	restore = captureStdout(t)
	err = n.runGet(optsList)
	out = restore()

	if err != nil {
		t.Fatalf("runGet failed: %v", err)
	}

	if !strings.Contains(out, "prod") {
		t.Errorf("expected 'prod' in output, got %q", out)
	}

	// Get single (human)
	optsGetOne := registry.OperationOpts{
		Name:   "prod",
		Output: "human",
	}
	restore = captureStdout(t)
	err = n.runGet(optsGetOne)
	out = restore()

	if err != nil {
		t.Fatalf("runGet failed: %v", err)
	}

	if !strings.Contains(out, "prod") {
		t.Errorf("expected 'prod' in output, got %q", out)
	}

	// Get single (yaml)
	optsGetOneYAML := registry.OperationOpts{
		Name:   "prod",
		Output: "yaml",
	}
	restore = captureStdout(t)
	err = n.runGet(optsGetOneYAML)
	out = restore()

	if err != nil {
		t.Fatalf("runGet failed: %v", err)
	}

	if !strings.Contains(out, "hostname: 127.0.0.1") {
		t.Errorf("expected 'hostname: 127.0.0.1', got %q", out)
	}

	// Get nonexistent
	optsGetMissing := registry.OperationOpts{
		Name: "nonexistent",
	}
	restore = captureStdout(t)
	err = n.runGet(optsGetMissing)
	_ = restore()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNamespace_RunDelete(t *testing.T) {
	fs := testutil.NewMemFS()
	n := newNamespace("/data", fs)

	// Delete nonexistent
	optsDelMissing := registry.OperationOpts{
		Name: "nonexistent",
	}

	err := n.runDelete(optsDelMissing)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Create
	optsCreate := registry.OperationOpts{
		Name: "prod",
		Flags: map[string]any{
			"hostname": "127.0.0.1",
		},
	}
	_ = n.runCreate(optsCreate)

	// Delete dry-run
	optsDelDry := registry.OperationOpts{
		Name:   "prod",
		DryRun: true,
	}
	restore := captureStdout(t)
	err = n.runDelete(optsDelDry)
	out := restore()

	if err != nil {
		t.Fatalf("runDelete failed: %v", err)
	}

	if !strings.Contains(out, "Would delete namespace") {
		t.Errorf("unexpected output: %q", out)
	}

	// Delete success (Yes = true)
	optsDel := registry.OperationOpts{
		Name: "prod",
		Yes:  true,
	}
	restore = captureStdout(t)
	err = n.runDelete(optsDel)
	out = restore()

	if err != nil {
		t.Fatalf("runDelete failed: %v", err)
	}

	if !strings.Contains(out, "Deleted namespace \"prod\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Verify directory deleted
	exists, _ := fs.Exists("/data/namespaces/prod")
	if exists {
		t.Error("namespace directory should be deleted")
	}
}

func TestNamespace_RunDoctor(t *testing.T) {
	fs := testutil.NewMemFS()
	n := newNamespace("/data", fs)

	// Doctor nonexistent single
	optsDocMissing := registry.OperationOpts{
		Name: "nonexistent",
	}

	err := n.runDoctor(optsDocMissing)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Create valid
	optsCreate := registry.OperationOpts{
		Name: "prod",
		Flags: map[string]any{
			"hostname": "127.0.0.1",
		},
	}
	_ = n.runCreate(optsCreate)

	// Doctor valid list
	optsDoc := registry.OperationOpts{
		Output: "human",
	}

	err = n.runDoctor(optsDoc)
	if err != nil {
		t.Fatalf("runDoctor failed: %v", err)
	}

	// Write invalid manifest
	manifestPath := "/data/namespaces/prod/.namespace.yaml"
	_ = fs.WriteFile(manifestPath, []byte("invalid yaml {["))

	err = n.runDoctor(optsDoc)
	if err == nil {
		t.Fatal("expected doctor to find errors, got nil")
	}
}
