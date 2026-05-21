package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
)

const (
	sampleQuinielaName = "Mi Quiniela"
	samplePendingSince = "2026-05-21T08:00:00Z"
)

// ValidateTemplate checks that a NotificationTemplate is safe to persist for
// the given event type.  It performs three checks in order:
//
//  1. event_type must exist in notification.KnownEventTypes.
//  2. Every template field renders without error against a realistic sample
//     payload for the event type.  renderTmplStrict uses missingkey=error, so
//     any variable that does not exist in the payload (e.g. a typo like
//     {{.hometeam}} instead of {{.home_team}}) causes an immediate failure.
//  3. If action_url_tmpl is non-empty, the rendered path must start with "/".
func ValidateTemplate(eventType string, t *domain.NotificationTemplate) error {
	et := notification.EventType(eventType)
	if _, ok := notification.KnownEventTypes[et]; !ok {
		return fmt.Errorf("unknown event_type %q: not in the notification event catalogue", eventType)
	}

	sample := samplePayloadFor(et)
	var data map[string]any
	if err := json.Unmarshal(sample, &data); err != nil {
		return fmt.Errorf("dispatcher: validate: internal sample payload error: %w", err)
	}

	if _, err := renderTmplStrict(t.TitleTmpl, data); err != nil {
		return fmt.Errorf("title_tmpl: %w", err)
	}
	if _, err := renderTmplStrict(t.BodyTmpl, data); err != nil {
		return fmt.Errorf("body_tmpl: %w", err)
	}
	if t.ActionURLTmpl != "" {
		actionURL, err := renderTmplStrict(t.ActionURLTmpl, data)
		if err != nil {
			return fmt.Errorf("action_url_tmpl: %w", err)
		}
		if len(actionURL) > 0 && actionURL[0] != '/' {
			return fmt.Errorf("action_url_tmpl must render to a relative path starting with '/'; got %q", actionURL)
		}
	}
	if t.EmailSubjectTmpl != "" {
		if _, err := renderTmplStrict(t.EmailSubjectTmpl, data); err != nil {
			return fmt.Errorf("email_subject_tmpl: %w", err)
		}
	}
	return nil
}

// renderTmplStrict parses and executes a template string with missingkey=error.
// Any reference to a map key that is absent in data returns an error, making
// this function suitable for template validation (not production rendering).
func renderTmplStrict(tmplStr string, data map[string]any) (string, error) {
	t, err := template.New("").Funcs(notifTemplateFuncs).Option("missingkey=error").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("dispatcher: parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("dispatcher: execute template: %w", err)
	}
	return buf.String(), nil
}

// samplePayloadFor returns a realistic JSON payload for template validation.
// The payload is marshalled from the typed struct that production code writes
// into the outbox, so the JSON keys match what operators must use in templates.
// Admin and system events return a generic payload; user events return a
// struct with all fields populated so missingkey=error catches variable typos.
func samplePayloadFor(et notification.EventType) json.RawMessage {
	adminID := 99
	homeScore, awayScore := 2, 1

	var v any
	switch et {

	// ── Prediction ───────────────────────────────────────────────────────────

	case notification.EventPredictionConfirmed:
		v = notification.PredictionConfirmedPayload{
			UserID: 1, PredictionID: 10, MatchID: 5,
			HomeTeam: "Guatemala", AwayTeam: "Mexico",
		}

	case notification.EventPredictionDeadlineApproach, notification.EventPredictionMissingReminder:
		v = notification.PredictionDeadlinePayload{
			UserID: 1, MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			DeadlineAt:  time.Date(2026, 6, 15, 20, 30, 0, 0, time.UTC),
			MinutesLeft: 30,
		}

	case notification.EventPredictionLocked:
		v = notification.PredictionLockedPayload{
			UserID: 1, MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
		}

	case notification.EventPredictionScored:
		v = notification.PredictionScoredPayload{
			UserID: 1, PredictionID: 10, MatchID: 5,
			HomeTeam: "Guatemala", AwayTeam: "Mexico",
			HomeScore: 2, AwayScore: 1, PointsEarned: 3,
		}

	// ── Match ────────────────────────────────────────────────────────────────

	case notification.EventMatchResultEntered, notification.EventMatchPostponed, notification.EventMatchCancelled:
		v = notification.MatchEventPayload{
			MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			HomeScore: &homeScore, AwayScore: &awayScore,
		}

	// ── Group ────────────────────────────────────────────────────────────────

	case notification.EventGroupJoinRequested, notification.EventGroupJoinApproved,
		notification.EventGroupJoinRejected, notification.EventGroupMemberJoined, notification.EventGroupMemberLeft:
		v = notification.GroupJoinPayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, MembershipID: 7,
			UserID: 1, OwnerID: 2,
		}

	case notification.EventGroupLeaderboardMilestone:
		v = notification.GroupLeaderboardMilestonePayload{
			UserID: 1, QuinielaID: 3, QuinielaName: sampleQuinielaName,
			NewRank: 2, TotalPoints: 15,
		}

	case notification.EventGroupDisbanded:
		v = notification.GroupDisbandedPayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, OwnerID: 2,
		}

	case notification.EventGroupDeadline24h:
		v = notification.GroupDeadlinePayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName,
			DeadlineAt: time.Date(2026, 6, 15, 20, 30, 0, 0, time.UTC),
		}

	// ── Payment ──────────────────────────────────────────────────────────────

	case notification.EventPaymentConfirmed, notification.EventPaymentFailed, notification.EventPaymentPendingTimeout:
		v = notification.PaymentPayload{
			UserID: 1, PaymentID: 42, AmountCents: 12500,
			Currency: "GTQ", Reference: "REF-001", Reason: "Insufficient funds",
		}

	case notification.EventPaymentBankTransferSubmitted, notification.EventPaymentBankTransferApproved,
		notification.EventPaymentBankTransferRejected:
		v = notification.BankTransferPayload{
			UserID: 1, ProofID: 7, AmountCents: 12500,
			Currency: "GTQ", AdminID: &adminID, Notes: "Amount does not match",
		}

	// ── Withdrawal ───────────────────────────────────────────────────────────

	case notification.EventWithdrawalRequested, notification.EventWithdrawalApproved,
		notification.EventWithdrawalRejected, notification.EventWithdrawalProcessing,
		notification.EventWithdrawalCompleted, notification.EventWithdrawalFailed,
		notification.EventWithdrawalPendingTimeout:
		v = notification.WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: "Daily limit exceeded",
		}

	// ── Account ──────────────────────────────────────────────────────────────

	case notification.EventAccountWelcome:
		v = notification.AccountWelcomePayload{
			UserID: 1, UserName: "Juan García", Email: "juan@example.com",
		}

	case notification.EventAccountBalanceCredited, notification.EventAccountBalanceDebited,
		notification.EventAccountLowBalance:
		v = notification.AccountBalancePayload{
			UserID: 1, AmountCents: 5000, BalanceAfter: 15000, Currency: "GTQ",
		}

	// ── Admin ────────────────────────────────────────────────────────────────

	case notification.EventAdminBankTransferPending, notification.EventAdminBankTransferStale,
		notification.EventAdminBankTransferQueueDepth:
		v = notification.AdminBankTransferPayload{
			ProofID: 7, UserID: 1, AmountCents: 12500, Currency: "GTQ",
			QueueDepth: 5, PendingSince: samplePendingSince,
		}

	case notification.EventAdminWithdrawalPending, notification.EventAdminWithdrawalStale,
		notification.EventAdminHighValueWithdrawal:
		v = notification.AdminWithdrawalPayload{
			RequestID: 15, UserID: 1, AmountCents: 50000, Currency: "GTQ",
			PendingSince: samplePendingSince,
		}

	case notification.EventAdminMatchResultPending:
		v = notification.AdminMatchResultPayload{
			MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			FinishedAt:     time.Date(2026, 6, 15, 22, 0, 0, 0, time.UTC),
			MinutesElapsed: 90,
		}

	case notification.EventAdminPendingReminder:
		v = notification.AdminPendingReminderPayload{
			PendingTransfers: 3, PendingWithdrawals: 2,
			OldestPendingSince: samplePendingSince,
		}

	case notification.EventAdminDailySummary:
		v = notification.AdminDailySummaryPayload{
			Date: "2026-05-21", NewUsers: 5, NewTransfers: 10, ApprovedTransfers: 8,
			TotalCreditedCents: 125000, NewWithdrawals: 3,
			PendingTransfers: 2, PendingWithdrawals: 1,
		}

	case notification.EventAdminWeeklyReport:
		v = notification.AdminWeeklyReportPayload{
			WeekStartDate: "2026-05-15", WeekEndDate: "2026-05-21",
			TotalRevenueCents: 500000, NewUsers: 25, ActiveQuinielas: 12,
			TopGroupName: "Los Campeones", TopGroupPoints: 42,
			TotalWithdrawals: 8, WithdrawalCents: 200000,
		}

	// ── System / Admin alerts (shared payload shape) ─────────────────────────

	case notification.EventAdminPaymentDispute, notification.EventAdminScoringDiscrepancy,
		notification.EventAdminGroupReported,
		notification.EventSystemCircuitBreakerOpened, notification.EventSystemCircuitBreakerHalfOpen,
		notification.EventSystemWebhookSignatureFailed, notification.EventSystemWebhookSignatureRepeated,
		notification.EventSystemTxRetryExhausted, notification.EventSystemBalanceLedgerMismatch,
		notification.EventSystemRateLimitAbuse, notification.EventSystemIdempotencyCollision,
		notification.EventSystemFileStoreUnavailable:
		v = notification.SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}

	default:
		return json.RawMessage(`{}`)
	}

	b, _ := json.Marshal(v)
	return b
}
