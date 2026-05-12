// Package storage provides an object storage abstraction used to persist
// binary assets (e.g. bank transfer proof images) independently of the
// primary relational database.
//
// All implementations must be safe for concurrent use by multiple goroutines.
// The LocalFileStore is intended for local development only.  Production
// deployments must use the S3FileStore backed by Cloudflare R2.
package storage

import (
	"context"
	"fmt"
	"io"
)

// FileStore is the interface for object storage.
//
// Keys are opaque path-like strings (e.g. "bank-transfers/uuid.jpg").
// Callers are responsible for generating collision-resistant keys.
type FileStore interface {
	// Put stores the data from r under key with the given contentType.
	// If an object already exists at key it is overwritten.
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error

	// Get retrieves the object stored under key.  The caller must close the
	// returned ReadCloser.  Returns ErrNotFound if key does not exist.
	Get(ctx context.Context, key string) (rc io.ReadCloser, contentType string, err error)

	// Delete removes the object at key.  Succeeds silently if key does not exist.
	Delete(ctx context.Context, key string) error
}

// ErrNotFound is returned by Get when the requested key does not exist.
var ErrNotFound = fmt.Errorf("storage: object not found")

// New constructs the FileStore selected by cfg.Driver.
// Recognised drivers: "local".
func New(cfg Config) (FileStore, error) {
	switch cfg.Driver {
	case "local":
		return NewLocalFileStore(cfg.LocalDir)
	default:
		return nil, fmt.Errorf("storage: unknown driver %q", cfg.Driver)
	}
}
