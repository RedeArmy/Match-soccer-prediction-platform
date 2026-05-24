package observability_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/observability"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// testServer records the last request received and returns 200.
type testServer struct {
	mu      sync.Mutex
	bodies  []string
	paths   []string
	headers []http.Header
}

func (ts *testServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ts.mu.Lock()
		ts.bodies = append(ts.bodies, string(b))
		ts.paths = append(ts.paths, r.URL.Path)
		ts.headers = append(ts.headers, r.Header.Clone())
		ts.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func (ts *testServer) lastBody() string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.bodies) == 0 {
		return ""
	}
	return ts.bodies[len(ts.bodies)-1]
}

func (ts *testServer) lastPath() string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.paths) == 0 {
		return ""
	}
	return ts.paths[len(ts.paths)-1]
}

func (ts *testServer) lastHeader() http.Header {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.headers) == 0 {
		return nil
	}
	return ts.headers[len(ts.headers)-1]
}

func (ts *testServer) count() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.bodies)
}

// waitForRequest blocks until count rises to n or 2 s elapses.
func (ts *testServer) waitForCount(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ts.count() >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d webhook call(s); got %d", n, ts.count())
}

func newNotifier(t *testing.T, baseURL, secret string) *observability.Notifier {
	t.Helper()
	log := zaptest.NewLogger(t)
	return observability.New(observability.NotifierConfig{
		BaseURL: baseURL,
		Secret:  secret,
		Log:     log,
	})
}

// ── Enabled / disabled ────────────────────────────────────────────────────────

func TestObservabilityNotifier_Disabled_WhenBaseURLEmpty(t *testing.T) {
	n := newNotifier(t, "", "")
	if n.Enabled() {
		t.Error("expected Enabled() == false when BaseURL is empty")
	}
}

func TestObservabilityNotifier_Enabled_WhenBaseURLSet(t *testing.T) {
	n := newNotifier(t, "http://localhost:1234", "secret")
	if !n.Enabled() {
		t.Error("expected Enabled() == true when BaseURL is set")
	}
}

func TestObservabilityNotifier_Disabled_AllMethodsAreNoOps(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	// baseURL empty → disabled; the real server should never receive a request.
	n := newNotifier(t, "", "")
	ctx := context.Background()

	n.NotifyDLQOverflow(ctx, 100, 50)
	n.NotifyCircuitBreakerOpen(ctx, "s3", "open", time.Now())
	n.NotifyPaymentError(ctx, "recurrente", "ERR", "1", "500")
	n.NotifyOutboxLag(ctx, 45.0, 10)
	n.NotifyPayoutApproved(ctx, 1, 500, "bank_gt", "admin_1")
	n.NotifyTransferUploaded(ctx, 1, 500, "bank-transfers/1/abc.jpg")
	n.NotifyBalanceCredited(ctx, 1, 500, "recurrente")

	// Give goroutines time to fire (they shouldn't).
	time.Sleep(50 * time.Millisecond)
	if ts.count() != 0 {
		t.Errorf("expected 0 requests when disabled, got %d", ts.count())
	}
}

// ── Webhook paths ─────────────────────────────────────────────────────────────

func TestNotifyDLQOverflow_PostsToCorrectPath(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyDLQOverflow(context.Background(), 75, 50)
	ts.waitForCount(t, 1)

	if got := ts.lastPath(); got != "/webhook/dlq-overflow" {
		t.Errorf("expected /webhook/dlq-overflow, got %s", got)
	}
}

func TestNotifyCircuitBreakerOpen_PostsToCorrectPath(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyCircuitBreakerOpen(context.Background(), "s3", "open", time.Now())
	ts.waitForCount(t, 1)

	if got := ts.lastPath(); got != "/webhook/circuit-breaker" {
		t.Errorf("expected /webhook/circuit-breaker, got %s", got)
	}
}

func TestNotifyPaymentError_PostsToCorrectPath(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyPaymentError(context.Background(), "paypal", "5xx", "42", "1000")
	ts.waitForCount(t, 1)

	if got := ts.lastPath(); got != "/webhook/payment-error" {
		t.Errorf("expected /webhook/payment-error, got %s", got)
	}
}

func TestNotifyPayoutApproved_PostsToCorrectPath(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyPayoutApproved(context.Background(), 1, 1000, "bank_gt", "admin_1")
	ts.waitForCount(t, 1)

	if got := ts.lastPath(); got != "/webhook/payout-approved" {
		t.Errorf("expected /webhook/payout-approved, got %s", got)
	}
}

func TestNotifyTransferUploaded_PostsToCorrectPath(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyTransferUploaded(context.Background(), 1, 500, "bank-transfers/1/proof.jpg")
	ts.waitForCount(t, 1)

	if got := ts.lastPath(); got != "/webhook/transfer-uploaded" {
		t.Errorf("expected /webhook/transfer-uploaded, got %s", got)
	}
}

func TestNotifyBalanceCredited_PostsToCorrectPath(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyBalanceCredited(context.Background(), 1, 500, "recurrente")
	ts.waitForCount(t, 1)

	if got := ts.lastPath(); got != "/webhook/balance-credited" {
		t.Errorf("expected /webhook/balance-credited, got %s", got)
	}
}

// ── Payload fields ────────────────────────────────────────────────────────────

func TestNotifyDLQOverflow_PayloadFields(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyDLQOverflow(context.Background(), 75, 50)
	ts.waitForCount(t, 1)

	var p observability.DLQOverflowPayload
	if err := json.Unmarshal([]byte(ts.lastBody()), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.DLQDepth != 75 {
		t.Errorf("dlq_depth: want 75, got %d", p.DLQDepth)
	}
	if p.Threshold != 50 {
		t.Errorf("threshold: want 50, got %d", p.Threshold)
	}
	if p.Timestamp == "" {
		t.Error("timestamp must not be empty")
	}
}

func TestNotifyCircuitBreakerOpen_PayloadFields(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	tripAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	n.NotifyCircuitBreakerOpen(context.Background(), "gdrive", "open", tripAt)
	ts.waitForCount(t, 1)

	var p observability.CircuitBreakerPayload
	if err := json.Unmarshal([]byte(ts.lastBody()), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Backend != "gdrive" {
		t.Errorf("backend: want gdrive, got %s", p.Backend)
	}
	if p.State != "open" {
		t.Errorf("state: want open, got %s", p.State)
	}
	if !strings.Contains(p.LastTripAt, "2026-01-01") {
		t.Errorf("last_trip_at: want 2026-01-01, got %s", p.LastTripAt)
	}
}

func TestNotifyPaymentError_PayloadFields(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyPaymentError(context.Background(), "recurrente", "credit_failed", "42", "2500")
	ts.waitForCount(t, 1)

	var p observability.PaymentErrorPayload
	if err := json.Unmarshal([]byte(ts.lastBody()), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Provider != "recurrente" {
		t.Errorf("provider: want recurrente, got %s", p.Provider)
	}
	if p.ErrorCode != "credit_failed" {
		t.Errorf("error_code: want credit_failed, got %s", p.ErrorCode)
	}
	if p.UserID != "42" {
		t.Errorf("user_id: want 42, got %s", p.UserID)
	}
}

// ── HMAC signing ──────────────────────────────────────────────────────────────

func TestNotifier_HMACSignature_PresentWhenSecretSet(t *testing.T) {
	const secret = "test-secret"
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, secret)
	n.NotifyDLQOverflow(context.Background(), 10, 5)
	ts.waitForCount(t, 1)

	sig := ts.lastHeader().Get("X-Signature")
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("X-Signature header absent or malformed: %q", sig)
	}
	// Verify the HMAC matches the body.
	body := []byte(ts.lastBody())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if sig != expected {
		t.Errorf("HMAC mismatch:\n  got  %s\n  want %s", sig, expected)
	}
}

func TestNotifier_HMACSignature_AbsentWhenSecretEmpty(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	// Suppress the unsigned-request warning in logs.
	log, _ := zap.NewDevelopment()
	n := observability.New(observability.NotifierConfig{
		BaseURL: srv.URL,
		Secret:  "", // no secret
		Log:     log,
	})
	n.NotifyDLQOverflow(context.Background(), 10, 5)
	ts.waitForCount(t, 1)

	if sig := ts.lastHeader().Get("X-Signature"); sig != "" {
		t.Errorf("expected no X-Signature when secret is empty, got %q", sig)
	}
}

// ── Non-2xx response ──────────────────────────────────────────────────────────

func TestNotifier_Non2xxResponse_LoggedNotPanicked(t *testing.T) {
	// Server always returns 503 — notifier must log a warning and not crash.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	log := zaptest.NewLogger(t)
	n := observability.New(observability.NotifierConfig{
		BaseURL: srv.URL,
		Secret:  "",
		Log:     log,
	})

	// Must not panic.
	n.NotifyDLQOverflow(context.Background(), 10, 5)
	time.Sleep(200 * time.Millisecond)
}

// ── Content-Type ──────────────────────────────────────────────────────────────

func TestNotifier_ContentType_IsApplicationJSON(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	n.NotifyBalanceCredited(context.Background(), 1, 100, "win")
	ts.waitForCount(t, 1)

	if ct := ts.lastHeader().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %s", ct)
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestNotifier_ConcurrentCalls_AllDelivered(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	const calls = 20
	var wg sync.WaitGroup
	wg.Add(calls)
	for i := range calls {
		go func(i int) {
			defer wg.Done()
			n.NotifyDLQOverflow(context.Background(), int64(i), 5)
		}(i)
	}
	wg.Wait()
	ts.waitForCount(t, calls)

	if ts.count() != calls {
		t.Errorf("expected %d deliveries, got %d", calls, ts.count())
	}
}
