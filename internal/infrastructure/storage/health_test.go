package storage_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

func newStorageChecker(t *testing.T, inner storage.FileStore, maxFails int) (*storage.ResilientFileStore, *storage.Checker) {
	t.Helper()
	b := breaker.New("test-store", maxFails, 0)
	log := zaptest.NewLogger(t)
	rfs := storage.NewResilientFileStore(inner, b, log)
	return rfs, storage.NewChecker("file-store", rfs)
}

func TestStorageChecker_CircuitOpen_ReturnsDegraded(t *testing.T) {
	providerErr := errors.New("provider down")
	// putErr is needed to trip the circuit via openCircuit; getErr is what the
	// health probe would see if we ever reached the inner store.
	inner := &stubStore{putErr: providerErr, getErr: providerErr}
	rfs, checker := newStorageChecker(t, inner, 1)
	// Trip the circuit by exhausting maxFails.
	openCircuit(t, rfs, 1)

	result := checker.Check(context.Background())

	if result.Status != "degraded" {
		t.Errorf("status = %q, want %q", result.Status, "degraded")
	}
	if result.Error == "" {
		t.Error("expected non-empty error string for degraded status")
	}
}

func TestStorageChecker_ErrNotFound_ReturnsOK(t *testing.T) {
	inner := &stubStore{getErr: storage.ErrNotFound}
	_, checker := newStorageChecker(t, inner, 3)

	result := checker.Check(context.Background())

	if result.Status != "ok" {
		t.Errorf("status = %q, want %q", result.Status, "ok")
	}
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}

func TestStorageChecker_SuccessfulGet_ReturnsOK(t *testing.T) {
	inner := &stubStore{body: "probe-data"}
	_, checker := newStorageChecker(t, inner, 3)

	result := checker.Check(context.Background())

	if result.Status != "ok" {
		t.Errorf("status = %q, want %q", result.Status, "ok")
	}
}

func TestStorageChecker_ProviderError_ReturnsError(t *testing.T) {
	inner := &stubStore{getErr: errors.New("connection refused")}
	_, checker := newStorageChecker(t, inner, 5) // high threshold so circuit stays closed

	result := checker.Check(context.Background())

	if result.Status != "error" {
		t.Errorf("status = %q, want %q", result.Status, "error")
	}
	if result.Error == "" {
		t.Error("expected non-empty error string")
	}
}

func TestStorageChecker_Name(t *testing.T) {
	_, checker := newStorageChecker(t, &stubStore{}, 3)
	if checker.Name() != "file-store" {
		t.Errorf("Name() = %q, want %q", checker.Name(), "file-store")
	}
}
