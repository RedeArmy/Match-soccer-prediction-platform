package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubKYCSvc struct {
	profile       *domain.KYCProfile
	profiles      []*domain.KYCProfile
	doc           *domain.KYCDocument
	docs          []*domain.KYCDocument
	events        []*domain.KYCEvent
	frozen        []*domain.FrozenBalanceSummary
	err           error
	listEventsErr error // overrides err for ListEvents only
	listDocsErr   error // overrides err for ListDocuments only
	uploadDocErr  error // overrides err for UploadDocument only
}

func (s *stubKYCSvc) GetProfile(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return s.profile, s.err
}
func (s *stubKYCSvc) Submit(_ context.Context, _ int, _ service.SubmitKYCRequest) (*domain.KYCProfile, error) {
	return s.profile, s.err
}
func (s *stubKYCSvc) UploadDocument(_ context.Context, _ int, _ service.UploadDocRequest) (*domain.KYCDocument, error) {
	if s.uploadDocErr != nil {
		return nil, s.uploadDocErr
	}
	return s.doc, s.err
}
func (s *stubKYCSvc) ListDocuments(_ context.Context, _ int) ([]*domain.KYCDocument, error) {
	if s.listDocsErr != nil {
		return nil, s.listDocsErr
	}
	return s.docs, s.err
}
func (s *stubKYCSvc) GetRequirements(_ context.Context, _ int) (*service.KYCRequirements, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &service.KYCRequirements{}, nil
}
func (s *stubKYCSvc) ListEvents(_ context.Context, _ int, _ domain.KYCProfileType, _ repository.CursorPage) ([]*domain.KYCEvent, string, error) {
	if s.listEventsErr != nil {
		return nil, "", s.listEventsErr
	}
	return s.events, "", s.err
}
func (s *stubKYCSvc) ListQueue(_ context.Context, _ repository.KYCProfileFilters, _ repository.Pagination) ([]*domain.KYCProfile, error) {
	return s.profiles, s.err
}
func (s *stubKYCSvc) GetProfileByID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return s.profile, s.err
}
func (s *stubKYCSvc) Approve(_ context.Context, _, _ int, _ domain.KYCTier) error { return s.err }
func (s *stubKYCSvc) Reject(_ context.Context, _, _ int, _ string) error          { return s.err }
func (s *stubKYCSvc) Escalate(_ context.Context, _, _ int, _ string) error        { return s.err }
func (s *stubKYCSvc) RequestDocument(_ context.Context, _, _ int, _ domain.KYCDocumentType, _ string) error {
	return s.err
}
func (s *stubKYCSvc) VerifyDocument(_ context.Context, _ int64, _ int) error { return s.err }
func (s *stubKYCSvc) ListFrozenBalances(_ context.Context) ([]*domain.FrozenBalanceSummary, error) {
	return s.frozen, s.err
}
func (s *stubKYCSvc) ReleaseFrozenBalance(_ context.Context, _, _ int) error    { return s.err }
func (s *stubKYCSvc) FreezeBalance(_ context.Context, _, _ int, _ string) error { return s.err }
func (s *stubKYCSvc) ListDueForReview(_ context.Context) ([]*domain.KYCProfile, error) {
	return nil, s.err
}
func (s *stubKYCSvc) GetRiskDashboard(_ context.Context) (*domain.KYCRiskDashboardStats, error) {
	return &domain.KYCRiskDashboardStats{TierDistribution: map[domain.KYCTier]int64{}}, s.err
}
func (s *stubKYCSvc) RecalculateRiskScore(_ context.Context, _ int) (int, error) { return 0, s.err }

// ── helpers ───────────────────────────────────────────────────────────────────

func kycUploadRouter(t *testing.T, svc service.KYCService) http.Handler {
	// stubFileStore is defined in stubs_test.go; its Put drains nothing which
	// breaks TeeReader. Use a draining variant here.
	h := handler.NewKYCHandler(svc, &drainingFileStore{}, 10*1024*1024, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /kyc/documents", h.UploadDocument)
	return mux
}

func withKYCUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.ContextWithUser(r.Context(), &domain.User{ID: 7})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// buildKYCMultipart creates a multipart/form-data request for KYC document upload.
func buildKYCMultipart(t *testing.T, docType, contentType string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("document_type", docType)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="doc%s"`, extensionForCT(contentType)))
	h.Set("Content-Type", contentType)
	pw, _ := mw.CreatePart(h)
	_, _ = pw.Write(body)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/kyc/documents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func extensionForCT(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}

// minimalJPEGBytes returns the minimum byte sequence that http.DetectContentType
// identifies as image/jpeg (JFIF magic: FF D8 FF E0 plus padding to 512 bytes).
func minimalJPEGBytes() []byte {
	b := make([]byte, 16)
	b[0], b[1], b[2], b[3] = 0xff, 0xd8, 0xff, 0xe0
	return b
}

// drainingFileStore is a test double that drains the reader so the TeeReader
// completes and the SHA-256 hash is computed correctly.
type drainingFileStore struct{ putErr error }

func (d *drainingFileStore) Put(_ context.Context, _, _ string, r io.Reader, _ int64) error {
	_, _ = io.Copy(io.Discard, r)
	return d.putErr
}
func (d *drainingFileStore) Get(_ context.Context, _ string) (io.ReadCloser, string, error) {
	return nil, "", nil
}
func (d *drainingFileStore) Delete(_ context.Context, _ string) error { return nil }

// ── tests ─────────────────────────────────────────────────────────────────────

func TestKYCHandler_UploadDocument_Success(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7, Status: domain.KYCStatusPending}
	doc := &domain.KYCDocument{ID: 42, DocumentType: domain.KYCDocGovID, StorageKey: "kyc/7/abc.jpg"}
	svc := &stubKYCSvc{profile: profile, doc: doc}

	router := withKYCUser(kycUploadRouter(t, svc))
	req := buildKYCMultipart(t, "gov_id", "image/jpeg", minimalJPEGBytes())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", rec.Code, rec.Body.String())
	}
	var resp handler.KYCDocumentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != 42 {
		t.Errorf("document ID: got %d, want 42", resp.ID)
	}
}

func TestKYCHandler_UploadDocument_InvalidDocumentType(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7}
	svc := &stubKYCSvc{profile: profile}

	router := withKYCUser(kycUploadRouter(t, svc))
	req := buildKYCMultipart(t, "drivers_license", "image/jpeg", []byte("data"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestKYCHandler_UploadDocument_Unauthenticated(t *testing.T) {
	svc := &stubKYCSvc{}
	router := kycUploadRouter(t, svc) // no auth wrapper
	req := buildKYCMultipart(t, "gov_id", "image/jpeg", []byte("data"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestKYCHandler_UploadDocument_NoProfile(t *testing.T) {
	svc := &stubKYCSvc{profile: nil}
	router := withKYCUser(kycUploadRouter(t, svc))
	req := buildKYCMultipart(t, "gov_id", "image/jpeg", []byte("data"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for missing profile, got %d", rec.Code)
	}
}

// ── helpers for the remaining handlers ───────────────────────────────────────

func newKYCStatusRouter(t *testing.T, svc service.KYCService) http.Handler {
	t.Helper()
	h := handler.NewKYCHandler(svc, &drainingFileStore{}, 10*1024*1024, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Get("/kyc/status", h.GetStatus)
	r.Post("/kyc/submit", h.Submit)
	r.Get("/kyc/requirements", h.GetRequirements)
	r.Get("/kyc/documents", h.ListDocuments)
	r.Get("/kyc/events", h.ListEvents)
	return r
}

// authedKYCRouter wraps newKYCStatusRouter with the KYC user middleware so that
// handlers requiring authentication can be exercised without repeating the
// withKYCUser wrap at every call site.
func authedKYCRouter(t *testing.T, svc service.KYCService) http.Handler {
	t.Helper()
	return withKYCUser(newKYCStatusRouter(t, svc))
}

// ── GetStatus ─────────────────────────────────────────────────────────────────

func TestKYCHandler_GetStatus_NoProfile_ReturnsUnverifiedPlaceholder(t *testing.T) {
	svc := &stubKYCSvc{profile: nil}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/status", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_GetStatus_WithProfile_Returns200(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7, Status: domain.KYCStatusPending}
	svc := &stubKYCSvc{profile: profile}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/status", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_GetStatus_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	rec := httptest.NewRecorder()
	newKYCStatusRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rec.Code)
	}
}

func TestKYCHandler_GetStatus_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/status", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestKYCHandler_Submit_ValidBody_Returns201(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7, Status: domain.KYCStatusPending}
	svc := &stubKYCSvc{profile: profile}
	body := `{"full_name":"Juan Pérez","nationality":"GT","document_type":"gov_id","document_number":"1234"}`
	req := httptest.NewRequest(http.MethodPost, "/kyc/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — body: %s", rec.Code, rec.Body.String())
	}
}

func TestKYCHandler_Submit_BadDateOfBirth_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	body := `{"full_name":"Juan","nationality":"GT","document_type":"gov_id","document_number":"1","date_of_birth":"not-a-date"}`
	req := httptest.NewRequest(http.MethodPost, "/kyc/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestKYCHandler_Submit_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	body := `{"full_name":"Juan"}`
	req := httptest.NewRequest(http.MethodPost, "/kyc/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newKYCStatusRouter(t, svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rec.Code)
	}
}

// ── GetRequirements ───────────────────────────────────────────────────────────

func TestKYCHandler_GetRequirements_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/requirements", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_GetRequirements_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	rec := httptest.NewRecorder()
	newKYCStatusRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/requirements", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rec.Code)
	}
}

// ── ListDocuments ─────────────────────────────────────────────────────────────

func TestKYCHandler_ListDocuments_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/documents", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_ListDocuments_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	rec := httptest.NewRecorder()
	newKYCStatusRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/documents", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rec.Code)
	}
}

// ── ListEvents ────────────────────────────────────────────────────────────────

func TestKYCHandler_ListEvents_NoProfile_ReturnsEmptyList(t *testing.T) {
	svc := &stubKYCSvc{profile: nil}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/events", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_ListEvents_WithProfile_Returns200(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7, Status: domain.KYCStatusPending}
	svc := &stubKYCSvc{profile: profile}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/events", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_ListEvents_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	rec := httptest.NewRecorder()
	newKYCStatusRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/events", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rec.Code)
	}
}

func TestKYCHandler_ListEvents_WithEvents_Returns200(t *testing.T) {
	svc := &stubKYCSvc{events: []*domain.KYCEvent{{ID: 1, EventType: domain.KYCEventSubmitted, NewStatus: domain.KYCStatusPending}}}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/events", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_ListEvents_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/events", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

// ── ListDocuments additional ──────────────────────────────────────────────────

func TestKYCHandler_ListDocuments_WithDocs_Returns200(t *testing.T) {
	docs := []*domain.KYCDocument{{ID: 10, DocumentType: domain.KYCDocGovID}}
	svc := &stubKYCSvc{docs: docs}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/documents", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_ListDocuments_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/documents", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

// ── Submit additional ─────────────────────────────────────────────────────────

func TestKYCHandler_Submit_BadJSON_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	req := httptest.NewRequest(http.MethodPost, "/kyc/submit", strings.NewReader(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestKYCHandler_Submit_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	body := `{"full_name":"Juan","document_type":"gov_id","document_number":"1234"}`
	req := httptest.NewRequest(http.MethodPost, "/kyc/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

// ── GetRequirements additional ────────────────────────────────────────────────

func TestKYCHandler_GetRequirements_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/requirements", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

// ── NewKYCHandler ─────────────────────────────────────────────────────────────

func TestNewKYCStatusRouter_ZeroMaxUploadBytes_UsesDefault(t *testing.T) {
	svc := &stubKYCSvc{}
	h := handler.NewKYCHandler(svc, &drainingFileStore{}, 0, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Get("/kyc/status", h.GetStatus)
	rec := httptest.NewRecorder()
	withKYCUser(r).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/status", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

// ── Submit additional ─────────────────────────────────────────────────────────

func TestKYCHandler_Submit_ValidDateOfBirth_Returns201(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7, Status: domain.KYCStatusPending}
	svc := &stubKYCSvc{profile: profile}
	body := `{"full_name":"Juan","document_type":"gov_id","document_number":"1234","date_of_birth":"1990-03-15"}`
	req := httptest.NewRequest(http.MethodPost, "/kyc/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — body: %s", rec.Code, rec.Body.String())
	}
}

// ── ListEvents additional ─────────────────────────────────────────────────────

func TestKYCHandler_ListEvents_ServiceListEventsError_Returns500(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7}
	svc := &stubKYCSvc{profile: profile, listEventsErr: apperrors.Internal(nil)}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/events", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

func TestKYCHandler_ListEvents_CustomLimit_Returns200(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7}
	svc := &stubKYCSvc{profile: profile}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/events?limit=50", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestKYCHandler_ListEvents_WithProfileAndEvents_Returns200(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7}
	events := []*domain.KYCEvent{{ID: 1, EventType: domain.KYCEventSubmitted, NewStatus: domain.KYCStatusPending}}
	svc := &stubKYCSvc{profile: profile, events: events}
	rec := httptest.NewRecorder()
	authedKYCRouter(t, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kyc/events", nil))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

// ── UploadDocument additional ─────────────────────────────────────────────────

func TestKYCHandler_UploadDocument_GetProfileError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	router := withKYCUser(kycUploadRouter(t, svc))
	req := buildKYCMultipart(t, "gov_id", "image/jpeg", []byte("data"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

func TestKYCHandler_UploadDocument_UnsupportedContentType_Returns422(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1}
	svc := &stubKYCSvc{profile: profile}
	router := withKYCUser(kycUploadRouter(t, svc))
	req := buildKYCMultipart(t, "gov_id", "application/octet-stream", []byte("data"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestKYCHandler_UploadDocument_NoFileField_Returns422(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1}
	svc := &stubKYCSvc{profile: profile}
	router := withKYCUser(kycUploadRouter(t, svc))
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("document_type", "gov_id")
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/kyc/documents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestKYCHandler_UploadDocument_SvcError_Returns500(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1}
	svc := &stubKYCSvc{profile: profile, uploadDocErr: apperrors.Internal(nil)}
	router := withKYCUser(kycUploadRouter(t, svc))
	req := buildKYCMultipart(t, "gov_id", "image/jpeg", minimalJPEGBytes())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

func TestKYCHandler_UploadDocument_StorePutError_Returns500(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7}
	svc := &stubKYCSvc{profile: profile}
	h := handler.NewKYCHandler(svc, &drainingFileStore{putErr: apperrors.Internal(nil)}, 10*1024*1024, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /kyc/documents", h.UploadDocument)
	router := withKYCUser(mux)
	req := buildKYCMultipart(t, "gov_id", "image/jpeg", minimalJPEGBytes())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

func TestKYCHandler_UploadDocument_BodyTooLarge_Returns413(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 7}
	svc := &stubKYCSvc{profile: profile}
	// 1-byte limit forces ParseMultipartForm to fail with MaxBytesError.
	h := handler.NewKYCHandler(svc, &drainingFileStore{}, 1, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /kyc/documents", h.UploadDocument)
	router := withKYCUser(mux)
	req := buildKYCMultipart(t, "gov_id", "image/jpeg", []byte("fake-image-bytes"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rec.Code)
	}
}
