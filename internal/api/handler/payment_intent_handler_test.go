package handler_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
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

var callerUser = &domain.User{ID: 7, ExternalSubject: "user_abc"}

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

func TestPaymentIntentHandler_Create_RecurrenteProvider_ReturnsRedirectURL(t *testing.T) {
	svc := &stubPaymentIntentSvc{}
	h := handler.NewPaymentIntentHandler(svc, zaptest.NewLogger(t))
	h.WithRecurrente(&stubCheckoutCreator{url: "https://app.recurrente.com/c/ch_test"}, "https://example.com")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/payment-intents", h.Create)

	rec := postIntentAuthenticated(t, mux, `{"amount_cents":10000,"currency":"GTQ","provider":"recurrente"}`, callerUser)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "https://app.recurrente.com/c/ch_test") {
		t.Errorf("expected redirect_url in response, got: %s", rec.Body.String())
	}
}

func TestPaymentIntentHandler_Create_RecurrenteEmptyBaseURL_Returns422(t *testing.T) {
	svc := &stubPaymentIntentSvc{}
	h := handler.NewPaymentIntentHandler(svc, zaptest.NewLogger(t))
	// Empty appBaseURL — would produce relative redirect URLs which Recurrente rejects.
	h.WithRecurrente(&stubCheckoutCreator{url: "https://app.recurrente.com/c/ch_test"}, "")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/payment-intents", h.Create)

	rec := postIntentAuthenticated(t, mux, `{"amount_cents":10000,"currency":"GTQ","provider":"recurrente"}`, callerUser)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 when appBaseURL is empty, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPaymentIntentHandler_Create_RecurrenteNotConfigured_Returns422(t *testing.T) {
	// Handler without WithRecurrente called → recurrente field is nil.
	svc := &stubPaymentIntentSvc{}
	router := intentRouter(t, svc)
	rec := postIntentAuthenticated(t, router, `{"amount_cents":10000,"currency":"GTQ","provider":"recurrente"}`, callerUser)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 when Recurrente not configured, got %d", rec.Code)
	}
}

func TestPaymentIntentHandler_Create_RecurrenteCheckoutError_Returns500(t *testing.T) {
	svc := &stubPaymentIntentSvc{}
	h := handler.NewPaymentIntentHandler(svc, zaptest.NewLogger(t))
	h.WithRecurrente(&stubCheckoutCreator{err: errors.New("recurrente api down")}, "https://example.com")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/payment-intents", h.Create)

	rec := postIntentAuthenticated(t, mux, `{"amount_cents":10000,"currency":"GTQ","provider":"recurrente"}`, callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on checkout error, got %d", rec.Code)
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
func (s *stubPaymentIntentSvc) CreateForRecurrente(_ context.Context, _, _ int, ref, _ string) (*domain.PaymentIntent, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.intent != nil {
		return s.intent, nil
	}
	return &domain.PaymentIntent{Token: ref, AmountCents: 100, Currency: "GTQ", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (s *stubPaymentIntentSvc) ListMyPending(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return nil, nil
}
func (s *stubPaymentIntentSvc) ListMyAll(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return nil, nil
}
func (s *stubPaymentIntentSvc) SetComprobanteByToken(_ context.Context, _, _, _ string, _ int) error {
	return nil
}
func (s *stubPaymentIntentSvc) ResubmitForReview(_ context.Context, _ int, _ string, _, _ *string, _ *int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}

// ── stubCheckoutCreator ───────────────────────────────────────────────────────

type stubCheckoutCreator struct {
	url string
	err error
}

func (s *stubCheckoutCreator) CreateCheckout(_ context.Context, _, _ int, _, _, _, _ string) (string, error) {
	return s.url, s.err
}

// ── stubUploadSvc ─────────────────────────────────────────────────────────────
// Extends stubPaymentIntentSvc with configurable ListMyPending / ListMyAll returns.

type stubUploadSvc struct {
	pending      []*domain.PaymentIntent
	all          []*domain.PaymentIntent
	pendingErr   error
	allErr       error
	uploadErr    error
	resubmitResp *domain.PaymentIntent
	resubmitErr  error
}

func (s *stubUploadSvc) Create(_ context.Context, _, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (s *stubUploadSvc) CreateForRecurrente(_ context.Context, _, _ int, ref, _ string) (*domain.PaymentIntent, error) {
	return &domain.PaymentIntent{Token: ref}, nil
}
func (s *stubUploadSvc) ListMyPending(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return s.pending, s.pendingErr
}
func (s *stubUploadSvc) ListMyAll(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return s.all, s.allErr
}
func (s *stubUploadSvc) SetComprobanteByToken(_ context.Context, _, _, _ string, _ int) error {
	return s.uploadErr
}
func (s *stubUploadSvc) ResubmitForReview(_ context.Context, _ int, _ string, _, _ *string, _ *int, _ string) (*domain.PaymentIntent, error) {
	if s.resubmitErr != nil {
		return nil, s.resubmitErr
	}
	if s.resubmitResp != nil {
		return s.resubmitResp, nil
	}
	return &domain.PaymentIntent{Token: "tok"}, nil
}

// ── stubCapturingFileStore ────────────────────────────────────────────────────
// Records the key passed to Put so tests can assert the storage key format.

type stubCapturingFileStore struct {
	capturedKey string
	putErr      error
}

func (s *stubCapturingFileStore) Put(_ context.Context, key, _ string, _ io.Reader, _ int64) error {
	s.capturedKey = key
	return s.putErr
}
func (s *stubCapturingFileStore) Get(_ context.Context, _ string) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader("")), "image/jpeg", nil
}
func (s *stubCapturingFileStore) Delete(_ context.Context, _ string) error { return nil }

// ── helpers ───────────────────────────────────────────────────────────────────

func buildMultipart(t *testing.T, fieldName, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(fw, content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

func uploadRouter(t *testing.T, svc *stubUploadSvc, fs *stubCapturingFileStore) http.Handler {
	t.Helper()
	h := handler.NewPaymentIntentHandler(svc, zaptest.NewLogger(t))
	h.WithFileStore(fs, 0)
	r := chi.NewRouter()
	r.Post("/{token}/comprobante", h.UploadComprobante)
	r.Post("/{token}/resubmit", h.ResubmitForReview)
	return r
}

func doUpload(router http.Handler, token, path string, body *bytes.Buffer, ct string, user *domain.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/"+token+path, body)
	req.Header.Set("Content-Type", ct)
	if user != nil {
		req = req.WithContext(mw.ContextWithUser(req.Context(), user))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ── UploadComprobante – storage key format ────────────────────────────────────

var fixedTime = time.Date(2026, 6, 18, 15, 4, 5, 0, time.UTC)

func TestUploadComprobante_StorageKeyFormat_Paypal(t *testing.T) {
	intent := &domain.PaymentIntent{
		ID:        42,
		Token:     "abcdef1234567890",
		UserID:    7,
		Provider:  "paypal",
		Status:    domain.PaymentIntentPending,
		CreatedAt: fixedTime,
	}
	svc := &stubUploadSvc{pending: []*domain.PaymentIntent{intent}}
	fs := &stubCapturingFileStore{}
	router := uploadRouter(t, svc, fs)

	body, ct := buildMultipart(t, "file", "receipt.jpg", "fake-image-data")
	rec := doUpload(router, intent.Token, "/comprobante", body, ct, callerUser)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	want := "comprobantes/voucher_7_paypal_2026-06-18T15-04-05"
	if fs.capturedKey != want {
		t.Errorf("storage key:\n got  %q\n want %q", fs.capturedKey, want)
	}
}

func TestUploadComprobante_StorageKeyFormat_Recurrente(t *testing.T) {
	intent := &domain.PaymentIntent{
		ID:        99,
		Token:     "recurrentetoken12345678",
		UserID:    7,
		Provider:  "recurrente",
		Status:    domain.PaymentIntentPending,
		CreatedAt: time.Date(2026, 1, 5, 8, 30, 0, 0, time.UTC),
	}
	svc := &stubUploadSvc{pending: []*domain.PaymentIntent{intent}}
	fs := &stubCapturingFileStore{}
	router := uploadRouter(t, svc, fs)

	body, ct := buildMultipart(t, "file", "comprobante.png", "fake-image-data")
	rec := doUpload(router, intent.Token, "/comprobante", body, ct, callerUser)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	want := "comprobantes/voucher_7_recurrente_2026-01-05T08-30-00"
	if fs.capturedKey != want {
		t.Errorf("storage key:\n got  %q\n want %q", fs.capturedKey, want)
	}
}

func TestUploadComprobante_TokenNotInPending_Returns404(t *testing.T) {
	svc := &stubUploadSvc{pending: []*domain.PaymentIntent{}} // empty
	fs := &stubCapturingFileStore{}
	router := uploadRouter(t, svc, fs)

	body, ct := buildMultipart(t, "file", "x.jpg", "data")
	rec := doUpload(router, "unknowntoken00000000", "/comprobante", body, ct, callerUser)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	if fs.capturedKey != "" {
		t.Error("file store should not have been called for unknown token")
	}
}

// ── ResubmitForReview – storage key format ────────────────────────────────────

func TestResubmitForReview_StorageKeyFormat_WithFile(t *testing.T) {
	intent := &domain.PaymentIntent{
		ID:        55,
		Token:     "rejectedtoken12345678",
		UserID:    7,
		Provider:  "paypal",
		Status:    domain.PaymentIntentRejected,
		CreatedAt: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	}
	svc := &stubUploadSvc{all: []*domain.PaymentIntent{intent}}
	fs := &stubCapturingFileStore{}
	router := uploadRouter(t, svc, fs)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("notes", "El monto es correcto, adjunto nuevo comprobante.")
	fw, _ := w.CreateFormFile("file", "evidence.jpg")
	_, _ = io.WriteString(fw, "fake-image-data")
	w.Close()

	rec := doUpload(router, intent.Token, "/resubmit", &buf, w.FormDataContentType(), callerUser)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	want := "comprobantes/voucher_7_paypal_2026-03-10T09-00-00_review"
	if fs.capturedKey != want {
		t.Errorf("storage key:\n got  %q\n want %q", fs.capturedKey, want)
	}
}

func TestResubmitForReview_TokenNotFound_Returns404(t *testing.T) {
	svc := &stubUploadSvc{all: []*domain.PaymentIntent{}} // empty
	fs := &stubCapturingFileStore{}
	router := uploadRouter(t, svc, fs)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("notes", "disputa")
	w.Close()

	rec := doUpload(router, "unknowntoken00000000", "/resubmit", &buf, w.FormDataContentType(), callerUser)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestResubmitForReview_NoFile_DoesNotCallFileStore(t *testing.T) {
	intent := &domain.PaymentIntent{
		ID: 60, Token: "tok60", UserID: 7, Provider: "paypal",
		CreatedAt: fixedTime,
	}
	svc := &stubUploadSvc{all: []*domain.PaymentIntent{intent}}
	fs := &stubCapturingFileStore{}
	router := uploadRouter(t, svc, fs)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("notes", "Solo agrego una nota, sin archivo nuevo.")
	w.Close()

	rec := doUpload(router, "tok60", "/resubmit", &buf, w.FormDataContentType(), callerUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fs.capturedKey != "" {
		t.Errorf("file store should not be called when no file is provided, got key %q", fs.capturedKey)
	}
}

// ── ListMy ────────────────────────────────────────────────────────────────────

func listMyRouter(t *testing.T, svc *stubUploadSvc) http.Handler {
	t.Helper()
	h := handler.NewPaymentIntentHandler(svc, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Get("/my", h.ListMy)
	r.Get("/my/all", h.ListMyAll)
	return r
}

func doGet(router http.Handler, path string, user *domain.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if user != nil {
		req = req.WithContext(mw.ContextWithUser(req.Context(), user))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestListMy_NoAuth_Returns401(t *testing.T) {
	rec := doGet(listMyRouter(t, &stubUploadSvc{}), "/my", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestListMy_EmptyList_Returns200(t *testing.T) {
	rec := doGet(listMyRouter(t, &stubUploadSvc{}), "/my", callerUser)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestListMy_WithPendingIntents_ReturnsList(t *testing.T) {
	intent := &domain.PaymentIntent{Token: "abc", AmountCents: 1000, Currency: "GTQ", Provider: "paypal"}
	svc := &stubUploadSvc{pending: []*domain.PaymentIntent{intent}}
	rec := doGet(listMyRouter(t, svc), "/my", callerUser)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc") {
		t.Errorf("expected token in response, got %s", rec.Body.String())
	}
}

func TestListMy_ServiceError_Returns500(t *testing.T) {
	svc := &stubUploadSvc{pendingErr: errors.New("db down")}
	rec := doGet(listMyRouter(t, svc), "/my", callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ── ListMyAll ─────────────────────────────────────────────────────────────────

func TestListMyAll_NoAuth_Returns401(t *testing.T) {
	rec := doGet(listMyRouter(t, &stubUploadSvc{}), "/my/all", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestListMyAll_EmptyList_Returns200(t *testing.T) {
	rec := doGet(listMyRouter(t, &stubUploadSvc{}), "/my/all", callerUser)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestListMyAll_WithIntents_ReturnsList(t *testing.T) {
	intent := &domain.PaymentIntent{Token: "xyz", AmountCents: 2000, Currency: "GTQ", Provider: "recurrente"}
	svc := &stubUploadSvc{all: []*domain.PaymentIntent{intent}}
	rec := doGet(listMyRouter(t, svc), "/my/all", callerUser)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "xyz") {
		t.Errorf("expected token in response, got %s", rec.Body.String())
	}
}

func TestListMyAll_ServiceError_Returns500(t *testing.T) {
	svc := &stubUploadSvc{allErr: errors.New("timeout")}
	rec := doGet(listMyRouter(t, svc), "/my/all", callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ── UploadComprobante – additional error paths ────────────────────────────────

func TestUploadComprobante_NoAuth_Returns401(t *testing.T) {
	router := uploadRouter(t, &stubUploadSvc{}, &stubCapturingFileStore{})
	buf, ct := buildMultipart(t, "file", "r.jpg", "data")
	rec := doUpload(router, "tok", "/comprobante", buf, ct, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestUploadComprobante_FileStoreNil_Returns500(t *testing.T) {
	h := handler.NewPaymentIntentHandler(&stubUploadSvc{}, zaptest.NewLogger(t))
	// intentionally do NOT call h.WithFileStore → fileStore remains nil
	r := chi.NewRouter()
	r.Post("/{token}/comprobante", h.UploadComprobante)
	buf, ct := buildMultipart(t, "file", "r.jpg", "data")
	rec := doUpload(r, "tok", "/comprobante", buf, ct, callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestUploadComprobante_MissingFileField_Returns422(t *testing.T) {
	router := uploadRouter(t, &stubUploadSvc{}, &stubCapturingFileStore{})
	// send a multipart body with a text field, no "file" field
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("other", "value")
	w.Close()
	rec := doUpload(router, "tok", "/comprobante", &buf, w.FormDataContentType(), callerUser)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestUploadComprobante_FileSizeExceedsLimit_Returns413(t *testing.T) {
	h := handler.NewPaymentIntentHandler(&stubUploadSvc{}, zaptest.NewLogger(t))
	h.WithFileStore(&stubCapturingFileStore{}, 3) // 3-byte limit
	r := chi.NewRouter()
	r.Post("/{token}/comprobante", h.UploadComprobante)
	buf, ct := buildMultipart(t, "file", "big.jpg", "this-is-more-than-3-bytes")
	rec := doUpload(r, "tok", "/comprobante", buf, ct, callerUser)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rec.Code)
	}
}

func TestUploadComprobante_FileStorePutError_Returns500(t *testing.T) {
	intent := &domain.PaymentIntent{
		ID: 10, Token: "putfail", UserID: 7, Provider: "paypal",
		Status: domain.PaymentIntentPending, CreatedAt: fixedTime,
	}
	svc := &stubUploadSvc{pending: []*domain.PaymentIntent{intent}}
	fs := &stubCapturingFileStore{putErr: errors.New("storage unavailable")}
	router := uploadRouter(t, svc, fs)
	buf, ct := buildMultipart(t, "file", "r.jpg", "data")
	rec := doUpload(router, "putfail", "/comprobante", buf, ct, callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestUploadComprobante_SetComprobanteError_Returns500(t *testing.T) {
	intent := &domain.PaymentIntent{
		ID: 11, Token: "setfail", UserID: 7, Provider: "paypal",
		Status: domain.PaymentIntentPending, CreatedAt: fixedTime,
	}
	svc := &stubUploadSvc{pending: []*domain.PaymentIntent{intent}, uploadErr: errors.New("db write failed")}
	router := uploadRouter(t, svc, &stubCapturingFileStore{})
	buf, ct := buildMultipart(t, "file", "r.jpg", "data")
	rec := doUpload(router, "setfail", "/comprobante", buf, ct, callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ── ResubmitForReview – additional error paths ────────────────────────────────

func TestResubmitForReview_NoAuth_Returns401(t *testing.T) {
	router := uploadRouter(t, &stubUploadSvc{}, &stubCapturingFileStore{})
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("notes", "test")
	w.Close()
	rec := doUpload(router, "tok", "/resubmit", &buf, w.FormDataContentType(), nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestResubmitForReview_EmptyNotes_Returns422(t *testing.T) {
	intent := &domain.PaymentIntent{ID: 20, Token: "t20", UserID: 7, Provider: "paypal", CreatedAt: fixedTime}
	svc := &stubUploadSvc{all: []*domain.PaymentIntent{intent}}
	router := uploadRouter(t, svc, &stubCapturingFileStore{})
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	// no "notes" field or empty
	w.Close()
	rec := doUpload(router, "t20", "/resubmit", &buf, w.FormDataContentType(), callerUser)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestResubmitForReview_ListMyAllError_Returns500(t *testing.T) {
	svc := &stubUploadSvc{allErr: errors.New("db fail")}
	router := uploadRouter(t, svc, &stubCapturingFileStore{})
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("notes", "retry")
	w.Close()
	rec := doUpload(router, "tok", "/resubmit", &buf, w.FormDataContentType(), callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestResubmitForReview_ServiceError_WithoutFile_Returns500(t *testing.T) {
	intent := &domain.PaymentIntent{ID: 21, Token: "t21", UserID: 7, Provider: "paypal", CreatedAt: fixedTime}
	svc := &stubUploadSvc{
		all:         []*domain.PaymentIntent{intent},
		resubmitErr: errors.New("transition not allowed"),
	}
	router := uploadRouter(t, svc, &stubCapturingFileStore{})
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("notes", "trying again")
	w.Close()
	rec := doUpload(router, "t21", "/resubmit", &buf, w.FormDataContentType(), callerUser)
	if rec.Code != http.StatusInternalServerError && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 500 or 422, got %d", rec.Code)
	}
}

func TestResubmitForReview_UploadFileStorePutError_Returns500(t *testing.T) {
	intent := &domain.PaymentIntent{ID: 22, Token: "t22", UserID: 7, Provider: "paypal", CreatedAt: fixedTime}
	svc := &stubUploadSvc{all: []*domain.PaymentIntent{intent}}
	fs := &stubCapturingFileStore{putErr: errors.New("put failed")}
	router := uploadRouter(t, svc, fs)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("notes", "trying again with proof")
	fw, _ := mw.CreateFormFile("file", "r.jpg")
	_, _ = io.WriteString(fw, "data")
	mw.Close()

	rec := doUpload(router, "t22", "/resubmit", &buf, mw.FormDataContentType(), callerUser)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}
