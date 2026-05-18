package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
)

// newTestS3Store spins up an httptest server and returns an S3FileStore that
// points at it via the S3Endpoint override. All S3 requests use path-style
// addressing (/{bucket}/{key}) so no DNS is involved.
func newTestS3Store(t *testing.T, handler http.Handler) (*storage.S3FileStore, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	store, err := storage.NewS3FileStore(storage.Config{
		S3Bucket:      "test-bucket",
		S3Region:      "us-east-1",
		S3Endpoint:    srv.URL,
		S3AccessKeyID: "test-key",
		S3SecretKey:   "test-secret",
	})
	if err != nil {
		t.Fatalf("NewS3FileStore: %v", err)
	}
	return store, srv
}

func TestS3FileStore_Put_Success(t *testing.T) {
	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	err := store.Put(context.Background(), "uploads/proof.jpg", "image/jpeg", strings.NewReader("data"), 4)
	if err != nil {
		t.Fatalf("Put: unexpected error: %v", err)
	}
}

func TestS3FileStore_Put_ServerError(t *testing.T) {
	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	err := store.Put(context.Background(), "uploads/proof.jpg", "image/jpeg", strings.NewReader("data"), 4)
	if err == nil {
		t.Fatal("Put: expected error for server 500, got nil")
	}
}

func TestS3FileStore_Get_Success(t *testing.T) {
	const body = "file-content"
	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
	}))

	rc, ct, err := store.Get(context.Background(), "uploads/proof.jpg")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	defer rc.Close()

	if ct != "image/jpeg" {
		t.Errorf("Get: content-type = %q, want %q", ct, "image/jpeg")
	}
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Errorf("Get: body = %q, want %q", got, body)
	}
}

func TestS3FileStore_Get_EmptyContentTypeFallsBack(t *testing.T) {
	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No explicit Content-Type set. Binary body causes the httptest server to
		// sniff application/octet-stream, which is the expected fallback value.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0x00, 0x00, 0x00, 0x00})
	}))

	rc, ct, err := store.Get(context.Background(), "uploads/proof.jpg")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	defer rc.Close()
	if ct != "application/octet-stream" {
		t.Errorf("Get: content-type fallback = %q, want application/octet-stream", ct)
	}
}

func TestS3FileStore_Get_ServerError(t *testing.T) {
	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, _, err := store.Get(context.Background(), "uploads/proof.jpg")
	if err == nil {
		t.Fatal("Get: expected error for server 500, got nil")
	}
	if errors.Is(err, storage.ErrNotFound) {
		t.Fatal("Get: expected non-ErrNotFound error for server 500, got ErrNotFound")
	}
}

func TestS3FileStore_Get_NotFound(t *testing.T) {
	noSuchKeyXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)

	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(noSuchKeyXML)
	}))

	_, _, err := store.Get(context.Background(), "uploads/missing.jpg")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get: expected ErrNotFound, got %v", err)
	}
}

func TestS3FileStore_Delete_Success(t *testing.T) {
	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := store.Delete(context.Background(), "uploads/proof.jpg"); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}
}

func TestS3FileStore_Delete_Error(t *testing.T) {
	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	if err := store.Delete(context.Background(), "uploads/proof.jpg"); err == nil {
		t.Fatal("Delete: expected error for server 500, got nil")
	}
}

func TestS3FileStore_Put_ReaderError(t *testing.T) {
	store, _ := newTestS3Store(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Pass a reader that immediately returns an error to exercise the SDK path.
	bad := &errReader{err: errors.New("read failure")}
	// The SDK may or may not propagate the read error — either way the test must
	// not panic.
	_ = store.Put(context.Background(), "uploads/bad.jpg", "image/jpeg", bad, 100)
}

type errReader struct{ err error }

func (e *errReader) Read(_ []byte) (int, error) { return 0, e.err }
func (e *errReader) WriteTo(w io.Writer) (int64, error) {
	_, err := w.Write(bytes.NewBufferString("").Bytes())
	return 0, err
}
