package fs_test

import (
	"testing"

	"github.com/walheimlab/walheim-go/internal/config"
	"github.com/walheimlab/walheim-go/internal/fs"
)

func TestFromContext_Local(t *testing.T) {
	ctx := &config.Context{
		Name:    "local-test",
		DataDir: "/tmp/data",
	}

	filesystem, dataDir, err := fs.FromContext(ctx)
	if err != nil {
		t.Fatalf("FromContext failed: %v", err)
	}

	if dataDir != "/tmp/data" {
		t.Errorf("expected dataDir '/tmp/data', got %q", dataDir)
	}

	// Verify it's a LocalFS
	if _, ok := filesystem.(*fs.LocalFS); !ok {
		t.Errorf("expected *fs.LocalFS, got %T", filesystem)
	}
}

func TestFromContext_S3(t *testing.T) {
	ctx := &config.Context{
		Name: "s3-test",
		S3: &config.S3Config{
			Endpoint:        "http://localhost:9000",
			Region:          "us-east-1",
			Bucket:          "mybucket",
			Prefix:          "myprefix",
			AccessKeyID:     "minio",
			SecretAccessKey: "miniosecret",
		},
	}

	filesystem, dataDir, err := fs.FromContext(ctx)
	if err != nil {
		t.Fatalf("FromContext failed: %v", err)
	}

	if dataDir != "" {
		t.Errorf("expected empty dataDir, got %q", dataDir)
	}

	// Verify it's an S3FS
	if _, ok := filesystem.(*fs.S3FS); !ok {
		t.Errorf("expected *fs.S3FS, got %T", filesystem)
	}
}

func TestFromContext_S3NoCredentials(t *testing.T) {
	ctx := &config.Context{
		Name: "s3-test-no-creds",
		S3: &config.S3Config{
			Endpoint: "http://localhost:9000",
			Region:   "us-east-1",
			Bucket:   "mybucket",
			Prefix:   "myprefix",
		},
	}

	filesystem, dataDir, err := fs.FromContext(ctx)
	if err != nil {
		t.Fatalf("FromContext failed: %v", err)
	}

	if dataDir != "" {
		t.Errorf("expected empty dataDir, got %q", dataDir)
	}

	if _, ok := filesystem.(*fs.S3FS); !ok {
		t.Errorf("expected *fs.S3FS, got %T", filesystem)
	}
}
