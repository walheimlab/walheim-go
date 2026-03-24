package config

import "testing"

func TestConfigError_Error(t *testing.T) {
	e := &ConfigError{message: "something failed"}

	if e.Error() != "something failed" {
		t.Errorf("Error() = %q, want %q", e.Error(), "something failed")
	}
}

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{message: "invalid field"}

	if e.Error() != "invalid field" {
		t.Errorf("Error() = %q, want %q", e.Error(), "invalid field")
	}
}
