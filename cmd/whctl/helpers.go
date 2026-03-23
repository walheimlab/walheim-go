package main

import (
	"fmt"
	"regexp"

	"github.com/walheimlab/walheim-go/internal/config"
	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
)

// resolveBackend loads config and returns the FS implementation and dataDir for the active context.
// For local contexts, returns (LocalFS, dataDir, nil).
// For S3 contexts, returns (S3FS, "", nil) — dataDir is always empty for S3.
func resolveBackend(contextFlag, whconfigFlag string) (fs.FS, string, error) {
	cfg, err := config.Load(whconfigFlag)
	if err != nil {
		return nil, "", fmt.Errorf("no config found — run 'whctl context new' to create one\n(%v)", err)
	}

	ctx, err := cfg.ContextForName(contextFlag)
	if err != nil {
		return nil, "", fmt.Errorf("context error: %w", err)
	}

	filesystem, dataDir, err := fs.FromContext(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to initialise storage backend: %w", err)
	}

	return filesystem, dataDir, nil
}

// validNameRe allows alphanumeric, hyphen, underscore, dot.
var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9._\-]+$`)

// validateResourceName returns an error if name contains unsafe characters.
func validateResourceName(name string) error {
	if !validNameRe.MatchString(name) {
		return fmt.Errorf("invalid resource name %q: must match ^[a-zA-Z0-9._-]+$", name)
	}

	return nil
}

// exitErr wraps an error with an exit code using the shared exitcode.Error type.
// Returned from cobra RunE; main() reads the exit code via ExitCode().
func exitErr(code int, err error) error {
	return exitcode.New(code, err)
}
