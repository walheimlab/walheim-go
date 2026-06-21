package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := buildRoot()

	var outBuf bytes.Buffer

	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return outBuf.String(), err
}

func setupConfig(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configContent := []byte(`
apiVersion: walheim.io/v1
kind: Config
currentContext: local
contexts:
  - name: local
    dataDir: ` + tempDir + `
  - name: other
    dataDir: ` + tempDir + `/other
`)

	err := os.WriteFile(configPath, configContent, 0600)
	if err != nil {
		t.Fatalf("failed to write mock config: %v", err)
	}

	// Write a mock namespace to local context so we can test export
	nsPath := filepath.Join(tempDir, "namespaces", "prod")

	err = os.MkdirAll(nsPath, 0755)
	if err != nil {
		t.Fatalf("failed to create namespaces dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(nsPath, ".namespace.yaml"), []byte("apiVersion: walheim/v1alpha1\nkind: Namespace\nmetadata:\n  name: prod\nspec:\n  hostname: 127.0.0.1\n"), 0600)
	if err != nil {
		t.Fatalf("failed to write namespace: %v", err)
	}

	return configPath
}

func TestContextCmd_List(t *testing.T) {
	configPath := setupConfig(t)

	// List human mode
	restore := captureStdout(t)
	_, err := runCmd(t, "context", "list", "--whconfig", configPath)
	out := restore()

	if err != nil {
		t.Fatalf("context list failed: %v", err)
	}

	if !strings.Contains(out, "local") || !strings.Contains(out, "other") {
		t.Errorf("unexpected output: %q", out)
	}

	// List json mode
	restore = captureStdout(t)
	_, err = runCmd(t, "context", "list", "--whconfig", configPath, "-o", "json")
	out = restore()

	if err != nil {
		t.Fatalf("context list JSON failed: %v", err)
	}

	if !strings.Contains(out, `"name": "local"`) {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestContextCmd_Current(t *testing.T) {
	configPath := setupConfig(t)

	// Current human mode
	restore := captureStdout(t)
	_, err := runCmd(t, "context", "current", "--whconfig", configPath)
	out := restore()

	if err != nil {
		t.Fatalf("context current failed: %v", err)
	}

	if !strings.Contains(out, "local") {
		t.Errorf("unexpected output: %q", out)
	}

	// Current json mode
	restore = captureStdout(t)
	_, err = runCmd(t, "context", "current", "--whconfig", configPath, "-o", "json")
	out = restore()

	if err != nil {
		t.Fatalf("context current JSON failed: %v", err)
	}

	if !strings.Contains(out, `"name": "local"`) {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestContextCmd_Use(t *testing.T) {
	configPath := setupConfig(t)

	restore := captureStdout(t)
	_, err := runCmd(t, "context", "use", "other", "--whconfig", configPath)
	out := restore()

	if err != nil {
		t.Fatalf("context use failed: %v", err)
	}

	if !strings.Contains(out, "Switched to context \"other\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Verify switch by running current command
	restore = captureStdout(t)
	_, err = runCmd(t, "context", "current", "--whconfig", configPath)
	out = restore()

	if err != nil {
		t.Fatalf("context current after use failed: %v", err)
	}

	if !strings.Contains(out, "other") {
		t.Errorf("expected context to be other, got: %q", out)
	}
}

func TestContextCmd_New(t *testing.T) {
	configPath := setupConfig(t)
	tempDir := t.TempDir()

	restore := captureStdout(t)
	_, err := runCmd(t, "context", "new", "third", "--data-dir", tempDir, "--whconfig", configPath)
	out := restore()

	if err != nil {
		t.Fatalf("context new failed: %v", err)
	}

	if !strings.Contains(out, "Added context \"third\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Switch to new context and verify it works
	restore = captureStdout(t)
	_, err = runCmd(t, "context", "use", "third", "--whconfig", configPath)
	out = restore()

	if err != nil {
		t.Fatalf("context use failed: %v", err)
	}

	if !strings.Contains(out, "Switched to context \"third\"") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestContextCmd_Delete(t *testing.T) {
	configPath := setupConfig(t)

	restore := captureStdout(t)
	_, err := runCmd(t, "context", "delete", "other", "--whconfig", configPath)
	out := restore()

	if err != nil {
		t.Fatalf("context delete failed: %v", err)
	}

	if !strings.Contains(out, "Deleted context \"other\"") {
		t.Errorf("unexpected output: %q", out)
	}

	// Verify deletion
	restore = captureStdout(t)
	_, err = runCmd(t, "context", "list", "--whconfig", configPath)
	out = restore()

	if err != nil {
		t.Fatalf("context list failed: %v", err)
	}

	if strings.Contains(out, "other") {
		t.Errorf("expected context \"other\" to be deleted, got listing: %q", out)
	}
}

func TestContextCmd_Export(t *testing.T) {
	configPath := setupConfig(t)

	restore := captureStdout(t)
	_, err := runCmd(t, "context", "export", "--whconfig", configPath)
	out := restore()

	if err != nil {
		t.Fatalf("context export failed: %v", err)
	}

	if !strings.Contains(out, "apiVersion: walheim/v1alpha1") || !strings.Contains(out, "kind: Namespace") {
		t.Errorf("unexpected output: %q", out)
	}
}
