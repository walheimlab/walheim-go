package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// generateHostKey generates a temporary RSA host key.
func generateHostKey(t *testing.T) gossh.Signer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	signer, err := gossh.ParsePrivateKey(pemBytes)
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	return signer
}

// generateKeyHelper generates an RSA key pair.
func generateKeyHelper(t *testing.T) (privatePEM []byte, public gossh.PublicKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	signer, err := gossh.ParsePrivateKey(pemBytes)
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	return pemBytes, signer.PublicKey()
}

// startMockSSHServer starts an SSH server listening on a local port.
// It returns the listener and the host/port.
func startMockSSHServer(t *testing.T, hostKey gossh.Signer, user string, clientPubKey gossh.PublicKey) (net.Listener, string, int) {
	t.Helper()

	config := &gossh.ServerConfig{
		PublicKeyCallback: func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if conn.User() == user && string(key.Marshal()) == string(clientPubKey.Marshal()) {
				return nil, nil
			}

			return nil, fmt.Errorf("auth failed for user %q", conn.User())
		},
	}
	config.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}

			go func(c net.Conn) {
				_, chans, reqs, err := gossh.NewServerConn(c, config)
				if err != nil {
					return
				}

				// Discard out-of-band requests
				go gossh.DiscardRequests(reqs)

				for newChannel := range chans {
					if newChannel.ChannelType() != "session" {
						_ = newChannel.Reject(gossh.UnknownChannelType, "unknown channel type")
						continue
					}

					channel, requests, err := newChannel.Accept()
					if err != nil {
						continue
					}

					go func(ch gossh.Channel, reqs <-chan *gossh.Request) {
						defer func() {
							_ = ch.Close()
						}()

						for req := range reqs {
							switch req.Type {
							case "exec":
								if len(req.Payload) >= 4 {
									length := uint32(req.Payload[0])<<24 | uint32(req.Payload[1])<<16 | uint32(req.Payload[2])<<8 | uint32(req.Payload[3])

									if len(req.Payload) >= 4+int(length) {
										cmd := string(req.Payload[4 : 4+length])

										switch cmd {
										case "hello":
											_, _ = ch.Write([]byte("world\n"))
											_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
										case "fail":
											_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 1})
										default:
											_, _ = ch.Write([]byte("unknown command\n"))
											_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
										}
									}
								}

								_ = req.Reply(true, nil)

								return
							case "pty-req":
								_ = req.Reply(true, nil)
							case "shell":
								_ = req.Reply(true, nil)

								return
							default:
								_ = req.Reply(false, nil)
							}
						}
					}(channel, requests)
				}
			}(conn)
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)

	return listener, addr.IP.String(), addr.Port
}

func TestNewClient_defaults(t *testing.T) {
	c := NewClient("myhost.local")
	if c.RemoteHost != "myhost.local" {
		t.Errorf("RemoteHost = %q, want %q", c.RemoteHost, "myhost.local")
	}

	if c.ConnectTimeout != 5 {
		t.Errorf("ConnectTimeout = %d, want %d", c.ConnectTimeout, 5)
	}
}

func TestParseRemote_withUser(t *testing.T) {
	u, h := parseRemote("alice@example.com")
	if u != "alice" {
		t.Errorf("user = %q, want %q", u, "alice")
	}

	if h != "example.com" {
		t.Errorf("host = %q, want %q", h, "example.com")
	}
}

func TestParseRemote_noUser_fallsBackToOSUser(t *testing.T) {
	u, h := parseRemote("example.com")
	if u == "" {
		t.Error("expected non-empty user from OS fallback")
	}

	if h != "example.com" {
		t.Errorf("host = %q, want %q", h, "example.com")
	}
}

func TestParseRemote_atInHost(t *testing.T) {
	u, h := parseRemote("user@host@example.com")
	if u != "user@host" {
		t.Errorf("user = %q, want %q", u, "user@host")
	}

	if h != "example.com" {
		t.Errorf("host = %q, want %q", h, "example.com")
	}
}

func TestMockSSH_Success(t *testing.T) {
	hostKey := generateHostKey(t)
	clientKey, clientPub := generateKeyHelper(t)

	listener, host, port := startMockSSHServer(t, hostKey, "testuser", clientPub)

	defer func() { _ = listener.Close() }()

	c := NewClient("testuser@" + host)
	c.Port = port
	c.IdentityKey = clientKey

	// Test connection
	if !c.TestConnection() {
		t.Fatal("expected connection to succeed")
	}

	// Test run output
	out, err := c.RunOutput("hello")
	if err != nil {
		t.Fatalf("RunOutput failed: %v", err)
	}

	if out != "world\n" {
		t.Errorf("expected 'world\n', got %q", out)
	}

	// Test run output fail command
	_, err = c.RunOutput("fail")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Test run output unknown command
	out, err = c.RunOutput("unknown")
	if err != nil {
		t.Fatalf("RunOutput failed: %v", err)
	}

	if out != "unknown command\n" {
		t.Errorf("expected 'unknown command\n', got %q", out)
	}

	// Test Run
	err = c.Run("hello")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Test Exec
	err = c.Exec("hello", false)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
}

func TestMockSSH_ConnectionFailures(t *testing.T) {
	// Wrong port / unreachable
	c := NewClient("testuser@127.0.0.1")
	c.Port = 9999 // likely unused port
	c.ConnectTimeout = 1

	if c.TestConnection() {
		t.Fatal("expected connection to fail")
	}

	_, err := c.RunOutput("hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	err = c.Run("hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	err = c.Exec("hello", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSignerFromBytes_Errors(t *testing.T) {
	_, err := signerFromBytes([]byte("invalid-key"))
	if err == nil {
		t.Fatal("expected error parsing invalid key, got nil")
	}
}

func TestSignerFromFile_Errors(t *testing.T) {
	// Nonexistent file
	_, err := signerFromFile("/nonexistent-file-path-ssh-test")
	if err == nil {
		t.Fatal("expected error reading nonexistent file, got nil")
	}

	// Invalid format file
	tempDir := t.TempDir()
	tempFile := tempDir + "/invalid-key.pem"

	err = os.WriteFile(tempFile, []byte("invalid-key-data"), 0600)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err = signerFromFile(tempFile)
	if err == nil {
		t.Fatal("expected error parsing invalid format key, got nil")
	}
}

func TestMockSSH_IdentityFile(t *testing.T) {
	hostKey := generateHostKey(t)
	clientKey, clientPub := generateKeyHelper(t)

	listener, host, port := startMockSSHServer(t, hostKey, "testuser", clientPub)

	defer func() { _ = listener.Close() }()

	tempDir := t.TempDir()
	keyFile := tempDir + "/client_key"

	err := os.WriteFile(keyFile, clientKey, 0600)
	if err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	c := NewClient("testuser@" + host)
	c.Port = port
	c.IdentityFile = keyFile

	if !c.TestConnection() {
		t.Fatal("expected connection to succeed with IdentityFile")
	}

	out, err := c.RunOutput("hello")
	if err != nil {
		t.Fatalf("RunOutput failed: %v", err)
	}

	if out != "world\n" {
		t.Errorf("expected 'world\n', got %q", out)
	}
}

func TestMockSSH_ExecTTY(t *testing.T) {
	hostKey := generateHostKey(t)
	clientKey, clientPub := generateKeyHelper(t)

	listener, host, port := startMockSSHServer(t, hostKey, "testuser", clientPub)

	defer func() { _ = listener.Close() }()

	c := NewClient("testuser@" + host)
	c.Port = port
	c.IdentityKey = clientKey

	err := c.Exec("hello", true)
	if err != nil {
		t.Fatalf("Exec with TTY failed: %v", err)
	}
}
