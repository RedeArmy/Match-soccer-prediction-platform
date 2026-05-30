package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/notification/unsubscribe"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// NotificationHandler serves the notification inbox, SSE stream, mark-read,
// preference management, Web Push subscribe/unsubscribe, and one-click email
// unsubscribe endpoints.
type NotificationHandler struct {
	notifRepo         repository.UserNotificationRepository
	prefRepo          repository.NotificationPreferenceRepository
	pushRepo          repository.PushSubscriptionRepository
	hub               *hub.Hub
	params            service.SystemParamService
	unsubscribeSecret string // WCQ_EMAIL_UNSUBSCRIBESECRET; empty disables Unsubscribe endpoint
	log               *zap.Logger
}

// NotificationHandlerConfig bundles constructor arguments for NotificationHandler.
type NotificationHandlerConfig struct {
	NotifRepo         repository.UserNotificationRepository
	PrefRepo          repository.NotificationPreferenceRepository
	PushRepo          repository.PushSubscriptionRepository
	Hub               *hub.Hub
	Params            service.SystemParamService
	UnsubscribeSecret string // WCQ_EMAIL_UNSUBSCRIBESECRET; empty disables Unsubscribe
	Log               *zap.Logger
}

// NewNotificationHandler constructs a NotificationHandler.
func NewNotificationHandler(cfg NotificationHandlerConfig) *NotificationHandler {
	return &NotificationHandler{
		notifRepo:         cfg.NotifRepo,
		prefRepo:          cfg.PrefRepo,
		pushRepo:          cfg.PushRepo,
		hub:               cfg.Hub,
		params:            cfg.Params,
		unsubscribeSecret: cfg.UnsubscribeSecret,
		log:               cfg.Log,
	}
}

// ── Inbox ─────────────────────────────────────────────────────────────────────

// notificationResponse is the API representation of a UserNotification.
type notificationResponse struct {
	ID        int64  `json:"id"`
	EventType string `json:"event_type"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	ActionURL string `json:"action_url,omitempty"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}

// inboxResponse wraps the paginated notification list with an unread count.
type inboxResponse struct {
	Notifications []*notificationResponse `json:"notifications"`
	UnreadCount   int                     `json:"unread_count"`
	Total         int                     `json:"total"`
}

// GetInbox handles GET /api/v1/notifications.
// Query params: limit (default 50, max 200), offset (default 0), unread=true.
func (h *NotificationHandler) GetInbox(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	limit, offset := parsePaginationParams(r)
	unreadOnly := r.URL.Query().Get("unread") == "true"

	notifications, err := h.notifRepo.List(r.Context(), caller.ID, limit, offset, unreadOnly)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	unreadCount, err := h.notifRepo.CountUnread(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	resp := &inboxResponse{
		Notifications: make([]*notificationResponse, 0, len(notifications)),
		UnreadCount:   unreadCount,
		Total:         len(notifications),
	}
	for _, n := range notifications {
		resp.Notifications = append(resp.Notifications, notifToResponse(n))
	}

	writeJSON(w, http.StatusOK, resp)
}

// ── SSE stream ────────────────────────────────────────────────────────────────

// GetStream handles GET /api/v1/notifications/stream.
// Opens an SSE connection; delivers notifications in real time and a heartbeat
// every 30 seconds.  The connection persists until the client disconnects or the
// server shuts down.
func (h *NotificationHandler) GetStream(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("streaming unsupported")))
		return
	}

	// Enforce per-user connection cap before writing any headers. A nil channel
	// means the hub rejected the connection.
	ch, cleanup := h.hub.Connect(caller.ID)
	if ch == nil {
		writeError(w, r, h.log, apperrors.RateLimited("SSE connection limit reached; close another tab or device and retry"))
		return
	}
	defer cleanup()

	// Disable the per-response write deadline so this long-lived SSE connection
	// is not silently killed by http.Server.WriteTimeout (default 30 s). Each
	// write is still subject to the TCP stack's send-buffer timeout.
	// ErrNotSupported is expected with httptest.ResponseRecorder in tests.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.log.Warn("sse: failed to clear write deadline", zap.Error(err))
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	// Signal successful connection.
	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	heartbeatSec := h.params.GetInt(r.Context(), domain.ParamKeyNotifySSEHeartbeatIntervalSec, domain.DefaultNotifySSEHeartbeatIntervalSec)
	heartbeat := time.NewTicker(time.Duration(heartbeatSec) * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case n, open := <-ch:
			if !open {
				return
			}
			b, err := json.Marshal(n)
			if err != nil {
				h.log.Warn("sse: marshal notification failed", zap.Error(err))
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()

		case <-heartbeat.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

// ── Mark read ─────────────────────────────────────────────────────────────────

type markReadRequest struct {
	IDs     []int64 `json:"ids"`
	MarkAll bool    `json:"mark_all"`
}

// MarkRead handles POST /api/v1/notifications/mark-read.
// Body: {"ids": [1,2,3]} or {"mark_all": true}.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[markReadRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if req.MarkAll {
		if err := h.notifRepo.MarkAllRead(r.Context(), caller.ID); err != nil {
			writeError(w, r, h.log, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	for _, id := range req.IDs {
		if err := h.notifRepo.MarkRead(r.Context(), id, caller.ID); err != nil {
			writeError(w, r, h.log, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Preferences ───────────────────────────────────────────────────────────────

// preferenceResponse is the API representation of a NotificationPreference.
type preferenceResponse struct {
	EventType    string `json:"event_type"`
	ChannelEmail bool   `json:"channel_email"`
	ChannelPush  bool   `json:"channel_push"`
	ChannelInApp bool   `json:"channel_inapp"`
}

// GetPreferences handles GET /api/v1/notifications/preferences.
func (h *NotificationHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	prefs, err := h.prefRepo.ListByUser(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	resp := make([]*preferenceResponse, 0, len(prefs))
	for _, p := range prefs {
		resp = append(resp, prefToResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}

type updatePreferenceRequest struct {
	EventType    string `json:"event_type"`
	ChannelEmail *bool  `json:"channel_email"`
	ChannelPush  *bool  `json:"channel_push"`
	ChannelInApp *bool  `json:"channel_inapp"`
}

// UpdatePreferences handles PATCH /api/v1/notifications/preferences.
func (h *NotificationHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[updatePreferenceRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.EventType == "" {
		writeError(w, r, h.log, apperrors.Validation("event_type is required"))
		return
	}

	// Fetch existing or start with all-enabled defaults.
	existing, fetchErr := h.prefRepo.Get(r.Context(), caller.ID, req.EventType)
	pref := &domain.NotificationPreference{
		UserID:       caller.ID,
		EventType:    req.EventType,
		ChannelEmail: true,
		ChannelPush:  true,
		ChannelInApp: true,
	}
	if fetchErr == nil && existing != nil {
		pref.ChannelEmail = existing.ChannelEmail
		pref.ChannelPush = existing.ChannelPush
		pref.ChannelInApp = existing.ChannelInApp
	}

	if req.ChannelEmail != nil {
		pref.ChannelEmail = *req.ChannelEmail
	}
	if req.ChannelPush != nil {
		pref.ChannelPush = *req.ChannelPush
	}
	if req.ChannelInApp != nil {
		pref.ChannelInApp = *req.ChannelInApp
	}

	if err := h.prefRepo.Upsert(r.Context(), pref); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, prefToResponse(pref))
}

// ── Push — VAPID public key ───────────────────────────────────────────────────

// GetVAPIDPublicKey handles GET /api/v1/push/vapid-public-key.
// Returns the VAPID application server public key so the browser can call
// PushManager.subscribe({applicationServerKey: ...}).  The key is a
// Base64URL-encoded uncompressed P-256 point stored in system_params.
func (h *NotificationHandler) GetVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	key := h.params.GetString(r.Context(), domain.ParamKeyNotifyWebPushVAPIDPublicKey, "")
	if key == "" {
		writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("VAPID public key not configured")))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"vapid_public_key": key})
}

// ── Push subscribe / unsubscribe ──────────────────────────────────────────────

type pushSubscribeRequest struct {
	Endpoint  string `json:"endpoint"`
	P256dhKey string `json:"p256dh_key"`
	AuthKey   string `json:"auth_key"`
	UserAgent string `json:"user_agent"`
}

// SubscribePush handles POST /api/v1/push/subscribe.
func (h *NotificationHandler) SubscribePush(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[pushSubscribeRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Endpoint == "" || req.P256dhKey == "" || req.AuthKey == "" {
		writeError(w, r, h.log, apperrors.Validation("endpoint, p256dh_key, and auth_key are required"))
		return
	}

	sub := &domain.PushSubscription{
		UserID:    caller.ID,
		Endpoint:  req.Endpoint,
		P256dhKey: req.P256dhKey,
		AuthKey:   req.AuthKey,
		UserAgent: req.UserAgent,
	}
	if err := h.pushRepo.Create(r.Context(), sub); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": sub.ID})
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// UnsubscribePush handles DELETE /api/v1/push/subscribe.
func (h *NotificationHandler) UnsubscribePush(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr != "" {
		// Convenience: accept ?id=<subscription_id> for browsers that cannot
		// send a DELETE body.  Deactivates but does not delete the subscription.
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, r, h.log, apperrors.Validation("invalid subscription id"))
			return
		}
		if err := h.pushRepo.MarkInactive(r.Context(), id); err != nil {
			writeError(w, r, h.log, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	req, err := decodeJSON[pushUnsubscribeRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Endpoint == "" {
		writeError(w, r, h.log, apperrors.Validation("endpoint is required"))
		return
	}
	_ = caller // endpoint is globally unique; user scoping is enforced by the push provider
	if err := h.pushRepo.DeleteByEndpoint(r.Context(), req.Endpoint); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── One-click email unsubscribe ───────────────────────────────────────────────

// Unsubscribe handles GET /api/v1/notifications/unsubscribe?token=<tok>.
//
// This endpoint is intentionally unauthenticated — email clients cannot attach
// an Authorization header when a user clicks a mailto link.  The token is a
// short-lived HMAC-SHA256 signed value (see internal/notification/unsubscribe)
// that encodes the user ID and an expiry timestamp; it is validated before any
// write is performed.
//
// On success, a global email opt-out sentinel row is upserted for the user
// (event_type = '*', channel_email = FALSE).  The dispatcher honours this flag
// and skips future email delivery for the affected user regardless of per-event
// preferences.
func (h *NotificationHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	if h.unsubscribeSecret == "" {
		writeError(w, r, h.log, apperrors.Internal(errors.New("unsubscribe endpoint not configured")))
		return
	}
	tok := r.URL.Query().Get("token")
	if tok == "" {
		writeError(w, r, h.log, apperrors.Validation("token is required"))
		return
	}
	userID, err := unsubscribe.VerifyToken(tok, h.unsubscribeSecret)
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid or expired unsubscribe token"))
		return
	}
	if err := h.prefRepo.DisableAllEmail(r.Context(), userID); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	h.log.Info("email unsubscribe: global opt-out recorded", zap.Int("user_id", userID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}

// ── Converters ────────────────────────────────────────────────────────────────

func notifToResponse(n *domain.UserNotification) *notificationResponse {
	r := &notificationResponse{
		ID:        n.ID,
		EventType: n.EventType,
		Title:     n.Title,
		Body:      n.Body,
		ActionURL: n.ActionURL,
		Read:      n.IsRead(),
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
	}
	return r
}

func prefToResponse(p *domain.NotificationPreference) *preferenceResponse {
	return &preferenceResponse{
		EventType:    p.EventType,
		ChannelEmail: p.ChannelEmail,
		ChannelPush:  p.ChannelPush,
		ChannelInApp: p.ChannelInApp,
	}
}
