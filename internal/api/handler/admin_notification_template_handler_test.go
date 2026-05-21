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
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// ── stub repository ───────────────────────────────────────────────────────────

type stubNotifTemplateRepo struct {
	templates        []*domain.NotificationTemplate
	single           *domain.NotificationTemplate
	listErr          error
	getErr           error
	postUpsertGetErr error // if set, Get returns this error after the first Upsert call
	upsertErr        error
	deleteErr        error
}

func (s *stubNotifTemplateRepo) List(_ context.Context) ([]*domain.NotificationTemplate, error) {
	return s.templates, s.listErr
}
func (s *stubNotifTemplateRepo) Get(_ context.Context, _, _ string) (*domain.NotificationTemplate, error) {
	return s.single, s.getErr
}
func (s *stubNotifTemplateRepo) Upsert(_ context.Context, t *domain.NotificationTemplate) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.single = t
	if s.postUpsertGetErr != nil {
		s.getErr = s.postUpsertGetErr
	}
	return nil
}
func (s *stubNotifTemplateRepo) Delete(_ context.Context, _, _ string) error {
	return s.deleteErr
}

// ── router factory ────────────────────────────────────────────────────────────

func newNotifTemplateRouter(repo *stubNotifTemplateRepo) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminNotificationTemplateHandler(repo, zap.NewNop())
	r.Get("/notification-templates", h.List)
	r.Get("/notification-templates/{event_type}/{locale}", h.Get)
	r.Put("/notification-templates/{event_type}/{locale}", h.Upsert)
	r.Delete("/notification-templates/{event_type}/{locale}", h.Delete)
	r.Post("/notification-templates/{event_type}/{locale}/preview", h.Preview)
	return r
}

func notifTemplateFix() *domain.NotificationTemplate {
	return &domain.NotificationTemplate{
		EventType:     "payment.confirmed",
		Locale:        "en",
		TitleTmpl:     "Payment confirmed",
		BodyTmpl:      "Your payment of {{formatCents .amount_cents .currency}} is confirmed.",
		ActionURLTmpl: "",
		UpdatedAt:     time.Now(),
	}
}

// withAdminCaller injects the shared adminCaller into the request context.
func withAdminCallerReq(req *http.Request) *http.Request {
	return req.WithContext(middleware.ContextWithUser(req.Context(), adminCaller))
}

// doWithCaller dispatches a request through a router after injecting a caller into context.
func doWithCaller(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(headerContentType, contentTypeJSON)
	req = withAdminCallerReq(req)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestNotifTemplateList_Success_Returns200(t *testing.T) {
	repo := &stubNotifTemplateRepo{templates: []*domain.NotificationTemplate{notifTemplateFix()}}
	w := do(newNotifTemplateRouter(repo), http.MethodGet, "/notification-templates", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifTemplateList_Empty_Returns200(t *testing.T) {
	repo := &stubNotifTemplateRepo{templates: []*domain.NotificationTemplate{}}
	w := do(newNotifTemplateRouter(repo), http.MethodGet, "/notification-templates", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifTemplateList_MultipleTemplates_SortedByEventTypeThenLocale(t *testing.T) {
	// Two templates with the same event_type; they must be sorted by locale.
	templates := []*domain.NotificationTemplate{
		{EventType: "payment.confirmed", Locale: "es", TitleTmpl: "t", BodyTmpl: "b", UpdatedAt: time.Now()},
		{EventType: "payment.confirmed", Locale: "en", TitleTmpl: "t", BodyTmpl: "b", UpdatedAt: time.Now()},
	}
	repo := &stubNotifTemplateRepo{templates: templates}
	w := do(newNotifTemplateRouter(repo), http.MethodGet, "/notification-templates", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifTemplateList_RepoError_Returns500(t *testing.T) {
	repo := &stubNotifTemplateRepo{listErr: errors.New("db error")}
	w := do(newNotifTemplateRouter(repo), http.MethodGet, "/notification-templates", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestNotifTemplateGet_Found_Returns200(t *testing.T) {
	repo := &stubNotifTemplateRepo{single: notifTemplateFix()}
	w := do(newNotifTemplateRouter(repo), http.MethodGet, "/notification-templates/payment.confirmed/en", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifTemplateGet_NotFound_Returns404(t *testing.T) {
	repo := &stubNotifTemplateRepo{single: nil}
	w := do(newNotifTemplateRouter(repo), http.MethodGet, "/notification-templates/payment.confirmed/en", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestNotifTemplateGet_RepoError_Returns500(t *testing.T) {
	repo := &stubNotifTemplateRepo{getErr: errors.New("db error")}
	w := do(newNotifTemplateRouter(repo), http.MethodGet, "/notification-templates/payment.confirmed/en", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestNotifTemplateGet_InvalidLocale_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{single: notifTemplateFix()}
	w := do(newNotifTemplateRouter(repo), http.MethodGet, "/notification-templates/payment.confirmed/fr", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── Upsert ────────────────────────────────────────────────────────────────────

func TestNotifTemplateUpsert_Success_Returns200(t *testing.T) {
	fix := notifTemplateFix()
	repo := &stubNotifTemplateRepo{single: fix}
	body := `{"title_tmpl":"Payment confirmed","body_tmpl":"Your payment is confirmed."}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPut, "/notification-templates/payment.confirmed/en", body)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifTemplateUpsert_MissingTitle_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"body_tmpl":"Some body"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPut, "/notification-templates/payment.confirmed/en", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestNotifTemplateUpsert_MissingBody_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"Some title"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPut, "/notification-templates/payment.confirmed/en", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestNotifTemplateUpsert_InvalidTemplateSyntax_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"{{.unclosed","body_tmpl":"valid"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPut, "/notification-templates/payment.confirmed/en", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestNotifTemplateUpsert_NoCaller_Returns401(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"t","body_tmpl":"b"}`
	// do() does not inject a caller — handler must reject with 401.
	w := do(newNotifTemplateRouter(repo), http.MethodPut, "/notification-templates/payment.confirmed/en", body)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestNotifTemplateUpsert_PostSaveGetFails_StillReturns200WithCandidate(t *testing.T) {
	// Upsert succeeds but the re-fetch fails; handler falls back to returning the candidate.
	repo := &stubNotifTemplateRepo{postUpsertGetErr: errors.New("cache miss")}
	body := `{"title_tmpl":"Payment confirmed","body_tmpl":"Your payment is confirmed."}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPut, "/notification-templates/payment.confirmed/en", body)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifTemplateUpsert_RepoUpsertError_Returns500(t *testing.T) {
	repo := &stubNotifTemplateRepo{upsertErr: errors.New("db error")}
	body := `{"title_tmpl":"Payment confirmed","body_tmpl":"Your payment is confirmed."}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPut, "/notification-templates/payment.confirmed/en", body)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestNotifTemplateUpsert_InvalidLocale_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"t","body_tmpl":"b"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPut, "/notification-templates/payment.confirmed/de", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestNotifTemplateDelete_Success_Returns204(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	w := do(newNotifTemplateRouter(repo), http.MethodDelete, "/notification-templates/payment.confirmed/en", "")
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestNotifTemplateDelete_RepoError_Returns500(t *testing.T) {
	repo := &stubNotifTemplateRepo{deleteErr: errors.New("db error")}
	w := do(newNotifTemplateRouter(repo), http.MethodDelete, "/notification-templates/payment.confirmed/en", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestNotifTemplateDelete_InvalidLocale_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	w := do(newNotifTemplateRouter(repo), http.MethodDelete, "/notification-templates/payment.confirmed/xx", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── Preview ───────────────────────────────────────────────────────────────────

func TestNotifTemplatePreview_Success_Returns200(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"Hello {{.name}}","body_tmpl":"Body {{.name}}","sample_payload":{"name":"Alice"}}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPost, "/notification-templates/any.event/en/preview", body)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifTemplatePreview_NoSamplePayload_DefaultsToEmpty(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"Hello","body_tmpl":"World"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPost, "/notification-templates/any.event/es/preview", body)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifTemplatePreview_MissingTitle_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"body_tmpl":"Some body"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPost, "/notification-templates/any.event/en/preview", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestNotifTemplatePreview_MissingBody_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"Some title"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPost, "/notification-templates/any.event/en/preview", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestNotifTemplatePreview_InvalidTemplateSyntax_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"{{.unclosed","body_tmpl":"valid"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPost, "/notification-templates/any.event/en/preview", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestNotifTemplatePreview_InvalidLocale_Returns422(t *testing.T) {
	repo := &stubNotifTemplateRepo{}
	body := `{"title_tmpl":"t","body_tmpl":"b"}`
	w := doWithCaller(newNotifTemplateRouter(repo), http.MethodPost, "/notification-templates/any.event/zz/preview", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}
