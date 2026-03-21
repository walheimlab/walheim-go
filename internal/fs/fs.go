package fs

// FS is the filesystem abstraction used by all resource implementations.
// The local implementation wraps the OS filesystem.
// A future S3 implementation would satisfy this same interface.
type FS interface {
	// ReadFile reads a file, returns its contents.
	ReadFile(path string) ([]byte, error)

	// WriteFile writes data atomically (temp file + rename).
	WriteFile(path string, data []byte) error

	// MkdirAll creates path and all parents, like os.MkdirAll.
	MkdirAll(path string) error

	// RemoveAll removes path and all children, like os.RemoveAll.
	RemoveAll(path string) error

	// Exists reports whether a path exists.
	Exists(path string) (bool, error)

	// IsDir reports whether path is a directory.
	IsDir(path string) (bool, error)

	// ReadDir returns a sorted list of non-hidden child names in a directory.
	// Hidden entries (names starting with ".") are excluded.
	ReadDir(path string) ([]string, error)
}
