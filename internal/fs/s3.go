package fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
)

// S3FSConfig holds configuration for constructing an S3FS.
type S3FSConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	Prefix          string
	AccessKeyID     string
	SecretAccessKey string
}

// S3FS implements the FS interface backed by an S3-compatible object store.
// Supports Cloudflare R2, DigitalOcean Spaces, MinIO, and standard AWS S3.
type S3FS struct {
	client *s3.Client
	bucket string
	prefix string // optional key prefix, no trailing slash
}

// NewS3FS creates a new S3FS from the given config.
// If AccessKeyID and SecretAccessKey are non-empty, static credentials are used.
// Otherwise, the AWS SDK default credential chain is used (env vars, ~/.aws/credentials, etc.).
func NewS3FS(cfg S3FSConfig) (*S3FS, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("load S3 config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, s3.WithEndpointResolverV2(
			&staticEndpointResolver{rawURL: cfg.Endpoint},
		))
		// Enable path-style addressing for custom endpoints so requests go to
		// <endpoint>/<bucket>/key instead of <bucket>.<endpoint>/key.
		// This is required for MinIO and most self-hosted S3-compatible services,
		// and works equally well with Cloudflare R2 and DigitalOcean Spaces.
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	return &S3FS{
		client: s3.NewFromConfig(awsCfg, clientOpts...),
		bucket: cfg.Bucket,
		prefix: strings.TrimSuffix(cfg.Prefix, "/"),
	}, nil
}

// key converts an FS path to the full S3 object key, applying the prefix.
// Paths are forward-slash normalized for cross-platform compatibility.
func (s *S3FS) key(p string) string {
	if p == "" {
		if s.prefix != "" {
			return s.prefix
		}
		return ""
	}
	// Normalize to forward slashes (important on Windows)
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	p = strings.TrimPrefix(p, "/")
	if s.prefix == "" {
		return p
	}
	return s.prefix + "/" + p
}

// Ping verifies connectivity by performing a HeadBucket request.
// Returns an error if the bucket is unreachable or credentials are invalid.
func (s *S3FS) Ping() error {
	_, err := s.client.HeadBucket(context.Background(), &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return fmt.Errorf("S3 bucket %q is not accessible: %w", s.bucket, err)
	}
	return nil
}

// ReadFile reads a file from S3 by key.
func (s *S3FS) ReadFile(p string) ([]byte, error) {
	out, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(p)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("open %s: no such key", p)
		}
		return nil, fmt.Errorf("S3 GetObject %s: %w", p, err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

// WriteFile writes data to S3. PutObject is effectively atomic per-key.
func (s *S3FS) WriteFile(p string, data []byte) error {
	_, err := s.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(s.key(p)),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return fmt.Errorf("S3 PutObject %s: %w", p, err)
	}
	return nil
}

// MkdirAll is a no-op for S3: key hierarchies are implicit.
func (s *S3FS) MkdirAll(_ string) error { return nil }

// RemoveAll deletes all objects under path/ as well as the exact key path.
func (s *S3FS) RemoveAll(p string) error {
	prefix := s.key(p) + "/"
	var toDelete []types.ObjectIdentifier

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return fmt.Errorf("S3 list for RemoveAll %s: %w", p, err)
		}
		for _, obj := range page.Contents {
			toDelete = append(toDelete, types.ObjectIdentifier{Key: obj.Key})
		}
	}

	// Also delete the exact key in case it's a plain file (not a "directory")
	exactKey := s.key(p)
	toDelete = append(toDelete, types.ObjectIdentifier{Key: aws.String(exactKey)})

	// Batch delete in chunks of 1000 (S3 limit)
	for len(toDelete) > 0 {
		n := len(toDelete)
		if n > 1000 {
			n = 1000
		}
		batch := toDelete[:n]
		toDelete = toDelete[n:]

		_, err := s.client.DeleteObjects(context.Background(), &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{Objects: batch, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("S3 DeleteObjects: %w", err)
		}
	}
	return nil
}

// Exists reports whether a key or key-prefix exists in S3.
func (s *S3FS) Exists(p string) (bool, error) {
	_, err := s.client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(p)),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return s.hasChildren(p)
	}
	return false, fmt.Errorf("S3 HeadObject %s: %w", p, err)
}

// IsDir reports whether path is a "directory" in S3 (i.e. has objects under it as a prefix).
func (s *S3FS) IsDir(p string) (bool, error) {
	// If the exact key exists, it's a file, not a directory
	_, err := s.client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(p)),
	})
	if err == nil {
		return false, nil
	}
	if !isNotFound(err) {
		return false, fmt.Errorf("S3 HeadObject %s: %w", p, err)
	}
	return s.hasChildren(p)
}

// ReadDir returns a sorted list of non-hidden immediate child names under path.
// Uses S3 list with delimiter to emulate directory listing.
func (s *S3FS) ReadDir(p string) ([]string, error) {
	prefix := s.key(p) + "/"
	var names []string
	seen := map[string]bool{}

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf("S3 ReadDir %s: %w", p, err)
		}

		// CommonPrefixes = immediate "subdirectories" (e.g. "prefix/subdir/")
		for _, cp := range page.CommonPrefixes {
			if cp.Prefix == nil {
				continue
			}
			// Strip the leading prefix and trailing slash to get the child name
			rel := strings.TrimSuffix(strings.TrimPrefix(*cp.Prefix, prefix), "/")
			if rel != "" && !strings.HasPrefix(rel, ".") && !seen[rel] {
				names = append(names, rel)
				seen[rel] = true
			}
		}
	}

	sort.Strings(names)
	return names, nil
}

// hasChildren checks whether any objects exist under path/ as a prefix.
func (s *S3FS) hasChildren(p string) (bool, error) {
	out, err := s.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(s.key(p) + "/"),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return false, fmt.Errorf("S3 list %s: %w", p, err)
	}
	return len(out.Contents) > 0, nil
}

// isNotFound returns true for S3 404-style errors.
func isNotFound(err error) bool {
	var noSuchKey *types.NoSuchKey
	var notFound *types.NotFound
	return errors.As(err, &noSuchKey) || errors.As(err, &notFound)
}

// staticEndpointResolver implements s3.EndpointResolverV2 for custom S3-compatible endpoints.
type staticEndpointResolver struct {
	rawURL string
}

func (r *staticEndpointResolver) ResolveEndpoint(_ context.Context, _ s3.EndpointParameters) (smithyendpoints.Endpoint, error) {
	u, err := url.Parse(r.rawURL)
	if err != nil {
		return smithyendpoints.Endpoint{}, fmt.Errorf("invalid S3 endpoint URL %q: %w", r.rawURL, err)
	}
	return smithyendpoints.Endpoint{URI: *u}, nil
}
