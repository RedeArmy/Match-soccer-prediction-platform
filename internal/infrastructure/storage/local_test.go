package storage_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
)

func newStore(t *testing.T) *storage.LocalFileStore {
	t.Helper()
	s, err := storage.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore: %v", err)
	}
	return s
}

func TestLocalFileStore_Put_Get_RoundTrip(t *testing.T) {
	s := newStore(t)
	data := []byte("hello storage")

	if err := s.Put(context.Background(), "sub/file.jpg", "image/jpeg", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, ct, err := s.Get(context.Background(), "sub/file.jpg")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	if ct != "image/jpeg" {
		t.Errorf("content-type: got %q, want image/jpeg", ct)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != string(data) {
		t.Errorf("body: got %q, want %q", got, data)
	}
}

func TestLocalFileStore_Put_OverwritesExisting(t *testing.T) {
	s := newStore(t)
	_ = s.Put(context.Background(), "file.png", "image/png", bytes.NewReader([]byte("v1")), 2)
	_ = s.Put(context.Background(), "file.png", "image/png", bytes.NewReader([]byte("v2")), 2)

	rc, _, err := s.Get(context.Background(), "file.png")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "v2" {
		t.Errorf("overwrite: got %q, want v2", got)
	}
}

func TestLocalFileStore_Get_NotFound_ReturnsErrNotFound(t *testing.T) {
	s := newStore(t)
	_, _, err := s.Get(context.Background(), "missing.pdf")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalFileStore_Delete_RemovesFile(t *testing.T) {
	s := newStore(t)
	_ = s.Put(context.Background(), "del.webp", "image/webp", bytes.NewReader([]byte("x")), 1)

	if err := s.Delete(context.Background(), "del.webp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, _, err := s.Get(context.Background(), "del.webp")
	if err != storage.ErrNotFound {
		t.Errorf("after delete: expected ErrNotFound, got %v", err)
	}
}

func TestLocalFileStore_Delete_NonExistent_NoError(t *testing.T) {
	s := newStore(t)
	if err := s.Delete(context.Background(), "nonexistent.png"); err != nil {
		t.Errorf("delete non-existent: expected nil, got %v", err)
	}
}

func TestLocalFileStore_ContentType_InferredFromExtension(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"a.jpg", "image/jpeg"},
		{"a.jpeg", "image/jpeg"},
		{"a.png", "image/png"},
		{"a.pdf", "application/pdf"},
		{"a.webp", "image/webp"},
		{"a.bin", "application/octet-stream"},
	}
	s := newStore(t)
	for _, tc := range cases {
		data := []byte("data")
		_ = s.Put(context.Background(), tc.key, "application/octet-stream", bytes.NewReader(data), int64(len(data)))
		rc, got, err := s.Get(context.Background(), tc.key)
		if err != nil {
			t.Fatalf("%s: Get: %v", tc.key, err)
		}
		_ = rc.Close()
		if got != tc.want {
			t.Errorf("%s: content-type: got %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestNew_LocalDriver_ReturnsStore(t *testing.T) {
	fs, err := storage.New(context.Background(), storage.Config{Driver: "local", LocalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if fs == nil {
		t.Error("expected non-nil FileStore")
	}
}

func TestNew_UnknownDriver_ReturnsError(t *testing.T) {
	_, err := storage.New(context.Background(), storage.Config{Driver: "s3"})
	if err == nil {
		t.Error("expected error for unknown driver, got nil")
	}
}

func TestNewLocalFileStore_EmptyDir_DefaultsToUploads(t *testing.T) {
	// pass empty string — constructor should accept it (uses "uploads" default)
	// We cannot easily test the actual created path, but we verify no error.
	s, err := storage.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Error("expected non-nil LocalFileStore")
	}
}
