package storage_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
)

func TestNewS3FileStore_MissingBucket(t *testing.T) {
	_, err := storage.NewS3FileStore(context.Background(), storage.Config{
		Driver:   "s3",
		S3Region: "us-east-1",
	})
	if err == nil {
		t.Fatal("expected error for missing S3Bucket, got nil")
	}
}

func TestNewS3FileStore_MissingRegion(t *testing.T) {
	_, err := storage.NewS3FileStore(context.Background(), storage.Config{
		Driver:   "s3",
		S3Bucket: "my-bucket",
	})
	if err == nil {
		t.Fatal("expected error for missing S3Region, got nil")
	}
}

func TestNew_S3DriverConstructsStore(t *testing.T) {
	store, err := storage.New(context.Background(), storage.Config{
		Driver:        "s3",
		S3Bucket:      "test-bucket",
		S3Region:      "us-east-1",
		S3AccessKeyID: "key",
		S3SecretKey:   "secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil FileStore, got nil")
	}
}

func TestNew_UnknownDriverReturnsError(t *testing.T) {
	_, err := storage.New(context.Background(), storage.Config{Driver: "gcs"})
	if err == nil {
		t.Fatal("expected error for unknown driver, got nil")
	}
}
