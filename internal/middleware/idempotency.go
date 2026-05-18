package middleware

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/idempotency"
)

const idempotencyHeader = "Idempotency-Key"

// Idempotency returns middleware that deduplicates write requests using the
// client-supplied Idempotency-Key header.
//
// ttl is how long a committed response is retained; keyMaxLen is the maximum
// accepted Idempotency-Key byte length (requests exceeding it get 422).
//
// Behaviour:
//   - No header → pass through; idempotency is opt-in.
//   - Header present, key not yet seen → reserve key as in-flight, execute
//     handler, commit successful (2xx) response; error responses release the
//     reservation so the client may retry.
//   - Header present, key in-flight (concurrent duplicate) → 409 Conflict.
//   - Header present, key committed → replay cached response with
//     X-Idempotency-Replayed: true, no handler execution.
//
// Keys are scoped per Clerk subject so two users cannot collide on the same
// string. The middleware must be placed after RequireAuth in the chain.
//
// Only 2xx responses are committed to the store. Handlers returning 4xx or 5xx
// release the reservation; the client may retry with the same key once the
// underlying problem is resolved.
func Idempotency(store idempotency.Store, log *zap.Logger, ttl time.Duration, keyMaxLen int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveIdempotent(w, r, next, store, log, ttl, keyMaxLen)
		})
	}
}

// serveIdempotent contains the per-request idempotency logic, extracted from
// the closure in Idempotency to keep each function within the cognitive
// complexity budget.
func serveIdempotent(w http.ResponseWriter, r *http.Request, next http.Handler,
	store idempotency.Store, log *zap.Logger, ttl time.Duration, keyMaxLen int,
) {
	clientKey := r.Header.Get(idempotencyHeader)
	if clientKey == "" {
		next.ServeHTTP(w, r)
		return
	}
	if len(clientKey) > keyMaxLen {
		WriteError(w, r, log, apperrors.Validation(
			fmt.Sprintf("Idempotency-Key must be at most %d characters", keyMaxLen),
		))
		return
	}

	// Scope per-user so two different users cannot share a key slot.
	// UserIDFromContext returns the Clerk subject; always set by RequireAuth.
	subject, _ := UserIDFromContext(r.Context())
	scopedKey := "idem:" + subject + ":" + clientKey

	entry, found, err := store.Load(r.Context(), scopedKey)
	if err != nil {
		log.Warn("idempotency: store load error, degrading to pass-through",
			zap.String("key", clientKey),
			zap.String("request_id", GetRequestID(r.Context())),
			zap.Error(err),
		)
		next.ServeHTTP(w, r)
		return
	}
	if found {
		handleExistingEntry(w, r, log, clientKey, entry)
		return
	}

	// Reserve: atomically claim the key as in-flight.
	reserved, err := store.Reserve(r.Context(), scopedKey, ttl)
	if err != nil {
		log.Warn("idempotency: reserve error, degrading to pass-through",
			zap.String("key", clientKey),
			zap.String("request_id", GetRequestID(r.Context())),
			zap.Error(err),
		)
		next.ServeHTTP(w, r)
		return
	}
	if !reserved {
		// Concurrent request won the race between Load and Reserve.
		WriteError(w, r, log, apperrors.Conflict(
			"a request with this Idempotency-Key is already in progress; retry after it completes",
		))
		return
	}

	// Execute the handler with a capturing writer so the response can be
	// committed to the store and simultaneously written to the real wire.
	cw := newCaptureWriter(w)
	next.ServeHTTP(cw, r)
	commitOrRelease(r.Context(), store, log, scopedKey, clientKey, ttl, cw)
}

// handleExistingEntry dispatches a found store entry: replays a committed
// response or rejects a concurrent in-flight duplicate with 409.
func handleExistingEntry(w http.ResponseWriter, r *http.Request, log *zap.Logger, clientKey string, entry idempotency.Entry) {
	switch entry.State {
	case idempotency.InFlight:
		WriteError(w, r, log, apperrors.Conflict(
			"a request with this Idempotency-Key is already in progress; retry after it completes",
		))
	case idempotency.Committed:
		replayResponse(w, clientKey, entry)
	}
}

// commitOrRelease persists a successful response to the store, or releases the
// reservation so the client may retry after an error response.
func commitOrRelease(ctx context.Context, store idempotency.Store, log *zap.Logger,
	scopedKey, clientKey string, ttl time.Duration, cw *captureWriter,
) {
	if cw.statusCode >= 200 && cw.statusCode < 300 {
		committed := idempotency.Entry{
			State:      idempotency.Committed,
			StatusCode: cw.statusCode,
			Headers:    cw.capturedHeaders,
			Body:       cw.buf.Bytes(),
		}
		if err := store.Commit(ctx, scopedKey, committed, ttl); err != nil {
			log.Warn("idempotency: commit error — response was already sent",
				zap.String("key", clientKey),
				zap.String("request_id", GetRequestID(ctx)),
				zap.Error(err),
			)
		}
		return
	}
	if err := store.Release(ctx, scopedKey); err != nil {
		log.Warn("idempotency: release error after non-2xx response",
			zap.String("key", clientKey),
			zap.Error(err),
		)
	}
}

// replayResponse writes the stored entry back to w, adding the idempotency
// headers that signal a replayed response to the client.
func replayResponse(w http.ResponseWriter, clientKey string, e idempotency.Entry) {
	for k, vals := range e.Headers {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set(idempotencyHeader, clientKey)
	w.Header().Set("X-Idempotency-Replayed", "true")
	w.WriteHeader(e.StatusCode)
	_, _ = w.Write(e.Body)
}

// captureWriter implements http.ResponseWriter. It writes to the underlying
// ResponseWriter (wire) while simultaneously capturing status code, headers,
// and body for later storage in the idempotency store.
type captureWriter struct {
	http.ResponseWriter
	statusCode      int
	buf             bytes.Buffer
	capturedHeaders http.Header
	headerWritten   bool
}

func newCaptureWriter(w http.ResponseWriter) *captureWriter {
	return &captureWriter{
		ResponseWriter:  w,
		statusCode:      http.StatusOK,
		capturedHeaders: make(http.Header),
	}
}

func (c *captureWriter) WriteHeader(code int) {
	if c.headerWritten {
		return
	}
	c.headerWritten = true
	c.statusCode = code
	// Snapshot headers at WriteHeader time; any Set/Add calls before this
	// point are already in c.ResponseWriter.Header() which we delegate to.
	for k, v := range c.Header() {
		c.capturedHeaders[k] = append([]string(nil), v...)
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.headerWritten {
		c.WriteHeader(http.StatusOK)
	}
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}
