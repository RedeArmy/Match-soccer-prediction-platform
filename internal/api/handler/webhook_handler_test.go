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
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	// testWebhookSecret is a well-formed whsec_ value used in signature tests.
	// Decoded bytes = "test-secret-key". Not a real credential.
	testWebhookSecret = "whsec_dGVzdC1zZWNyZXQta2V5" // NOSONAR
	// testMsgID is the Svix message-id header value used across all signature tests.
	testMsgID = "msg_test_01"
)

// svixRequest builds a request with Svix HMAC-SHA256 headers signed against
// secret using the provided timestamp.
func svixRequest(t *testing.T, body, secret string, ts time.Time) *http.Request {
	t.Helper()
	msgID := testMsgID
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

	req := httptest.NewRequest(http.MethodPost, pathWebhookClerk, strings.NewReader(body))
	req.Header.Set(headerSvixID, msgID)
	req.Header.Set(headerSvixTimestamp, tsStr)
	req.Header.Set(headerSvixSignature, sig)
	return req
}

func doWebhook(h *handler.WebhookHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, pathWebhookClerk, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	return w
}

const userCreatedBody = `{"type":"user.created","data":{"id":"user_abc123","first_name":"Alice","last_name":"Smith","email_addresses":[{"email_address":"alice@example.com"}]}}`
const userUpdatedBody = `{"type":"user.updated","data":{"id":"user_abc123","first_name":"Alice","last_name":"Updated","email_addresses":[{"email_address":"alice2@example.com"}]}}`

// stubClerkUserSyncer is a test double for service.ClerkUserSyncer.
// err controls the value returned by Upsert; softDeleteErr controls SoftDelete.
// Zero values mean success, which keeps existing tests unchanged.
type stubClerkUserSyncer struct {
	err           error
	softDeleteErr error
}

func (s *stubClerkUserSyncer) Upsert(_ context.Context, _, _, _, _ string, _ []service.ClerkEmail) error {
	return s.err
}

func (s *stubClerkUserSyncer) SoftDelete(_ context.Context, _ string) error {
	return s.softDeleteErr
}

// ── no signature verification ─────────────────────────────────────────────────

func TestWebhook_NoSecret_UserCreated_Returns204(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	if w := doWebhook(h, userCreatedBody); w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestWebhook_NoSecret_UserUpdated_Returns204(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	if w := doWebhook(h, userUpdatedBody); w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestWebhook_NoSecret_UnknownEventType_Returns204(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	if w := doWebhook(h, `{"type":"organization.created","data":{}}`); w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for unknown event, got %d", w.Code)
	}
}

func TestWebhook_NoSecret_InvalidJSON_Returns422(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	if w := doWebhook(h, `not json`); w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestWebhook_NoSecret_InvalidUserData_Returns422(t *testing.T) {
	// data is a JSON string, not an object - unmarshal into clerkUserPayload fails.
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	if w := doWebhook(h, `{"type":"user.created","data":"not-an-object"}`); w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestWebhook_NoSecret_SyncerError_Returns500(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{err: errors.New("db unavailable")}, "", zap.NewNop())
	if w := doWebhook(h, userCreatedBody); w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestWebhook_NoSecret_SyncerValidationError_Returns422(t *testing.T) {
	// The syncer returns a validation error (e.g. invalid email address) - handler must propagate 422.
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{err: apperrors.Validation("invalid email")}, "", zap.NewNop())
	body := `{"type":"user.created","data":{"id":"user_bademail","first_name":"Bad","last_name":"Email","email_addresses":[{"email_address":"notanemail"}]}}`
	if w := doWebhook(h, body); w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestWebhook_NoSecret_EmptyName_Returns204(t *testing.T) {
	// Name normalisation (fallback to subject ID when blank) is the service's responsibility.
	// The handler should pass through and return 204 when the syncer succeeds.
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	body := `{"type":"user.created","data":{"id":"user_noname","first_name":"","last_name":"","email_addresses":[]}}`
	if w := doWebhook(h, body); w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

// ── Svix signature verification ───────────────────────────────────────────────

func TestWebhook_WithSecret_ValidSignature_Returns204(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, testWebhookSecret, zap.NewNop())
	req := svixRequest(t, userCreatedBody, testWebhookSecret, time.Now())
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestWebhook_WithSecret_MissingHeaders_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, testWebhookSecret, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, pathWebhookClerk, strings.NewReader(userCreatedBody))
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf(fmtExpect400, w.Code)
	}
}

func TestWebhook_WithSecret_WrongSignature_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, testWebhookSecret, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, pathWebhookClerk, strings.NewReader(userCreatedBody))
	req.Header.Set(headerSvixID, testMsgID)
	req.Header.Set(headerSvixTimestamp, fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set(headerSvixSignature, "v1,invalidsignature==")
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf(fmtExpect400, w.Code)
	}
}

func TestWebhook_WithSecret_OldTimestamp_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, testWebhookSecret, zap.NewNop())
	req := svixRequest(t, userCreatedBody, testWebhookSecret, time.Now().Add(-10*time.Minute))
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for old timestamp, got %d", w.Code)
	}
}

func TestWebhook_WithSecret_InvalidBase64Secret_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "whsec_!!!notbase64!!!", zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, pathWebhookClerk, strings.NewReader(userCreatedBody))
	req.Header.Set(headerSvixID, testMsgID)
	req.Header.Set(headerSvixTimestamp, fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set(headerSvixSignature, "v1,anysignature==")
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid base64 secret, got %d", w.Code)
	}
}

func TestWebhook_WithSecret_InvalidTimestampFormat_Returns400(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, testWebhookSecret, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, pathWebhookClerk, strings.NewReader(userCreatedBody))
	req.Header.Set(headerSvixID, testMsgID)
	req.Header.Set(headerSvixTimestamp, "not-a-number")
	req.Header.Set(headerSvixSignature, "v1,anysignature==")
	w := httptest.NewRecorder()
	h.HandleClerkWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-numeric timestamp, got %d", w.Code)
	}
}

// ── user.deleted ──────────────────────────────────────────────────────────────

const userDeletedBody = `{"type":"user.deleted","data":{"id":"user_del123","deleted":true}}`

// TestWebhook_NoSecret_UserDeleted_Returns204 verifies the happy path: a
// well-formed user.deleted event triggers SoftDelete and returns 204.
func TestWebhook_NoSecret_UserDeleted_Returns204(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	if w := doWebhook(h, userDeletedBody); w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

// TestWebhook_NoSecret_UserDeleted_InvalidData_Returns422 verifies that a
// malformed data object (not JSON) is rejected with 422.
func TestWebhook_NoSecret_UserDeleted_InvalidData_Returns422(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	body := `{"type":"user.deleted","data":"not-an-object"}`
	if w := doWebhook(h, body); w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// TestWebhook_NoSecret_UserDeleted_MissingID_Returns422 verifies that a
// user.deleted payload without a subject ID is rejected rather than silently
// ignored, preventing a no-op that would leave the row active.
func TestWebhook_NoSecret_UserDeleted_MissingID_Returns422(t *testing.T) {
	h := handler.NewWebhookHandler(&stubClerkUserSyncer{}, "", zap.NewNop())
	body := `{"type":"user.deleted","data":{"deleted":true}}`
	if w := doWebhook(h, body); w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// TestWebhook_NoSecret_UserDeleted_SoftDeleteError_Returns500 verifies that
// a service error during soft-deletion is surfaced as 500 so Clerk retries.
func TestWebhook_NoSecret_UserDeleted_SoftDeleteError_Returns500(t *testing.T) {
	stub := &stubClerkUserSyncer{softDeleteErr: errors.New("db unavailable")}
	h := handler.NewWebhookHandler(stub, "", zap.NewNop())
	if w := doWebhook(h, userDeletedBody); w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
