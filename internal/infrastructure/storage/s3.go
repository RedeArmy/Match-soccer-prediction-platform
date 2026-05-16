package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3FileStore implements FileStore against any S3-compatible object storage
// service (AWS S3, Cloudflare R2, MinIO).
//
// When S3Endpoint is set, path-style addressing is enabled automatically,
// which is required for self-hosted or third-party endpoints such as
// Cloudflare R2 (https://accountid.r2.cloudflarestorage.com) and MinIO.
type S3FileStore struct {
	client *s3.Client
	bucket string
}

// NewS3FileStore constructs an S3FileStore from cfg.
//
// If S3AccessKeyID and S3SecretKey are both non-empty, static credentials are
// used. Otherwise the SDK falls back to the standard credential chain
// (environment variables, shared credentials file, IAM instance profile).
func NewS3FileStore(cfg Config) (*S3FileStore, error) {
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("storage: S3Bucket is required for s3 driver")
	}
	if cfg.S3Region == "" {
		return nil, fmt.Errorf("storage: S3Region is required for s3 driver")
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.S3Region),
	}
	if cfg.S3AccessKeyID != "" && cfg.S3SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.S3AccessKeyID, cfg.S3SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("storage: loading AWS config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.S3Endpoint != "" {
		endpoint := cfg.S3Endpoint
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			// Path-style is required for non-AWS endpoints (R2, MinIO).
			o.UsePathStyle = true
		})
	}

	return &S3FileStore{
		client: s3.NewFromConfig(awsCfg, clientOpts...),
		bucket: cfg.S3Bucket,
	}, nil
}

// Put stores the content of r under key with the given contentType.
// An existing object at key is overwritten.
func (s *S3FileStore) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return fmt.Errorf("storage: put %q: %w", key, err)
	}
	return nil
}

// Get retrieves the object stored under key.
// The caller must close the returned ReadCloser.
// Returns ErrNotFound when the key does not exist.
func (s *S3FileStore) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("storage: get %q: %w", key, err)
	}
	ct := aws.ToString(out.ContentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	return out.Body, ct, nil
}

// Delete removes the object at key.
// Succeeds silently when the key does not exist (S3 DeleteObject is idempotent).
func (s *S3FileStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}
