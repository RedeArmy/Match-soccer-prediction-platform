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
}

// minUserPayload extracts only the user_id field from any outbox payload.
type minUserPayload struct {
	UserID int `json:"user_id"`
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
//  2. Extracts the target user ID from the payload.
//  3. Renders the notification title and body.
//  4. Persists a UserNotification row with idempotency guard.
//  5. Fires pg_notify('user_notifications', …) for the SSE bridge.
//  6. Checks notification preferences; if channel_push=true and active
//     subscriptions exist, sends Web Push (marks 410 Gone subscriptions inactive).
//  7. For P0/P1 events with channel_email=true, delivers email via mailer.
//  8. On persist failure, writes a DLQ entry and returns the error for retry.
type UserDispatcher struct {
	notifRepo     repository.UserNotificationRepository
	prefRepo      repository.NotificationPreferenceRepository
	pushRepo      repository.PushSubscriptionRepository
	dlqRepo       repository.NotificationDLQEntryCreator
	hub           *hub.Hub
	pusher        infrapush.Sender
	mailer        infraemail.Sender
	emailResolver UserEmailResolver
	fromAddr      string
	pgNotifier    PgNotifier
	params        service.SystemParamService
	log           *zap.Logger
}

// UserDispatcherConfig bundles constructor arguments for UserDispatcher.
type UserDispatcherConfig struct {
	NotifRepo     repository.UserNotificationRepository
	PrefRepo      repository.NotificationPreferenceRepository
	PushRepo      repository.PushSubscriptionRepository
	DLQRepo       repository.NotificationDLQEntryCreator
	Hub           *hub.Hub
	Pusher        infrapush.Sender
	Mailer        infraemail.Sender
	EmailResolver UserEmailResolver // nil disables email delivery
	FromAddr      string
	PgNotifier    PgNotifier                 // nil disables pg_notify (tests without DB)
	Params        service.SystemParamService // nil uses defaults
	Log           *zap.Logger
}

// NewUserDispatcher constructs a UserDispatcher.
func NewUserDispatcher(cfg UserDispatcherConfig) *UserDispatcher {
	return &UserDispatcher{
		notifRepo:     cfg.NotifRepo,
		prefRepo:      cfg.PrefRepo,
		pushRepo:      cfg.PushRepo,
		dlqRepo:       cfg.DLQRepo,
		hub:           cfg.Hub,
		pusher:        cfg.Pusher,
		mailer:        cfg.Mailer,
		emailResolver: cfg.EmailResolver,
		fromAddr:      cfg.FromAddr,
		pgNotifier:    cfg.PgNotifier,
		params:        cfg.Params,
		log:           cfg.Log,
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

	// Extract target user ID.
	var up minUserPayload
	if err := json.Unmarshal(entry.Payload, &up); err != nil || up.UserID == 0 {
		log.Warn("user dispatcher: cannot extract user_id from payload; skipping")
		return nil
	}
	userID := up.UserID

	// Render title/body.
	content := buildUserContent(entry)

	// Persist notification (idempotency-safe).
	n := &domain.UserNotification{
		UserID:         userID,
		EventType:      string(entry.EventType),
		Title:          content.title,
		Body:           content.body,
		ActionURL:      content.actionURL,
		IdempotencyKey: fmt.Sprintf("outbox-%d", entry.ID),
	}
	inserted, err := d.notifRepo.Create(ctx, n)
	if err != nil {
		d.writeDLQEntry(ctx, entry, userID, "inapp", err)
		log.Error("user dispatcher: failed to persist notification", zap.Error(err))
		return fmt.Errorf("user dispatcher: persist: %w", err)
	}

	// Broadcast via pg_notify (best-effort, SSE bridge on API server picks it up).
	if inserted && d.pgNotifier != nil {
		d.notifyPg(ctx, n, entry, log)
	}

	// Delivery channels depend on priority and preferences.
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
		code, sendErr := d.pusher.Send(ctx, infrapush.Message{
			Endpoint:  sub.Endpoint,
			P256dhKey: sub.P256dhKey,
			AuthKey:   sub.AuthKey,
			Body:      body,
			TTL:       ttl,
		})
		if sendErr != nil {
			log.Warn("user dispatcher: push send failed",
				zap.Int64("sub_id", sub.ID),
				zap.Error(sendErr),
			)
			continue
		}
		if code == http.StatusGone {
			// Subscription has expired at the push service.  Mark it inactive
			// so future dispatches skip it, write a DLQ entry for observability,
			// and continue to any remaining subscriptions for this user.
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

// buildUserContent returns the rendered title and body for the given event.
func buildUserContent(entry *notification.OutboxEntry) userContent {
	switch entry.EventType {

	case notification.EventPredictionConfirmed:
		var p notification.PredictionConfirmedPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Prediction confirmed",
			body:  fmt.Sprintf("Your prediction for %s vs %s has been recorded.", p.HomeTeam, p.AwayTeam),
		}

	case notification.EventPredictionDeadlineApproach:
		var p notification.PredictionDeadlinePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Prediction deadline approaching",
			body:  fmt.Sprintf("%s vs %s kicks off in %d minutes — submit your prediction now.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
		}

	case notification.EventPredictionMissingReminder:
		var p notification.PredictionDeadlinePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Missing prediction reminder",
			body:  fmt.Sprintf("You haven't predicted %s vs %s yet. Deadline is in %d minutes.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
		}

	case notification.EventPredictionLocked:
		var p notification.PredictionLockedPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Predictions locked",
			body:  fmt.Sprintf("Predictions for %s vs %s are now locked.", p.HomeTeam, p.AwayTeam),
		}

	case notification.EventPredictionScored:
		var p notification.PredictionScoredPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Match scored",
			body:  fmt.Sprintf("%s vs %s finished %d-%d. You earned %d points.", p.HomeTeam, p.AwayTeam, p.HomeScore, p.AwayScore, p.PointsEarned),
		}

	case notification.EventMatchResultEntered:
		var p notification.MatchEventPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Match result entered",
			body:  fmt.Sprintf("The result for %s vs %s has been recorded.", p.HomeTeam, p.AwayTeam),
		}

	case notification.EventMatchPostponed:
		var p notification.MatchEventPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Match postponed",
			body:  fmt.Sprintf("%s vs %s has been postponed.", p.HomeTeam, p.AwayTeam),
		}

	case notification.EventMatchCancelled:
		var p notification.MatchEventPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Match cancelled",
			body:  fmt.Sprintf("%s vs %s has been cancelled.", p.HomeTeam, p.AwayTeam),
		}

	case notification.EventGroupJoinApproved:
		var p notification.GroupJoinPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Group join approved",
			body:  fmt.Sprintf("You have been approved to join %s.", p.QuinielaNam),
		}

	case notification.EventGroupJoinRejected:
		var p notification.GroupJoinPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Group join request rejected",
			body:  fmt.Sprintf("Your request to join %s was not approved.", p.QuinielaNam),
		}

	case notification.EventGroupDisbanded:
		var p notification.GroupDisbandedPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Group disbanded",
			body:  fmt.Sprintf("The group %s has been disbanded.", p.QuinielaNam),
		}

	case notification.EventGroupDeadline24h:
		var p notification.GroupDeadlinePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Group deadline in 24 hours",
			body:  fmt.Sprintf("The prediction window for %s closes in 24 hours.", p.QuinielaNam),
		}

	case notification.EventGroupLeaderboardMilestone:
		var p notification.GroupLeaderboardMilestonePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Leaderboard milestone",
			body:  fmt.Sprintf("You are now ranked #%d in %s with %d points.", p.NewRank, p.QuinielaNam, p.TotalPoints),
		}

	case notification.EventPaymentConfirmed:
		var p notification.PaymentPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Payment confirmed",
			body:  fmt.Sprintf("Your payment of %s has been confirmed.", formatCents(p.AmountCents, p.Currency)),
		}

	case notification.EventPaymentFailed:
		var p notification.PaymentPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Payment failed",
			body:  fmt.Sprintf("Your payment of %s could not be processed. %s", formatCents(p.AmountCents, p.Currency), p.Reason),
		}

	case notification.EventPaymentBankTransferSubmitted:
		var p notification.BankTransferPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Bank transfer proof submitted",
			body:  fmt.Sprintf("Your transfer proof for %s has been submitted and is awaiting review.", formatCents(p.AmountCents, p.Currency)),
		}

	case notification.EventPaymentBankTransferApproved:
		var p notification.BankTransferPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Bank transfer approved",
			body:  fmt.Sprintf("%s has been credited to your account.", formatCents(p.AmountCents, p.Currency)),
		}

	case notification.EventPaymentBankTransferRejected:
		var p notification.BankTransferPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Bank transfer rejected",
			body:  fmt.Sprintf("Your transfer proof for %s was rejected. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
		}

	case notification.EventWithdrawalRequested:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Withdrawal requested",
			body:  fmt.Sprintf("Your withdrawal of %s is pending admin approval.", formatCents(p.AmountCents, p.Currency)),
		}

	case notification.EventWithdrawalApproved:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Withdrawal approved",
			body:  fmt.Sprintf("Your withdrawal of %s has been approved.", formatCents(p.AmountCents, p.Currency)),
		}

	case notification.EventWithdrawalRejected:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Withdrawal rejected",
			body:  fmt.Sprintf("Your withdrawal of %s was rejected. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
		}

	case notification.EventWithdrawalCompleted:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Withdrawal completed",
			body:  fmt.Sprintf("Your withdrawal of %s has been processed successfully.", formatCents(p.AmountCents, p.Currency)),
		}

	case notification.EventWithdrawalFailed:
		var p notification.WithdrawalPayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Withdrawal failed",
			body:  fmt.Sprintf("Your withdrawal of %s could not be completed. Please contact support.", formatCents(p.AmountCents, p.Currency)),
		}

	case notification.EventAccountWelcome:
		var p notification.AccountWelcomePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Welcome to World Cup Quiniela!",
			body:  fmt.Sprintf("Hi %s! Your account is ready. Start predicting now.", p.UserName),
		}

	case notification.EventAccountBalanceCredited:
		var p notification.AccountBalancePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Balance credited",
			body:  fmt.Sprintf("%s has been added to your account. New balance: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
		}

	case notification.EventAccountLowBalance:
		var p notification.AccountBalancePayload
		_ = entry.DecodePayload(&p)
		return userContent{
			title: "Low balance alert",
			body:  fmt.Sprintf("Your balance is %s. Top up to continue participating.", formatCents(p.BalanceAfter, p.Currency)),
		}

	default:
		return userContent{
			title: "New notification",
			body:  "You have a new notification. Open the app for details.",
		}
	}
}
