package rsync

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"

	wfs "github.com/walheimlab/walheim-go/internal/fs"
)

// Syncer uploads files from a walheim FS to a remote host via SFTP over a
// pure-Go SSH connection — no system ssh binary required.
type Syncer struct {
	// Port overrides the default SSH port (22). 0 means use the default.
	Port int
	// IdentityFile is the path to a PEM-encoded private key for authentication.
	IdentityFile string
}

// NewSyncer creates a new Syncer.
func NewSyncer() *Syncer { return &Syncer{} }

// Sync uploads all files from filesystem under localRoot to remoteHost:remoteDir.
// Hidden files (names starting with ".") are skipped, matching ReadDir semantics.
// Existing remote files are overwritten; remote-only files are left in place.
func (s *Syncer) Sync(filesystem wfs.FS, localRoot, remoteHost, remoteDir string) error {
	sshUser, host := parseRemote(remoteHost)

	port := 22
	if s.Port != 0 {
		port = s.Port
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	var authMethods []gossh.AuthMethod

	if s.IdentityFile != "" {
		key, err := os.ReadFile(s.IdentityFile)
		if err != nil {
			return fmt.Errorf("read identity file: %w", err)
		}

		signer, err := gossh.ParsePrivateKey(key)
		if err != nil {
			return fmt.Errorf("parse private key: %w", err)
		}

		// For RSA keys, restrict to rsa-sha2-256/512. Modern OpenSSH servers
		// (8.8+) disable the legacy ssh-rsa (SHA-1) algorithm by default.
		if signer.PublicKey().Type() == gossh.KeyAlgoRSA {
			if algSigner, ok := signer.(gossh.AlgorithmSigner); ok {
				signer, err = gossh.NewSignerWithAlgorithms(algSigner,
					[]string{gossh.KeyAlgoRSASHA256, gossh.KeyAlgoRSASHA512})
				if err != nil {
					return fmt.Errorf("rsa algorithm signer: %w", err)
				}
			}
		}

		authMethods = append(authMethods, gossh.PublicKeys(signer))
	}

	cfg := &gossh.ClientConfig{
		User:            sshUser,
		Auth:            authMethods,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         5 * time.Second,
	}

	conn, err := gossh.Dial("tcp", addr, cfg)
	if err != nil {
		return fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	defer conn.Close() //nolint:errcheck

	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("sftp client: %w", err)
	}

	defer client.Close() //nolint:errcheck

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

// parseRemote splits "user@host" into (user, host).
// If no user is present, the current OS username is used.
func parseRemote(remote string) (sshUser, host string) {
	if idx := strings.LastIndex(remote, "@"); idx >= 0 {
		return remote[:idx], remote[idx+1:]
	}

	if u, err := user.Current(); err == nil {
		return u.Username, remote
	}

	return "", remote
}
