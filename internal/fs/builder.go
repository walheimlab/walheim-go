package fs

import (
	"github.com/walheimlab/walheim-go/internal/config"
)

// FromContext builds the appropriate FS implementation for the given context.
// For local contexts it returns a LocalFS and the context's dataDir.
// For S3 contexts it returns an S3FS and an empty dataDir (keys are relative to the bucket prefix).
func FromContext(ctx *config.Context) (FS, string, error) {
	if !ctx.IsS3() {
		return NewLocalFS(), ctx.DataDir, nil
	}

	s3cfg := ctx.S3
	fsImpl, err := NewS3FS(S3FSConfig{
		Endpoint:        s3cfg.Endpoint,
		Region:          s3cfg.Region,
		Bucket:          s3cfg.Bucket,
		Prefix:          s3cfg.Prefix,
		AccessKeyID:     s3cfg.AccessKeyID,
		SecretAccessKey: s3cfg.SecretAccessKey,
	})
	if err != nil {
		return nil, "", err
	}
	return fsImpl, "", nil
}
