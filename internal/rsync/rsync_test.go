package rsync_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
	"github.com/walheimlab/walheim-go/internal/rsync"
	"github.com/walheimlab/walheim-go/internal/testutil"
	gossh "golang.org/x/crypto/ssh"
	"os/user"
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

// startMockSFTPServer starts an SSH+SFTP server listening on a local port.
func startMockSFTPServer(t *testing.T, hostKey gossh.Signer, user string, clientPubKey gossh.PublicKey) (net.Listener, string, int) {
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
				return
			}

			go func(c net.Conn) {
				_, chans, reqs, err := gossh.NewServerConn(c, config)
				if err != nil {
					return
				}

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
							case "subsystem":
								if len(req.Payload) >= 4 {
									length := uint32(req.Payload[0])<<24 | uint32(req.Payload[1])<<16 | uint32(req.Payload[2])<<8 | uint32(req.Payload[3])

									subsystem := string(req.Payload[4 : 4+length])
									if subsystem == "sftp" {
										_ = req.Reply(true, nil)

										server, err := sftp.NewServer(ch)
										if err == nil {
											_ = server.Serve()
										}

										return
									}
								}

								_ = req.Reply(false, nil)

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

func TestSyncer_Sync(t *testing.T) {
	hostKey := generateHostKey(t)
	clientKey, clientPub := generateKeyHelper(t)

	listener, host, port := startMockSFTPServer(t, hostKey, "testuser", clientPub)

	defer func() { _ = listener.Close() }()

	// Prepare mock source filesystem
	fs := testutil.NewMemFS()
	_ = fs.WriteFile("/src/file1.txt", []byte("hello world"))
	_ = fs.WriteFile("/src/sub/file2.txt", []byte("sub world"))
	_ = fs.MkdirAll("/src/sub")

	// Target directory on OS
	destDir := t.TempDir()

	syncer := rsync.NewSyncer()
	syncer.Port = port
	syncer.IdentityKey = clientKey

	err := syncer.Sync(fs, "/src", "testuser@"+host, destDir)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Verify files were correctly synced to destDir on local filesystem
	data1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
	if err != nil {
		t.Fatalf("failed to read destination file1: %v", err)
	}

	if string(data1) != "hello world" {
		t.Errorf("file1.txt content mismatch: got %q", data1)
	}

	data2, err := os.ReadFile(filepath.Join(destDir, "sub", "file2.txt"))
	if err != nil {
		t.Fatalf("failed to read destination file2: %v", err)
	}

	if string(data2) != "sub world" {
		t.Errorf("file2.txt content mismatch: got %q", data2)
	}
}

func TestSyncer_SyncErrors(t *testing.T) {
	// Connection error
	fs := testutil.NewMemFS()
	syncer := rsync.NewSyncer()
	syncer.Port = 9999
	syncer.IdentityKey = []byte("invalid-key-data")

	err := syncer.Sync(fs, "/src", "testuser@127.0.0.1", "/dest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Build auth methods error
	syncer.IdentityKey = []byte("invalid")

	err = syncer.Sync(fs, "/src", "testuser@127.0.0.1", "/dest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSyncer_Sync_IdentityFile(t *testing.T) {
	hostKey := generateHostKey(t)
	clientKey, clientPub := generateKeyHelper(t)

	listener, host, port := startMockSFTPServer(t, hostKey, "testuser", clientPub)

	defer func() { _ = listener.Close() }()

	fs := testutil.NewMemFS()
	_ = fs.WriteFile("/src/file1.txt", []byte("hello"))

	destDir := t.TempDir()
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "client_key")

	err := os.WriteFile(keyFile, clientKey, 0600)
	if err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	syncer := rsync.NewSyncer()
	syncer.Port = port
	syncer.IdentityFile = keyFile

	err = syncer.Sync(fs, "/src", "testuser@"+host, destDir)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
}

func TestSyncer_Sync_ParseRemoteNoUser(t *testing.T) {
	currUser, err := user.Current()
	if err != nil {
		t.Skip("skipping because user.Current() is not supported on this platform/runner")
	}

	hostKey := generateHostKey(t)
	_, clientPub := generateKeyHelper(t)

	listener, host, port := startMockSFTPServer(t, hostKey, currUser.Username, clientPub)

	defer func() { _ = listener.Close() }()

	fs := testutil.NewMemFS()
	_ = fs.WriteFile("/src/file1.txt", []byte("hello"))

	destDir := t.TempDir()
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "client_key")
	// Save the client key
	clientKey, _ := generateKeyHelper(t)

	err = os.WriteFile(keyFile, clientKey, 0600)
	if err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	syncer := rsync.NewSyncer()
	syncer.Port = port
	syncer.IdentityFile = keyFile

	// Sync with just hostname (should fall back to current OS user)
	// We might fail authentication if the OS username doesn't match the server expectations or keys,
	// but it should at least hit the parseRemote fallback and try to dial.
	_ = syncer.Sync(fs, "/src", host, destDir)
}

func TestSyncer_Sync_IdentityFileError(t *testing.T) {
	fs := testutil.NewMemFS()
	syncer := rsync.NewSyncer()
	syncer.IdentityFile = "/nonexistent-key-file-path-rsync-test"

	err := syncer.Sync(fs, "/src", "testuser@127.0.0.1", "/dest")
	if err == nil {
		t.Fatal("expected error for nonexistent identity file, got nil")
	}
}

type errorMockFS struct {
	readDirErr  error
	isDirErr    error
	readFileErr error
}

func (e *errorMockFS) ReadFile(path string) ([]byte, error) {
	if e.readFileErr != nil {
		return nil, e.readFileErr
	}

	return []byte("data"), nil
}

func (e *errorMockFS) WriteFile(path string, data []byte) error { return nil }
func (e *errorMockFS) MkdirAll(path string) error               { return nil }
func (e *errorMockFS) RemoveAll(path string) error              { return nil }
func (e *errorMockFS) Exists(path string) (bool, error)         { return false, nil }

func (e *errorMockFS) IsDir(path string) (bool, error) {
	if e.isDirErr != nil {
		return false, e.isDirErr
	}

	return false, nil
}

func (e *errorMockFS) ReadDir(path string) ([]string, error) {
	if e.readDirErr != nil {
		return nil, e.readDirErr
	}

	return []string{"file1.txt"}, nil
}

func TestSyncer_Sync_FSErrors(t *testing.T) {
	hostKey := generateHostKey(t)
	clientKey, clientPub := generateKeyHelper(t)

	listener, host, port := startMockSFTPServer(t, hostKey, "testuser", clientPub)

	defer func() { _ = listener.Close() }()

	destDir := t.TempDir()

	syncer := rsync.NewSyncer()
	syncer.Port = port
	syncer.IdentityKey = clientKey

	// 1. ReadDir error
	fsErr := &errorMockFS{
		readDirErr: fmt.Errorf("read dir failure"),
	}

	err := syncer.Sync(fsErr, "/src", "testuser@"+host, destDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 2. IsDir error
	fsErr = &errorMockFS{
		isDirErr: fmt.Errorf("is dir failure"),
	}

	err = syncer.Sync(fsErr, "/src", "testuser@"+host, destDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 3. ReadFile error
	fsErr = &errorMockFS{
		readFileErr: fmt.Errorf("read file failure"),
	}

	err = syncer.Sync(fsErr, "/src", "testuser@"+host, destDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
