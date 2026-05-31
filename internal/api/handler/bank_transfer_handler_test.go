package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	r.Get("/bank-transfers/{id}/download", h.DownloadProof)
	r.Get("/bank-transfers/pending", h.AdminListPending)
	r.Get("/admin/bank-transfers/{id}/download", h.AdminDownloadProof)
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

// jpegMagic is the minimal JPEG byte signature that http.DetectContentType
// recognises as "image/jpeg". Use this instead of arbitrary placeholder bytes
// in tests that exercise paths reaching the server-side MIME sniffing check.
var jpegMagic = []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}

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
	req := buildUploadRequest(t, 5000, "GTQ", "image/jpeg", jpegMagic)
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
	req := buildUploadRequest(t, 5000, "GTQ", "image/jpeg", jpegMagic)
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

func TestBankTransferHandler_AdminApprove_NoBody_OK(t *testing.T) {
	proof := fixedProof()
	proof.Status = domain.BankTransferApproved
	svc := &stubBankTransferSvc{proof: proof}
	router := bankTransferRouter(t, svc, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers/1/approve", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with no body, got %d", rec.Code)
	}
}

func TestBankTransferHandler_AdminApprove_BodyTooLarge_Returns413(t *testing.T) {
	router := bankTransferRouter(t, &stubBankTransferSvc{}, nil)
	rec := httptest.NewRecorder()
	padded := `{"notes":"` + string(bytes.Repeat([]byte("x"), 5000)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers/1/approve", bytes.NewReader([]byte(padded)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for oversized approve body, got %d", rec.Code)
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
	req := buildUploadRequest(t, minCents, "GTQ", "image/jpeg", jpegMagic)
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	h.Upload(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 for amount exactly at minimum, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBankTransferHandler_Upload_SpoofedContentType_Returns422 verifies that
// a client cannot bypass the MIME check by setting Content-Type: image/jpeg on
// the multipart part while sending a non-image payload. The handler sniffs the
// actual bytes and rejects the file regardless of the declared header.
func TestBankTransferHandler_Upload_SpoofedContentType_Returns422(t *testing.T) {
	svc := &stubBankTransferSvc{proof: fixedProof()}
	h := handler.NewBankTransferHandler(svc, &stubFileStore{}, 1<<20, 0, 0, zaptest.NewLogger(t))

	// File content is plain text ("fake-data"), but the part header claims image/jpeg.
	rec := httptest.NewRecorder()
	req := buildUploadRequest(t, 5000, "GTQ", "image/jpeg", []byte("<?php echo 'pwned'; ?>"))
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	h.Upload(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for spoofed Content-Type, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBankTransferHandler_Upload_EmptyFile_Returns422 verifies that uploading
// a zero-byte file is rejected before reaching the file store.
func TestBankTransferHandler_Upload_EmptyFile_Returns422(t *testing.T) {
	svc := &stubBankTransferSvc{proof: fixedProof()}
	h := handler.NewBankTransferHandler(svc, &stubFileStore{}, 1<<20, 0, 0, zaptest.NewLogger(t))

	rec := httptest.NewRecorder()
	req := buildUploadRequest(t, 5000, "GTQ", "image/jpeg", []byte{})
	ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 10})
	req = req.WithContext(ctx)
	h.Upload(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for empty file, got %d: %s", rec.Code, rec.Body.String())
	}
}

// extractBoundary is a no-op used to satisfy the compiler; actual extraction
// is inlined where needed.
func extractBoundary(_ *bytes.Buffer) string { return "boundary" }

// ── Response field security ───────────────────────────────────────────────────

func TestBankTransferHandler_ListMine_ResponseDoesNotExposeStorageKey(t *testing.T) {
	svc := &stubBankTransferSvc{proofs: []*domain.BankTransferProof{fixedProof()}}
	router := bankTransferRouter(t, svc, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Verify the raw JSON body does not contain the storage key path.
	body := rec.Body.String()
	if strings.Contains(body, "storage_key") {
		t.Errorf("response must not contain storage_key field — internal FileStore path must not be exposed to clients: body=%s", body)
	}
	if strings.Contains(body, "bank-transfers/10/abc.jpg") {
		t.Errorf("response must not contain internal FileStore key value: body=%s", body)
	}
}

// ── DownloadProof ─────────────────────────────────────────────────────────────

func TestBankTransferHandler_DownloadProof_OK_StreamsFile(t *testing.T) {
	fileBytes := []byte("fake-jpeg-content")
	svc := &stubBankTransferSvc{proof: fixedProof()} // proof.UserID = 10 = caller.ID
	fs := &stubFileStore{getContent: fileBytes, getType: "image/jpeg"}
	router := bankTransferRouter(t, svc, fs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/1/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg, got %q", ct)
	}
	if rec.Body.String() != string(fileBytes) {
		t.Errorf("expected body %q, got %q", fileBytes, rec.Body.String())
	}
}

func TestBankTransferHandler_DownloadProof_WrongUser_Returns404(t *testing.T) {
	// Proof belongs to user 99, but caller is user 10.
	otherUserProof := &domain.BankTransferProof{ID: 5, UserID: 99, StorageKey: "bank-transfers/99/x.pdf"}
	svc := &stubBankTransferSvc{proof: otherUserProof}
	fs := &stubFileStore{getContent: []byte("content"), getType: "application/pdf"}
	router := bankTransferRouter(t, svc, fs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/5/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for proof belonging to another user (prevents enumeration), got %d", rec.Code)
	}
}

func TestBankTransferHandler_DownloadProof_Unauthenticated_Returns401(t *testing.T) {
	h := handler.NewBankTransferHandler(&stubBankTransferSvc{}, &stubFileStore{}, 1<<20, 0, 0, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Get("/bank-transfers/{id}/download", h.DownloadProof)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/1/download", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth context, got %d", rec.Code)
	}
}

func TestBankTransferHandler_DownloadProof_ProofNotFound_Returns404(t *testing.T) {
	svc := &stubBankTransferSvc{proof: nil}
	router := bankTransferRouter(t, svc, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/999/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing proof, got %d", rec.Code)
	}
}

func TestBankTransferHandler_DownloadProof_FileStoreError_Returns500(t *testing.T) {
	svc := &stubBankTransferSvc{proof: fixedProof()}
	fs := &stubFileStore{getErr: errors.New("s3 timeout")}
	router := bankTransferRouter(t, svc, fs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/1/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on FileStore error, got %d", rec.Code)
	}
}

func TestBankTransferHandler_DownloadProof_FileNotInStorage_Returns404(t *testing.T) {
	svc := &stubBankTransferSvc{proof: fixedProof()}
	// stubFileStore.Get returns ErrNotFound by default (getContent nil, getErr nil)
	router := bankTransferRouter(t, svc, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/1/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when file not in storage, got %d", rec.Code)
	}
}

// ── AdminDownloadProof ────────────────────────────────────────────────────────

func TestBankTransferHandler_AdminDownloadProof_OK_NoOwnershipCheck(t *testing.T) {
	// Admin can download any user's proof — no ownership restriction.
	otherUserProof := &domain.BankTransferProof{ID: 7, UserID: 42, StorageKey: "bank-transfers/42/doc.pdf", ContentType: "application/pdf"}
	fileBytes := []byte("pdf-content")
	svc := &stubBankTransferSvc{proof: otherUserProof}
	fs := &stubFileStore{getContent: fileBytes, getType: "application/pdf"}
	router := bankTransferRouter(t, svc, fs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/bank-transfers/7/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin download, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(fileBytes) {
		t.Errorf("expected file body %q, got %q", fileBytes, rec.Body.String())
	}
}

func TestBankTransferHandler_AdminDownloadProof_NotFound_Returns404(t *testing.T) {
	svc := &stubBankTransferSvc{proof: nil}
	router := bankTransferRouter(t, svc, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/bank-transfers/999/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing proof (admin), got %d", rec.Code)
	}
}

func TestBankTransferHandler_DownloadProof_InvalidID_Returns422(t *testing.T) {
	router := bankTransferRouter(t, &stubBankTransferSvc{}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/abc/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for non-integer id, got %d", rec.Code)
	}
}

func TestBankTransferHandler_DownloadProof_ServiceError_Returns500(t *testing.T) {
	svc := &stubBankTransferSvc{err: errors.New("db error")}
	router := bankTransferRouter(t, svc, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/1/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on service error, got %d", rec.Code)
	}
}

func TestBankTransferHandler_AdminDownloadProof_InvalidID_Returns422(t *testing.T) {
	router := bankTransferRouter(t, &stubBankTransferSvc{}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/bank-transfers/bad/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for non-integer id, got %d", rec.Code)
	}
}

func TestBankTransferHandler_AdminDownloadProof_ServiceError_Returns500(t *testing.T) {
	svc := &stubBankTransferSvc{err: errors.New("db error")}
	router := bankTransferRouter(t, svc, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/bank-transfers/1/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on service error (admin download), got %d", rec.Code)
	}
}

func TestBankTransferHandler_ServeProofFile_FallsBackToProofContentType(t *testing.T) {
	// FileStore returns empty content-type; handler should fall back to proof.ContentType.
	proof := fixedProof() // ContentType: "image/jpeg"
	svc := &stubBankTransferSvc{proof: proof}
	fs := &stubFileStore{
		getContent: []byte("jpeg-data"),
		getType:    "", // empty — triggers fallback
	}
	router := bankTransferRouter(t, svc, fs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bank-transfers/1/download", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("expected fallback Content-Type image/jpeg, got %q", ct)
	}
}

func TestBankTransferHandler_SetNotifier_DoesNotPanic(t *testing.T) {
	h := handler.NewBankTransferHandler(&stubBankTransferSvc{}, &stubFileStore{}, 1<<20, 0, 0, zaptest.NewLogger(t))
	// SetNotifier must not panic; it simply stores the notifier for later use.
	h.SetNotifier(nil)
}

func TestBankTransferHandler_AdminApprove_ReturnsApprovedAt(t *testing.T) {
	now := time.Now().UTC()
	approved := fixedProof()
	approved.Status = domain.BankTransferApproved
	approved.ApprovedAt = &now
	svc := &stubBankTransferSvc{proof: approved}
	router := bankTransferRouter(t, svc, nil)

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"notes":"ok"}`)
	req := httptest.NewRequest(http.MethodPost, "/bank-transfers/1/approve", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp handler.BankTransferResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ApprovedAt == nil {
		t.Error("expected non-nil approved_at in response")
	}
}
