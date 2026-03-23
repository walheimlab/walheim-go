// Package testutil provides test helpers shared across packages.
package testutil

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// MemFS is a simple in-memory implementation of fs.FS for use in tests.
// It is safe for concurrent use.
type MemFS struct {
	mu    sync.RWMutex
	files map[string][]byte // path → content
	dirs  map[string]bool   // path → true
}

// NewMemFS creates an empty MemFS.
func NewMemFS() *MemFS {
	return &MemFS{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (m *MemFS) ReadFile(path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	cp := make([]byte, len(data))
	copy(cp, data)

	return cp, nil
}

func (m *MemFS) WriteFile(path string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)
	m.files[path] = cp
	// Ensure all parent directories exist.
	m.ensureParents(path)

	return nil
}

func (m *MemFS) MkdirAll(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dirs[path] = true
	m.ensureParents(path)

	return nil
}

func (m *MemFS) RemoveAll(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for k := range m.files {
		if k == path || strings.HasPrefix(k, path+"/") {
			delete(m.files, k)
		}
	}

	for k := range m.dirs {
		if k == path || strings.HasPrefix(k, path+"/") {
			delete(m.dirs, k)
		}
	}

	return nil
}

func (m *MemFS) Exists(path string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, fileOK := m.files[path]
	_, dirOK := m.dirs[path]

	return fileOK || dirOK, nil
}

func (m *MemFS) IsDir(path string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.dirs[path], nil
}

// ReadDir returns sorted non-hidden direct children of path.
func (m *MemFS) ReadDir(path string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefix := path + "/"
	seen := map[string]bool{}

	for k := range m.files {
		if strings.HasPrefix(k, prefix) {
			rel := strings.TrimPrefix(k, prefix)
			// Direct child only (no slashes in the remainder).
			if !strings.Contains(rel, "/") && !strings.HasPrefix(rel, ".") {
				seen[rel] = true
			} else if idx := strings.Index(rel, "/"); idx > 0 {
				child := rel[:idx]
				if !strings.HasPrefix(child, ".") {
					seen[child] = true
				}
			}
		}
	}

	for k := range m.dirs {
		if strings.HasPrefix(k, prefix) {
			rel := strings.TrimPrefix(k, prefix)
			if !strings.Contains(rel, "/") && !strings.HasPrefix(rel, ".") {
				seen[rel] = true
			}
		}
	}

	if len(seen) == 0 {
		// Distinguish "dir does not exist" from "dir is empty".
		if !m.dirs[path] {
			return nil, fmt.Errorf("directory not found: %s", path)
		}

		return nil, nil
	}

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}

	sort.Strings(out)

	return out, nil
}

// ensureParents creates all ancestor directory entries for path.
// Caller must hold m.mu.Lock.
func (m *MemFS) ensureParents(path string) {
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		dir := strings.Join(parts[:i], "/")
		if dir != "" {
			m.dirs[dir] = true
		}
	}
}
