package exitcode_test

import (
	"errors"
	"testing"

	"github.com/walheimlab/walheim-go/internal/exitcode"
)

func TestNew(t *testing.T) {
	inner := errors.New("something failed")
	err := exitcode.New(exitcode.NotFound, inner)

	var ec *exitcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitcode.Error, got %T", err)
	}

	if ec.ExitCode() != exitcode.NotFound {
		t.Errorf("ExitCode() = %d, want %d", ec.ExitCode(), exitcode.NotFound)
	}

	if ec.Error() != "something failed" {
		t.Errorf("Error() = %q, want %q", ec.Error(), "something failed")
	}
}

func TestNew_wrapsMessage(t *testing.T) {
	err := exitcode.New(exitcode.Failure, errors.New("disk full"))
	if err.Error() != "disk full" {
		t.Errorf("Error() = %q, want %q", err.Error(), "disk full")
	}
}

func TestConstants_unique(t *testing.T) {
	codes := map[int]string{
		exitcode.OK:         "OK",
		exitcode.Failure:    "Failure",
		exitcode.UsageError: "UsageError",
		exitcode.NotFound:   "NotFound",
		exitcode.Forbidden:  "Forbidden",
		exitcode.Conflict:   "Conflict",
	}
	if len(codes) != 6 {
		t.Errorf("expected 6 unique exit codes, got %d", len(codes))
	}
}
