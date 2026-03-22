package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LocalFS implements the FS interface using the local OS filesystem.
type LocalFS struct{}

// NewLocalFS creates a new LocalFS instance.
func NewLocalFS() *LocalFS {
	return &LocalFS{}
}

// ReadFile reads and returns the file contents.
func (lfs *LocalFS) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes data atomically by writing to a temp file and then renaming it.
func (lfs *LocalFS) WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)

	// Ensure parent directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create temp file in the same directory for atomic rename
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	_ = tmpFile.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// MkdirAll creates the directory and all parent directories.
func (lfs *LocalFS) MkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}

// RemoveAll removes the path and all its children.
func (lfs *LocalFS) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Exists checks if a path exists.
func (lfs *LocalFS) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// IsDir checks if a path is a directory.
func (lfs *LocalFS) IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// ReadDir returns a sorted list of non-hidden child entry names.
// Hidden entries (starting with ".") are excluded.
func (lfs *LocalFS) ReadDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		// Skip hidden entries (starting with ".")
		if entry.Name()[0] != '.' {
			names = append(names, entry.Name())
		}
	}

	sort.Strings(names)
	return names, nil
}
