package middleware_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/idempotency"
)

// ── failStore: a Store whose operations always return errors ──────────────────

type failStore struct{}

func (failStore) Load(_ context.Context, _ string) (idempotency.Entry, bool, error) {
	return idempotency.Entry{}, false, errors.New("load failed")
}
func (failStore) Reserve(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return false, errors.New("reserve failed")
}
func (failStore) Commit(_ context.Context, _ string, _ idempotency.Entry, _ time.Duration) error {
	return errors.New("commit failed")
}
func (failStore) Release(_ context.Context, _ string) error {
	return errors.New("release failed")
}

// idempotencyChain applies the idempotency middleware around h, stamping the
// Clerk subject into context the way RequireAuth does in production.
func idempotencyChain(store idempotency.Store, subject string, h http.Handler, log interface{ Named(string) interface{} }) func(http.Handler) http.Handler {
	return nil // placeholder — see idempotencyRun below
}

// idempotencyRun creates a fresh MemoryStore + middleware and executes one
// HTTP request through the chain. subject simulates the Clerk user ID that
// RequireAuth would set.
func idempotencyRun(t *testing.T, store idempotency.Store, subject, idemKey string, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	log := zaptest.NewLogger(t)
	idem := middleware.Idempotency(store, nil, log, 24*time.Hour, 255)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pay", bytes.NewReader([]byte(`{}`)))
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	ctx := middleware.ContextWithUserID(req.Context(), subject)
	idem(h).ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

// ── Pass-through (no header) ──────────────────────────────────────────────────

func TestIdempotency_NoHeader_PassesThrough(t *testing.T) {
	called := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusCreated)
	})
	rec := idempotencyRun(t, idempotency.NewMemoryStore(), "u1", "", h)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	if called != 1 {
		t.Errorf("handler must be called once; got %d", called)
	}
	if rec.Header().Get("X-Idempotency-Replayed") != "" {
		t.Error("must not set replayed header when no key is supplied")
	}
}

// ── First request executes handler ───────────────────────────────────────────

func TestIdempotency_FirstRequest_ExecutesHandler(t *testing.T) {
	called := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":42}`))
	})
	store := idempotency.NewMemoryStore()
	rec := idempotencyRun(t, store, "u1", "key-abc", h)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	if called != 1 {
		t.Errorf("expected handler called once, got %d", called)
	}
}

// ── Second request replays cached response ────────────────────────────────────

func TestIdempotency_SecondRequest_ReplaysCachedResponse(t *testing.T) {
	called := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Resource-ID", "99")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":99}`))
	})

	store := idempotency.NewMemoryStore()
	log := zaptest.NewLogger(t)
	idem := middleware.Idempotency(store, nil, log, 24*time.Hour, 255)

	run := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/pay", nil)
		req.Header.Set("Idempotency-Key", "dup-key")
		ctx := middleware.ContextWithUserID(req.Context(), "u1")
		idem(h).ServeHTTP(rec, req.WithContext(ctx))
		return rec
	}

	rec1 := run()
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first call: expected 201, got %d", rec1.Code)
	}

	rec2 := run()
	if rec2.Code != http.StatusCreated {
		t.Errorf("replay: expected 201, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-Idempotency-Replayed") != "true" {
		t.Error("replay must set X-Idempotency-Replayed: true")
	}
	if rec2.Header().Get("X-Resource-ID") != "99" {
		t.Errorf("replay must carry original headers; X-Resource-ID=%q", rec2.Header().Get("X-Resource-ID"))
	}
	if rec2.Body.String() != `{"id":99}` {
		t.Errorf("replay body mismatch: got %q", rec2.Body.String())
	}
	if called != 1 {
		t.Errorf("handler must be called only once; got %d", called)
	}
}

// ── Error response releases reservation so client can retry ──────────────────

func TestIdempotency_ErrorResponse_ReleasesReservation(t *testing.T) {
	attempt := 0
	statuses := []int{http.StatusInternalServerError, http.StatusCreated}
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(statuses[attempt])
		attempt++
	})

	store := idempotency.NewMemoryStore()
	log := zaptest.NewLogger(t)
	idem := middleware.Idempotency(store, nil, log, 24*time.Hour, 255)

	call := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/pay", nil)
		req.Header.Set("Idempotency-Key", "retry-key")
		ctx := middleware.ContextWithUserID(req.Context(), "u1")
		idem(h).ServeHTTP(rec, req.WithContext(ctx))
		return rec
	}

	// First call returns 500 → reservation must be released
	if code := call().Code; code != http.StatusInternalServerError {
		t.Fatalf("first call: expected 500, got %d", code)
	}
	// Second call must reach the handler (not replay) and get 201
	rec2 := call()
	if rec2.Code != http.StatusCreated {
		t.Errorf("retry after 500: expected 201, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-Idempotency-Replayed") != "" {
		t.Error("retry after error must not set replayed header")
	}
}

// ── Key too long → 422 ────────────────────────────────────────────────────────

func TestIdempotency_KeyTooLong_Returns422(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	rec := idempotencyRun(t, idempotency.NewMemoryStore(), "u1", string(bytes.Repeat([]byte("a"), 256)), h)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for key > 255 chars, got %d", rec.Code)
	}
}

// ── Different users do not share a key ───────────────────────────────────────

func TestIdempotency_DifferentUsers_DoNotShareKey(t *testing.T) {
	called := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusCreated)
	})

	store := idempotency.NewMemoryStore()
	log := zaptest.NewLogger(t)
	idem := middleware.Idempotency(store, nil, log, 24*time.Hour, 255)

	callAs := func(subject string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/pay", nil)
		req.Header.Set("Idempotency-Key", "same-client-key")
		ctx := middleware.ContextWithUserID(req.Context(), subject)
		idem(h).ServeHTTP(rec, req.WithContext(ctx))
		return rec.Code
	}

	if code := callAs("alice"); code != http.StatusCreated {
		t.Errorf("alice: expected 201, got %d", code)
	}
	if code := callAs("bob"); code != http.StatusCreated {
		t.Errorf("bob: expected 201, got %d", code)
	}
	if called != 2 {
		t.Errorf("both users must hit the handler; called=%d", called)
	}
}

// ── MemoryStore unit tests ────────────────────────────────────────────────────

func TestMemoryStore_ReserveAndLoad(t *testing.T) {
	s := idempotency.NewMemoryStore()
	ctx := t.Context()

	reserved, err := s.Reserve(ctx, "k1", time.Second)
	if err != nil || !reserved {
		t.Fatalf("Reserve: want (true, nil), got (%v, %v)", reserved, err)
	}
	e, found, err := s.Load(ctx, "k1")
	if err != nil || !found {
		t.Fatalf("Load after Reserve: want found=true, got found=%v err=%v", found, err)
	}
	if e.State != idempotency.InFlight {
		t.Errorf("state after Reserve: want InFlight, got %v", e.State)
	}
}

func TestMemoryStore_ReserveTwice_SecondFails(t *testing.T) {
	s := idempotency.NewMemoryStore()
	ctx := t.Context()
	_, _ = s.Reserve(ctx, "k2", time.Second)
	got, err := s.Reserve(ctx, "k2", time.Second)
	if err != nil || got {
		t.Errorf("second Reserve: want (false, nil), got (%v, %v)", got, err)
	}
}

func TestMemoryStore_Commit_ChangesState(t *testing.T) {
	s := idempotency.NewMemoryStore()
	ctx := t.Context()
	_, _ = s.Reserve(ctx, "k3", time.Second)
	committed := idempotency.Entry{State: idempotency.Committed, StatusCode: 201, Body: []byte("ok")}
	if err := s.Commit(ctx, "k3", committed, time.Second); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	e, found, err := s.Load(ctx, "k3")
	if err != nil || !found {
		t.Fatalf("Load after Commit: found=%v err=%v", found, err)
	}
	if e.State != idempotency.Committed || e.StatusCode != 201 || string(e.Body) != "ok" {
		t.Errorf("committed entry mismatch: %+v", e)
	}
}

func TestMemoryStore_Release_RemovesKey(t *testing.T) {
	s := idempotency.NewMemoryStore()
	ctx := t.Context()
	_, _ = s.Reserve(ctx, "k4", time.Second)
	_ = s.Release(ctx, "k4")
	_, found, _ := s.Load(ctx, "k4")
	if found {
		t.Error("Load after Release must return found=false")
	}
}

func TestMemoryStore_ExpiredEntry_NotFound(t *testing.T) {
	s := idempotency.NewMemoryStore()
	ctx := t.Context()
	_, _ = s.Reserve(ctx, "k5", -time.Millisecond) // already expired
	_, found, _ := s.Load(ctx, "k5")
	if found {
		t.Error("expired entry must not be returned by Load")
	}
}

func TestMemoryStore_ExpiredEntry_AllowsNewReservation(t *testing.T) {
	s := idempotency.NewMemoryStore()
	ctx := t.Context()
	_, _ = s.Reserve(ctx, "k6", -time.Millisecond) // expire immediately
	reserved, err := s.Reserve(ctx, "k6", time.Second)
	if err != nil || !reserved {
		t.Errorf("after expiry, Reserve should succeed; got (%v, %v)", reserved, err)
	}
}

// ── Store error degradation: middleware passes through on store failure ────────

func TestIdempotency_StoreLoadError_DegradesToPassThrough(t *testing.T) {
	called := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusCreated)
	})
	rec := idempotencyRun(t, failStore{}, "u1", "key-fail-load", h)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	if called != 1 {
		t.Errorf("handler must be called on store Load error; got %d", called)
	}
}

func TestIdempotency_StoreReserveError_DegradesToPassThrough(t *testing.T) {
	// reserveFailStore: Load succeeds (key not found), Reserve fails.
	type reserveFailStore struct{ idempotency.Store }
	store := struct{ idempotency.Store }{idempotency.NewMemoryStore()}
	_ = store // ensure compile — we use failStore which also fails Reserve
	called := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	})
	// failStore fails both Load and Reserve; Load error path already exercised above.
	// Here we use a store that returns (not-found, nil) on Load but fails Reserve.
	partialFail := &partialFailStore{loadNotFound: true}
	rec := idempotencyRun(t, partialFail, "u1", "key-reserve-fail", h)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if called != 1 {
		t.Errorf("handler must be called on Reserve error; got %d", called)
	}
}

// partialFailStore: Load returns (not-found, nil); Reserve returns an error.
type partialFailStore struct{ loadNotFound bool }

func (s *partialFailStore) Load(_ context.Context, _ string) (idempotency.Entry, bool, error) {
	if s.loadNotFound {
		return idempotency.Entry{}, false, nil
	}
	return idempotency.Entry{}, false, errors.New("load error")
}
func (*partialFailStore) Reserve(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return false, errors.New("reserve error")
}
func (*partialFailStore) Commit(_ context.Context, _ string, _ idempotency.Entry, _ time.Duration) error {
	return nil
}
func (*partialFailStore) Release(_ context.Context, _ string) error { return nil }

func TestIdempotency_StoreCommitError_ResponseAlreadySent(t *testing.T) {
	// Commit fails after a 2xx response — the response was already sent to wire;
	// the client should still receive the correct status code.
	commitFail := &commitFailStore{}
	called := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusCreated)
	})
	rec := idempotencyRun(t, commitFail, "u1", "key-commit-fail", h)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 even when Commit fails; got %d", rec.Code)
	}
	if called != 1 {
		t.Errorf("handler must be called once; got %d", called)
	}
}

type commitFailStore struct{}

func (*commitFailStore) Load(_ context.Context, _ string) (idempotency.Entry, bool, error) {
	return idempotency.Entry{}, false, nil // key not found
}
func (*commitFailStore) Reserve(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil // reservation succeeds
}
func (*commitFailStore) Commit(_ context.Context, _ string, _ idempotency.Entry, _ time.Duration) error {
	return errors.New("commit error")
}
func (*commitFailStore) Release(_ context.Context, _ string) error { return nil }

func TestIdempotency_StoreReleaseError_NonTwoXxStillReturned(t *testing.T) {
	// Release fails after a 5xx response — the client must still see the 500.
	releaseFail := &releaseFailStore{}
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	rec := idempotencyRun(t, releaseFail, "u1", "key-release-fail", h)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 even when Release fails; got %d", rec.Code)
	}
}

type releaseFailStore struct{}

func (*releaseFailStore) Load(_ context.Context, _ string) (idempotency.Entry, bool, error) {
	return idempotency.Entry{}, false, nil
}
func (*releaseFailStore) Reserve(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}
func (*releaseFailStore) Commit(_ context.Context, _ string, _ idempotency.Entry, _ time.Duration) error {
	return nil
}
func (*releaseFailStore) Release(_ context.Context, _ string) error {
	return errors.New("release error")
}

// ── captureWriter edge cases ──────────────────────────────────────────────────

func TestCaptureWriter_Write_WithoutPriorWriteHeader_Sends200(t *testing.T) {
	// Write without a prior WriteHeader call should implicitly send 200.
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("implicit ok")) // no explicit WriteHeader
	})
	store := idempotency.NewMemoryStore()
	rec := idempotencyRun(t, store, "u1", "key-implicit-header", h)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (implicit WriteHeader), got %d", rec.Code)
	}
	if rec.Body.String() != "implicit ok" {
		t.Errorf("body: got %q", rec.Body.String())
	}
}

func TestIdempotency_ConcurrentInFlight_Returns409(t *testing.T) {
	// Simulate the race: a second call arrives while the first is in-flight.
	// We achieve this by pre-loading the store with an InFlight entry.
	store := idempotency.NewMemoryStore()
	ctx := context.Background()
	_, _ = store.Reserve(ctx, "idem:u1:dup-inflight", 24*time.Hour)

	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	rec := idempotencyRun(t, store, "u1", "dup-inflight", h)
	if rec.Code != http.StatusConflict {
		t.Errorf("in-flight duplicate: expected 409, got %d", rec.Code)
	}
}

// ── degradation counter (wcq_idempotency_degraded_total) ─────────────────────

// idempotencyRunWithMeter is like idempotencyRun but accepts a real OTel meter
// so that counter increments can be observed in tests.
func idempotencyRunWithMeter(t *testing.T, store idempotency.Store, subject, idemKey string, h http.Handler, meter sdkmetric.Reader) *httptest.ResponseRecorder {
	t.Helper()
	log := zaptest.NewLogger(t)
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(meter))
	idem := middleware.Idempotency(store, mp.Meter("test"), log, 24*time.Hour, 255)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pay", bytes.NewReader([]byte(`{}`)))
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	ctx := middleware.ContextWithUserID(req.Context(), subject)
	idem(h).ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

// collectSumInt64 reads the cumulative value of an Int64 Sum (counter) metric
// from rm. Returns 0 when the metric is absent.
func collectSumInt64(rm metricdata.ResourceMetrics, name string) int64 {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				return 0
			}
			var total int64
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
			return total
		}
	}
	return 0
}

// TestIdempotency_StoreLoadError_IncrementsDegradedCounter verifies that a
// Redis load failure increments wcq_idempotency_degraded_total and passes
// the request through.
func TestIdempotency_StoreLoadError_IncrementsDegradedCounter(t *testing.T) {
	reader := sdkmetric.NewManualReader()

	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	rec := idempotencyRunWithMeter(t, failStore{}, "u1", "key-load-fail", h, reader)

	// Request passes through despite the store error.
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 pass-through, got %d", rec.Code)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if n := collectSumInt64(rm, "wcq_idempotency_degraded_total"); n != 1 {
		t.Errorf("expected wcq_idempotency_degraded_total=1, got %d", n)
	}
}

// TestIdempotency_StoreReserveError_IncrementsDegradedCounter verifies that a
// reserve failure (after a successful Load that found nothing) also increments
// the degradation counter.
func TestIdempotency_StoreReserveError_IncrementsDegradedCounter(t *testing.T) {
	// loadThenFailStore returns not-found on Load (so we proceed to Reserve)
	// and then fails on Reserve.
	store := &loadOKReserveFailStore{}
	reader := sdkmetric.NewManualReader()

	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	rec := idempotencyRunWithMeter(t, store, "u1", "key-reserve-fail", h, reader)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 pass-through, got %d", rec.Code)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if n := collectSumInt64(rm, "wcq_idempotency_degraded_total"); n != 1 {
		t.Errorf("expected wcq_idempotency_degraded_total=1, got %d", n)
	}
}

// TestIdempotency_NilMeter_NoPanic verifies that passing a nil meter does not
// panic even when degradation occurs, preserving backward compatibility.
func TestIdempotency_NilMeter_NoPanic(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	// nil meter + failStore: must not panic
	rec := idempotencyRun(t, failStore{}, "u1", "nil-meter-key", h)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 pass-through, got %d", rec.Code)
	}
}

// loadOKReserveFailStore succeeds on Load (key not found) but fails on Reserve.
type loadOKReserveFailStore struct{}

func (loadOKReserveFailStore) Load(_ context.Context, _ string) (idempotency.Entry, bool, error) {
	return idempotency.Entry{}, false, nil // not found, no error
}
func (loadOKReserveFailStore) Reserve(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return false, errors.New("reserve unavailable")
}
func (loadOKReserveFailStore) Commit(_ context.Context, _ string, _ idempotency.Entry, _ time.Duration) error {
	return nil
}
func (loadOKReserveFailStore) Release(_ context.Context, _ string) error {
	return nil
}
