package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalFileStore persists objects as files under a base directory.
// It is intended for local development only.
type LocalFileStore struct {
	baseDir string
}

// NewLocalFileStore returns a LocalFileStore rooted at baseDir.
// The directory is created if it does not already exist.
func NewLocalFileStore(baseDir string) (*LocalFileStore, error) {
	if baseDir == "" {
		baseDir = "uploads"
	}
	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return nil, fmt.Errorf("storage: create base dir %q: %w", baseDir, err)
	}
	return &LocalFileStore{baseDir: baseDir}, nil
}

// Put stores r under key, creating parent directories as needed.
// contentType is accepted for interface compatibility but not stored on disk.
func (s *LocalFileStore) Put(_ context.Context, key, _ string, r io.Reader, _ int64) error {
	dest := s.path(key)
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("storage: mkdir for %q: %w", key, err)
	}
	f, err := os.Create(dest) //nolint:gosec // G304: path constructed by LocalFileStore.path() from a config base dir and a sanitised key
	if err != nil {
		return fmt.Errorf("storage: create %q: %w", key, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("storage: write %q: %w", key, err)
	}
	return nil
}

// Get returns the file stored under key as a ReadCloser.
// The caller must close the returned ReadCloser.
func (s *LocalFileStore) Get(_ context.Context, key string) (io.ReadCloser, string, error) {
	f, err := os.Open(s.path(key))
	if os.IsNotExist(err) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("storage: open %q: %w", key, err)
	}
	return f, contentTypeFromKey(key), nil
}

// Delete removes the file at key. Succeeds silently if the file does not exist.
func (s *LocalFileStore) Delete(_ context.Context, key string) error {
	err := os.Remove(s.path(key))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}

func (s *LocalFileStore) path(key string) string {
	return filepath.Join(s.baseDir, filepath.FromSlash(key))
}

func contentTypeFromKey(key string) string {
	ext := strings.ToLower(filepath.Ext(key))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".pdf":
		return "application/pdf"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
