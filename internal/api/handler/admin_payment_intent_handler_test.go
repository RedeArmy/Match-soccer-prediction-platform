package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stubs ──────────────────────────────────────────────────────────────────

type stubAdminIntentStore struct {
	intent  *domain.PaymentIntent
	intents []*domain.PaymentIntent
	total   int
	err     error
}

func (s *stubAdminIntentStore) ListForAdmin(_ context.Context, _ repository.PaymentIntentFilters, _ repository.Pagination) ([]*domain.PaymentIntent, int, error) {
	return s.intents, s.total, s.err
}
func (s *stubAdminIntentStore) GetByID(_ context.Context, _ int64) (*domain.PaymentIntent, error) {
	return s.intent, s.err
}
func (s *stubAdminIntentStore) AdminCreditExpired(_ context.Context, _ int64, _, _ int, _ string) (*domain.PaymentIntent, error) {
	return s.intent, s.err
}
func (s *stubAdminIntentStore) AdminReject(_ context.Context, _ int64, _ int, _ string) (*domain.PaymentIntent, error) {
	return s.intent, s.err
}
func (s *stubAdminIntentStore) RequestComprobante(_ context.Context, _ int64, _ int) (*domain.PaymentIntent, error) {
	return s.intent, s.err
}

// ── helpers ────────────────────────────────────────────────────────────────

func newAdminIntentRouter(t *testing.T, store *stubAdminIntentStore, fs *stubFileStore) http.Handler {
	t.Helper()
	var h *handler.AdminPaymentIntentHandler
	if fs != nil {
		h = handler.NewAdminPaymentIntentHandler(store, fs, zaptest.NewLogger(t))
	} else {
		h = handler.NewAdminPaymentIntentHandler(store, nil, zaptest.NewLogger(t))
	}
	r := chi.NewRouter()
	r.Get("/admin/payment-intents", h.List)
	r.Post("/admin/payment-intents/{id}/credit", h.Credit)
	r.Post("/admin/payment-intents/{id}/reject", h.Reject)
	r.Post("/admin/payment-intents/{id}/request-comprobante", h.RequestComprobante)
	r.Get("/admin/payment-intents/{id}/comprobante", h.DownloadComprobante)
	return r
}

func sampleIntent() *domain.PaymentIntent {
	return &domain.PaymentIntent{
		ID:          1,
		Token:       "tok_abc",
		UserID:      7,
		AmountCents: 5000,
		Currency:    "GTQ",
		Provider:    "paypal",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func doAdminIntent(router http.Handler, method, path, body string, auth bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req = withCaller(req, adminCaller)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── List ───────────────────────────────────────────────────────────────────

func TestAdminPaymentIntentHandler_List_Returns200(t *testing.T) {
	store := &stubAdminIntentStore{intents: []*domain.PaymentIntent{sampleIntent()}, total: 1}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents", "", true)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_List_EmptyReturns200(t *testing.T) {
	store := &stubAdminIntentStore{intents: []*domain.PaymentIntent{}, total: 0}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents", "", true)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_List_WithProviderFilter_Returns200(t *testing.T) {
	store := &stubAdminIntentStore{intents: []*domain.PaymentIntent{sampleIntent()}, total: 1}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents?provider=paypal", "", true)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_List_WithStatusFilter_Returns200(t *testing.T) {
	store := &stubAdminIntentStore{intents: []*domain.PaymentIntent{sampleIntent()}, total: 1}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents?status=pending", "", true)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_List_StoreError_Returns500(t *testing.T) {
	store := &stubAdminIntentStore{err: errors.New("db error")}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents", "", true)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_List_ResponseContainsTotal(t *testing.T) {
	store := &stubAdminIntentStore{intents: []*domain.PaymentIntent{sampleIntent()}, total: 42}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents", "", true)
	if !strings.Contains(w.Body.String(), "42") {
		t.Errorf("expected total in response, got: %s", w.Body.String())
	}
}

// ── Credit ─────────────────────────────────────────────────────────────────

func TestAdminPaymentIntentHandler_Credit_Returns200(t *testing.T) {
	store := &stubAdminIntentStore{intent: sampleIntent()}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/credit", `{"notes":"manual credit"}`, true)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_Credit_NoAuth_Returns401(t *testing.T) {
	store := &stubAdminIntentStore{intent: sampleIntent()}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/credit", `{}`, false)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_Credit_InvalidID_Returns422(t *testing.T) {
	store := &stubAdminIntentStore{}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/abc/credit", `{}`, true)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_Credit_IntentNotFound_Returns404(t *testing.T) {
	store := &stubAdminIntentStore{intent: nil, err: nil}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/99/credit", `{}`, true)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_Credit_GetByIDError_Returns500(t *testing.T) {
	store := &stubAdminIntentStore{intent: nil, err: errors.New("db error")}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/credit", `{}`, true)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_Credit_USDIntentWithFX_Returns200(t *testing.T) {
	intent := sampleIntent()
	intent.Currency = "USD"
	store := &stubAdminIntentStore{intent: intent}
	// stubFXSvc.ConvertUSDToGTQ returns (0, _, nil) — 0 GTQ credits are OK for this path test.
	h := handler.NewAdminPaymentIntentHandler(store, nil, zaptest.NewLogger(t))
	h.SetExchangeRateService(&stubFXSvc{})
	r2 := chi.NewRouter()
	r2.Post("/admin/payment-intents/{id}/credit", h.Credit)
	req := httptest.NewRequest(http.MethodPost, "/admin/payment-intents/1/credit", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = withCaller(req, adminCaller)
	w := httptest.NewRecorder()
	r2.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for USD intent with FX, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_Credit_USDIntentFXError_FallsBackToRawAmount(t *testing.T) {
	intent := sampleIntent()
	intent.Currency = "USD"
	store := &stubAdminIntentStore{intent: intent}
	h := handler.NewAdminPaymentIntentHandler(store, nil, zaptest.NewLogger(t))
	h.SetExchangeRateService(&stubFXSvc{err: errors.New("rate unavailable")})
	r2 := chi.NewRouter()
	r2.Post("/admin/payment-intents/{id}/credit", h.Credit)
	req := httptest.NewRequest(http.MethodPost, "/admin/payment-intents/1/credit", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = withCaller(req, adminCaller)
	w := httptest.NewRecorder()
	r2.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 even when FX conversion fails (falls back to raw amount), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_Reject_ResponseContainsRejectedAt(t *testing.T) {
	rejected := sampleIntent()
	now := time.Now()
	rejected.RejectedAt = &now
	store := &stubAdminIntentStore{intent: rejected}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/reject", `{"notes":"fraud"}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "rejected_at") {
		t.Errorf("expected rejected_at in response, got: %s", w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_Credit_CreditError_Returns500(t *testing.T) {
	// GetByID succeeds, then AdminCreditExpired fails.
	store := &stubAdminIntentStoreGetOKThenFail{
		getIntent: sampleIntent(),
		creditErr: errors.New("credit failed"),
	}
	r2 := chi.NewRouter()
	h := handler.NewAdminPaymentIntentHandler(store, nil, zaptest.NewLogger(t))
	r2.Post("/admin/payment-intents/{id}/credit", h.Credit)

	req := httptest.NewRequest(http.MethodPost, "/admin/payment-intents/1/credit", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = withCaller(req, adminCaller)
	w := httptest.NewRecorder()
	r2.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// stubAdminIntentStoreGetOKThenFail makes GetByID succeed and AdminCreditExpired fail.
type stubAdminIntentStoreGetOKThenFail struct {
	getIntent *domain.PaymentIntent
	creditErr error
}

func (s *stubAdminIntentStoreGetOKThenFail) ListForAdmin(_ context.Context, _ repository.PaymentIntentFilters, _ repository.Pagination) ([]*domain.PaymentIntent, int, error) {
	return nil, 0, nil
}
func (s *stubAdminIntentStoreGetOKThenFail) GetByID(_ context.Context, _ int64) (*domain.PaymentIntent, error) {
	return s.getIntent, nil
}
func (s *stubAdminIntentStoreGetOKThenFail) AdminCreditExpired(_ context.Context, _ int64, _, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, s.creditErr
}
func (s *stubAdminIntentStoreGetOKThenFail) AdminReject(_ context.Context, _ int64, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (s *stubAdminIntentStoreGetOKThenFail) RequestComprobante(_ context.Context, _ int64, _ int) (*domain.PaymentIntent, error) {
	return nil, nil
}

// ── Reject ─────────────────────────────────────────────────────────────────

func TestAdminPaymentIntentHandler_Reject_Returns200(t *testing.T) {
	store := &stubAdminIntentStore{intent: sampleIntent()}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/reject", `{"notes":"duplicate"}`, true)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_Reject_NoAuth_Returns401(t *testing.T) {
	store := &stubAdminIntentStore{intent: sampleIntent()}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/reject", `{"notes":"dup"}`, false)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_Reject_InvalidID_Returns422(t *testing.T) {
	r := newAdminIntentRouter(t, &stubAdminIntentStore{}, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/bad/reject", `{"notes":"x"}`, true)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_Reject_InvalidJSON_Returns422(t *testing.T) {
	r := newAdminIntentRouter(t, &stubAdminIntentStore{}, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/reject", `not-json`, true)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_Reject_EmptyNotes_Returns422(t *testing.T) {
	r := newAdminIntentRouter(t, &stubAdminIntentStore{}, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/reject", `{"notes":""}`, true)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for empty notes, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_Reject_StoreError_Returns500(t *testing.T) {
	store := &stubAdminIntentStore{intent: sampleIntent(), err: errors.New("db error")}
	r := newAdminIntentRouter(t, store, nil)
	// The store returns err for all calls; AdminReject will propagate it.
	// But GetByID is NOT called in Reject, so err applies to AdminReject directly.
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/reject", `{"notes":"problem"}`, true)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── RequestComprobante ─────────────────────────────────────────────────────

func TestAdminPaymentIntentHandler_RequestComprobante_Returns200(t *testing.T) {
	store := &stubAdminIntentStore{intent: sampleIntent()}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/request-comprobante", ``, true)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_RequestComprobante_NoAuth_Returns401(t *testing.T) {
	store := &stubAdminIntentStore{intent: sampleIntent()}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/request-comprobante", ``, false)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_RequestComprobante_InvalidID_Returns422(t *testing.T) {
	r := newAdminIntentRouter(t, &stubAdminIntentStore{}, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/xyz/request-comprobante", ``, true)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_RequestComprobante_StoreError_Returns500(t *testing.T) {
	store := &stubAdminIntentStore{err: errors.New("db error")}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/request-comprobante", ``, true)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_RequestComprobante_ResponseContainsComprobanteRequired(t *testing.T) {
	intent := sampleIntent()
	intent.ComprobanteRequired = true
	store := &stubAdminIntentStore{intent: intent}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodPost, "/admin/payment-intents/1/request-comprobante", ``, true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"comprobante_required":true`) {
		t.Errorf("expected comprobante_required:true in response, got: %s", w.Body.String())
	}
}

// ── DownloadComprobante ────────────────────────────────────────────────────

func TestAdminPaymentIntentHandler_DownloadComprobante_Returns200(t *testing.T) {
	key := "comprobantes/test.jpg"
	intent := sampleIntent()
	intent.ComprobanteKey = &key
	store := &stubAdminIntentStore{intent: intent}
	fs := &stubFileStore{getContent: []byte("JPEG"), getType: "image/jpeg"}
	r := newAdminIntentRouter(t, store, fs)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents/1/comprobante", ``, true)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPaymentIntentHandler_DownloadComprobante_InvalidID_Returns422(t *testing.T) {
	r := newAdminIntentRouter(t, &stubAdminIntentStore{}, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents/bad/comprobante", ``, true)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_DownloadComprobante_IntentNotFound_Returns404(t *testing.T) {
	store := &stubAdminIntentStore{intent: nil, err: apperrors.NotFound("not found")}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents/99/comprobante", ``, true)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_DownloadComprobante_NoComprobanteKey_Returns404(t *testing.T) {
	intent := sampleIntent()
	// ComprobanteKey is nil by default.
	store := &stubAdminIntentStore{intent: intent}
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents/1/comprobante", ``, true)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when comprobante key is nil, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_DownloadComprobante_FileStoreNil_Returns500(t *testing.T) {
	key := "comprobantes/test.pdf"
	intent := sampleIntent()
	intent.ComprobanteKey = &key
	store := &stubAdminIntentStore{intent: intent}
	// Pass nil file store → DownloadComprobante must return 500.
	r := newAdminIntentRouter(t, store, nil)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents/1/comprobante", ``, true)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when file store is nil, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_DownloadComprobante_FileStoreError_Returns500(t *testing.T) {
	key := "comprobantes/missing.jpg"
	intent := sampleIntent()
	intent.ComprobanteKey = &key
	store := &stubAdminIntentStore{intent: intent}
	fs := &stubFileStore{getErr: errors.New("not found in bucket")}
	r := newAdminIntentRouter(t, store, fs)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents/1/comprobante", ``, true)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on filestore error, got %d", w.Code)
	}
}

func TestAdminPaymentIntentHandler_DownloadComprobante_ContentTypeFromIntent(t *testing.T) {
	key := "comprobantes/doc.pdf"
	ct := "application/pdf"
	intent := sampleIntent()
	intent.ComprobanteKey = &key
	intent.ComprobanteContentType = &ct
	store := &stubAdminIntentStore{intent: intent}
	// File store returns empty content type — should fall back to intent's content type.
	fs := &stubFileStore{getContent: []byte("PDF"), getType: ""}
	r := newAdminIntentRouter(t, store, fs)
	w := doAdminIntent(r, http.MethodGet, "/admin/payment-intents/1/comprobante", ``, true)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type: got %q, want application/pdf", got)
	}
}
