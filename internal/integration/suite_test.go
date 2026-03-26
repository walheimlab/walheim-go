//go:build integration

package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh"
)

// ── SSH containers ────────────────────────────────────────────────────────────

const sshUser = "testuser"

// sshTarget holds the address and credentials for a containerised SSH server.
type sshTarget struct {
	Host         string
	Port         int
	User         string
	IdentityFile string
}

// Remote returns the "user@host" string expected by ssh.Client.
func (t sshTarget) Remote() string {
	return t.User + "@" + t.Host
}

var (
	target1 sshTarget
	target2 sshTarget
)

// ── MinIO container ───────────────────────────────────────────────────────────

const (
	minioUser     = "minioadmin"
	minioPassword = "minioadmin"
	minioBucket   = "walheim-test"
)

var minioEndpoint string // "http://host:port"

// ── TestMain ──────────────────────────────────────────────────────────────────

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run contains the actual setup so that deferred cleanup executes before os.Exit.
func run(m *testing.M) int {
	ctx := context.Background()

	keyFile, pubKey, err := generateTestKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: key setup: %v\n", err)
		return 1
	}
	defer os.Remove(keyFile)

	// Verify key round-trip: fingerprint of the parsed private key must match
	// the public key written to authorized_keys.
	if keyBytes, readErr := os.ReadFile(keyFile); readErr == nil {
		if signer, parseErr := gossh.ParsePrivateKey(keyBytes); parseErr == nil {
			privFP := gossh.FingerprintSHA256(signer.PublicKey())
			if pub, _, _, _, parseAuthErr := gossh.ParseAuthorizedKey([]byte(pubKey + "\n")); parseAuthErr == nil {
				pubFP := gossh.FingerprintSHA256(pub)
				fmt.Fprintf(os.Stderr, "DEBUG key fingerprints: priv=%s pub=%s match=%v\n", privFP, pubFP, privFP == pubFP)
			}
		}
	}

	c1, err := startSSHContainer(ctx, pubKey, "ssh-1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: container 1: %v\n", err)
		return 1
	}
	defer c1.Terminate(ctx) //nolint:errcheck

	c2, err := startSSHContainer(ctx, pubKey, "ssh-2")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: container 2: %v\n", err)
		return 1
	}
	defer c2.Terminate(ctx) //nolint:errcheck

	host1, port1, err := containerAddr(ctx, c1, "2222/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: addr container 1: %v\n", err)
		return 1
	}

	host2, port2, err := containerAddr(ctx, c2, "2222/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: addr container 2: %v\n", err)
		return 1
	}

	target1 = sshTarget{Host: host1, Port: port1, User: sshUser, IdentityFile: keyFile}
	target2 = sshTarget{Host: host2, Port: port2, User: sshUser, IdentityFile: keyFile}

	mc, err := startMinIOContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: minio container: %v\n", err)
		return 1
	}
	defer mc.Terminate(ctx) //nolint:errcheck

	minioHost, minioPort, err := containerAddr(ctx, mc, "9000/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: minio addr: %v\n", err)
		return 1
	}

	minioEndpoint = fmt.Sprintf("http://%s:%d", minioHost, minioPort)

	if err := ensureMinIOBucket(); err != nil {
		fmt.Fprintf(os.Stderr, "integration: minio bucket: %v\n", err)
		return 1
	}

	return m.Run()
}

// ── SSH key management ────────────────────────────────────────────────────────

// generateTestKey generates a fresh RSA key pair for each test run.
// The private key is written to a temp file (mode 0600) whose path is returned.
// The caller is responsible for removing the file when done.
func generateTestKey() (keyFile, pubKey string, err error) {
	priv, genErr := rsa.GenerateKey(rand.Reader, 4096)
	if genErr != nil {
		return "", "", fmt.Errorf("generate key: %w", genErr)
	}

	sshPub, marshalErr := ssh.NewPublicKey(&priv.PublicKey)
	if marshalErr != nil {
		return "", "", fmt.Errorf("marshal public key: %w", marshalErr)
	}

	authorisedKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	f, createErr := os.CreateTemp("", "walheim-integration-key-*")
	if createErr != nil {
		return "", "", fmt.Errorf("create temp key file: %w", createErr)
	}
	defer f.Close()

	if chmodErr := os.Chmod(f.Name(), 0o600); chmodErr != nil {
		_ = os.Remove(f.Name())
		return "", "", fmt.Errorf("chmod key file: %w", chmodErr)
	}

	if _, writeErr := f.Write(privPEM); writeErr != nil {
		_ = os.Remove(f.Name())
		return "", "", fmt.Errorf("write private key: %w", writeErr)
	}

	return f.Name(), authorisedKey, nil
}

// ── container helpers ─────────────────────────────────────────────────────────

// containerLogger satisfies both testcontainers.LogConsumer (container
// stdout/stderr) and testcontainers.Logging (lifecycle events) so that all
// output for a given container is prefixed with its name.
type containerLogger struct{ prefix string }

// Accept implements testcontainers.LogConsumer.
func (l containerLogger) Accept(log testcontainers.Log) {
	fmt.Fprintf(os.Stderr, "[%s] %s", l.prefix, log.Content)
}

// Printf implements testcontainers.Logging (lifecycle events).
func (l containerLogger) Printf(format string, v ...interface{}) {
	log.Printf("["+l.prefix+"] "+format, v...)
}

// startSSHContainer starts an SSH server container. The authorised public key
// is passed via the PUBLIC_KEY env var and written into authorized_keys as part
// of the setup script — before sshd starts, so no post-start exec is needed.
func startSSHContainer(ctx context.Context, pubKey, name string) (testcontainers.Container, error) {
	setup := strings.Join([]string{
		`apk add --no-cache openssh >/dev/null 2>&1`,
		`ssh-keygen -A >/dev/null 2>&1`,
		`adduser -D -s /bin/sh testuser`,
		// OpenSSH rejects login (even with pubkey) when the shadow entry starts
		// with '!' (locked). Set a dummy password to unlock the account.
		`printf 'testuser:x\n' | chpasswd`,
		`mkdir -p /home/testuser/.ssh`,
		`chmod 700 /home/testuser/.ssh`,
		`chown testuser:testuser /home/testuser/.ssh`,
		`printf '%s\n' "$PUBLIC_KEY" > /home/testuser/.ssh/authorized_keys`,
		`chmod 600 /home/testuser/.ssh/authorized_keys`,
		`chown testuser:testuser /home/testuser/.ssh/authorized_keys`,
		`exec /usr/sbin/sshd -D -e -p 2222`,
	}, " && ")

	logger := containerLogger{prefix: name}

	req := testcontainers.ContainerRequest{
		Name:         name,
		Image:        "alpine:3.21",
		ExposedPorts: []string{"2222/tcp"},
		Env:          map[string]string{"PUBLIC_KEY": pubKey},
		Cmd:          []string{"/bin/sh", "-c", setup},
		WaitingFor:   wait.ForListeningPort("2222/tcp"),
		LogConsumerCfg: &testcontainers.LogConsumerConfig{
			Consumers: []testcontainers.LogConsumer{logger},
		},
	}

	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           logger,
	})
}

// startMinIOContainer starts a MinIO container.
func startMinIOContainer(ctx context.Context) (testcontainers.Container, error) {
	logger := containerLogger{prefix: "minio"}

	req := testcontainers.ContainerRequest{
		Name:         "minio",
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     minioUser,
			"MINIO_ROOT_PASSWORD": minioPassword,
		},
		Cmd:        []string{"server", "/data"},
		WaitingFor: wait.ForListeningPort("9000/tcp"),
		LogConsumerCfg: &testcontainers.LogConsumerConfig{
			Consumers: []testcontainers.LogConsumer{logger},
		},
	}

	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           logger,
	})
}

// containerAddr returns the host and mapped port for a running container.
// port must be in "number/proto" form, e.g. "2222/tcp" or "9000/tcp".
func containerAddr(ctx context.Context, c testcontainers.Container, port string) (string, int, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("host: %w", err)
	}

	mapped, err := c.MappedPort(ctx, nat.Port(port))
	if err != nil {
		return "", 0, fmt.Errorf("mapped port %s: %w", port, err)
	}

	return host, mapped.Int(), nil
}

// ── MinIO bucket bootstrap ────────────────────────────────────────────────────

// ensureMinIOBucket creates the test bucket in MinIO if it doesn't exist yet.
func ensureMinIOBucket() error {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(minioUser, minioPassword, ""),
		),
	)
	if err != nil {
		return fmt.Errorf("load S3 config: %w", err)
	}

	endpoint := minioEndpoint

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	_, err = client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(minioBucket),
	})
	if err != nil {
		var (
			alreadyOwned  *s3types.BucketAlreadyOwnedByYou
			alreadyExists *s3types.BucketAlreadyExists
		)

		if errors.As(err, &alreadyOwned) || errors.As(err, &alreadyExists) {
			return nil
		}

		return fmt.Errorf("create bucket: %w", err)
	}

	return nil
}
