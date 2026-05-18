package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// passthroughHandler records calls; used as the sentinel next handler.
var passthroughHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func limiterRequest(subject string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if subject != "" {
		r = r.WithContext(middleware.ContextWithUserID(r.Context(), subject))
	}
	return r
}

// TestRateLimitByUserID_WithinBurst_Allows verifies that the full burst
// allowance passes through to the next handler without being throttled.
func TestRateLimitByUserID_WithinBurst_Allows(t *testing.T) {
	burst := 5
	store := middleware.NewLimiterStore(10.0, burst)
	handler := middleware.RateLimitByUserID(store, zaptest.NewLogger(t))(passthroughHandler)

	for i := range burst {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, limiterRequest("user_abc"))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

// TestRateLimitByUserID_ExceedBurst_Returns429 verifies that requests beyond
// the burst cap are rejected with HTTP 429 Too Many Requests.
func TestRateLimitByUserID_ExceedBurst_Returns429(t *testing.T) {
	store := middleware.NewLimiterStore(1.0, 2)
	handler := middleware.RateLimitByUserID(store, zaptest.NewLogger(t))(passthroughHandler)

	// Consume the burst.
	for i := range 2 {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, limiterRequest("user_xyz"))
		if w.Code != http.StatusOK {
			t.Fatalf("burst request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, limiterRequest("user_xyz"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("over-burst: expected 429, got %d", w.Code)
	}
}

// TestRateLimitByUserID_ExceedBurst_RetryAfterHeaderPresent verifies that a
// throttled response carries a Retry-After header with a positive integer value.
func TestRateLimitByUserID_ExceedBurst_RetryAfterHeaderPresent(t *testing.T) {
	// 0.1 token/s: one token every 10 seconds.
	store := middleware.NewLimiterStore(0.1, 1)
	handler := middleware.RateLimitByUserID(store, zaptest.NewLogger(t))(passthroughHandler)

	// Consume the single token.
	handler.ServeHTTP(httptest.NewRecorder(), limiterRequest("user_slow"))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, limiterRequest("user_slow"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	ra := w.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("Retry-After header missing on 429")
	}
	n, err := strconv.Atoi(ra)
	if err != nil || n < 1 {
		t.Fatalf("Retry-After %q is not a positive integer", ra)
	}
}

// TestRateLimitByUserID_BodyContainsRateLimitedCode verifies the response body
// carries the machine-readable RATE_LIMITED code for client-side handling.
func TestRateLimitByUserID_BodyContainsRateLimitedCode(t *testing.T) {
	store := middleware.NewLimiterStore(1.0, 1)
	handler := middleware.RateLimitByUserID(store, zaptest.NewLogger(t))(passthroughHandler)

	handler.ServeHTTP(httptest.NewRecorder(), limiterRequest("user_code"))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, limiterRequest("user_code"))

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != string(apperrors.CodeRateLimited) {
		t.Errorf("code: expected %q, got %q", apperrors.CodeRateLimited, body.Error.Code)
	}
}

// TestRateLimitByUserID_PerUser_IsolatesKeys verifies that throttling one user
// does not deplete a different user's token bucket.
func TestRateLimitByUserID_PerUser_IsolatesKeys(t *testing.T) {
	store := middleware.NewLimiterStore(1.0, 1)
	handler := middleware.RateLimitByUserID(store, zaptest.NewLogger(t))(passthroughHandler)

	// Drain user_a.
	handler.ServeHTTP(httptest.NewRecorder(), limiterRequest("user_a"))
	wa := httptest.NewRecorder()
	handler.ServeHTTP(wa, limiterRequest("user_a"))
	if wa.Code != http.StatusTooManyRequests {
		t.Fatalf("user_a: expected 429, got %d", wa.Code)
	}

	// user_b should still have their own fresh bucket.
	wb := httptest.NewRecorder()
	handler.ServeHTTP(wb, limiterRequest("user_b"))
	if wb.Code != http.StatusOK {
		t.Fatalf("user_b: expected 200, got %d", wb.Code)
	}
}

// TestNewUnlimitedLimiterStore_NeverThrottles verifies that a store created with
// NewUnlimitedLimiterStore allows an arbitrarily large number of requests for the
// same key without ever returning 429.
func TestNewUnlimitedLimiterStore_NeverThrottles(t *testing.T) {
	store := middleware.NewUnlimitedLimiterStore()
	handler := middleware.RateLimitByUserID(store, zaptest.NewLogger(t))(passthroughHandler)

	for i := range 200 {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, limiterRequest("user_unlimited"))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 from unlimited store, got %d", i+1, w.Code)
		}
	}
}

// TestRateLimitByUserID_NoSubjectInContext_PassesThrough verifies that requests
// without a Clerk subject in context bypass rate limiting entirely — the auth
// middleware upstream is responsible for rejecting unauthenticated requests.
func TestRateLimitByUserID_NoSubjectInContext_PassesThrough(t *testing.T) {
	store := middleware.NewLimiterStore(1.0, 1)
	handler := middleware.RateLimitByUserID(store, zaptest.NewLogger(t))(passthroughHandler)

	// Saturate the store with a named key so the test is non-trivial.
	handler.ServeHTTP(httptest.NewRecorder(), limiterRequest("other_user"))
	handler.ServeHTTP(httptest.NewRecorder(), limiterRequest("other_user"))

	// Request with no subject in context should still pass through.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, limiterRequest(""))
	if w.Code != http.StatusOK {
		t.Fatalf("no-subject pass-through: expected 200, got %d", w.Code)
	}
}
