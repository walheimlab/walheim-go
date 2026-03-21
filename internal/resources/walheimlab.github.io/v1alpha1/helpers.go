// Package v1alpha1 registers all walheimlab.github.io/v1alpha1 resource kinds.
package v1alpha1

import (
	"fmt"
	"os"
	"strings"

	"github.com/walheimlab/walheim-go/internal/exitcode"
	"github.com/walheimlab/walheim-go/internal/fs"
)

// readInput reads bytes from a file path or "-" for stdin.
func readInput(filePath string, filesystem fs.FS) ([]byte, error) {
	if filePath == "-" {
		return readStdin()
	}
	return filesystem.ReadFile(filePath)
}

func readStdin() ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 512)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

func exitErr(code int, err error) error {
	return exitcode.New(code, err)
}

func promptConfirm(yes bool, prompt string) error {
	if yes {
		return nil
	}
	fi, err := os.Stdin.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return exitcode.New(exitcode.UsageError,
			fmt.Errorf("stdin is not a TTY; pass --yes to confirm destructive operations"))
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	var answer string
	fmt.Fscan(os.Stdin, &answer)
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		return fmt.Errorf("aborted")
	}
	return nil
}
