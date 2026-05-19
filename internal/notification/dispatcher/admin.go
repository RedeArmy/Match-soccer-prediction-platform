// Package dispatcher implements the outbox.Dispatcher interface for Phase 1
// of the notification subsystem.
//
// AdminDispatcher routes admin.* and system.* events to admin recipients via
// email (Resend) and, for system-level alerts, to an optional n8n webhook for
// ops-channel delivery.  User-facing events (prediction.*, match.*, etc.) are
// silently acknowledged without delivery until Phase 2 wires their channels.
package dispatcher

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ParamReader is the subset of SystemParamService consumed by AdminDispatcher.
// Defined as a narrow interface so the dispatcher does not import the full
// service package (avoiding coupling and easing test doubles).
type ParamReader interface {
	GetString(ctx context.Context, key, defaultVal string) string
}

// AdminDispatcher implements outbox.Dispatcher for admin and system events.
//
// For every claimed outbox entry it:
//  1. Skips non-admin events (noop — they will be handled in Phase 2).
//  2. Resolves the admin recipient list from notify.admin_emails.
//  3. Renders the event-specific HTML email template.
//  4. Sends the email via the configured Client.
//  5. Appends an immutable row to admin_notification_log.
//  6. On email failure: appends a row to notification_dlq and returns the
//     error so the outbox worker applies exponential-backoff retry.
//  7. For system.* events: fires the optional n8n webhook (best-effort).
type AdminDispatcher struct {
	params     ParamReader
	logRepo    repository.AdminNotificationLogCreator
	dlqRepo    repository.NotificationDLQEntryCreator
	mailer     infraemail.Sender
	fromAddr   string
	n8nURL     string // empty disables webhook
	httpClient *http.Client
	log        *zap.Logger
}

// Config bundles the constructor arguments for AdminDispatcher.
type Config struct {
	Params   ParamReader
	LogRepo  repository.AdminNotificationLogCreator
	DLQRepo  repository.NotificationDLQEntryCreator
	Mailer   infraemail.Sender
	FromAddr string // e.g. "Quiniela <noreply@example.com>"
	N8nURL   string // optional; empty disables the n8n webhook
	Log      *zap.Logger
}

// NewAdminDispatcher constructs an AdminDispatcher.
func NewAdminDispatcher(cfg Config) *AdminDispatcher {
	return &AdminDispatcher{
		params:     cfg.Params,
		logRepo:    cfg.LogRepo,
		dlqRepo:    cfg.DLQRepo,
		mailer:     cfg.Mailer,
		fromAddr:   cfg.FromAddr,
		n8nURL:     cfg.N8nURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		log:        cfg.Log,
	}
}

// Dispatch implements outbox.Dispatcher.
func (d *AdminDispatcher) Dispatch(ctx context.Context, entry *notification.OutboxEntry) error {
	if !notification.IsAdminEvent(entry.EventType) {
		// Phase 2 will handle user-facing events (push, in-app, SSE).
		return nil
	}

	log := d.log.With(
		zap.Int64("outbox_id", entry.ID),
		zap.String("event_type", string(entry.EventType)),
	)

	recipients := d.resolveRecipients(ctx)
	if len(recipients) == 0 {
		log.Warn("no admin recipients configured — skipping email dispatch",
			zap.String("param_key", domain.ParamKeyNotifyAdminEmails),
		)
		return nil
	}

	subject, html, err := renderEmail(entry)
	if err != nil {
		log.Error("failed to render admin email template", zap.Error(err))
		return err
	}

	msgID, sendErr := d.mailer.Send(ctx, infraemail.Message{
		From:    d.fromAddr,
		To:      recipients,
		Subject: subject,
		HTML:    html,
	})

	d.writeLog(ctx, entry, recipients, subject, msgID, sendErr)

	if sendErr != nil {
		d.writeDLQ(ctx, entry, sendErr)
		log.Error("admin email delivery failed",
			zap.Strings("recipients", recipients),
			zap.String("subject", subject),
			zap.Error(sendErr),
		)
		return fmt.Errorf("dispatcher: email: %w", sendErr)
	}

	log.Info("admin email delivered",
		zap.Strings("recipients", recipients),
		zap.String("subject", subject),
		zap.String("resend_msg_id", msgID),
	)

	// n8n webhook is best-effort for system-level alerts; never blocks delivery.
	if d.n8nURL != "" && isSystemEvent(entry.EventType) {
		d.notifyN8n(ctx, entry, log)
	}

	return nil
}

// resolveRecipients parses the notify.admin_emails system param into a slice
// of trimmed, non-empty email addresses.
func (d *AdminDispatcher) resolveRecipients(ctx context.Context) []string {
	raw := d.params.GetString(ctx, domain.ParamKeyNotifyAdminEmails, "")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if addr := strings.TrimSpace(p); addr != "" {
			out = append(out, addr)
		}
	}
	return out
}

// writeLog appends an immutable row to admin_notification_log.  Errors are
// logged and swallowed — a log-write failure must never suppress or retry the
// primary dispatch path.
func (d *AdminDispatcher) writeLog(
	ctx context.Context,
	entry *notification.OutboxEntry,
	recipients []string,
	subject, msgID string,
	sendErr error,
) {
	logEntry := &domain.AdminNotificationLog{
		EventType:  string(entry.EventType),
		Recipients: recipients,
		Subject:    subject,
	}
	if sendErr == nil {
		logEntry.Status = domain.AdminNotifStatusSent
		logEntry.ResendMsgID = msgID
	} else {
		logEntry.Status = domain.AdminNotifStatusFailed
		logEntry.ErrorDetail = sendErr.Error()
	}
	if err := d.logRepo.Create(ctx, logEntry); err != nil {
		d.log.Warn("failed to write admin_notification_log (best-effort)",
			zap.String("event_type", string(entry.EventType)),
			zap.Error(err),
		)
	}
}

// writeDLQ inserts a notification_dlq row for the failed delivery so operators
// can inspect and replay it.  Errors are logged and swallowed.
func (d *AdminDispatcher) writeDLQ(ctx context.Context, entry *notification.OutboxEntry, sendErr error) {
	outboxID := entry.ID
	dlqEntry := &domain.NotificationDLQEntry{
		OutboxID:    &outboxID,
		Channel:     "email",
		EventType:   string(entry.EventType),
		Payload:     entry.Payload,
		ErrorDetail: sendErr.Error(),
	}
	if err := d.dlqRepo.CreateEntry(ctx, dlqEntry); err != nil {
		d.log.Warn("failed to write notification_dlq (best-effort)",
			zap.String("event_type", string(entry.EventType)),
			zap.Error(err),
		)
	}
}

// notifyN8n fires a best-effort POST to the configured n8n webhook URL.
// The payload is a minimal JSON object with event_type and aggregate_id.
func (d *AdminDispatcher) notifyN8n(ctx context.Context, entry *notification.OutboxEntry, log *zap.Logger) {
	body := fmt.Sprintf(
		`{"event_type":%q,"aggregate_type":%q,"aggregate_id":%q}`,
		entry.EventType, entry.AggregateType, entry.AggregateID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.n8nURL,
		strings.NewReader(body))
	if err != nil {
		log.Warn("n8n webhook: failed to build request", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Warn("n8n webhook: request failed", zap.Error(err))
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		log.Warn("n8n webhook: unexpected status", zap.Int("status", resp.StatusCode))
	}
}

// isSystemEvent reports whether et is in the system.* namespace, which
// warrants n8n webhook delivery in addition to email.
func isSystemEvent(et notification.EventType) bool {
	return strings.HasPrefix(string(et), "system.")
}
