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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// ParamReader is the subset of SystemParamService consumed by AdminDispatcher.
// Defined as a narrow interface so the dispatcher does not import the full
// service package (avoiding coupling and easing test doubles).
type ParamReader interface {
	GetString(ctx context.Context, key, defaultVal string) string
	GetInt(ctx context.Context, key string, defaultVal int) int
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
//  7. For system.* events: fires the optional n8n webhook (best-effort),
//     signed with HMAC-SHA256 when n8nSecret is configured.
type AdminDispatcher struct {
	params      ParamReader
	logRepo     repository.AdminNotificationLogCreator
	dlqRepo     repository.NotificationDLQEntryCreator
	mailer      infraemail.Sender
	fromAddr    string
	n8nURL      string // empty disables webhook
	n8nSecret   string // empty sends unsigned requests (warns at construction)
	httpClient  *http.Client
	log         *zap.Logger
	renderFn    func(*notification.OutboxEntry) (string, string, error)
	instruments dispatcherInstruments
}

// Config bundles the constructor arguments for AdminDispatcher.
type Config struct {
	Params    ParamReader
	LogRepo   repository.AdminNotificationLogCreator
	DLQRepo   repository.NotificationDLQEntryCreator
	Mailer    infraemail.Sender
	FromAddr  string // e.g. "Quiniela <noreply@example.com>"
	N8nURL    string // optional; empty disables the n8n webhook
	N8nSecret string // optional; empty sends unsigned requests (WCQ_N8N_WEBHOOKSECRET)
	Log       *zap.Logger
}

// NewAdminDispatcher constructs an AdminDispatcher.
func NewAdminDispatcher(cfg Config) *AdminDispatcher {
	if cfg.N8nURL != "" && cfg.N8nSecret == "" {
		cfg.Log.Warn("n8n webhook: WCQ_N8N_WEBHOOKSECRET not set — requests will be unsigned; any caller who discovers the URL can inject events")
	}
	return &AdminDispatcher{
		params:    cfg.Params,
		logRepo:   cfg.LogRepo,
		dlqRepo:   cfg.DLQRepo,
		mailer:    cfg.Mailer,
		fromAddr:  cfg.FromAddr,
		n8nURL:    cfg.N8nURL,
		n8nSecret: cfg.N8nSecret,
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		log:      cfg.Log,
		renderFn: renderEmail,
	}
}

// Dispatch implements outbox.Dispatcher.
func (d *AdminDispatcher) Dispatch(ctx context.Context, entry *notification.OutboxEntry) error {
	ctx, span := otel.Tracer("dispatcher").Start(ctx, "dispatcher.admin.dispatch")
	span.SetAttributes(
		attribute.String("event_type", string(entry.EventType)),
		attribute.Int64("outbox_id", entry.ID),
	)
	defer span.End()

	if !notification.IsAdminEvent(entry.EventType) {
		// Phase 2 will handle user-facing events (push, in-app, SSE).
		return nil
	}

	start := time.Now()

	log := d.log.With(
		append([]zap.Field{
			zap.Int64("outbox_id", entry.ID),
			zap.String("event_type", string(entry.EventType)),
		}, tracing.LogFields(ctx)...)...,
	)

	recipients := d.resolveRecipients(ctx)
	if len(recipients) == 0 {
		log.Warn("no admin recipients configured — skipping email dispatch",
			zap.String("param_key", domain.ParamKeyNotifyAdminEmails),
		)
		return nil
	}

	renderTimeout := time.Duration(
		d.params.GetInt(ctx, domain.ParamKeyNotifyRenderTimeoutMs, domain.DefaultNotifyRenderTimeoutMs),
	) * time.Millisecond

	var subject, html string
	if err := withRenderTimeout(ctx, renderTimeout, func() error {
		var e error
		subject, html, e = d.renderFn(entry)
		return e
	}); err != nil {
		log.Error("failed to render admin email template",
			zap.Duration("render_timeout", renderTimeout),
			zap.Error(err),
		)
		return fmt.Errorf("dispatcher: render: %w", err)
	}

	from := d.params.GetString(ctx, domain.ParamKeyNotifyFromAddress, d.fromAddr)

	msgID, sendErr := d.mailer.Send(ctx, infraemail.Message{
		From:    from,
		To:      recipients,
		Subject: subject,
		HTML:    html,
	})

	d.writeLog(ctx, entry, recipients, subject, msgID, sendErr)

	if sendErr != nil {
		span.RecordError(sendErr)
		span.SetStatus(codes.Error, "email delivery failed")
		d.writeDLQ(ctx, entry, sendErr)
		log.Error("admin email delivery failed",
			zap.Strings("recipients", recipients),
			zap.String("subject", subject),
			zap.Error(sendErr),
		)
		d.recordAdminDispatch(ctx, entry, time.Since(start), "failed")
		return fmt.Errorf("dispatcher: email: %w", sendErr)
	}

	log.Info("admin email delivered",
		zap.Strings("recipients", recipients),
		zap.String("subject", subject),
		zap.String("resend_msg_id", msgID),
	)

	d.recordAdminDispatch(ctx, entry, time.Since(start), "success")

	// n8n webhook is best-effort for system-level alerts; never blocks delivery.
	if d.n8nURL != "" && isSystemEvent(entry.EventType) {
		d.notifyN8n(ctx, entry, log)
	}

	return nil
}

func (d *AdminDispatcher) recordAdminDispatch(ctx context.Context, entry *notification.OutboxEntry, elapsed time.Duration, status string) {
	if d.instruments.events == nil {
		return
	}
	et := string(entry.EventType)
	d.instruments.events.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("event_type", et),
			attribute.String("status", status),
		),
	)
	d.instruments.duration.Record(ctx, elapsed.Seconds(),
		metric.WithAttributes(attribute.String("event_type", et)),
	)
	emailStatus := "sent"
	if status == "failed" {
		emailStatus = "failed"
	}
	d.instruments.emails.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", emailStatus)),
	)
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
// When n8nSecret is set the request carries an X-Signature: sha256=<hex>
// header computed as HMAC-SHA256(body, secret) so the n8n workflow can
// verify the sender's identity before acting on the payload.
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
	if d.n8nSecret != "" {
		mac := hmac.New(sha256.New, []byte(d.n8nSecret))
		mac.Write([]byte(body))
		req.Header.Set("X-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
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
