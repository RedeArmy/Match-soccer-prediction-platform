package storage

import (
	"context"
	"errors"
	"io"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

// ResilientFileStore wraps a FileStore with a circuit breaker so that an
// external storage outage (S3, GDrive, OneDrive) does not cascade into hung
// HTTP handlers.
//
// Degradation behaviour when the circuit is open:
//   - Put:    returns apperrors.Internal("storage unavailable") — the upload
//     handler propagates this as 500 so the client can retry.
//   - Get:    returns apperrors.Internal("storage unavailable").
//   - Delete: silently succeeds; best-effort cleanup must not block user flow.
//
// The circuit opens after maxFails consecutive errors and stays open for the
// configured cooldown duration before allowing a single trial request.
type ResilientFileStore struct {
	inner   FileStore
	breaker *breaker.Breaker
	log     *zap.Logger
}

// NewResilientFileStore wraps inner with the given circuit breaker.
func NewResilientFileStore(inner FileStore, b *breaker.Breaker, log *zap.Logger) *ResilientFileStore {
	return &ResilientFileStore{inner: inner, breaker: b, log: log}
}

// Put stores data under key. Returns immediately with an error when the circuit
// is open so the caller does not block waiting for a timeout.
func (s *ResilientFileStore) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error {
	err := s.breaker.Call(func() error {
		return s.inner.Put(ctx, key, contentType, r, size)
	})
	if errors.Is(err, breaker.ErrOpen) {
		s.log.Warn("storage.Put short-circuited: circuit open",
			zap.String("key", key),
			zap.String("breaker", s.breaker.Name()),
		)
		return apperrors.Internal(errStorageUnavailable)
	}
	return err
}

// Get retrieves the object at key. Returns immediately with an error when the
// circuit is open.
func (s *ResilientFileStore) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	var rc io.ReadCloser
	var ct string
	var innerErr error // captured here so ErrNotFound can be returned after the breaker call
	err := s.breaker.Call(func() error {
		rc, ct, innerErr = s.inner.Get(ctx, key)
		// ErrNotFound is a normal application response, not a storage outage;
		// do not count it as a breaker failure — return nil to the breaker but
		// preserve innerErr so we can propagate it to the caller below.
		if errors.Is(innerErr, ErrNotFound) {
			return nil
		}
		return innerErr
	})
	if errors.Is(err, breaker.ErrOpen) {
		s.log.Warn("storage.Get short-circuited: circuit open",
			zap.String("key", key),
			zap.String("breaker", s.breaker.Name()),
		)
		return nil, "", apperrors.Internal(errStorageUnavailable)
	}
	if err != nil {
		return nil, "", err
	}
	// err is nil: either the inner call succeeded or returned ErrNotFound (not a
	// breaker failure). Propagate innerErr so callers receive ErrNotFound correctly.
	return rc, ct, innerErr
}

// Delete removes the object at key. Silently succeeds when the circuit is open
// because delete is best-effort cleanup and must not block user-visible flow.
func (s *ResilientFileStore) Delete(ctx context.Context, key string) error {
	err := s.breaker.Call(func() error {
		return s.inner.Delete(ctx, key)
	})
	if errors.Is(err, breaker.ErrOpen) {
		s.log.Warn("storage.Delete short-circuited: circuit open — skipping best-effort cleanup",
			zap.String("key", key),
			zap.String("breaker", s.breaker.Name()),
		)
		return nil
	}
	return err
}

var errStorageUnavailable = errors.New("object storage temporarily unavailable")

var _ FileStore = (*ResilientFileStore)(nil)
