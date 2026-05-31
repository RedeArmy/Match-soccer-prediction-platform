package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

func withdrawalRouter(t *testing.T, svc *stubWithdrawalSvc) http.Handler {
	t.Helper()
	h := handler.NewWithdrawalHandler(svc, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Post("/withdrawals", h.Create)
	r.Get("/withdrawals", h.ListMine)
	r.Get("/withdrawals/pending", h.AdminListPending)
	r.Post("/withdrawals/{id}/approve", h.AdminApprove)
	r.Post("/withdrawals/{id}/reject", h.AdminReject)
	r.Post("/withdrawals/{id}/process", h.AdminProcess)
	return r
}

func withdrawalRouterNoUser(t *testing.T, svc *stubWithdrawalSvc) http.Handler {
	t.Helper()
	h := handler.NewWithdrawalHandler(svc, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Post("/withdrawals", h.Create)
	r.Get("/withdrawals", h.ListMine)
	return r
}

func fixedWithdrawal() *domain.WithdrawalRequest {
	now := time.Now().UTC()
	return &domain.WithdrawalRequest{
		ID: 1, UserID: 10, AmountCents: 5000, Currency: "GTQ",
		Method: domain.WithdrawalMethodBankGT, Status: domain.WithdrawalPending,
		CreatedAt: now, UpdatedAt: now,
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestWithdrawalHandler_Create_OK(t *testing.T) {
	svc := &stubWithdrawalSvc{req: fixedWithdrawal()}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"amount_cents":5000,"currency":"GTQ","method":"bank_gt","payout_details":{"account_number":"12345678901","bank_name":"BAC Guatemala"}}`))
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp handler.WithdrawalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != 1 {
		t.Errorf("expected ID 1, got %d", resp.ID)
	}
}

func TestWithdrawalHandler_Create_PayPal_OK(t *testing.T) {
	w := fixedWithdrawal()
	w.Method = domain.WithdrawalMethodPayPal
	svc := &stubWithdrawalSvc{req: w}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"amount_cents":2000,"currency":"USD","method":"paypal","payout_details":{"paypal_email":"user@example.com"}}`))
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_Create_DefaultCurrency(t *testing.T) {
	svc := &stubWithdrawalSvc{req: fixedWithdrawal()}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"amount_cents":1000,"method":"bank_gt","payout_details":{"account_number":"12345678901","bank_name":"BAC Guatemala"}}`))
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 with default currency, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_Create_Unauthenticated(t *testing.T) {
	router := withdrawalRouterNoUser(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"amount_cents":5000,"method":"bank_gt"}`))
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_Create_InvalidAmountCents(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"amount_cents":0,"method":"bank_gt"}`))
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_Create_InvalidMethod(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"amount_cents":5000,"method":"venmo"}`))
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid method, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_Create_InvalidJSON(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid JSON, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_Create_BodyTooLarge_Returns413(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	// Valid JSON with a large payout_details value that pushes the body over 4 KB.
	padded := `{"amount_cents":5000,"method":"bank_gt","payout_details":{"note":"` +
		string(bytes.Repeat([]byte("x"), 5000)) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader([]byte(padded)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for oversized body, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_Create_ServiceError(t *testing.T) {
	svc := &stubWithdrawalSvc{err: errors.New("insufficient balance")}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"amount_cents":9999999,"method":"bank_gt"}`))
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Errorf("expected error status, got %d", rec.Code)
	}
}

// ── ListMine ──────────────────────────────────────────────────────────────────

func TestWithdrawalHandler_ListMine_OK(t *testing.T) {
	svc := &stubWithdrawalSvc{reqs: []*domain.WithdrawalRequest{fixedWithdrawal()}}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/withdrawals", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp []handler.WithdrawalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("expected 1 withdrawal, got %d", len(resp))
	}
}

func TestWithdrawalHandler_ListMine_Empty(t *testing.T) {
	svc := &stubWithdrawalSvc{reqs: nil}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/withdrawals", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_ListMine_Unauthenticated(t *testing.T) {
	router := withdrawalRouterNoUser(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/withdrawals", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── AdminListPending ──────────────────────────────────────────────────────────

func TestWithdrawalHandler_AdminListPending_OK(t *testing.T) {
	svc := &stubWithdrawalSvc{reqs: []*domain.WithdrawalRequest{fixedWithdrawal(), fixedWithdrawal()}}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/withdrawals/pending", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp []handler.WithdrawalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 withdrawals, got %d", len(resp))
	}
}

func TestWithdrawalHandler_AdminListPending_MasksPayoutDetails(t *testing.T) {
	w := fixedWithdrawal()
	w.PayoutDetails = map[string]string{
		"account_number": "12345678901",
		"bank_name":      "BAC Guatemala",
	}
	svc := &stubWithdrawalSvc{reqs: []*domain.WithdrawalRequest{w}}
	router := withdrawalRouter(t, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/withdrawals/pending", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp []handler.WithdrawalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp))
	}

	pd := resp[0].PayoutDetails
	// account_number must be masked — last 4 digits only.
	if pd["account_number"] == "12345678901" {
		t.Error("admin list response must not expose plain account_number")
	}
	if pd["account_number"] != "*******8901" {
		t.Errorf("account_number mask: got %q, want %q", pd["account_number"], "*******8901")
	}
	// bank_name is non-sensitive routing info — must not be masked.
	if pd["bank_name"] != "BAC Guatemala" {
		t.Errorf("bank_name should not be masked, got %q", pd["bank_name"])
	}
}

func TestWithdrawalHandler_ListMine_DoesNotMaskPayoutDetails(t *testing.T) {
	// Users must see their own full account details.
	w := fixedWithdrawal()
	w.PayoutDetails = map[string]string{
		"account_number": "12345678901",
		"bank_name":      "BAC Guatemala",
	}
	svc := &stubWithdrawalSvc{reqs: []*domain.WithdrawalRequest{w}}
	router := withdrawalRouter(t, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/withdrawals", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp []handler.WithdrawalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp))
	}

	if resp[0].PayoutDetails["account_number"] != "12345678901" {
		t.Errorf("user list must return full account_number, got %q", resp[0].PayoutDetails["account_number"])
	}
}

func TestWithdrawalHandler_AdminListPending_ServiceError(t *testing.T) {
	svc := &stubWithdrawalSvc{err: errors.New("db error")}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/withdrawals/pending", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ── AdminApprove ──────────────────────────────────────────────────────────────

func TestWithdrawalHandler_AdminApprove_OK(t *testing.T) {
	w := fixedWithdrawal()
	w.Status = domain.WithdrawalApproved
	svc := &stubWithdrawalSvc{req: w}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/approve", bytes.NewReader([]byte(`{"notes":"looks good"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithdrawalHandler_AdminApprove_NoBody_OK(t *testing.T) {
	w := fixedWithdrawal()
	w.Status = domain.WithdrawalApproved
	svc := &stubWithdrawalSvc{req: w}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/approve", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with no body, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_AdminApprove_InvalidID(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/bad/approve", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_AdminApprove_ServiceError(t *testing.T) {
	svc := &stubWithdrawalSvc{err: errors.New("not found")}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/approve", nil)
	router.ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Errorf("expected error status, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_AdminApprove_BodyTooLarge_Returns413(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	padded := `{"notes":"` + string(bytes.Repeat([]byte("x"), 5000)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/approve", bytes.NewReader([]byte(padded)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for oversized approve body, got %d", rec.Code)
	}
}

// ── AdminReject ───────────────────────────────────────────────────────────────

func TestWithdrawalHandler_AdminReject_OK(t *testing.T) {
	w := fixedWithdrawal()
	w.Status = domain.WithdrawalRejected
	svc := &stubWithdrawalSvc{req: w}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/reject", bytes.NewReader([]byte(`{"notes":"duplicate request"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithdrawalHandler_AdminReject_MissingNotes(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/reject", bytes.NewReader([]byte(`{"notes":""}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for empty notes, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_AdminReject_InvalidID(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/bad/reject", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_AdminReject_Unauthenticated(t *testing.T) {
	h := handler.NewWithdrawalHandler(&stubWithdrawalSvc{}, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Post("/withdrawals/{id}/reject", h.AdminReject)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/reject",
		bytes.NewReader([]byte(`{"notes":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rec.Code)
	}
}

func TestWithdrawalHandler_AdminReject_BadJSON(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/reject",
		bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestWithdrawalHandler_AdminReject_ServiceError(t *testing.T) {
	svc := &stubWithdrawalSvc{err: errors.New("conflict")}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/reject",
		bytes.NewReader([]byte(`{"notes":"duplicate"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Errorf("expected error status, got %d", rec.Code)
	}
}

// ── AdminProcess ──────────────────────────────────────────────────────────────

func TestWithdrawalHandler_AdminProcess_OK(t *testing.T) {
	w := fixedWithdrawal()
	w.Status = domain.WithdrawalProcessed
	svc := &stubWithdrawalSvc{req: w}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/process", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithdrawalHandler_AdminProcess_InvalidID(t *testing.T) {
	router := withdrawalRouter(t, &stubWithdrawalSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/bad/process", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestWithdrawalHandler_AdminProcess_ServiceError(t *testing.T) {
	svc := &stubWithdrawalSvc{err: errors.New("already processed")}
	router := withdrawalRouter(t, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/withdrawals/1/process", nil)
	router.ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Errorf("expected error status, got %d", rec.Code)
	}
}
