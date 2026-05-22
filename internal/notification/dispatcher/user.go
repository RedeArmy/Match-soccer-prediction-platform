package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	infrapush "github.com/rede/world-cup-quiniela/internal/infrastructure/webpush"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// userContent holds the rendered title and body for a user-facing notification.
type userContent struct {
	title         string
	body          string
	actionURL     string
	emailSubject  string // overrides title as email subject when non-empty
	emailHTMLTmpl string // raw html/template string; non-empty replaces userBaseTemplate
	locale        Locale // propagated to email renderer for greeting/CTA localisation
}

// minUserPayload extracts only the user_id field from any outbox payload.
type minUserPayload struct {
	UserID int `json:"user_id"`
}

// ownerPayload extracts owner_id from payloads where the notification
// recipient is the group owner rather than the actor.
type ownerPayload struct {
	OwnerID int `json:"owner_id"`
}

// GroupMemberLister provides the minimal read access that UserDispatcher needs
// to fan-out broadcast events to every active group member.  A dedicated
// interface (instead of the full GroupMembershipRepository) keeps the
// dependency surface narrow and makes test doubles trivial.
type GroupMemberLister interface {
	ListActiveMemberIDsByGroup(ctx context.Context, quinielaID int) ([]int, error)
}

// broadcastEvents is the set of event types that must be delivered to all
// active members of a quiniela rather than to a single recipient.
var broadcastEvents = map[notification.EventType]struct{}{
	notification.EventGroupMemberJoined: {},
	notification.EventGroupMemberLeft:   {},
}

func isBroadcastEvent(et notification.EventType) bool {
	_, ok := broadcastEvents[et]
	return ok
}

// resolveRecipient determines the target user ID for single-recipient delivery.
// Most events address payload.user_id (the actor); a small set redirect to a
// different field — e.g. EventGroupJoinRequested notifies the group owner, not
// the requester.  Returns (0, false) when no valid recipient can be extracted.
func resolveRecipient(entry *notification.OutboxEntry) (int, bool) {
	if entry.EventType == notification.EventGroupJoinRequested {
		var p ownerPayload
		if err := json.Unmarshal(entry.Payload, &p); err == nil && p.OwnerID != 0 {
			return p.OwnerID, true
		}
	}
	var up minUserPayload
	if err := json.Unmarshal(entry.Payload, &up); err == nil && up.UserID != 0 {
		return up.UserID, true
	}
	return 0, false
}

// pgNotifyPayload is the JSON structure broadcast over pg_notify.
type pgNotifyPayload struct {
	UserID    int    `json:"user_id"`
	ID        int64  `json:"id"`
	EventType string `json:"event_type"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	ActionURL string `json:"action_url,omitempty"`
	CreatedAt string `json:"created_at"`
}

// PgNotifier executes a pg_notify call.  The interface makes the dependency
// injectable for tests without a real database connection.
type PgNotifier interface {
	Notify(ctx context.Context, channel, payload string) error
}

// notifPref carries channel opt-in flags resolved from the preference store.
type notifPref struct {
	ChannelEmail bool
	ChannelPush  bool
	ChannelInApp bool
}

// UserDispatcher implements outbox.Dispatcher for user-facing events.
//
// For every claimed outbox entry it:
//  1. Skips admin/system events (handled by AdminDispatcher).
//  2. For broadcast events (e.g. EventGroupMemberJoined): queries all active
//     group members and delivers to each, excluding the actor.
//  3. For all other events: resolves the single target user via resolveRecipient
//     (which redirects to OwnerID for EventGroupJoinRequested).
//  4. Renders the notification title and body.
//  5. Persists a UserNotification row with idempotency guard.
//  6. Fires pg_notify('user_notifications', …) for the SSE bridge.
//  7. Checks notification preferences; if channel_push=true and active
//     subscriptions exist, sends Web Push (marks 410 Gone subscriptions inactive).
//     P2/P3 bursts are throttled by DigestGate: after threshold individual pushes
//     within the window, one digest push is sent and further events are dropped.
//  8. For P0/P1 events with channel_email=true, delivers email via mailer.
//     Email includes a signed unsubscribe link when UnsubscribeSecret + AppBaseURL
//     are configured.  Global email opt-out (one-click unsubscribe) is honoured.
//  9. On persist failure, writes a DLQ entry and returns the error for retry.
type UserDispatcher struct {
	notifRepo         repository.UserNotificationRepository
	prefRepo          repository.NotificationPreferenceRepository
	pushRepo          repository.PushSubscriptionRepository
	dlqRepo           repository.NotificationDLQEntryCreator
	hub               *hub.Hub
	pusher            infrapush.Sender
	mailer            infraemail.Sender
	emailResolver     UserEmailResolver
	fromAddr          string
	unsubscribeSecret string // WCQ_EMAIL_UNSUBSCRIBESECRET; empty omits unsubscribe link
	appBaseURL        string // WCQ_SERVER_APPBASEURL; used to build absolute links in emails
	pgNotifier        PgNotifier
	params            service.SystemParamService
	templateRepo      repository.NotificationTemplateRepository // nil falls back to compiled defaults
	memberLister      GroupMemberLister                         // nil disables broadcast fan-out (tests without DB)
	digestGate        notification.DigestGate                   // nil disables digest (all pushes sent individually)
	log               *zap.Logger
}

// UserDispatcherConfig bundles constructor arguments for UserDispatcher.
type UserDispatcherConfig struct {
	NotifRepo         repository.UserNotificationRepository
	PrefRepo          repository.NotificationPreferenceRepository
	PushRepo          repository.PushSubscriptionRepository
	DLQRepo           repository.NotificationDLQEntryCreator
	Hub               *hub.Hub
	Pusher            infrapush.Sender
	Mailer            infraemail.Sender
	EmailResolver     UserEmailResolver // nil disables email delivery
	FromAddr          string
	UnsubscribeSecret string                                    // WCQ_EMAIL_UNSUBSCRIBESECRET; empty omits link
	AppBaseURL        string                                    // WCQ_SERVER_APPBASEURL; needed for absolute links
	PgNotifier        PgNotifier                                // nil disables pg_notify (tests without DB)
	Params            service.SystemParamService                // nil uses defaults
	TemplateRepo      repository.NotificationTemplateRepository // nil falls back to compiled defaults
	MemberLister      GroupMemberLister                         // nil disables broadcast fan-out (tests without DB)
	// DigestGate throttles P2/P3 push bursts cluster-wide. Use
	// notification.NewRedisPushDigestGate when Redis is available; fall back to
	// notification.NewPushDigestGate for single-process deployments. nil disables
	// digest gating (all pushes delivered individually).
	DigestGate notification.DigestGate
	Log        *zap.Logger
}

// NewUserDispatcher constructs a UserDispatcher.
func NewUserDispatcher(cfg UserDispatcherConfig) *UserDispatcher {
	return &UserDispatcher{
		notifRepo:         cfg.NotifRepo,
		prefRepo:          cfg.PrefRepo,
		pushRepo:          cfg.PushRepo,
		dlqRepo:           cfg.DLQRepo,
		hub:               cfg.Hub,
		pusher:            cfg.Pusher,
		mailer:            cfg.Mailer,
		emailResolver:     cfg.EmailResolver,
		fromAddr:          cfg.FromAddr,
		unsubscribeSecret: cfg.UnsubscribeSecret,
		appBaseURL:        cfg.AppBaseURL,
		pgNotifier:        cfg.PgNotifier,
		params:            cfg.Params,
		templateRepo:      cfg.TemplateRepo,
		memberLister:      cfg.MemberLister,
		digestGate:        cfg.DigestGate,
		log:               cfg.Log,
	}
}

// Dispatch implements outbox.Dispatcher.
func (d *UserDispatcher) Dispatch(ctx context.Context, entry *notification.OutboxEntry) error {
	if notification.IsAdminEvent(entry.EventType) {
		return nil
	}

	log := d.log.With(
		zap.Int64("outbox_id", entry.ID),
		zap.String("event_type", string(entry.EventType)),
	)

	if isBroadcastEvent(entry.EventType) {
		return d.dispatchBroadcast(ctx, entry, log)
	}

	userID, ok := resolveRecipient(entry)
	if !ok {
		log.Warn("user dispatcher: cannot resolve recipient; skipping")
		return nil
	}

	return d.deliverToUser(ctx, entry, userID, fmt.Sprintf("outbox-%d", entry.ID), log)
}

// deliverToUser persists, SSE-notifies, and optionally push/email-delivers a
// single notification to userID.  idempotencyKey must be unique per
// (outbox entry, recipient) pair; for fan-out events use the form
// "outbox-{id}-user-{uid}" so multiple recipients do not collide.
func (d *UserDispatcher) deliverToUser(ctx context.Context, entry *notification.OutboxEntry, userID int, idempotencyKey string, log *zap.Logger) error {
	content := d.resolveContent(ctx, entry, log)

	n := &domain.UserNotification{
		UserID:         userID,
		EventType:      string(entry.EventType),
		Title:          content.title,
		Body:           content.body,
		ActionURL:      content.actionURL,
		IdempotencyKey: idempotencyKey,
	}
	inserted, err := d.notifRepo.Create(ctx, n)
	if err != nil {
		d.writeDLQEntry(ctx, entry, userID, "inapp", err)
		log.Error("user dispatcher: failed to persist notification", zap.Error(err))
		return fmt.Errorf("user dispatcher: persist: %w", err)
	}

	if inserted && d.pgNotifier != nil {
		d.notifyPg(ctx, n, entry, log)
	}

	pref := d.resolvePreferences(ctx, userID, string(entry.EventType))
	priority := notification.PriorityOf(entry.EventType)

	if pref.ChannelPush {
		d.deliverPush(ctx, entry, userID, n.ID, content, log)
	}
	if pref.ChannelEmail && priority <= notification.PriorityP1High {
		d.deliverEmail(ctx, entry, userID, content, log)
	}

	return nil
}

// dispatchBroadcast fans out a group broadcast event to all active members of
// the quiniela referenced in the payload, excluding the actor (payload.user_id)
// so the user who triggered the event does not receive a self-notification.
// Each delivery is attempted independently; the last error (if any) is returned.
func (d *UserDispatcher) dispatchBroadcast(ctx context.Context, entry *notification.OutboxEntry, log *zap.Logger) error {
	if d.memberLister == nil {
		log.Warn("user dispatcher: broadcast event but MemberLister not configured; skipping")
		return nil
	}

	var p notification.GroupJoinPayload
	if err := entry.DecodePayload(&p); err != nil || p.QuinielaID == 0 {
		log.Warn("user dispatcher: broadcast event missing quiniela_id; skipping")
		return nil
	}

	memberIDs, err := d.memberLister.ListActiveMemberIDsByGroup(ctx, p.QuinielaID)
	if err != nil {
		log.Error("user dispatcher: broadcast fan-out: list members failed", zap.Error(err))
		return fmt.Errorf("user dispatcher: broadcast list: %w", err)
	}

	var lastErr error
	for _, uid := range memberIDs {
		if uid == p.UserID {
			continue // do not notify the actor
		}
		key := fmt.Sprintf("outbox-%d-user-%d", entry.ID, uid)
		if err := d.deliverToUser(ctx, entry, uid, key, log); err != nil {
			log.Error("user dispatcher: broadcast deliver failed",
				zap.Int("recipient_user_id", uid), zap.Error(err))
			lastErr = err
		}
	}
	return lastErr
}

func (d *UserDispatcher) notifyPg(ctx context.Context, n *domain.UserNotification, entry *notification.OutboxEntry, log *zap.Logger) {
	p := pgNotifyPayload{
		UserID:    n.UserID,
		ID:        n.ID,
		EventType: string(entry.EventType),
		Title:     n.Title,
		Body:      n.Body,
		ActionURL: n.ActionURL,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
	}
	b, err := json.Marshal(p)
	if err != nil {
		log.Warn("user dispatcher: pg_notify marshal failed", zap.Error(err))
		return
	}
	if err := d.pgNotifier.Notify(ctx, "user_notifications", string(b)); err != nil {
		log.Warn("user dispatcher: pg_notify failed", zap.Error(err))
	}
}

func (d *UserDispatcher) resolvePreferences(ctx context.Context, userID int, eventType string) notifPref {
	pref, err := d.prefRepo.Get(ctx, userID, eventType)
	if err != nil {
		return notifPref{ChannelEmail: true, ChannelPush: true, ChannelInApp: true}
	}
	return notifPref{
		ChannelEmail: pref.ChannelEmail,
		ChannelPush:  pref.ChannelPush,
		ChannelInApp: pref.ChannelInApp,
	}
}

// resolveContent returns the notification content for entry.  It queries the
// operator-editable template store first; on a miss or render failure it falls
// through to the compiled Go default.  The two-level fallback guarantees
// delivery even when the template store is unavailable or contains an invalid
// template.
func (d *UserDispatcher) resolveContent(ctx context.Context, entry *notification.OutboxEntry, log *zap.Logger) userContent {
	locale := d.resolveLocale(ctx)
	if d.templateRepo != nil {
		tmpl, err := d.templateRepo.Get(ctx, string(entry.EventType), string(locale))
		if err != nil {
			log.Warn("dispatcher: template store lookup failed; using compiled default",
				zap.String("event_type", string(entry.EventType)),
				zap.String("locale", string(locale)),
				zap.Error(err),
			)
		} else if tmpl != nil {
			title, body, actionURL, emailSubject, emailHTMLTmpl, renderErr := RenderTemplate(tmpl, entry.Payload)
			if renderErr != nil {
				log.Warn("dispatcher: template render failed; using compiled default",
					zap.String("event_type", string(entry.EventType)),
					zap.String("locale", string(locale)),
					zap.Error(renderErr),
				)
			} else {
				return userContent{title: title, body: body, actionURL: actionURL, emailSubject: emailSubject, emailHTMLTmpl: emailHTMLTmpl, locale: locale}
			}
		}
	}
	return buildUserContent(entry, locale)
}

// resolveLocale returns the active delivery locale.  Falls back to LocaleEN
// when params is nil (unit tests without a DB) or the key is absent so that
// existing tests continue to receive English content without any changes.
func (d *UserDispatcher) resolveLocale(ctx context.Context) Locale {
	if d.params == nil {
		return LocaleEN
	}
	raw := d.params.GetString(ctx, domain.ParamKeyNotifyDefaultLocale, string(LocaleEN))
	if Locale(raw) == LocaleES {
		return LocaleES
	}
	return LocaleEN
}

func (d *UserDispatcher) writeDLQEntry(ctx context.Context, entry *notification.OutboxEntry, userID int, channel string, sendErr error) {
	outboxID := entry.ID
	dlq := &domain.NotificationDLQEntry{
		OutboxID:    &outboxID,
		Channel:     channel,
		UserID:      &userID,
		EventType:   string(entry.EventType),
		Payload:     entry.Payload,
		ErrorDetail: sendErr.Error(),
	}
	if err := d.dlqRepo.CreateEntry(ctx, dlq); err != nil {
		d.log.Warn("user dispatcher: failed to write DLQ entry", zap.Error(err))
	}
}
