package ssh

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/user"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// Client holds SSH connection parameters for a remote host.
type Client struct {
	// RemoteHost is "user@hostname" or just "hostname" (uses current OS user).
	RemoteHost string
	// ConnectTimeout in seconds (default 5).
	ConnectTimeout int
	// Port overrides the default SSH port (22). 0 means use the default.
	Port int
	// IdentityFile is the path to a PEM-encoded private key for authentication.
	IdentityFile string
	// HostKeyCallback verifies the server's host key.
	// Defaults to InsecureIgnoreHostKey when nil.
	HostKeyCallback gossh.HostKeyCallback
}

// NewClient creates an SSH client for the given remote host.
func NewClient(remoteHost string) *Client {
	return &Client{
		RemoteHost:     remoteHost,
		ConnectTimeout: 5,
	}
}

// Run executes a command on the remote host, streaming stdout/stderr to
// os.Stdout/os.Stderr. Returns error if the command exits non-zero.
func (c *Client) Run(cmd string) error {
	conn, err := c.dial()
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck

	sess, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer sess.Close() //nolint:errcheck

	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	sess.Stdin = os.Stdin

	return sess.Run(cmd)
}

// RunOutput executes a command and returns stdout as a string.
// Stderr goes to os.Stderr.
func (c *Client) RunOutput(cmd string) (string, error) {
	conn, err := c.dial()
	if err != nil {
		return "", err
	}
	defer conn.Close() //nolint:errcheck

	sess, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer sess.Close() //nolint:errcheck

	var stdout bytes.Buffer

	sess.Stdout = &stdout
	sess.Stderr = os.Stderr

	err = sess.Run(cmd)

	return stdout.String(), err
}

// Exec runs a command on the remote host with stdin/stdout/stderr forwarded,
// optionally requesting a PTY for interactive use (log-following, shells).
func (c *Client) Exec(cmd string, tty bool) error {
	conn, err := c.dial()
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck

	sess, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer sess.Close() //nolint:errcheck

	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	sess.Stdin = os.Stdin

	if tty {
		w, h, _ := term.GetSize(int(os.Stdin.Fd()))
		if w == 0 {
			w = 80
		}

		if h == 0 {
			h = 24
		}

		modes := gossh.TerminalModes{
			gossh.ECHO:          1,
			gossh.TTY_OP_ISPEED: 14400,
			gossh.TTY_OP_OSPEED: 14400,
		}

		if reqErr := sess.RequestPty("xterm-256color", h, w, modes); reqErr != nil {
			return fmt.Errorf("request pty: %w", reqErr)
		}

		if term.IsTerminal(int(os.Stdin.Fd())) {
			oldState, rawErr := term.MakeRaw(int(os.Stdin.Fd()))
			if rawErr == nil {
				defer term.Restore(int(os.Stdin.Fd()), oldState) //nolint:errcheck
			}
		}
	}

	return sess.Run(cmd)
}

// TestConnection checks if the SSH server is reachable and accepts
// authentication. Returns true/false (no error propagated).
func (c *Client) TestConnection() bool {
	conn, err := c.dial()
	if err != nil {
		return false
	}

	conn.Close() //nolint:errcheck

	return true
}

// dial opens an authenticated SSH connection to the remote host.
func (c *Client) dial() (*gossh.Client, error) {
	sshUser, host := parseRemote(c.RemoteHost)

	port := 22
	if c.Port != 0 {
		port = c.Port
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	timeout := 5 * time.Second
	if c.ConnectTimeout > 0 {
		timeout = time.Duration(c.ConnectTimeout) * time.Second
	}

	auth, err := c.authMethods()
	if err != nil {
		return nil, err
	}

	hkc := c.HostKeyCallback
	if hkc == nil {
		hkc = gossh.InsecureIgnoreHostKey() //nolint:gosec
	}

	cfg := &gossh.ClientConfig{
		User:            sshUser,
		Auth:            auth,
		HostKeyCallback: hkc,
		Timeout:         timeout,
	}

	conn, dialErr := gossh.Dial("tcp", addr, cfg)
	if dialErr != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, dialErr)
	}

	return conn, nil
}

// authMethods builds the list of SSH auth methods from the client config.
func (c *Client) authMethods() ([]gossh.AuthMethod, error) {
	if c.IdentityFile == "" {
		return nil, nil
	}

	key, err := os.ReadFile(c.IdentityFile)
	if err != nil {
		return nil, fmt.Errorf("read identity file: %w", err)
	}

	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	// For RSA keys, restrict to rsa-sha2-256/512. Modern OpenSSH servers
	// (8.8+) disable the legacy ssh-rsa (SHA-1) algorithm by default.
	if signer.PublicKey().Type() == gossh.KeyAlgoRSA {
		if algSigner, ok := signer.(gossh.AlgorithmSigner); ok {
			signer, err = gossh.NewSignerWithAlgorithms(algSigner,
				[]string{gossh.KeyAlgoRSASHA256, gossh.KeyAlgoRSASHA512})
			if err != nil {
				return nil, fmt.Errorf("rsa algorithm signer: %w", err)
			}
		}
	}

	return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil
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
