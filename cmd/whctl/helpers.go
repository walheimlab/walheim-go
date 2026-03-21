package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"

	"github.com/walheimlab/walheim-go/internal/config"
	"github.com/walheimlab/walheim-go/internal/exitcode"
)

// resolveDataDir loads config and returns the active context's dataDir.
// contextFlag: if non-empty, overrides the active context name.
// whconfigFlag: if non-empty, overrides the config file path.
func resolveDataDir(contextFlag, whconfigFlag string) (string, error) {
	cfg, err := config.Load(whconfigFlag)
	if err != nil {
		return "", fmt.Errorf("no config found — run 'whctl context new' to create one\n(%v)", err)
	}

	dataDir, err := cfg.DataDir(contextFlag)
	if err != nil {
		return "", fmt.Errorf("context error: %w", err)
	}

	return dataDir, nil
}

// isTTY returns true if stdin is connected to a terminal.
func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// promptConfirm asks "y/N" if yes is false and stdin is a TTY.
// If stdin is not a TTY and yes is false, returns an error with --yes hint.
func promptConfirm(yes bool, prompt string) error {
	if yes {
		return nil
	}

	if !isTTY() {
		return exitErr(exitcode.UsageError,
			fmt.Errorf("stdin is not a TTY; pass --yes to confirm destructive operations"))
	}

	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	answer := strings.TrimSpace(strings.ToLower(line))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("aborted")
	}

	return nil
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

// exitErr wraps an error with an exit code. The error is returned from cobra RunE;
// the main function calls os.Exit when cobra returns a non-nil error.
// We use a special wrapper so the caller can inspect the exit code.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) ExitCode() int { return e.code }

// exitErr creates an exitError.
func exitErr(code int, err error) error {
	return &exitError{code: code, err: err}
}
