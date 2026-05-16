package storage_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
)

// newTestGDriveStore builds a GDriveFileStore wired to handler via an in-process
// httptest server. The Drive service is configured to use the test server as its
// endpoint and to skip OAuth2 entirely, so no real credentials are needed.
func newTestGDriveStore(t *testing.T, handler http.Handler) (*storage.GDriveFileStore, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	svc, err := drive.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()),
		option.WithoutAuthentication(),
		option.WithEndpoint(srv.URL+"/drive/v3/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	store := storage.NewGDriveFileStoreForTest(svc, "test-folder")
	return store, srv
}

// ── escapeDriveQuery ──────────────────────────────────────────────────────────

func TestEscapeDriveQuery_NoSpecialChars(t *testing.T) {
	if got := storage.EscapeDriveQuery("hello world"); got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestEscapeDriveQuery_SingleQuote(t *testing.T) {
	const want = `it\'s`
	if got := storage.EscapeDriveQuery("it's"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeDriveQuery_Backslash(t *testing.T) {
	const want = `a\\b`
	if got := storage.EscapeDriveQuery(`a\b`); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeDriveQuery_BackslashBeforeQuote(t *testing.T) {
	// Input: a\'b  →  backslash doubled first, then quote escaped
	// Steps: a\'b → a\\'b → a\\\'b
	const want = `a\\\'b`
	if got := storage.EscapeDriveQuery(`a\'b`); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGDriveFileStore_Get_NotFound(t *testing.T) {
	store, _ := newTestGDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// findByName — return empty files list
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"files":[]}`)
	}))

	_, _, err := store.Get(context.Background(), "missing.jpg")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get: expected ErrNotFound, got %v", err)
	}
}

func TestGDriveFileStore_Get_Success(t *testing.T) {
	const body = "image-bytes"
	store, _ := newTestGDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/files/"):
			// findByName — list request
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"files":[{"id":"file-abc"}]}`)
		case r.URL.Query().Get("alt") == "media":
			// download
			w.Header().Set("Content-Type", "image/jpeg")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, body)
		default:
			// metadata: GET /drive/v3/files/file-abc?fields=mimeType
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"mimeType":"image/jpeg","id":"file-abc"}`)
		}
	}))

	rc, ct, err := store.Get(context.Background(), "proof.jpg")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	defer rc.Close()

	if ct != "image/jpeg" {
		t.Errorf("Get: content-type = %q, want image/jpeg", ct)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Errorf("Get: body = %q, want %q", got, body)
	}
}

func TestGDriveFileStore_Get_EmptyMimeTypeFallsBack(t *testing.T) {
	store, _ := newTestGDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/files/"):
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"files":[{"id":"file-abc"}]}`)
		case r.URL.Query().Get("alt") == "media":
			// No Content-Type set — fallback to application/octet-stream
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "bytes")
		default:
			// metadata — mimeType is empty
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"mimeType":"","id":"file-abc"}`)
		}
	}))

	rc, ct, err := store.Get(context.Background(), "proof.bin")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	defer rc.Close()

	if ct != "application/octet-stream" {
		t.Errorf("Get: fallback content-type = %q, want application/octet-stream", ct)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestGDriveFileStore_Delete_NotFound(t *testing.T) {
	// findByName returns empty list — Delete must succeed silently (idempotent).
	store, _ := newTestGDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"files":[]}`)
	}))

	if err := store.Delete(context.Background(), "missing.jpg"); err != nil {
		t.Fatalf("Delete: expected no error for missing item, got %v", err)
	}
}

func TestGDriveFileStore_Delete_Success(t *testing.T) {
	callCount := 0
	store, _ := newTestGDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"files":[{"id":"file-xyz"}]}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))

	if err := store.Delete(context.Background(), "proof.jpg"); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("Delete: expected 2 HTTP calls (list + delete), got %d", callCount)
	}
}

// ── Put ───────────────────────────────────────────────────────────────────────

func TestGDriveFileStore_Put_Create(t *testing.T) {
	// findByName returns empty → Put takes the create path.
	store, _ := newTestGDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			// findByName
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"files":[]}`)
		case http.MethodPost:
			// Files.Create upload
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"new-file-id","name":"proof.jpg"}`)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))

	err := store.Put(context.Background(), "proof.jpg", "image/jpeg", strings.NewReader("data"), 4)
	if err != nil {
		t.Fatalf("Put (create): unexpected error: %v", err)
	}
}

func TestGDriveFileStore_Put_Update(t *testing.T) {
	// findByName returns an existing ID → Put takes the update path.
	store, _ := newTestGDriveStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			// findByName
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"files":[{"id":"existing-id"}]}`)
		case http.MethodPatch:
			// Files.Update upload
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"existing-id","name":"proof.jpg"}`)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))

	err := store.Put(context.Background(), "proof.jpg", "image/jpeg", strings.NewReader("updated"), 7)
	if err != nil {
		t.Fatalf("Put (update): unexpected error: %v", err)
	}
}
