package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

func bankTransferRouter(t *testing.T, svc *stubBankTransferSvc, fs *stubFileStore) http.Handler {
	t.Helper()
	if fs == nil {
		fs = &stubFileStore{}
	}
	h := handler.NewBankTransferHandler(svc, fs, 1<<20, 0, 0, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Post("/bank-transfers", h.Upload)
	r.Get("/bank-transfers", h.ListMine)
	r.Get("/bank-transfers/pending", h.AdminListPending)
	r.Post("/bank-transfers/{id}/approve", h.AdminApprove)
	r.Post("/bank-transfers/{id}/reject", h.AdminReject)
	return r
}

func bankTransferRouterNoUser(t *testing.T, svc *stubBankTransferSvc) http.Handler {
	t.Helper()
	h := handler.NewBankTransferHandler(svc, &stubFileStore{}, 1<<20, 0, 0, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Post("/bank-transfers", h.Upload)
	r.Get("/bank-transfers", h.ListMine)
	return r
}

func fixedProof() *domain.BankTransferProof {
	now := time.Now().UTC()
	return &domain.BankTransferProof{
		ID: 1, UserID: 10, AmountCents: 5000, Currency: "GTQ",
		StorageKey: "bank-transfers/10/abc.jpg", ContentType: "image/jpeg",
		Status: domain.BankTransferPending, CreatedAt: now, UpdatedAt: now,
	}
}

func multipartUpload(t *testing.T, amountCents int, currency, contentType string, fileData []byte) *bytes.Buffer {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("amount_cents", strconv.Itoa(amountCents))
	if currency != "" {
		_ = w.WriteField("currency", currency)
	}
	if fileData != nil {
		fw, _ := w.CreateFormFile("file", "proof.jpg")
		_, _ = fw.Write(fileData)
	}
	_ = w.Close()
	return body
}

// buildUploadRequest creates a multipart/form-data request with the file's
// Content-Type set on the part header so the handler can read it.
func buildUploadRequest(t *testing.T, amountCents int, currency, fileContentType string, fileData []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("amount_cents", strconv.Itoa(amountCents))
	if currency != "" {
		_ = mw.WriteField("currency", currency)
	}
	if fileData != nil {
		partHeader := make(map[string][]string)
		partHeader["Content-Disposition"] = []string{`form-data; name="file"; filename="proof.jpg"`}
		partHeader["Content-Type"] = []string{fileContentType}
		fw, _ := mw.CreatePart(partHeader)
		_, _ = fw.Write(fileData)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+mw.Boundary())
	return req
}

// ── Upload ────────────────────────────────────────────────────────────────────

func TestBankTransferHandler_Upload_OK(t *testing.T) {
	svc := &stubBankTransferSvc{proof: fixedProof()}
	h := handler.NewBankTransferHandler(svc, &stubFileStore{}, 1<<20, 0, 0, zaptest.NewLogger(t))

	rec := httptest.NewRecorder()
	req := buildUploadRequest(t, 5000, "GTQ", "image/jpeg", []byte("fake-data"))
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	h.Upload(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBankTransferHandler_Upload_Unauthenticated(t *testing.T) {
	router := bankTransferRouterNoUser(t, &stubBankTransferSvc{})
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("amount_cents", "5000")
	fw, _ := mw.CreateFormFile("file", "proof.jpg")
	_, _ = fw.Write([]byte("data"))
	_ = mw.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+mw.Boundary())
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestBankTransferHandler_Upload_InvalidAmountCents(t *testing.T) {
	svc := &stubBankTransferSvc{}
	router := bankTransferRouter(t, svc, nil)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("amount_cents", "0")
	fw, _ := mw.CreateFormFile("file", "proof.jpg")
	_, _ = fw.Write([]byte("data"))
	_ = mw.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+mw.Boundary())
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestBankTransferHandler_Upload_FileStorePutError(t *testing.T) {
	svc := &stubBankTransferSvc{}
	fs := &stubFileStore{putErr: errors.New("s3 down")}
	h := handler.NewBankTransferHandler(svc, fs, 1<<20, 0, 0, zaptest.NewLogger(t))

	rec := httptest.NewRecorder()
	req := buildUploadRequest(t, 5000, "GTQ", "image/jpeg", []byte("fake-data"))
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	h.Upload(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on filestore error, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── ListMine ──────────────────────────────────────────────────────────────────

func TestBankTransferHandler_ListMine_OK(t *testing.T) {
	svc := &stubBankTransferSvc{proofs: []*domain.BankTransferProof{fixedProof()}}
	router := bankTransferRouter(t, svc, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp []handler.BankTransferResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("expected 1 proof, got %d", len(resp))
	}
}

func TestBankTransferHandler_ListMine_Unauthenticated(t *testing.T) {
	router := bankTransferRouterNoUser(t, &stubBankTransferSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── AdminListPending ──────────────────────────────────────────────────────────

func TestBankTransferHandler_AdminListPending_OK(t *testing.T) {
	svc := &stubBankTransferSvc{proofs: []*domain.BankTransferProof{fixedProof(), fixedProof()}}
	router := bankTransferRouter(t, svc, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/pending", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// ── AdminApprove ──────────────────────────────────────────────────────────────

func TestBankTransferHandler_AdminApprove_OK(t *testing.T) {
	proof := fixedProof()
	proof.Status = domain.BankTransferApproved
	svc := &stubBankTransferSvc{proof: proof}
	router := bankTransferRouter(t, svc, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers/1/approve", bytes.NewReader([]byte(`{"notes":"looks good"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestBankTransferHandler_AdminApprove_InvalidID(t *testing.T) {
	router := bankTransferRouter(t, &stubBankTransferSvc{}, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers/bad/approve", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

// ── AdminReject ───────────────────────────────────────────────────────────────

func TestBankTransferHandler_AdminReject_OK(t *testing.T) {
	proof := fixedProof()
	proof.Status = domain.BankTransferRejected
	svc := &stubBankTransferSvc{proof: proof}
	router := bankTransferRouter(t, svc, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers/1/reject", bytes.NewReader([]byte(`{"notes":"bad proof"}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestBankTransferHandler_AdminReject_MissingNotes(t *testing.T) {
	router := bankTransferRouter(t, &stubBankTransferSvc{}, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers/1/reject", bytes.NewReader([]byte(`{"notes":""}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for empty notes, got %d", rec.Code)
	}
}

// ── Amount bounds ─────────────────────────────────────────────────────────────

func TestBankTransferHandler_Upload_BelowMinAmount(t *testing.T) {
	const minCents = 1_000
	h := handler.NewBankTransferHandler(&stubBankTransferSvc{}, &stubFileStore{}, 1<<20, minCents, 10_000_000, zaptest.NewLogger(t))

	rec := httptest.NewRecorder()
	req := buildUploadRequest(t, minCents-1, "GTQ", "image/jpeg", []byte("fake-data"))
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	h.Upload(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for amount below minimum, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBankTransferHandler_Upload_AboveMaxAmount(t *testing.T) {
	const maxCents = 10_000_000
	h := handler.NewBankTransferHandler(&stubBankTransferSvc{}, &stubFileStore{}, 1<<20, 1_000, maxCents, zaptest.NewLogger(t))

	rec := httptest.NewRecorder()
	req := buildUploadRequest(t, maxCents+1, "GTQ", "image/jpeg", []byte("fake-data"))
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	h.Upload(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for amount above maximum, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBankTransferHandler_Upload_AtMinAmount_OK(t *testing.T) {
	const minCents = 1_000
	svc := &stubBankTransferSvc{proof: fixedProof()}
	h := handler.NewBankTransferHandler(svc, &stubFileStore{}, 1<<20, minCents, 10_000_000, zaptest.NewLogger(t))

	rec := httptest.NewRecorder()
	req := buildUploadRequest(t, minCents, "GTQ", "image/jpeg", []byte("fake-data"))
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	h.Upload(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 for amount exactly at minimum, got %d: %s", rec.Code, rec.Body.String())
	}
}

// extractBoundary is a no-op used to satisfy the compiler; actual extraction
// is inlined where needed.
func extractBoundary(_ *bytes.Buffer) string { return "boundary" }
