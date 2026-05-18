package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

// stubStore is a minimal FileStore whose behaviour is controlled per-test.
type stubStore struct {
	putErr error
	getErr error
	delErr error
	body   string
}

func (s *stubStore) Put(_ context.Context, _, _ string, _ io.Reader, _ int64) error {
	return s.putErr
}
func (s *stubStore) Get(_ context.Context, _ string) (io.ReadCloser, string, error) {
	if s.getErr != nil {
		return nil, "", s.getErr
	}
	return io.NopCloser(strings.NewReader(s.body)), "application/octet-stream", nil
}
func (s *stubStore) Delete(_ context.Context, _ string) error { return s.delErr }

// newResilient constructs a ResilientFileStore around inner with the given
// breaker thresholds (maxFails, cooldownSec 0 → instant reset in tests).
func newResilient(t *testing.T, inner storage.FileStore, maxFails int) *storage.ResilientFileStore {
	t.Helper()
	b := breaker.New("test-store", maxFails, 0) // 0-duration cooldown: resets in tests
	log := zaptest.NewLogger(t)
	return storage.NewResilientFileStore(inner, b, log)
}

// openCircuit trips the breaker by calling Put maxFails times with a failing inner.
func openCircuit(t *testing.T, s *storage.ResilientFileStore, maxFails int) {
	t.Helper()
	for i := range maxFails {
		err := s.Put(context.Background(), "k", "ct", bytes.NewReader(nil), 0)
		if err == nil {
			t.Fatalf("openCircuit: Put #%d expected error, got nil", i)
		}
	}
}

// ── Put ──────────────────────────────────────────────────────────────────────

func TestResilientFileStore_Put_DelegatesToInner(t *testing.T) {
	inner := &stubStore{}
	s := newResilient(t, inner, 3)
	if err := s.Put(context.Background(), "k", "ct", bytes.NewReader([]byte("x")), 1); err != nil {
		t.Errorf("Put: expected nil, got %v", err)
	}
}

func TestResilientFileStore_Put_InnerError_PropagatesAndCountsAsFail(t *testing.T) {
	wantErr := errors.New("s3 timeout")
	inner := &stubStore{putErr: wantErr}
	s := newResilient(t, inner, 3)
	if err := s.Put(context.Background(), "k", "ct", bytes.NewReader(nil), 0); err == nil {
		t.Error("Put: expected error, got nil")
	}
}

func TestResilientFileStore_Put_CircuitOpen_ReturnsError(t *testing.T) {
	const maxFails = 2
	inner := &stubStore{putErr: errors.New("network error")}
	s := newResilient(t, inner, maxFails)

	openCircuit(t, s, maxFails)

	// Next call should be short-circuited by the open breaker.
	err := s.Put(context.Background(), "k", "ct", bytes.NewReader(nil), 0)
	if err == nil {
		t.Fatal("Put: expected error when circuit open, got nil")
	}
}

// ── Get ──────────────────────────────────────────────────────────────────────

func TestResilientFileStore_Get_DelegatesToInner(t *testing.T) {
	inner := &stubStore{body: "hello"}
	s := newResilient(t, inner, 3)
	rc, ct, err := s.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	if ct != "application/octet-stream" {
		t.Errorf("content-type: got %q", ct)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != "hello" {
		t.Errorf("body: got %q, want %q", got, "hello")
	}
}

func TestResilientFileStore_Get_ErrNotFound_DoesNotOpenCircuit(t *testing.T) {
	// ErrNotFound is a normal application response, not an outage; the breaker
	// must not count it as a failure even when hit maxFails times.
	const maxFails = 2
	inner := &stubStore{getErr: storage.ErrNotFound}
	s := newResilient(t, inner, maxFails)

	for i := range maxFails + 1 {
		_, _, err := s.Get(context.Background(), "missing")
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("Get #%d: expected ErrNotFound, got %v", i, err)
		}
	}
	// If the circuit had opened, further Put would return a breaker error rather
	// than delegating to inner. A successful Put confirms the circuit stayed closed.
	if err := s.Put(context.Background(), "k", "ct", bytes.NewReader(nil), 0); err != nil {
		t.Errorf("circuit must stay closed after ErrNotFound; Put got %v", err)
	}
}

func TestResilientFileStore_Get_CircuitOpen_ReturnsError(t *testing.T) {
	const maxFails = 2
	inner := &stubStore{putErr: errors.New("fail")}
	s := newResilient(t, inner, maxFails)
	openCircuit(t, s, maxFails)

	_, _, err := s.Get(context.Background(), "k")
	if err == nil {
		t.Fatal("Get: expected error when circuit open, got nil")
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestResilientFileStore_Delete_DelegatesToInner(t *testing.T) {
	inner := &stubStore{}
	s := newResilient(t, inner, 3)
	if err := s.Delete(context.Background(), "k"); err != nil {
		t.Errorf("Delete: expected nil, got %v", err)
	}
}

func TestResilientFileStore_Delete_InnerError_PropagatesWhenClosed(t *testing.T) {
	inner := &stubStore{delErr: errors.New("s3 delete failed")}
	s := newResilient(t, inner, 3)
	if err := s.Delete(context.Background(), "k"); err == nil {
		t.Error("Delete: expected error from inner, got nil")
	}
}

func TestResilientFileStore_Delete_CircuitOpen_SilentlySucceeds(t *testing.T) {
	const maxFails = 2
	inner := &stubStore{putErr: errors.New("fail")}
	s := newResilient(t, inner, maxFails)
	openCircuit(t, s, maxFails)

	// Delete must succeed silently even when the circuit is open.
	if err := s.Delete(context.Background(), "k"); err != nil {
		t.Errorf("Delete: expected nil when circuit open (best-effort), got %v", err)
	}
}
