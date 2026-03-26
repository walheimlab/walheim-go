package ssh

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/user"
	"strings"
	"time"

	sshconfig "github.com/kevinburke/ssh_config"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
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
	// IdentityKey is a PEM-encoded private key used for authentication.
	// Takes precedence over IdentityFile and all other auth methods.
	IdentityKey []byte
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
// Resolution order for each parameter:
//  1. Explicit Client field (highest priority)
//  2. ~/.ssh/config match for the host
//  3. Built-in defaults (port 22, current OS user, default key paths, SSH agent)
func (c *Client) dial() (*gossh.Client, error) {
	sshUser, host := parseRemote(c.RemoteHost)

	// Apply ~/.ssh/config overrides for fields not explicitly set on the Client.
	cfgHost, cfgUser, cfgPort, cfgIdentityFile := sshConfigLookup(host)
	if cfgHost != "" {
		host = cfgHost
	}

	if sshUser == "" && cfgUser != "" {
		sshUser = cfgUser
	}

	port := 22
	if c.Port != 0 {
		port = c.Port
	} else if cfgPort > 0 {
		port = cfgPort
	}

	identityFile := c.IdentityFile
	if identityFile == "" {
		identityFile = cfgIdentityFile
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	timeout := 5 * time.Second
	if c.ConnectTimeout > 0 {
		timeout = time.Duration(c.ConnectTimeout) * time.Second
	}

	auth, err := authMethods(c.IdentityKey, identityFile)
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

// sshConfigLookup reads ~/.ssh/config and returns overrides for the given host.
// Returns empty/zero values for any field not found in the config.
func sshConfigLookup(host string) (hostname, sshUser string, port int, identityFile string) {
	u, err := user.Current()
	if err != nil {
		return
	}

	f, err := os.Open(u.HomeDir + "/.ssh/config")
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck

	cfg, err := sshconfig.Decode(f)
	if err != nil {
		return
	}

	if h, _ := cfg.Get(host, "Hostname"); h != "" && h != host {
		hostname = h
	}

	sshUser, _ = cfg.Get(host, "User")

	if p, _ := cfg.Get(host, "Port"); p != "" && p != "22" {
		fmt.Sscanf(p, "%d", &port) //nolint:errcheck
	}

	identityFile, _ = cfg.Get(host, "IdentityFile")
	if identityFile != "" && strings.HasPrefix(identityFile, "~/") {
		identityFile = u.HomeDir + identityFile[1:]
	}

	return
}

// authMethods builds SSH auth methods in priority order:
//  1. IdentityKey bytes (highest — explicit key from namespace spec)
//  2. Explicit identity file path
//  3. Default key paths (~/.ssh/id_ed25519, id_rsa, id_ecdsa)
//  4. SSH agent via SSH_AUTH_SOCK
func authMethods(identityKey []byte, identityFile string) ([]gossh.AuthMethod, error) {
	var methods []gossh.AuthMethod

	if len(identityKey) > 0 {
		m, err := signerFromBytes(identityKey)
		if err != nil {
			return nil, fmt.Errorf("namespace private key: %w", err)
		}

		return []gossh.AuthMethod{gossh.PublicKeys(m)}, nil
	}

	if identityFile != "" {
		m, err := signerFromFile(identityFile)
		if err != nil {
			return nil, err
		}

		methods = append(methods, gossh.PublicKeys(m))
	} else {
		// Fall back to well-known default key paths.
		if u, err := user.Current(); err == nil {
			for _, path := range []string{
				u.HomeDir + "/.ssh/id_ed25519",
				u.HomeDir + "/.ssh/id_rsa",
				u.HomeDir + "/.ssh/id_ecdsa",
			} {
				m, err := signerFromFile(path)
				if err != nil {
					continue // key doesn't exist or can't be read — skip silently
				}

				methods = append(methods, gossh.PublicKeys(m))
			}
		}
	}

	// Always try the SSH agent as a final fallback.
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			methods = append(methods, gossh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}

	return methods, nil
}

// signerFromBytes parses a PEM private key from raw bytes and returns a gossh.Signer.
func signerFromBytes(key []byte) (gossh.Signer, error) {
	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	if signer.PublicKey().Type() == gossh.KeyAlgoRSA {
		if algSigner, ok := signer.(gossh.AlgorithmSigner); ok {
			signer, err = gossh.NewSignerWithAlgorithms(algSigner,
				[]string{gossh.KeyAlgoRSASHA256, gossh.KeyAlgoRSASHA512})
			if err != nil {
				return nil, fmt.Errorf("rsa algorithm signer: %w", err)
			}
		}
	}

	return signer, nil
}

// signerFromFile loads a PEM private key and returns a gossh.Signer.
// For RSA keys, it restricts algorithms to rsa-sha2-256/512 to avoid
// the legacy SHA-1 algorithm disabled in modern OpenSSH servers (8.8+).
func signerFromFile(path string) (gossh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity file %s: %w", path, err)
	}

	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key %s: %w", path, err)
	}

	if signer.PublicKey().Type() == gossh.KeyAlgoRSA {
		if algSigner, ok := signer.(gossh.AlgorithmSigner); ok {
			signer, err = gossh.NewSignerWithAlgorithms(algSigner,
				[]string{gossh.KeyAlgoRSASHA256, gossh.KeyAlgoRSASHA512})
			if err != nil {
				return nil, fmt.Errorf("rsa algorithm signer %s: %w", path, err)
			}
		}
	}

	return signer, nil
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
