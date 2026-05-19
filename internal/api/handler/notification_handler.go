package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// NotificationHandler serves the notification inbox, SSE stream, mark-read,
// preference management, and Web Push subscribe/unsubscribe endpoints.
type NotificationHandler struct {
	notifRepo repository.UserNotificationRepository
	prefRepo  repository.NotificationPreferenceRepository
	pushRepo  repository.PushSubscriptionRepository
	hub       *hub.Hub
	params    service.SystemParamService
	log       *zap.Logger
}

// NewNotificationHandler constructs a NotificationHandler.
func NewNotificationHandler(
	notifRepo repository.UserNotificationRepository,
	prefRepo repository.NotificationPreferenceRepository,
	pushRepo repository.PushSubscriptionRepository,
	h *hub.Hub,
	params service.SystemParamService,
	log *zap.Logger,
) *NotificationHandler {
	return &NotificationHandler{
		notifRepo: notifRepo,
		prefRepo:  prefRepo,
		pushRepo:  pushRepo,
		hub:       h,
		params:    params,
		log:       log,
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	ch, cleanup := h.hub.Connect(caller.ID)
	defer cleanup()

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
