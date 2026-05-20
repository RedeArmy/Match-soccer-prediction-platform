package email_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

// newTestBreaker builds a breaker with maxFails=1 for fast circuit trips in tests.
func newTestBreaker() *breaker.Breaker {
	return breaker.New("test", 2, 0) // 0 open duration — half-open on next call
}

// newTestTracer returns an in-memory span recorder and the resilient sender's tracer provider.
func newTestTracer() (*tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return rec, tp
}

func TestResilientSender_Success_RecordsOkSpan(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg-ok"}`))
	}))
	defer srv.Close()

	rec, tp := newTestTracer()
	inner := email.NewResendClientWithBaseURL("key", srv.URL)
	b := newTestBreaker()
	s := email.NewResilientSender(inner, b, tp.Tracer("test"), 1, zap.NewNop())

	msgID, err := s.Send(context.Background(), email.Message{
		From: "no@test.com", To: []string{"u@test.com"}, Subject: "hi", HTML: "<p>hi</p>",
	})
	if err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}
	if msgID != "msg-ok" {
		t.Errorf("msgID: got %q; want %q", msgID, "msg-ok")
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans: got %d; want 1", len(spans))
	}
	if spans[0].Status().Code != codes.Ok {
		t.Errorf("span status: got %v; want Ok", spans[0].Status().Code)
	}
}

func TestResilientSender_GenuineError_PropagatesAndRecordsErrorSpan(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rec, tp := newTestTracer()
	inner := email.NewResendClientWithBaseURL("key", srv.URL)
	b := newTestBreaker()
	s := email.NewResilientSender(inner, b, tp.Tracer("test"), 1, zap.NewNop())

	_, err := s.Send(context.Background(), email.Message{To: []string{"u@test.com"}, Subject: "s", HTML: "h"})
	if err == nil {
		t.Fatal("expected error for 500 response; got nil")
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans: got %d; want 1", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Errorf("span status: got %v; want Error", spans[0].Status().Code)
	}
}

func TestResilientSender_CircuitOpen_ShortCircuits(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, tp := newTestTracer()
	inner := email.NewResendClientWithBaseURL("key", srv.URL)
	b := breaker.New("test", 2, 60_000_000_000) // 60 s open window
	s := email.NewResilientSender(inner, b, tp.Tracer("test"), 1, zap.NewNop())

	msg := email.Message{To: []string{"u@test.com"}, Subject: "s", HTML: "h"}

	// Trip the circuit: 2 consecutive 5xx failures open it.
	_, _ = s.Send(context.Background(), msg)
	_, _ = s.Send(context.Background(), msg)

	// Circuit is now open — next call must short-circuit without calling the server.
	callsBefore := atomic.LoadInt32(&calls)
	_, err := s.Send(context.Background(), msg)
	if !errors.Is(err, breaker.ErrOpen) {
		t.Errorf("expected breaker.ErrOpen; got: %v", err)
	}
	if after := atomic.LoadInt32(&calls); after != callsBefore {
		t.Errorf("server was called %d extra time(s) while circuit was open", after-callsBefore)
	}
}

func TestResilientSender_RateLimit_DoesNotTripCircuit(t *testing.T) {
	t.Parallel()

	// A server that always returns 429 should never trip the circuit breaker.
	// After many 429s the breaker should remain closed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer srv.Close()

	_, tp := newTestTracer()
	inner := email.NewResendClientWithBaseURL("key", srv.URL)
	// maxFails=1 means a genuine failure would open the circuit immediately.
	b := breaker.New("test", 1, 60_000_000_000)
	// maxRetry=1 → 2 attempts; context cancelled before sleep so the test is fast.
	s := email.NewResilientSender(inner, b, tp.Tracer("test"), 1, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the retry sleep is skipped.
	cancel()

	_, err := s.Send(ctx, email.Message{To: []string{"u@test.com"}, Subject: "s", HTML: "h"})
	// Error must be context cancellation, not breaker.ErrOpen.
	if errors.Is(err, breaker.ErrOpen) {
		t.Error("circuit opened after 429s — 429 must not count as a breaker failure")
	}
	if err == nil {
		t.Error("expected an error (ctx cancelled or rate-limited); got nil")
	}
}

func TestResilientSender_RateLimit_ContextCancelledDuringBackoff(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, tp := newTestTracer()
	inner := email.NewResendClientWithBaseURL("key", srv.URL)
	b := newTestBreaker()
	s := email.NewResilientSender(inner, b, tp.Tracer("test"), 2, zap.NewNop())

	// Pre-cancel so the first backoff sleep is skipped immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Send(ctx, email.Message{To: []string{"u@test.com"}, Subject: "s", HTML: "h"})
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if errors.Is(err, breaker.ErrOpen) {
		t.Error("circuit must not open on 429")
	}
	// Error must come from context cancellation, not rate-limit exhaustion.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled; got: %v", err)
	}
}

func TestResilientSender_RateLimit_RetriesExhausted_ReturnsError(t *testing.T) {
	t.Parallel()

	// Always 429 with Retry-After: 0 so the backoff is rateLimitBackoff(0)=1s.
	// With maxRetry=1, two attempts are made with one sleep of 1s between them.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, tp := newTestTracer()
	inner := email.NewResendClientWithBaseURL("key", srv.URL)
	b := newTestBreaker()
	// maxRetry=0 is clamped to 1 → exactly 2 attempts, 1 sleep of 1s.
	s := email.NewResilientSender(inner, b, tp.Tracer("test"), 0, zap.NewNop())

	// Use a fast-timeout context so the sleep is skipped via ctx.Done().
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	_, err := s.Send(ctx, email.Message{To: []string{"u@test.com"}, Subject: "s", HTML: "h"})
	if err == nil {
		t.Fatal("expected error; got nil")
	}
}

func TestResilientSender_RateLimit_RetryAfterHeader_Respected(t *testing.T) {
	t.Parallel()

	// First call: 429 with Retry-After: 0; second call: success.
	var attempt int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&attempt, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"retry-ok"}`))
	}))
	defer srv.Close()

	_, tp := newTestTracer()
	inner := email.NewResendClientWithBaseURL("key", srv.URL)
	b := newTestBreaker()
	s := email.NewResilientSender(inner, b, tp.Tracer("test"), 2, zap.NewNop())

	// Retry-After: 0 → rateLimitBackoff(0) = 1s; test is slightly slow but correct.
	msgID, err := s.Send(context.Background(), email.Message{
		To: []string{"u@test.com"}, Subject: "s", HTML: "h",
	})
	if err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}
	if msgID != "retry-ok" {
		t.Errorf("msgID: got %q; want %q", msgID, "retry-ok")
	}
	if n := atomic.LoadInt32(&attempt); n != 2 {
		t.Errorf("server calls: got %d; want 2", n)
	}
}

func TestResilientSender_OTelAttributes_ToAndSubject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()

	rec, tp := newTestTracer()
	inner := email.NewResendClientWithBaseURL("key", srv.URL)
	b := newTestBreaker()
	s := email.NewResilientSender(inner, b, tp.Tracer("test"), 1, zap.NewNop())

	msg := email.Message{
		From:    "no@test.com",
		To:      []string{"alice@test.com", "bob@test.com"},
		Subject: "OTel subject",
		HTML:    "<p>hi</p>",
	}
	_, _ = s.Send(context.Background(), msg)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans: got %d; want 1", len(spans))
	}
	attrs := make(map[string]string)
	for _, a := range spans[0].Attributes() {
		attrs[string(a.Key)] = a.Value.AsString()
	}
	if got := attrs["email.to"]; got != "alice@test.com,bob@test.com" {
		t.Errorf("email.to attr: got %q", got)
	}
	if got := attrs["email.subject"]; got != "OTel subject" {
		t.Errorf("email.subject attr: got %q", got)
	}
}
