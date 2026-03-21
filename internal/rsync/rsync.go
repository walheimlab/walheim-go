package rsync

import (
	"os"
	"os/exec"
)

// Syncer wraps rsync for syncing local directories to remote hosts.
type Syncer struct{}

// NewSyncer creates a new Syncer instance.
func NewSyncer() *Syncer {
	return &Syncer{}
}

// Sync runs: rsync -avz --delete <localDir>/ <remoteHost>:<remoteDir>/
// Streams output to stdout/stderr. Returns error if rsync exits non-zero.
func (s *Syncer) Sync(localDir, remoteHost, remoteDir string) error {
	// Ensure trailing slashes for rsync (important for correct syncing)
	if localDir[len(localDir)-1] != '/' {
		localDir = localDir + "/"
	}

	remoteSpec := remoteHost + ":" + remoteDir
	if remoteDir[len(remoteDir)-1] != '/' {
		remoteSpec = remoteSpec + "/"
	}

	args := []string{
		"-avz",
		"--delete",
		localDir,
		remoteSpec,
	}

	cmd := exec.Command("rsync", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
