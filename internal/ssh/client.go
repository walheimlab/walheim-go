package ssh

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// Client holds SSH connection parameters for a remote host.
type Client struct {
	// e.g. "user@hostname" or just "hostname"
	RemoteHost string
	// ConnectTimeout in seconds (default 5)
	ConnectTimeout int
	// BatchMode disables password prompts (default true)
	BatchMode bool
}

// NewClient creates an SSH client for the given remote host.
func NewClient(remoteHost string) *Client {
	return &Client{
		RemoteHost:     remoteHost,
		ConnectTimeout: 5,
		BatchMode:      true,
	}
}

// Run executes a command on the remote host, streaming stdout/stderr to os.Stdout/os.Stderr.
// Returns error if the command exits non-zero.
func (c *Client) Run(cmd string) error {
	args := c.buildArgs(cmd, false)
	sshCmd := exec.Command("ssh", args...)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	sshCmd.Stdin = os.Stdin

	return sshCmd.Run()
}

// RunOutput executes a command and returns stdout as a string. Stderr is discarded.
func (c *Client) RunOutput(cmd string) (string, error) {
	args := c.buildArgs(cmd, false)
	sshCmd := exec.Command("ssh", args...)

	var stdout bytes.Buffer

	sshCmd.Stdout = &stdout
	sshCmd.Stderr = os.Stderr
	sshCmd.Stdin = os.Stdin

	err := sshCmd.Run()

	return stdout.String(), err
}

// Exec replaces the current process with an SSH session (for log-following, interactive exec).
// Uses syscall.Exec under the hood.
func (c *Client) Exec(cmd string, tty bool) error {
	args := c.buildArgs(cmd, tty)

	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found in PATH: %w", err)
	}

	// syscall.Exec replaces the current process with the SSH process
	// This ensures signals and TTY are properly inherited
	err = syscall.Exec(sshPath, append([]string{"ssh"}, args...), os.Environ())

	return fmt.Errorf("syscall.Exec failed: %w", err)
}

// TestConnection checks if SSH is reachable. Returns true/false (no error propagated).
func (c *Client) TestConnection() bool {
	return exec.Command("ssh", c.buildArgs("true", false)...).Run() == nil
}

// buildArgs returns the ssh argument slice for the given remote command.
// Handles -o ConnectTimeout=N, -o BatchMode=yes, -o StrictHostKeyChecking=accept-new, and the remote command.
func (c *Client) buildArgs(cmd string, tty bool) []string {
	var args []string

	// Connection timeout
	args = append(args, "-o", "ConnectTimeout="+strconv.Itoa(c.ConnectTimeout))

	// Batch mode (no password prompts)
	if c.BatchMode {
		args = append(args, "-o", "BatchMode=yes")
	}

	// Always accept new host keys to prevent interactive prompts
	args = append(args, "-o", "StrictHostKeyChecking=accept-new")

	// TTY allocation
	if tty {
		args = append(args, "-t")
	}

	// Remote host
	args = append(args, c.RemoteHost)

	// Remote command
	args = append(args, cmd)

	return args
}
