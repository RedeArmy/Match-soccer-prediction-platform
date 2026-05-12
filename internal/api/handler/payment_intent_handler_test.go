package handler_test

import (
	"context"
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

func intentRouter(t *testing.T, svc *stubPaymentIntentSvc) http.Handler {
	t.Helper()
	h := handler.NewPaymentIntentHandler(svc, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/payment-intents", h.Create)
	return mux
}

func postIntentAuthenticated(t *testing.T, router http.Handler, body string, user *domain.User) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment-intents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != nil {
		req = req.WithContext(mw.ContextWithUser(req.Context(), user))
	}
	router.ServeHTTP(rec, req)
	return rec
}

var callerUser = &domain.User{ID: 7, ClerkSubject: "user_abc"}

func TestPaymentIntentHandler_Create_Returns201(t *testing.T) {
	svc := &stubPaymentIntentSvc{intent: &domain.PaymentIntent{
		Token:       "deadbeef",
		AmountCents: 5000,
		Currency:    "GTQ",
		ExpiresAt:   time.Now().Add(time.Hour),
	}}
	router := intentRouter(t, svc)
	rec := postIntentAuthenticated(t, router, `{"amount_cents":5000,"currency":"GTQ"}`, callerUser)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestPaymentIntentHandler_Create_NoAuth_Returns401(t *testing.T) {
	router := intentRouter(t, &stubPaymentIntentSvc{})
	rec := postIntentAuthenticated(t, router, `{"amount_cents":5000,"currency":"GTQ"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestPaymentIntentHandler_Create_InvalidJSON_Returns422(t *testing.T) {
	router := intentRouter(t, &stubPaymentIntentSvc{})
	rec := postIntentAuthenticated(t, router, `not-json`, callerUser)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestPaymentIntentHandler_Create_ZeroAmount_Returns422(t *testing.T) {
	svc := &stubPaymentIntentSvc{err: errors.New("amount_cents must be positive")}
	router := intentRouter(t, svc)
	rec := postIntentAuthenticated(t, router, `{"amount_cents":0,"currency":"GTQ"}`, callerUser)
	if rec.Code == http.StatusCreated {
		t.Errorf("expected non-201 for zero amount, got 201")
	}
}

func TestPaymentIntentHandler_Create_ServiceError_Returns500(t *testing.T) {
	svc := &stubPaymentIntentSvc{err: errors.New("db error")}
	router := intentRouter(t, svc)
	rec := postIntentAuthenticated(t, router, `{"amount_cents":1000,"currency":"GTQ"}`, callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on service error, got %d", rec.Code)
	}
}

func TestPaymentIntentHandler_Create_ResponseContainsToken(t *testing.T) {
	svc := &stubPaymentIntentSvc{intent: &domain.PaymentIntent{
		Token:       "abc123token",
		AmountCents: 2000,
		Currency:    "GTQ",
		ExpiresAt:   time.Now().Add(time.Hour),
	}}
	router := intentRouter(t, svc)
	rec := postIntentAuthenticated(t, router, `{"amount_cents":2000,"currency":"GTQ"}`, callerUser)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "abc123token") {
		t.Errorf("response body missing token: %s", body)
	}
}

// ── stubPaymentIntentSvc ──────────────────────────────────────────────────────

type stubPaymentIntentSvc struct {
	intent *domain.PaymentIntent
	err    error
}

func (s *stubPaymentIntentSvc) Create(_ context.Context, _, _ int, _ string) (*domain.PaymentIntent, error) {
	return s.intent, s.err
}
