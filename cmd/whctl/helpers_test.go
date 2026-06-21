package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
)

func TestValidateResourceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid letters", "my-resource", false},
		{"valid numbers", "resource123", false},
		{"valid underscore/dot", "my_resource.yaml", false},
		{"invalid characters slash", "my/resource", true},
		{"invalid characters space", "my resource", true},
		{"invalid characters spec", "my$resource", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResourceName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	origHome := os.Getenv("HOME")

	defer func() { _ = os.Setenv("HOME", origHome) }()

	_ = os.Setenv("HOME", "/mock/home")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"no tilde", "/some/absolute/path", "/some/absolute/path"},
		{"tilde only", "~", "/mock/home"},
		{"tilde with slash", "~/foo/bar", "/mock/home/foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.input)
			if got != tt.expected {
				t.Errorf("expandHome() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestExitErr(t *testing.T) {
	origErr := errors.New("something went wrong")
	err := exitErr(exitcode.Conflict, origErr)

	var exitErr *exitcode.Error
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exitcode.Error, got %v", err)
	}

	if exitErr.Code != exitcode.Conflict {
		t.Errorf("expected exitcode.Conflict, got %d", exitErr.Code)
	}

	if exitErr.Err != origErr {
		t.Errorf("expected wrapped error to match origErr")
	}
}

func TestResolveBackend_Error(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nonexistent.yaml")

	_, _, err := resolveBackend("some-context", configPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "no config found") {
		t.Errorf("expected no config found message, got %v", err)
	}
}

func TestResolveBackend_Success(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	configContent := []byte(`
apiVersion: walheim.io/v1
kind: Config
currentContext: local
contexts:
  - name: local
    dataDir: ` + tempDir + `
`)

	err := os.WriteFile(configPath, configContent, 0600)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	fs, dataDir, err := resolveBackend("local", configPath)
	if err != nil {
		t.Fatalf("resolveBackend failed: %v", err)
	}

	if dataDir != tempDir {
		t.Errorf("expected dataDir %q, got %q", tempDir, dataDir)
	}

	if fs == nil {
		t.Error("expected filesystem to be initialized, got nil")
	}
}
