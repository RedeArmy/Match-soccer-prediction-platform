package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	title     string
	body      string
	actionURL string
	locale    Locale // propagated to email renderer for greeting/CTA localisation
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
	switch entry.EventType {
	case notification.EventGroupJoinRequested:
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
	Log               *zap.Logger
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
	if d.params != nil {
		icon = d.params.GetString(ctx, domain.ParamKeyNotifyPushIconURL, domain.DefaultNotifyPushIconURL)
		badge = d.params.GetString(ctx, domain.ParamKeyNotifyPushBadgeURL, domain.DefaultNotifyPushBadgeURL)
		ttl = d.params.GetInt(ctx, domain.ParamKeyNotifyWebPushTTLSec, domain.DefaultNotifyWebPushTTLSec)
	}

	body, _ := json.Marshal(pushPayload{
		NotificationID: notifID,
		Type:           string(entry.EventType),
		Title:          content.title,
		Body:           content.body,
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
	if code != http.StatusGone {
		// Successful delivery.  Update last_used_at as best-effort metadata so
		// cleanup jobs can identify stale subscriptions (browsers not reached in N days).
		// Fire-and-forget: a slow or failed write must not block the delivery path.
		subID := sub.ID
		go func() {
			updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := d.pushRepo.UpdateLastUsed(updateCtx, subID); err != nil {
				log.Warn("user dispatcher: update push subscription last_used_at failed",
					zap.Int64("sub_id", subID),
					zap.Error(err),
				)
			}
		}()
		return
	}
	// Subscription has expired at the push service.  Mark it inactive so future
	// dispatches skip it, write a DLQ entry for observability.
	if inactiveErr := d.pushRepo.MarkInactive(ctx, sub.ID); inactiveErr != nil {
		log.Warn("user dispatcher: mark subscription inactive failed",
			zap.Int64("sub_id", sub.ID),
			zap.Error(inactiveErr),
		)
	}
	if d.dlqRepo != nil {
		d.writeDLQEntry(ctx, entry, userID, "push",
			fmt.Errorf("HTTP 410 Gone: subscription %d expired", sub.ID))
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
			title, body, actionURL, renderErr := RenderTemplate(tmpl, entry.Payload)
			if renderErr != nil {
				log.Warn("dispatcher: template render failed; using compiled default",
					zap.String("event_type", string(entry.EventType)),
					zap.String("locale", string(locale)),
					zap.Error(renderErr),
				)
			} else {
				return userContent{title: title, body: body, actionURL: actionURL, locale: locale}
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

// Action URL path constants used by buildUserContent.
// Defined once here to prevent silent drift when the API route changes.
const (
	urlMatchDetail = "/api/v1/matches/%d"
	urlGroupsMe    = "/api/v1/groups/me"
	urlBalance     = "/api/v1/users/me/balance"
	urlWithdrawals = "/api/v1/withdrawals"
)

// buildUserContent returns the rendered title, body, and locale for the given
// event.  All user-visible strings are selected based on locale so the returned
// content is ready for both in-app persistence and email rendering.
func buildUserContent(entry *notification.OutboxEntry, locale Locale) userContent {
	c := resolveUserContent(entry, locale)
	c.locale = locale
	return c
}

// resolveUserContent maps an outbox entry to its title/body/actionURL for the
// given locale.  It is extracted so buildUserContent can attach the locale field
// after the switch without threading it through every return statement.
func resolveUserContent(entry *notification.OutboxEntry, locale Locale) userContent { //nolint:cyclop
	switch entry.EventType {

	case notification.EventPredictionConfirmed:
		var p notification.PredictionConfirmedPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Prediction confirmed", "Predicción confirmada", locale),
			body: localeStr(
				fmt.Sprintf("Your prediction for %s vs %s has been recorded.", p.HomeTeam, p.AwayTeam),
				fmt.Sprintf("Tu predicción para %s vs %s ha sido registrada.", p.HomeTeam, p.AwayTeam),
				locale,
			),
			actionURL: "/api/v1/predictions/me",
		}

	case notification.EventPredictionDeadlineApproach:
		var p notification.PredictionDeadlinePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Prediction deadline approaching", "Límite de predicción se acerca", locale),
			body: localeStr(
				fmt.Sprintf("%s vs %s kicks off in %d minutes — submit your prediction now.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
				fmt.Sprintf("%s vs %s empieza en %d minutos — envía tu predicción ahora.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
				locale,
			),
			actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
		}

	case notification.EventPredictionMissingReminder:
		var p notification.PredictionDeadlinePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Missing prediction reminder", "Recordatorio de predicción pendiente", locale),
			body: localeStr(
				fmt.Sprintf("You haven't predicted %s vs %s yet. Deadline is in %d minutes.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
				fmt.Sprintf("Aún no has predicho %s vs %s. El límite cierra en %d minutos.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
				locale,
			),
			actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
		}

	case notification.EventPredictionLocked:
		var p notification.PredictionLockedPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Predictions locked", "Predicciones cerradas", locale),
			body: localeStr(
				fmt.Sprintf("Predictions for %s vs %s are now locked.", p.HomeTeam, p.AwayTeam),
				fmt.Sprintf("Las predicciones para %s vs %s ya están cerradas.", p.HomeTeam, p.AwayTeam),
				locale,
			),
			actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
		}

	case notification.EventPredictionScored:
		var p notification.PredictionScoredPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Match scored", "Partido puntuado", locale),
			body: localeStr(
				fmt.Sprintf("%s vs %s finished %d-%d. You earned %d points.", p.HomeTeam, p.AwayTeam, p.HomeScore, p.AwayScore, p.PointsEarned),
				fmt.Sprintf("%s vs %s terminó %d-%d. Ganaste %d puntos.", p.HomeTeam, p.AwayTeam, p.HomeScore, p.AwayScore, p.PointsEarned),
				locale,
			),
			actionURL: "/api/v1/predictions/me",
		}

	case notification.EventMatchResultEntered:
		var p notification.MatchEventPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Match result entered", "Resultado registrado", locale),
			body: localeStr(
				fmt.Sprintf("The result for %s vs %s has been recorded.", p.HomeTeam, p.AwayTeam),
				fmt.Sprintf("El resultado de %s vs %s ha sido registrado.", p.HomeTeam, p.AwayTeam),
				locale,
			),
			actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
		}

	case notification.EventMatchPostponed, notification.EventMatchCancelled:
		var p notification.MatchEventPayload
		_ = entry.DecodePayload(&p)
		title := localeStr("Match postponed", "Partido aplazado", locale)
		body := localeStr(
			fmt.Sprintf("%s vs %s has been postponed.", p.HomeTeam, p.AwayTeam),
			fmt.Sprintf("%s vs %s ha sido aplazado.", p.HomeTeam, p.AwayTeam),
			locale,
		)
		if entry.EventType == notification.EventMatchCancelled {
			title = localeStr("Match cancelled", "Partido cancelado", locale)
			body = localeStr(
				fmt.Sprintf("%s vs %s has been cancelled.", p.HomeTeam, p.AwayTeam),
				fmt.Sprintf("%s vs %s ha sido cancelado.", p.HomeTeam, p.AwayTeam),
				locale,
			)
		}
		return userContent{title: title, body: body, actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID)}

	case notification.EventGroupJoinRequested:
		var p notification.GroupJoinPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("New join request", "Nueva solicitud de unión", locale),
			body: localeStr(
				fmt.Sprintf("Someone has requested to join %s. Review and approve or reject the request.", p.QuinielaName),
				fmt.Sprintf("Alguien ha solicitado unirse a %s. Revisa y aprueba o rechaza la solicitud.", p.QuinielaName),
				locale,
			),
			actionURL: fmt.Sprintf("/api/v1/groups/%d/members", p.QuinielaID),
		}

	case notification.EventGroupJoinApproved:
		var p notification.GroupJoinPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Group join approved", "Solicitud de grupo aprobada", locale),
			body: localeStr(
				fmt.Sprintf("You have been approved to join %s.", p.QuinielaName),
				fmt.Sprintf("Has sido aprobado para unirte a %s.", p.QuinielaName),
				locale,
			),
			actionURL: fmt.Sprintf("/api/v1/groups/%d", p.QuinielaID),
		}

	case notification.EventGroupJoinRejected:
		var p notification.GroupJoinPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Group join request rejected", "Solicitud de grupo rechazada", locale),
			body: localeStr(
				fmt.Sprintf("Your request to join %s was not approved.", p.QuinielaName),
				fmt.Sprintf("Tu solicitud para unirte a %s no fue aprobada.", p.QuinielaName),
				locale,
			),
			actionURL: urlGroupsMe,
		}

	case notification.EventGroupDisbanded:
		var p notification.GroupDisbandedPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Group disbanded", "Grupo disuelto", locale),
			body: localeStr(
				fmt.Sprintf("The group %s has been disbanded.", p.QuinielaName),
				fmt.Sprintf("El grupo %s ha sido disuelto.", p.QuinielaName),
				locale,
			),
			actionURL: urlGroupsMe,
		}

	case notification.EventGroupDeadline24h:
		var p notification.GroupDeadlinePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Group deadline in 24 hours", "Límite de grupo en 24 horas", locale),
			body: localeStr(
				fmt.Sprintf("The prediction window for %s closes in 24 hours.", p.QuinielaName),
				fmt.Sprintf("La ventana de predicciones para %s cierra en 24 horas.", p.QuinielaName),
				locale,
			),
			actionURL: fmt.Sprintf("/api/v1/groups/%d", p.QuinielaID),
		}

	case notification.EventGroupLeaderboardMilestone:
		var p notification.GroupLeaderboardMilestonePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Leaderboard milestone", "Hito en el marcador", locale),
			body: localeStr(
				fmt.Sprintf("You are now ranked #%d in %s with %d points.", p.NewRank, p.QuinielaName, p.TotalPoints),
				fmt.Sprintf("Ahora estás en el puesto #%d en %s con %d puntos.", p.NewRank, p.QuinielaName, p.TotalPoints),
				locale,
			),
			actionURL: fmt.Sprintf("/api/v1/groups/%d/leaderboard", p.QuinielaID),
		}

	case notification.EventPaymentConfirmed:
		var p notification.PaymentPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Payment confirmed", "Pago confirmado", locale),
			body: localeStr(
				fmt.Sprintf("Your payment of %s has been confirmed.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu pago de %s ha sido confirmado.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: urlBalance,
		}

	case notification.EventPaymentFailed:
		var p notification.PaymentPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Payment failed", "Pago fallido", locale),
			body: localeStr(
				fmt.Sprintf("Your payment of %s could not be processed. %s", formatCents(p.AmountCents, p.Currency), p.Reason),
				fmt.Sprintf("Tu pago de %s no pudo procesarse. %s", formatCents(p.AmountCents, p.Currency), p.Reason),
				locale,
			),
			actionURL: "/api/v1/payment-intents",
		}

	case notification.EventPaymentBankTransferSubmitted:
		var p notification.BankTransferPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Bank transfer proof submitted", "Comprobante de transferencia enviado", locale),
			body: localeStr(
				fmt.Sprintf("Your transfer proof for %s has been submitted and is awaiting review.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu comprobante de transferencia por %s ha sido enviado y está pendiente de revisión.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: "/api/v1/bank-transfers",
		}

	case notification.EventPaymentBankTransferApproved:
		var p notification.BankTransferPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Bank transfer approved", "Transferencia bancaria aprobada", locale),
			body: localeStr(
				fmt.Sprintf("%s has been credited to your account.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("%s ha sido acreditado a tu cuenta.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: urlBalance,
		}

	case notification.EventPaymentBankTransferRejected:
		var p notification.BankTransferPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Bank transfer rejected", "Transferencia bancaria rechazada", locale),
			body: localeStr(
				fmt.Sprintf("Your transfer proof for %s was rejected. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
				fmt.Sprintf("Tu comprobante de transferencia por %s fue rechazado. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
				locale,
			),
			actionURL: "/api/v1/bank-transfers",
		}

	case notification.EventWithdrawalRequested:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Withdrawal requested", "Retiro solicitado", locale),
			body: localeStr(
				fmt.Sprintf("Your withdrawal of %s is pending admin approval.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu retiro de %s está pendiente de aprobación del administrador.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: urlWithdrawals,
		}

	case notification.EventWithdrawalApproved:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Withdrawal approved", "Retiro aprobado", locale),
			body: localeStr(
				fmt.Sprintf("Your withdrawal of %s has been approved.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu retiro de %s ha sido aprobado.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: urlWithdrawals,
		}

	case notification.EventWithdrawalRejected:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Withdrawal rejected", "Retiro rechazado", locale),
			body: localeStr(
				fmt.Sprintf("Your withdrawal of %s was rejected. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
				fmt.Sprintf("Tu retiro de %s fue rechazado. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
				locale,
			),
			actionURL: urlWithdrawals,
		}

	case notification.EventWithdrawalCompleted:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Withdrawal completed", "Retiro completado", locale),
			body: localeStr(
				fmt.Sprintf("Your withdrawal of %s has been processed successfully.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu retiro de %s ha sido procesado exitosamente.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: urlBalance,
		}

	case notification.EventWithdrawalFailed:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Withdrawal failed", "Retiro fallido", locale),
			body: localeStr(
				fmt.Sprintf("Your withdrawal of %s could not be completed. Please contact support.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu retiro de %s no pudo completarse. Por favor, contacta al soporte.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: urlWithdrawals,
		}

	case notification.EventAccountWelcome:
		var p notification.AccountWelcomePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Welcome to World Cup Quiniela!", "¡Bienvenido a World Cup Quiniela!", locale),
			body: localeStr(
				fmt.Sprintf("Hi %s! Your account is ready. Start predicting now.", p.UserName),
				fmt.Sprintf("¡Hola %s! Tu cuenta está lista. Empieza a predecir ahora.", p.UserName),
				locale,
			),
			actionURL: urlGroupsMe,
		}

	case notification.EventAccountBalanceCredited, notification.EventAccountBalanceDebited:
		var p notification.AccountBalancePayload
		_ = entry.DecodePayload(&p)
		if entry.EventType == notification.EventAccountBalanceCredited {
			return userContent{
				title: localeStr("Balance credited", "Saldo acreditado", locale),
				body: localeStr(
					fmt.Sprintf("%s has been added to your account. New balance: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
					fmt.Sprintf("%s ha sido añadido a tu cuenta. Nuevo saldo: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
					locale,
				),
				actionURL: urlBalance,
			}
		}
		return userContent{
			title: localeStr("Balance debited", "Saldo debitado", locale),
			body: localeStr(
				fmt.Sprintf("%s has been deducted from your account. New balance: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
				fmt.Sprintf("%s ha sido deducido de tu cuenta. Nuevo saldo: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
				locale,
			),
			actionURL: urlBalance,
		}

	case notification.EventAccountLowBalance:
		var p notification.AccountBalancePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Low balance alert", "Alerta de saldo bajo", locale),
			body: localeStr(
				fmt.Sprintf("Your balance is %s. Top up to continue participating.", formatCents(p.BalanceAfter, p.Currency)),
				fmt.Sprintf("Tu saldo es %s. Recarga para seguir participando.", formatCents(p.BalanceAfter, p.Currency)),
				locale,
			),
			actionURL: urlBalance,
		}

	case notification.EventGroupMemberJoined, notification.EventGroupMemberLeft:
		var p notification.GroupJoinPayload
		_ = entry.DecodePayload(&p)
		if entry.EventType == notification.EventGroupMemberJoined {
			return userContent{
				title: localeStr("New member joined your group", "Nuevo miembro en tu grupo", locale),
				body: localeStr(
					fmt.Sprintf("A new member has joined %s.", p.QuinielaName),
					fmt.Sprintf("Un nuevo miembro se ha unido a %s.", p.QuinielaName),
					locale,
				),
				actionURL: fmt.Sprintf("/api/v1/groups/%d/members", p.QuinielaID),
			}
		}
		return userContent{
			title: localeStr("Member left the group", "Miembro abandonó el grupo", locale),
			body: localeStr(
				fmt.Sprintf("A member has left %s.", p.QuinielaName),
				fmt.Sprintf("Un miembro ha abandonado %s.", p.QuinielaName),
				locale,
			),
			actionURL: fmt.Sprintf("/api/v1/groups/%d/members", p.QuinielaID),
		}

	case notification.EventPaymentPendingTimeout:
		var p notification.PaymentPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Payment expired", "Pago expirado", locale),
			body: localeStr(
				fmt.Sprintf("Your payment of %s has expired without confirmation. Please try again.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu pago de %s ha expirado sin confirmación. Por favor, inténtalo de nuevo.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: "/api/v1/payment-intents",
		}

	case notification.EventWithdrawalProcessing:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Withdrawal being processed", "Retiro en proceso", locale),
			body: localeStr(
				fmt.Sprintf("Your withdrawal of %s is now being processed. Funds will be transferred shortly.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu retiro de %s está siendo procesado. Los fondos serán transferidos pronto.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: urlWithdrawals,
		}

	case notification.EventWithdrawalPendingTimeout:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: localeStr("Withdrawal request expired", "Solicitud de retiro expirada", locale),
			body: localeStr(
				fmt.Sprintf("Your withdrawal of %s has expired without admin action. Please submit a new request or contact support.", formatCents(p.AmountCents, p.Currency)),
				fmt.Sprintf("Tu retiro de %s ha expirado sin acción del administrador. Envía una nueva solicitud o contacta al soporte.", formatCents(p.AmountCents, p.Currency)),
				locale,
			),
			actionURL: urlWithdrawals,
		}

	default:
		return userContent{
			title: localeStr("New notification", "Nueva notificación", locale),
			body:  localeStr("You have a new notification. Open the app for details.", "Tienes una nueva notificación. Abre la aplicación para más detalles.", locale),
		}
	}
}
