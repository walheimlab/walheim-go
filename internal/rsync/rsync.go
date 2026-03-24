package rsync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/sftp"

	wfs "github.com/walheimlab/walheim-go/internal/fs"
)

// Syncer uploads files from a walheim FS to a remote host via SFTP.
// It starts the system ssh binary with -s sftp as the transport, reusing
// system SSH key management and agent forwarding.
type Syncer struct{}

// NewSyncer creates a new Syncer.
func NewSyncer() *Syncer { return &Syncer{} }

// Sync uploads all files from filesystem under localRoot to remoteHost:remoteDir.
// Hidden files (names starting with ".") are skipped, matching ReadDir semantics.
// Existing remote files are overwritten; remote-only files are left in place.
func (s *Syncer) Sync(filesystem wfs.FS, localRoot, remoteHost, remoteDir string) error {
	cmd := exec.Command("ssh",
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-s",
		remoteHost,
		"sftp",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("sftp stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("sftp stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ssh sftp: %w", err)
	}

	defer func() { _ = cmd.Wait() }()

	client, err := sftp.NewClientPipe(stdout, stdin)
	if err != nil {
		return fmt.Errorf("sftp client: %w", err)
	}

	defer func() { _ = client.Close() }()

	if err := client.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("mkdir remote %s: %w", remoteDir, err)
	}

	return upload(client, filesystem, localRoot, remoteDir)
}

// upload recursively walks filesystem at localPath and mirrors it to remotePath via SFTP.
func upload(client *sftp.Client, filesystem wfs.FS, localPath, remotePath string) error {
	entries, err := filesystem.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", localPath, err)
	}

	for _, entry := range entries {
		localEntry := filepath.Join(localPath, entry)
		remoteEntry := remotePath + "/" + entry

		isDir, err := filesystem.IsDir(localEntry)
		if err != nil {
			return fmt.Errorf("stat %s: %w", localEntry, err)
		}

		if isDir {
			if err := client.MkdirAll(remoteEntry); err != nil {
				return fmt.Errorf("mkdir remote %s: %w", remoteEntry, err)
			}

			if err := upload(client, filesystem, localEntry, remoteEntry); err != nil {
				return err
			}
		} else {
			data, err := filesystem.ReadFile(localEntry)
			if err != nil {
				return fmt.Errorf("read %s: %w", localEntry, err)
			}

			f, err := client.Create(remoteEntry)
			if err != nil {
				return fmt.Errorf("create remote %s: %w", remoteEntry, err)
			}

			_, writeErr := f.Write(data)
			_ = f.Close()

			if writeErr != nil {
				return fmt.Errorf("write remote %s: %w", remoteEntry, writeErr)
			}
		}
	}

	return nil
}
