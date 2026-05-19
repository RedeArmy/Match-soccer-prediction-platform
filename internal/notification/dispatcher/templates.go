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

// emailData is the bag of values injected into every admin email template.
type emailData struct {
	EventType   string
	Subject     string
	Headline    string
	Body        string
	Details     map[string]string
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
        {{range $k,$v := .Details}}<tr><td>{{$k}}</td><td>{{$v}}</td></tr>{{end}}
      </table>
    </div>
    {{end}}
  </div>
  <div class="footer">Generated at {{.GeneratedAt}} &bull; This is an automated alert — do not reply.</div>
</div>
</body>
</html>`))

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

// buildEmailData extracts a human-readable subject, headline, and detail table
// from the outbox entry.  It falls back to a generic layout for unlisted types
// so new events are always delivered even before a dedicated template is added.
func buildEmailData(entry *notification.OutboxEntry) emailData {
	now := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
	et := string(entry.EventType)

	switch entry.EventType {

	// ── Bank Transfer ─────────────────────────────────────────────────────────

	case notification.EventAdminBankTransferPending:
		var p notification.AdminBankTransferPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[ACTION REQUIRED] Bank transfer proof awaiting review",
			Headline:  "New bank transfer proof submitted",
			Body:      "A user has submitted a bank transfer proof that requires your review before the balance is credited.",
			Details: map[string]string{
				"Proof ID":      fmt.Sprintf("%d", p.ProofID),
				detailKeyUserID: fmt.Sprintf("%d", p.UserID),
				"Amount":        formatCents(p.AmountCents, p.Currency),
				"Submitted":     now,
			},
			GeneratedAt: now,
		}

	case notification.EventAdminBankTransferStale:
		var p notification.AdminBankTransferPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[URGENT] Bank transfer proof overdue — no admin action in 12+ hours",
			Headline:  "Bank transfer proof has been waiting too long",
			Body:      "A bank transfer proof has exceeded the stale threshold without admin review. Immediate action is required.",
			Details: map[string]string{
				"Proof ID":      fmt.Sprintf("%d", p.ProofID),
				detailKeyUserID: fmt.Sprintf("%d", p.UserID),
				"Amount":        formatCents(p.AmountCents, p.Currency),
				"Pending Since": p.PendingSince,
			},
			GeneratedAt: now,
		}

	case notification.EventAdminBankTransferQueueDepth:
		var p notification.AdminBankTransferPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[ACTION REQUIRED] Bank transfer queue is backing up",
			Headline:  "Multiple bank transfer proofs awaiting review",
			Body:      "The bank transfer review queue has reached a high depth. Please process pending proofs to avoid further delays.",
			Details: map[string]string{
				"Queue Depth": fmt.Sprintf("%d", p.QueueDepth),
			},
			GeneratedAt: now,
		}

	// ── Withdrawal ────────────────────────────────────────────────────────────

	case notification.EventAdminWithdrawalPending:
		var p notification.AdminWithdrawalPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[ACTION REQUIRED] Withdrawal request awaiting approval",
			Headline:  "New withdrawal request submitted",
			Body:      "A user has submitted a withdrawal request that requires admin approval.",
			Details: map[string]string{
				detailKeyRequestID: fmt.Sprintf("%d", p.RequestID),
				detailKeyUserID:    fmt.Sprintf("%d", p.UserID),
				"Amount":           formatCents(p.AmountCents, p.Currency),
				"Submitted":        now,
			},
			GeneratedAt: now,
		}

	case notification.EventAdminWithdrawalStale:
		var p notification.AdminWithdrawalPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[URGENT] Withdrawal request overdue — no admin action in 24+ hours",
			Headline:  "Withdrawal request has been waiting too long",
			Body:      "A withdrawal request has exceeded the stale threshold without admin action. Immediate review is required.",
			Details: map[string]string{
				detailKeyRequestID: fmt.Sprintf("%d", p.RequestID),
				detailKeyUserID:    fmt.Sprintf("%d", p.UserID),
				"Amount":           formatCents(p.AmountCents, p.Currency),
				"Pending Since":    p.PendingSince,
			},
			GeneratedAt: now,
		}

	case notification.EventAdminHighValueWithdrawal:
		var p notification.AdminWithdrawalPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[CRITICAL] High-value withdrawal request requires immediate review",
			Headline:  "High-value withdrawal detected",
			Body:      "A withdrawal request above the high-value threshold has been submitted. This event requires heightened scrutiny before approval.",
			Details: map[string]string{
				detailKeyRequestID: fmt.Sprintf("%d", p.RequestID),
				detailKeyUserID:    fmt.Sprintf("%d", p.UserID),
				"Amount":           formatCents(p.AmountCents, p.Currency),
			},
			GeneratedAt: now,
		}

	// ── System Alerts ─────────────────────────────────────────────────────────

	case notification.EventSystemCircuitBreakerOpened:
		var p notification.SystemAlertPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[CRITICAL] Circuit breaker opened — " + p.Component,
			Headline:  "Circuit breaker is OPEN",
			Body:      "A circuit breaker has opened, indicating repeated failures in a system component. Downstream operations are degraded.",
			Details: map[string]string{
				"Component": p.Component,
				"Detail":    p.Detail,
				"Severity":  p.Severity,
			},
			GeneratedAt: now,
		}

	case notification.EventSystemBalanceLedgerMismatch:
		var p notification.SystemAlertPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[CRITICAL] Balance ledger mismatch detected — financial integrity at risk",
			Headline:  "Balance ledger integrity check failed",
			Body:      "The balance ledger consistency check has detected a mismatch. This is a P0 financial integrity issue requiring immediate investigation.",
			Details: map[string]string{
				"Component": p.Component,
				"Detail":    p.Detail,
				"Severity":  p.Severity,
			},
			GeneratedAt: now,
		}

	case notification.EventSystemWebhookSignatureFailed:
		var p notification.SystemAlertPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[SECURITY] Webhook signature verification failed",
			Headline:  "Invalid webhook signature detected",
			Body:      "An incoming webhook failed signature verification. This could indicate a replay attack or misconfiguration.",
			Details: map[string]string{
				"Component": p.Component,
				"Detail":    p.Detail,
			},
			GeneratedAt: now,
		}

	case notification.EventSystemWebhookSignatureRepeated:
		var p notification.SystemAlertPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[SECURITY] Repeated webhook signature failures",
			Headline:  "Multiple webhook signature failures in succession",
			Body:      "Repeated webhook signature verification failures suggest a potential attack or a broken provider configuration.",
			Details: map[string]string{
				"Component": p.Component,
				"Detail":    p.Detail,
			},
			GeneratedAt: now,
		}

	case notification.EventAdminPaymentDispute:
		var p notification.SystemAlertPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[ACTION REQUIRED] Payment dispute reported",
			Headline:  "A payment dispute has been raised",
			Body:      "A user or payment provider has raised a dispute on a payment. Please review and respond within the dispute window.",
			Details: map[string]string{
				"Component": p.Component,
				"Detail":    p.Detail,
			},
			GeneratedAt: now,
		}

	// ── Scheduler digest events ───────────────────────────────────────────────

	case notification.EventAdminPendingReminder:
		var p notification.AdminPendingReminderPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   "[ACTION REQUIRED] Pending items awaiting admin review",
			Headline:  "Periodic pending-items reminder",
			Body:      fmt.Sprintf("%d bank transfer proof(s) and %d withdrawal request(s) are awaiting your review.", p.PendingTransfers, p.PendingWithdrawals),
			Details: func() map[string]string {
				d := map[string]string{
					"Pending Transfers":   fmt.Sprintf("%d", p.PendingTransfers),
					"Pending Withdrawals": fmt.Sprintf("%d", p.PendingWithdrawals),
				}
				if p.OldestPendingSince != "" {
					d["Oldest Pending Since"] = p.OldestPendingSince
				}
				return d
			}(),
			GeneratedAt: now,
		}

	case notification.EventAdminDailySummary:
		var p notification.AdminDailySummaryPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   fmt.Sprintf("[DAILY SUMMARY] Operations summary for %s", p.Date),
			Headline:  "Daily operations summary",
			Body:      fmt.Sprintf("Here is the operations summary for %s.", p.Date),
			Details: map[string]string{
				"Date":                p.Date,
				"New Users":           fmt.Sprintf("%d", p.NewUsers),
				"New Transfers":       fmt.Sprintf("%d", p.NewTransfers),
				"Approved Transfers":  fmt.Sprintf("%d", p.ApprovedTransfers),
				"Total Credited":      formatCents(p.TotalCreditedCents, "GTQ"),
				"New Withdrawals":     fmt.Sprintf("%d", p.NewWithdrawals),
				"Pending Transfers":   fmt.Sprintf("%d", p.PendingTransfers),
				"Pending Withdrawals": fmt.Sprintf("%d", p.PendingWithdrawals),
			},
			GeneratedAt: now,
		}

	case notification.EventAdminWeeklyReport:
		var p notification.AdminWeeklyReportPayload
		_ = entry.DecodePayload(&p)
		return emailData{
			EventType: et,
			Subject:   fmt.Sprintf("[WEEKLY REPORT] %s – %s", p.WeekStartDate, p.WeekEndDate),
			Headline:  "Weekly operations report",
			Body:      fmt.Sprintf("Weekly summary for the period %s to %s.", p.WeekStartDate, p.WeekEndDate),
			Details: func() map[string]string {
				d := map[string]string{
					"Period":            fmt.Sprintf("%s – %s", p.WeekStartDate, p.WeekEndDate),
					"Total Revenue":     formatCents(p.TotalRevenueCents, "GTQ"),
					"New Users":         fmt.Sprintf("%d", p.NewUsers),
					"Active Quinielas":  fmt.Sprintf("%d", p.ActiveQuinielas),
					"Total Withdrawals": fmt.Sprintf("%d", p.TotalWithdrawals),
					"Withdrawal Amount": formatCents(p.WithdrawalCents, "GTQ"),
				}
				if p.TopGroupName != "" {
					d["Top Group"] = fmt.Sprintf("%s (%d pts)", p.TopGroupName, p.TopGroupPoints)
				}
				return d
			}(),
			GeneratedAt: now,
		}

	// ── Generic fallback ──────────────────────────────────────────────────────

	default:
		return emailData{
			EventType:   et,
			Subject:     "[ADMIN ALERT] " + et,
			Headline:    "Admin notification: " + et,
			Body:        "An admin-level event has been raised. Please review the details below.",
			Details:     map[string]string{"Event Type": et, "Aggregate ID": entry.AggregateID},
			GeneratedAt: now,
		}
	}
}

// formatCents converts an integer cent value to a human-readable amount string.
func formatCents(cents int, currency string) string {
	if cents == 0 {
		return fmt.Sprintf("0.00 %s", currency)
	}
	return fmt.Sprintf("%.2f %s", float64(cents)/100.0, currency)
}
