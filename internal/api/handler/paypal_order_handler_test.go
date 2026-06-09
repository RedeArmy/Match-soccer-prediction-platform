package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	mw "github.com/rede/world-cup-quiniela/internal/middleware"
)

// ── stub PayPal server ────────────────────────────────────────────────────────

type paypalServerOpts struct {
	tokenStatus int
	tokenBody   string
	orderStatus int
	orderBody   string
}

func newPayPalServer(t *testing.T, opts paypalServerOpts) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/oauth2/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(opts.tokenStatus)
		_, _ = w.Write([]byte(opts.tokenBody))
	})
	mux.HandleFunc("/v2/checkout/orders", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(opts.orderStatus)
		_, _ = w.Write([]byte(opts.orderBody))
	})
	return httptest.NewServer(mux)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func paypalRouter(t *testing.T, svc *stubPaymentIntentSvc, baseURL string) http.Handler {
	t.Helper()
	h := handler.NewPayPalOrderHandler("test-client-id", "test-secret", baseURL, true, svc, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/paypal/create-order", h.Create)
	return mux
}

func postPayPalOrder(t *testing.T, router http.Handler, body string, user *domain.User) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/paypal/create-order", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != nil {
		req = req.WithContext(mw.ContextWithUser(req.Context(), user))
	}
	router.ServeHTTP(rec, req)
	return rec
}

var paypalTestUser = &domain.User{ID: 42, ExternalSubject: "user_paypal_test"}

var successIntent = &domain.PaymentIntent{
	Token:       "intent-tok-abc",
	AmountCents: 1000,
	Currency:    "USD",
	ExpiresAt:   time.Now().Add(time.Hour),
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestPayPalOrderHandler_NewDefaultURLDev(t *testing.T) {
	h := handler.NewPayPalOrderHandler("id", "secret", "", true, &stubPaymentIntentSvc{}, zaptest.NewLogger(t))
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestPayPalOrderHandler_NewDefaultURLProd(t *testing.T) {
	h := handler.NewPayPalOrderHandler("id", "secret", "", false, &stubPaymentIntentSvc{}, zaptest.NewLogger(t))
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestPayPalOrderHandler_MissingCredentials_Returns503(t *testing.T) {
	h := handler.NewPayPalOrderHandler("", "", "http://unused", true, &stubPaymentIntentSvc{}, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/paypal/create-order", h.Create)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/paypal/create-order", strings.NewReader(`{}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_NoAuth_Returns401(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"tok"}`,
		orderStatus: 201, orderBody: `{"id":"ORD"}`,
	})
	defer srv.Close()
	router := paypalRouter(t, &stubPaymentIntentSvc{}, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":1000,"currency":"USD"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_BadJSON_Returns400(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"tok"}`,
		orderStatus: 201, orderBody: `{"id":"ORD"}`,
	})
	defer srv.Close()
	router := paypalRouter(t, &stubPaymentIntentSvc{}, srv.URL)
	rec := postPayPalOrder(t, router, `not-json`, paypalTestUser)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad JSON, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_ZeroAmount_Returns400(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"tok"}`,
		orderStatus: 201, orderBody: `{"id":"ORD"}`,
	})
	defer srv.Close()
	router := paypalRouter(t, &stubPaymentIntentSvc{}, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":0,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on zero amount, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_MissingCurrency_Returns400(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"tok"}`,
		orderStatus: 201, orderBody: `{"id":"ORD"}`,
	})
	defer srv.Close()
	router := paypalRouter(t, &stubPaymentIntentSvc{}, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":500}`, paypalTestUser)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on missing currency, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_ServiceCreateError_Returns500(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"tok"}`,
		orderStatus: 201, orderBody: `{"id":"ORD"}`,
	})
	defer srv.Close()
	svc := &stubPaymentIntentSvc{err: errors.New("db connection lost")}
	router := paypalRouter(t, svc, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":1000,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on service error, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_PayPalAuthFails_Returns502(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 401, tokenBody: `{"error":"unauthorized"}`,
		orderStatus: 201, orderBody: `{"id":"ORD"}`,
	})
	defer srv.Close()
	svc := &stubPaymentIntentSvc{intent: successIntent}
	router := paypalRouter(t, svc, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":1000,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502 on PayPal auth failure, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_PayPalOrderFails_Returns502(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"tok"}`,
		orderStatus: 422, orderBody: `{"name":"UNPROCESSABLE_ENTITY"}`,
	})
	defer srv.Close()
	svc := &stubPaymentIntentSvc{intent: successIntent}
	router := paypalRouter(t, svc, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":1000,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502 on PayPal order failure, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_PayPalNetworkError_Returns502(t *testing.T) {
	// Close the server immediately so the HTTP client gets a connection-refused error.
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"tok"}`,
		orderStatus: 201, orderBody: `{"id":"ORD"}`,
	})
	srv.Close()
	svc := &stubPaymentIntentSvc{intent: successIntent}
	router := paypalRouter(t, svc, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":1000,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502 on network error, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_PayPalTokenBadJSON_Returns502(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `not-valid-json`,
		orderStatus: 201, orderBody: `{"id":"ORD"}`,
	})
	defer srv.Close()
	svc := &stubPaymentIntentSvc{intent: successIntent}
	router := paypalRouter(t, svc, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":1000,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502 on bad token JSON, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_PayPalOrderNetworkError_Returns502(t *testing.T) {
	// The token endpoint succeeds, but the orders endpoint drops the connection
	// immediately so h.client.Do() returns a network error inside createOrder.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/oauth2/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"tok"}`))
	})
	mux.HandleFunc("/v2/checkout/orders", func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack not supported", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	svc := &stubPaymentIntentSvc{intent: successIntent}
	router := paypalRouter(t, svc, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":1000,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502 on order network error, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_PayPalOrderBadJSON_Returns502(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"tok"}`,
		orderStatus: 201, orderBody: `not-valid-json`,
	})
	defer srv.Close()
	svc := &stubPaymentIntentSvc{intent: successIntent}
	router := paypalRouter(t, svc, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":1000,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502 on bad order JSON, got %d", rec.Code)
	}
}

func TestPayPalOrderHandler_Success_Returns201WithOrderID(t *testing.T) {
	srv := newPayPalServer(t, paypalServerOpts{
		tokenStatus: 200, tokenBody: `{"access_token":"test-access-token"}`,
		orderStatus: 201, orderBody: `{"id":"ORDER-XYZ-789"}`,
	})
	defer srv.Close()
	svc := &stubPaymentIntentSvc{intent: successIntent}
	router := paypalRouter(t, svc, srv.URL)
	rec := postPayPalOrder(t, router, `{"amount_cents":2500,"currency":"USD"}`, paypalTestUser)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if resp.ID != "ORDER-XYZ-789" {
		t.Errorf("expected order id ORDER-XYZ-789, got %q", resp.ID)
	}
}
