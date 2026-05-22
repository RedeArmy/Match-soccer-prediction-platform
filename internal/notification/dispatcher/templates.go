package dispatcher

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// Detail-table key constants used across multiple email templates.
const (
	detailKeyUserID    = "User ID"
	detailKeyRequestID = "Request ID"
)

// emailDetail is a single key-value row in the admin email detail table.
// Using a slice (not a map) guarantees deterministic row order across invocations.
type emailDetail struct {
	Key   string
	Value string
}

// details builds an ordered emailDetail slice from alternating key/value pairs.
// Callers that need conditional rows may append to the returned slice.
func details(pairs ...string) []emailDetail {
	d := make([]emailDetail, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		d = append(d, emailDetail{Key: pairs[i], Value: pairs[i+1]})
	}
	return d
}

// emailData is the bag of values injected into every admin email template.
type emailData struct {
	EventType   string
	Subject     string
	Headline    string
	Body        string
	Details     []emailDetail
	GeneratedAt string
}

var baseTemplate = template.Must(template.New("base").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Subject}}</title>
<style>
  body{font-family:Arial,sans-serif;background:#f4f4f4;margin:0;padding:0}
  .wrap{max-width:600px;margin:40px auto;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,.1)}
  .header{background:#1a1a2e;color:#fff;padding:24px 32px}
  .header h1{margin:0;font-size:20px;letter-spacing:.5px}
  .badge{display:inline-block;background:#e74c3c;color:#fff;font-size:11px;font-weight:700;padding:2px 8px;border-radius:12px;margin-left:8px;vertical-align:middle;letter-spacing:.8px}
  .content{padding:32px}
  .content h2{color:#1a1a2e;margin-top:0}
  .content p{color:#444;line-height:1.6}
  .details{background:#f8f9fa;border-left:4px solid #e74c3c;padding:16px 20px;border-radius:0 4px 4px 0;margin:24px 0}
  .details table{border-collapse:collapse;width:100%}
  .details td{padding:4px 0;color:#444;font-size:14px}
  .details td:first-child{font-weight:600;width:40%;color:#222}
  .footer{background:#f8f9fa;padding:16px 32px;font-size:12px;color:#888;text-align:center}
</style>
</head>
<body>
<div class="wrap">
  <div class="header">
    <h1>World Cup Quiniela &mdash; Admin Alert<span class="badge">{{.EventType}}</span></h1>
  </div>
  <div class="content">
    <h2>{{.Headline}}</h2>
    <p>{{.Body}}</p>
    {{if .Details}}
    <div class="details">
      <table>
        {{range .Details}}<tr><td>{{.Key}}</td><td>{{.Value}}</td></tr>{{end}}
      </table>
    </div>
    {{end}}
  </div>
  <div class="footer">Generated at {{.GeneratedAt}} &bull; This is an automated alert — do not reply.</div>
</div>
</body>
</html>`))

// renderEmailFn is the function used to render admin/system email templates.
// Replaced in tests to inject render failures without depending on broken templates.
var renderEmailFn = renderEmail

// renderEmail returns the email subject and HTML body for the given outbox entry.
// For unknown event types a generic template is used so no admin alert is silently dropped.
func renderEmail(entry *notification.OutboxEntry) (subject, html string, err error) {
	data := buildEmailData(entry)
	var buf bytes.Buffer
	if err := baseTemplate.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("dispatcher: render template: %w", err)
	}
	return data.Subject, buf.String(), nil
}

// emailDataBuilder constructs the template data bag for a single admin/system event.
// The now parameter is a pre-formatted UTC timestamp shared across the render call.
type emailDataBuilder func(entry *notification.OutboxEntry, now string) emailData

// emailBuilders is the exhaustive registry mapping every admin/system EventType to
// its dedicated template builder. Events absent from this map fall through to the
// generic layout in buildEmailData — which is always safe but rarely the right copy.
//
// Exhaustiveness is enforced by TestAllAdminEvents_HaveEmailBuilder: the test fails
// if any admin/system event from AllEventTypes() is missing from this map.
// When adding a new admin/system event constant, register it here and write its builder.
var emailBuilders = map[notification.EventType]emailDataBuilder{
	// Bank transfer
	notification.EventAdminBankTransferPending:    buildAdminBankTransferPending,
	notification.EventAdminBankTransferStale:      buildAdminBankTransferStale,
	notification.EventAdminBankTransferQueueDepth: buildAdminBankTransferQueueDepth,
	// Withdrawal
	notification.EventAdminWithdrawalPending:   buildAdminWithdrawalPending,
	notification.EventAdminWithdrawalStale:     buildAdminWithdrawalStale,
	notification.EventAdminHighValueWithdrawal: buildAdminHighValueWithdrawal,
	// Payment / financial
	notification.EventAdminPaymentDispute: buildAdminPaymentDispute,
	// Scheduler digests
	notification.EventAdminPendingReminder: buildAdminPendingReminder,
	notification.EventAdminDailySummary:    buildAdminDailySummary,
	notification.EventAdminWeeklyReport:    buildAdminWeeklyReport,
	// Match / scoring
	notification.EventAdminMatchResultPending: buildAdminMatchResultPending,
	notification.EventAdminScoringDiscrepancy: buildAdminScoringDiscrepancy,
	// Group moderation
	notification.EventAdminGroupReported: buildAdminGroupReported,
	// Circuit breaker
	notification.EventSystemCircuitBreakerOpened:   buildSystemCircuitBreakerOpened,
	notification.EventSystemCircuitBreakerHalfOpen: buildSystemCircuitBreakerHalfOpen,
	// Webhook security
	notification.EventSystemWebhookSignatureFailed:   buildSystemWebhookSignatureFailed,
	notification.EventSystemWebhookSignatureRepeated: buildSystemWebhookSignatureRepeated,
	// Infrastructure integrity
	notification.EventSystemTxRetryExhausted:      buildSystemTxRetryExhausted,
	notification.EventSystemBalanceLedgerMismatch: buildSystemBalanceLedgerMismatch,
	// Abuse / anomaly detection
	notification.EventSystemRateLimitAbuse:       buildSystemRateLimitAbuse,
	notification.EventSystemIdempotencyCollision: buildSystemIdempotencyCollision,
	// Storage
	notification.EventSystemFileStoreUnavailable: buildSystemFileStoreUnavailable,
}

// buildEmailData extracts a human-readable subject, headline, and detail table
// from the outbox entry. It falls back to a generic layout for event types that
// have no dedicated builder so new events are always delivered even before a
// custom template is written. TestAllAdminEvents_HaveEmailBuilder enforces that
// every admin/system event eventually gets a real builder.
func buildEmailData(entry *notification.OutboxEntry) emailData {
	now := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
	if build, ok := emailBuilders[entry.EventType]; ok {
		return build(entry, now)
	}
	return buildGenericLayout(entry, now)
}

// ── Bank Transfer ─────────────────────────────────────────────────────────────

func buildAdminBankTransferPending(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminBankTransferPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[ACTION REQUIRED] Bank transfer proof awaiting review",
		Headline:  "New bank transfer proof submitted",
		Body:      "A user has submitted a bank transfer proof that requires your review before the balance is credited.",
		Details: details(
			"Proof ID", fmt.Sprintf("%d", p.ProofID),
			detailKeyUserID, fmt.Sprintf("%d", p.UserID),
			"Amount", formatCents(p.AmountCents, p.Currency),
			"Submitted", now,
		),
		GeneratedAt: now,
	}
}

func buildAdminBankTransferStale(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminBankTransferPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[URGENT] Bank transfer proof overdue — no admin action in 12+ hours",
		Headline:  "Bank transfer proof has been waiting too long",
		Body:      "A bank transfer proof has exceeded the stale threshold without admin review. Immediate action is required.",
		Details: details(
			"Proof ID", fmt.Sprintf("%d", p.ProofID),
			detailKeyUserID, fmt.Sprintf("%d", p.UserID),
			"Amount", formatCents(p.AmountCents, p.Currency),
			"Pending Since", p.PendingSince,
		),
		GeneratedAt: now,
	}
}

func buildAdminBankTransferQueueDepth(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminBankTransferPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType:   string(entry.EventType),
		Subject:     "[ACTION REQUIRED] Bank transfer queue is backing up",
		Headline:    "Multiple bank transfer proofs awaiting review",
		Body:        "The bank transfer review queue has reached a high depth. Please process pending proofs to avoid further delays.",
		Details:     details("Queue Depth", fmt.Sprintf("%d", p.QueueDepth)),
		GeneratedAt: now,
	}
}

// ── Withdrawal ────────────────────────────────────────────────────────────────

func buildAdminWithdrawalPending(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminWithdrawalPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[ACTION REQUIRED] Withdrawal request awaiting approval",
		Headline:  "New withdrawal request submitted",
		Body:      "A user has submitted a withdrawal request that requires admin approval.",
		Details: details(
			detailKeyRequestID, fmt.Sprintf("%d", p.RequestID),
			detailKeyUserID, fmt.Sprintf("%d", p.UserID),
			"Amount", formatCents(p.AmountCents, p.Currency),
			"Submitted", now,
		),
		GeneratedAt: now,
	}
}

func buildAdminWithdrawalStale(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminWithdrawalPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[URGENT] Withdrawal request overdue — no admin action in 24+ hours",
		Headline:  "Withdrawal request has been waiting too long",
		Body:      "A withdrawal request has exceeded the stale threshold without admin action. Immediate review is required.",
		Details: details(
			detailKeyRequestID, fmt.Sprintf("%d", p.RequestID),
			detailKeyUserID, fmt.Sprintf("%d", p.UserID),
			"Amount", formatCents(p.AmountCents, p.Currency),
			"Pending Since", p.PendingSince,
		),
		GeneratedAt: now,
	}
}

func buildAdminHighValueWithdrawal(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminWithdrawalPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[CRITICAL] High-value withdrawal request requires immediate review",
		Headline:  "High-value withdrawal detected",
		Body:      "A withdrawal request above the high-value threshold has been submitted. This event requires heightened scrutiny before approval.",
		Details: details(
			detailKeyRequestID, fmt.Sprintf("%d", p.RequestID),
			detailKeyUserID, fmt.Sprintf("%d", p.UserID),
			"Amount", formatCents(p.AmountCents, p.Currency),
		),
		GeneratedAt: now,
	}
}

// ── Payment / financial ───────────────────────────────────────────────────────

func buildAdminPaymentDispute(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[ACTION REQUIRED] Payment dispute reported",
		Headline:  "A payment dispute has been raised",
		Body:      "A user or payment provider has raised a dispute on a payment. Please review and respond within the dispute window.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
		),
		GeneratedAt: now,
	}
}

// ── Scheduler digest events ───────────────────────────────────────────────────

func buildAdminPendingReminder(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminPendingReminderPayload
	_ = entry.DecodePayload(&p)
	d := details(
		"Pending Transfers", fmt.Sprintf("%d", p.PendingTransfers),
		"Pending Withdrawals", fmt.Sprintf("%d", p.PendingWithdrawals),
	)
	if p.OldestPendingSince != "" {
		d = append(d, emailDetail{Key: "Oldest Pending Since", Value: p.OldestPendingSince})
	}
	return emailData{
		EventType:   string(entry.EventType),
		Subject:     "[ACTION REQUIRED] Pending items awaiting admin review",
		Headline:    "Periodic pending-items reminder",
		Body:        fmt.Sprintf("%d bank transfer proof(s) and %d withdrawal request(s) are awaiting your review.", p.PendingTransfers, p.PendingWithdrawals),
		Details:     d,
		GeneratedAt: now,
	}
}

func buildAdminDailySummary(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminDailySummaryPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   fmt.Sprintf("[DAILY SUMMARY] Operations summary for %s", p.Date),
		Headline:  "Daily operations summary",
		Body:      fmt.Sprintf("Here is the operations summary for %s.", p.Date),
		Details: details(
			"Date", p.Date,
			"New Users", fmt.Sprintf("%d", p.NewUsers),
			"New Transfers", fmt.Sprintf("%d", p.NewTransfers),
			"Approved Transfers", fmt.Sprintf("%d", p.ApprovedTransfers),
			"Total Credited", formatCents(p.TotalCreditedCents, "GTQ"),
			"New Withdrawals", fmt.Sprintf("%d", p.NewWithdrawals),
			"Pending Transfers", fmt.Sprintf("%d", p.PendingTransfers),
			"Pending Withdrawals", fmt.Sprintf("%d", p.PendingWithdrawals),
		),
		GeneratedAt: now,
	}
}

func buildAdminWeeklyReport(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminWeeklyReportPayload
	_ = entry.DecodePayload(&p)
	d := details(
		"Period", fmt.Sprintf("%s – %s", p.WeekStartDate, p.WeekEndDate),
		"Total Revenue", formatCents(p.TotalRevenueCents, "GTQ"),
		"New Users", fmt.Sprintf("%d", p.NewUsers),
		"Active Quinielas", fmt.Sprintf("%d", p.ActiveQuinielas),
		"Total Withdrawals", fmt.Sprintf("%d", p.TotalWithdrawals),
		"Withdrawal Amount", formatCents(p.WithdrawalCents, "GTQ"),
	)
	if p.TopGroupName != "" {
		d = append(d, emailDetail{Key: "Top Group", Value: fmt.Sprintf("%s (%d pts)", p.TopGroupName, p.TopGroupPoints)})
	}
	return emailData{
		EventType:   string(entry.EventType),
		Subject:     fmt.Sprintf("[WEEKLY REPORT] %s – %s", p.WeekStartDate, p.WeekEndDate),
		Headline:    "Weekly operations report",
		Body:        fmt.Sprintf("Weekly summary for the period %s to %s.", p.WeekStartDate, p.WeekEndDate),
		Details:     d,
		GeneratedAt: now,
	}
}

// ── Match / scoring admin events ──────────────────────────────────────────────

func buildAdminMatchResultPending(entry *notification.OutboxEntry, now string) emailData {
	var p notification.AdminMatchResultPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   fmt.Sprintf("[ACTION REQUIRED] Match result not entered — %s vs %s", p.HomeTeam, p.AwayTeam),
		Headline:  "Match result awaiting entry",
		Body:      fmt.Sprintf("%s vs %s finished %d minutes ago but no result has been recorded. Predictions cannot be scored until the result is entered.", p.HomeTeam, p.AwayTeam, p.MinutesElapsed),
		Details: details(
			"Match ID", fmt.Sprintf("%d", p.MatchID),
			"Teams", fmt.Sprintf("%s vs %s", p.HomeTeam, p.AwayTeam),
			"Finished At", p.FinishedAt.UTC().Format(time.RFC3339),
			"Minutes Elapsed", fmt.Sprintf("%d", p.MinutesElapsed),
		),
		GeneratedAt: now,
	}
}

func buildAdminScoringDiscrepancy(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[CRITICAL] Scoring discrepancy detected — manual review required",
		Headline:  "Scoring calculation discrepancy",
		Body:      "A discrepancy has been detected in the scoring calculation. Affected predictions may carry incorrect points. Immediate investigation is required to preserve leaderboard integrity.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
			"Severity", p.Severity,
		),
		GeneratedAt: now,
	}
}

// ── Group moderation ──────────────────────────────────────────────────────────

func buildAdminGroupReported(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[ACTION REQUIRED] Quiniela group reported by a user",
		Headline:  "User report filed against a group",
		Body:      "A user has reported a quiniela group for a potential policy violation. Please review the group and take appropriate action within the required response window.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
		),
		GeneratedAt: now,
	}
}

// ── Circuit breaker ───────────────────────────────────────────────────────────

func buildSystemCircuitBreakerOpened(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[CRITICAL] Circuit breaker opened — " + p.Component,
		Headline:  "Circuit breaker is OPEN",
		Body:      "A circuit breaker has opened, indicating repeated failures in a system component. Downstream operations are degraded.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
			"Severity", p.Severity,
		),
		GeneratedAt: now,
	}
}

func buildSystemCircuitBreakerHalfOpen(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[WARNING] Circuit breaker in half-open state — " + p.Component,
		Headline:  "Circuit breaker transitioning to half-open",
		Body:      "A circuit breaker has entered the half-open state, indicating cautious recovery from a failure period. Monitor closely — further failures will re-open the breaker and resume degraded operation.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
			"Severity", p.Severity,
		),
		GeneratedAt: now,
	}
}

// ── Webhook security ──────────────────────────────────────────────────────────

func buildSystemWebhookSignatureFailed(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[SECURITY] Webhook signature verification failed",
		Headline:  "Invalid webhook signature detected",
		Body:      "An incoming webhook failed signature verification. This could indicate a replay attack or misconfiguration.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
		),
		GeneratedAt: now,
	}
}

func buildSystemWebhookSignatureRepeated(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[SECURITY] Repeated webhook signature failures",
		Headline:  "Multiple webhook signature failures in succession",
		Body:      "Repeated webhook signature verification failures suggest a potential attack or a broken provider configuration.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
		),
		GeneratedAt: now,
	}
}

// ── Infrastructure integrity ──────────────────────────────────────────────────

func buildSystemTxRetryExhausted(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	d := details(
		"Component", p.Component,
		"Detail", p.Detail,
		"Severity", p.Severity,
	)
	if len(p.AffectedIDs) > 0 {
		d = append(d, emailDetail{Key: "Affected IDs", Value: fmt.Sprint(p.AffectedIDs)})
	}
	return emailData{
		EventType:   string(entry.EventType),
		Subject:     "[CRITICAL] Transaction retry limit exhausted — potential data loss",
		Headline:    "Transaction retry attempts exhausted",
		Body:        "A database transaction has exhausted all retry attempts and was abandoned. Data integrity may be at risk. Immediate investigation is required to determine whether any domain writes were lost.",
		Details:     d,
		GeneratedAt: now,
	}
}

func buildSystemBalanceLedgerMismatch(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[CRITICAL] Balance ledger mismatch detected — financial integrity at risk",
		Headline:  "Balance ledger integrity check failed",
		Body:      "The balance ledger consistency check has detected a mismatch. This is a P0 financial integrity issue requiring immediate investigation.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
			"Severity", p.Severity,
		),
		GeneratedAt: now,
	}
}

// ── Abuse / anomaly detection ─────────────────────────────────────────────────

func buildSystemRateLimitAbuse(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[SECURITY] Suspected rate limit abuse detected",
		Headline:  "Abnormal request rate detected",
		Body:      "A client has triggered the rate limiter at an unusually high frequency, suggesting automated abuse or a misconfigured integration. Review the client and consider blocking the source.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
		),
		GeneratedAt: now,
	}
}

func buildSystemIdempotencyCollision(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[WARNING] Idempotency key collision detected",
		Headline:  "Duplicate request with conflicting payload",
		Body:      "Two requests were received with the same idempotency key but differing payloads, indicating a potential client bug or replay attack. The conflicting request was rejected. No data was modified.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
		),
		GeneratedAt: now,
	}
}

// ── Storage ───────────────────────────────────────────────────────────────────

func buildSystemFileStoreUnavailable(entry *notification.OutboxEntry, now string) emailData {
	var p notification.SystemAlertPayload
	_ = entry.DecodePayload(&p)
	return emailData{
		EventType: string(entry.EventType),
		Subject:   "[CRITICAL] File storage service unavailable",
		Headline:  "File store is unreachable",
		Body:      "The file storage service is unavailable. Proof uploads and all file-dependent operations are currently failing. Immediate action is required to restore service.",
		Details: details(
			"Component", p.Component,
			"Detail", p.Detail,
			"Severity", p.Severity,
		),
		GeneratedAt: now,
	}
}

// ── Generic fallback ──────────────────────────────────────────────────────────

func buildGenericLayout(entry *notification.OutboxEntry, now string) emailData {
	return emailData{
		EventType:   string(entry.EventType),
		Subject:     "[ADMIN ALERT] " + string(entry.EventType),
		Headline:    "Admin notification: " + string(entry.EventType),
		Body:        "An admin-level event has been raised. Please review the details below.",
		Details:     details("Event Type", string(entry.EventType), "Aggregate ID", entry.AggregateID),
		GeneratedAt: now,
	}
}

// ── Utilities ─────────────────────────────────────────────────────────────────

// formatCents converts an integer cent value to a human-readable amount string.
func formatCents(cents int, currency string) string {
	if cents == 0 {
		return fmt.Sprintf("0.00 %s", currency)
	}
	return fmt.Sprintf("%.2f %s", float64(cents)/100.0, currency)
}
