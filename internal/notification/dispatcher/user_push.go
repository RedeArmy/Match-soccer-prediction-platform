package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	infrapush "github.com/rede/world-cup-quiniela/internal/infrastructure/webpush"
	"github.com/rede/world-cup-quiniela/internal/notification"
)

// pushPayload is the JSON contract delivered to the browser Service Worker.
// All fields must remain stable — the Service Worker reads them by name.
type pushPayload struct {
	NotificationID int64  `json:"notification_id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	ActionURL      string `json:"action_url,omitempty"`
	Icon           string `json:"icon"`
	Badge          string `json:"badge"`
}

func (d *UserDispatcher) deliverPush(ctx context.Context, entry *notification.OutboxEntry, userID int, notifID int64, content userContent, log *zap.Logger) {
	if d.pusher == nil || d.pushRepo == nil {
		return
	}
	subs, err := d.pushRepo.ListActiveByUser(ctx, userID)
	if err != nil || len(subs) == 0 {
		return
	}

	icon := domain.DefaultNotifyPushIconURL
	badge := domain.DefaultNotifyPushBadgeURL
	ttl := domain.DefaultNotifyWebPushTTLSec
	titleMax := domain.DefaultNotifyPushTitleMaxChars
	bodyMax := domain.DefaultNotifyPushBodyMaxChars
	if d.params != nil {
		icon = d.params.GetString(ctx, domain.ParamKeyNotifyPushIconURL, domain.DefaultNotifyPushIconURL)
		badge = d.params.GetString(ctx, domain.ParamKeyNotifyPushBadgeURL, domain.DefaultNotifyPushBadgeURL)
		ttl = d.params.GetInt(ctx, domain.ParamKeyNotifyWebPushTTLSec, domain.DefaultNotifyWebPushTTLSec)
		titleMax = d.params.GetInt(ctx, domain.ParamKeyNotifyPushTitleMaxChars, domain.DefaultNotifyPushTitleMaxChars)
		bodyMax = d.params.GetInt(ctx, domain.ParamKeyNotifyPushBodyMaxChars, domain.DefaultNotifyPushBodyMaxChars)
	}

	body, _ := json.Marshal(pushPayload{
		NotificationID: notifID,
		Type:           string(entry.EventType),
		Title:          truncateRunes(content.title, titleMax),
		Body:           truncateRunes(content.body, bodyMax),
		ActionURL:      content.actionURL,
		Icon:           icon,
		Badge:          badge,
	})

	for _, sub := range subs {
		d.sendPushToSubscription(ctx, entry, userID, sub, body, ttl, log)
	}
}

func (d *UserDispatcher) sendPushToSubscription(ctx context.Context, entry *notification.OutboxEntry, userID int, sub *domain.PushSubscription, body []byte, ttl int, log *zap.Logger) {
	code, err := d.pusher.Send(ctx, infrapush.Message{
		Endpoint:  sub.Endpoint,
		P256dhKey: sub.P256dhKey,
		AuthKey:   sub.AuthKey,
		Body:      body,
		TTL:       ttl,
	})
	if err != nil {
		log.Warn("user dispatcher: push send failed",
			zap.Int64("sub_id", sub.ID),
			zap.Error(err),
		)
		return
	}
	// HTTP 404 and 410 both signal that the endpoint no longer exists at the
	// push service.  404 = not found (service never heard of it); 410 = Gone
	// (service explicitly confirms it is deleted).  Both are permanent failures
	// that require deactivation — treating 404 as success would permanently
	// include dead endpoints in every fan-out.
	if code == http.StatusNotFound || code == http.StatusGone {
		if inactiveErr := d.pushRepo.MarkInactive(ctx, sub.ID); inactiveErr != nil {
			log.Warn("user dispatcher: mark subscription inactive failed",
				zap.Int64("sub_id", sub.ID),
				zap.Error(inactiveErr),
			)
		}
		if d.dlqRepo != nil {
			d.writeDLQEntry(ctx, entry, userID, "push",
				fmt.Errorf("HTTP %d: subscription %d expired", code, sub.ID))
		}
		return
	}
	// Successful delivery. Update last_used_at as best-effort metadata so
	// cleanup jobs can identify stale subscriptions. Fire-and-forget: a slow or
	// failed write must not block the delivery path.
	subID := sub.ID
	go func() {
		// WithoutCancel strips request cancellation so this outlives the HTTP
		// response, while still propagating tracing context from the parent.
		updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := d.pushRepo.UpdateLastUsed(updateCtx, subID); err != nil {
			log.Warn("user dispatcher: update push subscription last_used_at failed",
				zap.Int64("sub_id", subID),
				zap.Error(err),
			)
		}
	}()
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
