package storage_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
)

// newTestOneDriveStore builds an OneDriveFileStore wired to handler via a plain
// http.Client. The driveID is set to "test-drive" and the base URL points at
// the test server, bypassing real OAuth flows entirely.
func newTestOneDriveStore(t *testing.T, handler http.Handler) (*storage.OneDriveFileStore, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	store := storage.NewOneDriveFileStoreForTest(srv.Client(), "test-drive", srv.URL)
	return store, srv
}

// ── Put ───────────────────────────────────────────────────────────────────────

func TestOneDriveFileStore_Put_Returns201(t *testing.T) {
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))

	err := store.Put(context.Background(), "bank-transfers/proof.jpg", "image/jpeg", strings.NewReader("data"), 4)
	if err != nil {
		t.Fatalf("Put: unexpected error: %v", err)
	}
}

func TestOneDriveFileStore_Put_Returns200(t *testing.T) {
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	if err := store.Put(context.Background(), "key.jpg", "image/jpeg", strings.NewReader("x"), 1); err != nil {
		t.Fatalf("Put: unexpected error: %v", err)
	}
}

func TestOneDriveFileStore_Put_UnexpectedStatus(t *testing.T) {
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	if err := store.Put(context.Background(), "key.jpg", "image/jpeg", strings.NewReader("x"), 1); err == nil {
		t.Fatal("Put: expected error for status 500, got nil")
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestOneDriveFileStore_Get_Success(t *testing.T) {
	const body = "binary-content"
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
	}))

	rc, ct, err := store.Get(context.Background(), "bank-transfers/proof.png")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	defer rc.Close()

	if ct != "image/png" {
		t.Errorf("Get: content-type = %q, want image/png", ct)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Errorf("Get: body = %q, want %q", got, body)
	}
}

func TestOneDriveFileStore_Get_EmptyContentTypeFallsBack(t *testing.T) {
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No explicit Content-Type set. Binary body causes the httptest server to
		// sniff application/octet-stream, which is the expected fallback value.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0x00, 0x00, 0x00, 0x00})
	}))

	rc, ct, err := store.Get(context.Background(), "key")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	defer rc.Close()
	if ct != "application/octet-stream" {
		t.Errorf("Get: fallback content-type = %q, want application/octet-stream", ct)
	}
}

func TestOneDriveFileStore_Get_NotFound(t *testing.T) {
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, _, err := store.Get(context.Background(), "missing.jpg")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get: expected ErrNotFound, got %v", err)
	}
}

func TestOneDriveFileStore_Get_UnexpectedStatus(t *testing.T) {
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, _, err := store.Get(context.Background(), "key.jpg")
	if err == nil {
		t.Fatal("Get: expected error for status 403, got nil")
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestOneDriveFileStore_Delete_ItemNotFound_IsNoop(t *testing.T) {
	// resolveItemID returns 404 → Delete treats it as already-deleted (idempotent).
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	if err := store.Delete(context.Background(), "missing.jpg"); err != nil {
		t.Fatalf("Delete: expected no error for missing item, got %v", err)
	}
}

func TestOneDriveFileStore_Delete_Success(t *testing.T) {
	// First GET (resolveItemID) returns JSON with an ID.
	// Second DELETE returns 204.
	callCount := 0
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"item-abc"}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))

	if err := store.Delete(context.Background(), "proof.jpg"); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("Delete: expected 2 HTTP calls (resolve + delete), got %d", callCount)
	}
}

func TestOneDriveFileStore_Delete_UnexpectedStatus(t *testing.T) {
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"item-abc"}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	if err := store.Delete(context.Background(), "proof.jpg"); err == nil {
		t.Fatal("Delete: expected error for status 500, got nil")
	}
}

// ── resolveItemID ─────────────────────────────────────────────────────────────

func TestOneDriveFileStore_ResolveItemID_JSONDecodeError(t *testing.T) {
	// resolveItemID returns 200 but with invalid JSON — must return an error.
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `not-json`)
		}
	}))

	if err := store.Delete(context.Background(), "key"); err == nil {
		t.Fatal("Delete: expected decode error, got nil")
	}
}

func TestOneDriveFileStore_ResolveItemID_UnexpectedStatus(t *testing.T) {
	store, _ := newTestOneDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	if err := store.Delete(context.Background(), "key"); err == nil {
		t.Fatal("Delete: expected error for status 500, got nil")
	}
}
