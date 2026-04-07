package handler_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
)

// testWebhookSecret is a well-formed whsec_ value used in signature tests.
// Decoded bytes = "test-secret-key". Not a real credential.
const testWebhookSecret = "whsec_dGVzdC1zZWNyZXQta2V5" // NOSONAR

// svixRequest builds a request with Svix HMAC-SHA256 headers signed against
// secret using the provided timestamp.
func svixRequest(t *testing.T, body, secret string, ts time.Time) *http.Request {
	t.Helper()
	msgID := "msg_test_01"
	tsStr := fmt.Sprintf("%d", ts.Unix())

	secretBase64 := strings.TrimPrefix(secret, "whsec_")
	secretBytes, err := base64.StdEncoding.DecodeString(secretBase64)
	if err != nil {
		t.Fatalf("decode test secret: %v", err)
	}
	toSign := fmt.Sprintf("%s.%s.%s", msgID, tsStr, body)
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write([]byte(toSign))
	sig := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/clerk", strings.NewReader(body))
	req.Header.Set("svix-id", msgID)
	req.Header.Set("svix-timestamp", tsStr)
	req.Header.Set("svix-signature", sig)
	return req
}

func doWebhook(h *handler.WebhookHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/clerk", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	return w
}

const userCreatedBody = `{"type":"user.created","data":{"id":"user_abc123","first_name":"Alice","last_name":"Smith","email_addresses":[{"email_address":"alice@example.com"}]}}`
const userUpdatedBody = `{"type":"user.updated","data":{"id":"user_abc123","first_name":"Alice","last_name":"Updated","email_addresses":[{"email_address":"alice2@example.com"}]}}`

// ── no signature verification ─────────────────────────────────────────────────

func TestWebhook_NoSecret_UserCreated_Returns204(t *testing.T) {
	// GetByClerkSubject → nil (new user) → Create path
	h := handler.NewWebhookHandler(&stubUserRepo{}, "", zap.NewNop())
	if w := doWebhook(h, userCreatedBody); w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestWebhook_NoSecret_UserUpdated_Returns204(t *testing.T) {
	// GetByClerkSubject → existing user → Update path
	existing := &domain.User{ID: 1, Name: "Alice", Role: domain.RolePlayer}
	h := handler.NewWebhookHandler(&stubUserRepo{user: existing}, "", zap.NewNop())
	if w := doWebhook(h, userUpdatedBody); w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestWebhook_NoSecret_UnknownEventType_Returns204(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, "", zap.NewNop())
	if w := doWebhook(h, `{"type":"organization.created","data":{}}`); w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for unknown event, got %d", w.Code)
	}
}

func TestWebhook_NoSecret_InvalidJSON_Returns422(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, "", zap.NewNop())
	if w := doWebhook(h, `not json`); w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestWebhook_NoSecret_InvalidUserData_Returns422(t *testing.T) {
	// data is a JSON string, not an object — unmarshal into clerkUserPayload fails.
	h := handler.NewWebhookHandler(&stubUserRepo{}, "", zap.NewNop())
	if w := doWebhook(h, `{"type":"user.created","data":"not-an-object"}`); w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestWebhook_NoSecret_GetBySubjectError_Returns500(t *testing.T) {
	userRepo := &stubUserRepo{err: errors.New("db unavailable")}
	h := handler.NewWebhookHandler(userRepo, "", zap.NewNop())
	if w := doWebhook(h, userCreatedBody); w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestWebhook_NoSecret_CreateError_Returns500(t *testing.T) {
	h := handler.NewWebhookHandler(&createErrRepo{createErr: errors.New("insert failed")}, "", zap.NewNop())
	if w := doWebhook(h, userCreatedBody); w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestWebhook_NoSecret_UpdateError_Returns500(t *testing.T) {
	existing := &domain.User{ID: 1, Name: "Alice", Role: domain.RolePlayer}
	h := handler.NewWebhookHandler(&updateErrRepo{user: existing, updateErr: errors.New("update failed")}, "", zap.NewNop())
	if w := doWebhook(h, userUpdatedBody); w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestWebhook_NoSecret_EmptyName_FallsBackToSubjectID(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, "", zap.NewNop())
	body := `{"type":"user.created","data":{"id":"user_noname","first_name":"","last_name":"","email_addresses":[]}}`
	if w := doWebhook(h, body); w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

// ── Svix signature verification ───────────────────────────────────────────────

func TestWebhook_WithSecret_ValidSignature_Returns204(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, testWebhookSecret, zap.NewNop())
	req := svixRequest(t, userCreatedBody, testWebhookSecret, time.Now())
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestWebhook_WithSecret_MissingHeaders_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, testWebhookSecret, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/webhooks/clerk", strings.NewReader(userCreatedBody))
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf(fmtExpect400, w.Code)
	}
}

func TestWebhook_WithSecret_WrongSignature_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, testWebhookSecret, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/webhooks/clerk", strings.NewReader(userCreatedBody))
	req.Header.Set("svix-id", "msg_test_01")
	req.Header.Set("svix-timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("svix-signature", "v1,invalidsignature==")
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf(fmtExpect400, w.Code)
	}
}

func TestWebhook_WithSecret_OldTimestamp_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, testWebhookSecret, zap.NewNop())
	req := svixRequest(t, userCreatedBody, testWebhookSecret, time.Now().Add(-10*time.Minute))
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for old timestamp, got %d", w.Code)
	}
}

func TestWebhook_WithSecret_InvalidBase64Secret_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, "whsec_!!!notbase64!!!", zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/webhooks/clerk", strings.NewReader(userCreatedBody))
	req.Header.Set("svix-id", "msg_test_01")
	req.Header.Set("svix-timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("svix-signature", "v1,anysignature==")
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid base64 secret, got %d", w.Code)
	}
}

func TestWebhook_WithSecret_InvalidTimestampFormat_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubUserRepo{}, testWebhookSecret, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/webhooks/clerk", strings.NewReader(userCreatedBody))
	req.Header.Set("svix-id", "msg_test_01")
	req.Header.Set("svix-timestamp", "not-a-number")
	req.Header.Set("svix-signature", "v1,anysignature==")
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-numeric timestamp, got %d", w.Code)
	}
}

// ── targeted stubs for isolated error paths ───────────────────────────────────

// createErrRepo returns nil from GetByClerkSubject (no existing user) and
// errors from Create, isolating the Create error path in upsertUser.
type createErrRepo struct{ createErr error }

func (r *createErrRepo) Create(_ context.Context, _ *domain.User) error { return r.createErr }
func (r *createErrRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return nil, nil
}
func (r *createErrRepo) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *createErrRepo) Update(_ context.Context, _ *domain.User) error { return nil }
func (r *createErrRepo) Delete(_ context.Context, _ int) error          { return nil }
func (r *createErrRepo) List(_ context.Context) ([]*domain.User, error) { return nil, nil }

// updateErrRepo returns an existing user from GetByClerkSubject and errors
// from Update, isolating the Update error path in upsertUser.
type updateErrRepo struct {
	user      *domain.User
	updateErr error
}

func (r *updateErrRepo) Create(_ context.Context, _ *domain.User) error { return nil }
func (r *updateErrRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return nil, nil
}
func (r *updateErrRepo) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return r.user, nil
}
func (r *updateErrRepo) Update(_ context.Context, _ *domain.User) error { return r.updateErr }
func (r *updateErrRepo) Delete(_ context.Context, _ int) error          { return nil }
func (r *updateErrRepo) List(_ context.Context) ([]*domain.User, error) { return nil, nil }
