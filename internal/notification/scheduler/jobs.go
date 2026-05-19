package scheduler

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
)

// OutboxWriter is the write-side contract consumed by scheduler jobs.
// The production implementation is *outbox.Writer; tests may supply a stub.
type OutboxWriter interface {
	Write(ctx context.Context, eventType notification.EventType, aggregateType, aggregateID string, payload any) error
}

// SchedulerStore provides the aggregated query results that scheduler jobs need.
// Each method maps to a narrow, purpose-built repository query so the interface
// stays small and the production implementation can be incrementally built.
type SchedulerStore interface {
	// CountPendingTransfers returns the number of bank transfer proofs in pending state.
	CountPendingTransfers(ctx context.Context) (int, error)

	// CountPendingWithdrawals returns the number of withdrawal requests awaiting approval.
	CountPendingWithdrawals(ctx context.Context) (int, error)

	// OldestPendingTransferSince returns the created_at of the oldest pending proof,
	// or the zero value if none exist.
	OldestPendingTransferSince(ctx context.Context) (time.Time, error)

	// DailySummary returns aggregated operational metrics for the preceding 24 hours.
	DailySummary(ctx context.Context, since time.Time) (DailySummaryRow, error)

	// WeeklySummary returns aggregated metrics for the preceding 7 days.
	WeeklySummary(ctx context.Context, since time.Time) (WeeklySummaryRow, error)

	// ListFinishedMatchesMissingResult returns matches in "finished" status that
	// have no home_score/away_score recorded, ordered by kickoff_at ascending.
	ListFinishedMatchesMissingResult(ctx context.Context) ([]*domain.Match, error)

	// ListUpcomingMatchesWithDeadline returns scheduled matches whose kickoff_at
	// is within the next deadlineWindow, for which there are users with missing
	// predictions.  Each returned DeadlineMatch carries the list of user IDs.
	ListUpcomingMatchesWithDeadline(ctx context.Context, deadlineWindow time.Duration) ([]DeadlineMatch, error)
}

// DailySummaryRow carries the aggregated stats returned by SchedulerStore.DailySummary.
type DailySummaryRow struct {
	NewUsers           int
	NewTransfers       int
	ApprovedTransfers  int
	TotalCreditedCents int
	NewWithdrawals     int
	PendingTransfers   int
	PendingWithdrawals int
}

// WeeklySummaryRow carries the aggregated stats returned by SchedulerStore.WeeklySummary.
type WeeklySummaryRow struct {
	TotalRevenueCents int
	NewUsers          int
	ActiveQuinielas   int
	TopGroupName      string
	TopGroupPoints    int
	TotalWithdrawals  int
	WithdrawalCents   int
}

// DeadlineMatch pairs a match with the user IDs that have not yet submitted
// a prediction for it.
type DeadlineMatch struct {
	Match          *domain.Match
	MissingUserIDs []int
	MinutesLeft    int
}

// Jobs holds the dependencies shared across all scheduler job functions and
// returns pre-built job functions that can be registered with a Scheduler.
type Jobs struct {
	store  SchedulerStore
	writer OutboxWriter
	log    *zap.Logger
}

// NewJobs constructs a Jobs instance.
func NewJobs(store SchedulerStore, writer OutboxWriter, log *zap.Logger) *Jobs {
	return &Jobs{store: store, writer: writer, log: log}
}

// PredictionDeadlineApproaching queries upcoming matches and emits
// EventPredictionMissingReminder for every user that has not yet submitted a
// prediction.  Intended to run every 5 minutes.
func (j *Jobs) PredictionDeadlineApproaching(ctx context.Context) error {
	upcoming, err := j.store.ListUpcomingMatchesWithDeadline(ctx, 60*time.Minute)
	if err != nil {
		return fmt.Errorf("scheduler: prediction deadline: list upcoming: %w", err)
	}

	for _, dm := range upcoming {
		for _, uid := range dm.MissingUserIDs {
			payload := notification.PredictionDeadlinePayload{
				UserID:      uid,
				MatchID:     dm.Match.ID,
				HomeTeam:    dm.Match.HomeTeam,
				AwayTeam:    dm.Match.AwayTeam,
				DeadlineAt:  dm.Match.KickoffAt,
				MinutesLeft: dm.MinutesLeft,
			}
			if err := j.writer.Write(ctx,
				notification.EventPredictionMissingReminder,
				"match",
				fmt.Sprintf("%d", dm.Match.ID),
				payload,
			); err != nil {
				j.log.Warn("scheduler: prediction deadline: write outbox",
					zap.Int("match_id", dm.Match.ID),
					zap.Int("user_id", uid),
					zap.Error(err),
				)
			}
		}
	}
	return nil
}

// AdminPendingReminder counts pending bank transfers and withdrawals and emits
// EventAdminPendingReminder when either count is non-zero.  Intended to run
// every 4 hours.
func (j *Jobs) AdminPendingReminder(ctx context.Context) error {
	transfers, err := j.store.CountPendingTransfers(ctx)
	if err != nil {
		return fmt.Errorf("scheduler: pending reminder: count transfers: %w", err)
	}
	withdrawals, err := j.store.CountPendingWithdrawals(ctx)
	if err != nil {
		return fmt.Errorf("scheduler: pending reminder: count withdrawals: %w", err)
	}
	if transfers == 0 && withdrawals == 0 {
		return nil
	}

	var oldestStr string
	oldest, err := j.store.OldestPendingTransferSince(ctx)
	if err != nil {
		j.log.Warn("scheduler: pending reminder: oldest pending", zap.Error(err))
	} else if !oldest.IsZero() {
		oldestStr = oldest.UTC().Format(time.RFC3339)
	}

	payload := notification.AdminPendingReminderPayload{
		PendingTransfers:   transfers,
		PendingWithdrawals: withdrawals,
		OldestPendingSince: oldestStr,
	}
	if err := j.writer.Write(ctx,
		notification.EventAdminPendingReminder,
		"scheduler",
		"pending_reminder",
		payload,
	); err != nil {
		return fmt.Errorf("scheduler: pending reminder: write outbox: %w", err)
	}
	return nil
}

// AdminDailySummary collects 24-hour operational metrics and emits
// EventAdminDailySummary.  Intended to run once a day at 08:00 local time.
func (j *Jobs) AdminDailySummary(ctx context.Context) error {
	since := time.Now().UTC().Truncate(24 * time.Hour)
	row, err := j.store.DailySummary(ctx, since)
	if err != nil {
		return fmt.Errorf("scheduler: daily summary: query: %w", err)
	}

	payload := notification.AdminDailySummaryPayload{
		Date:               since.Format("2006-01-02"),
		NewUsers:           row.NewUsers,
		NewTransfers:       row.NewTransfers,
		ApprovedTransfers:  row.ApprovedTransfers,
		TotalCreditedCents: row.TotalCreditedCents,
		NewWithdrawals:     row.NewWithdrawals,
		PendingTransfers:   row.PendingTransfers,
		PendingWithdrawals: row.PendingWithdrawals,
	}
	if err := j.writer.Write(ctx,
		notification.EventAdminDailySummary,
		"scheduler",
		since.Format("2006-01-02"),
		payload,
	); err != nil {
		return fmt.Errorf("scheduler: daily summary: write outbox: %w", err)
	}
	return nil
}

// AdminWeeklyReport collects 7-day metrics and emits EventAdminWeeklyReport.
// Intended to run once per week on Monday at 08:00 local time.
func (j *Jobs) AdminWeeklyReport(ctx context.Context) error {
	now := time.Now().UTC()
	since := now.AddDate(0, 0, -7).Truncate(24 * time.Hour)

	row, err := j.store.WeeklySummary(ctx, since)
	if err != nil {
		return fmt.Errorf("scheduler: weekly report: query: %w", err)
	}

	payload := notification.AdminWeeklyReportPayload{
		WeekStartDate:     since.Format("2006-01-02"),
		WeekEndDate:       now.Truncate(24 * time.Hour).Format("2006-01-02"),
		TotalRevenueCents: row.TotalRevenueCents,
		NewUsers:          row.NewUsers,
		ActiveQuinielas:   row.ActiveQuinielas,
		TopGroupName:      row.TopGroupName,
		TopGroupPoints:    row.TopGroupPoints,
		TotalWithdrawals:  row.TotalWithdrawals,
		WithdrawalCents:   row.WithdrawalCents,
	}
	if err := j.writer.Write(ctx,
		notification.EventAdminWeeklyReport,
		"scheduler",
		since.Format("2006-01-02"),
		payload,
	); err != nil {
		return fmt.Errorf("scheduler: weekly report: write outbox: %w", err)
	}
	return nil
}

// AdminMatchResultPending finds finished matches that have no score recorded
// and emits EventAdminMatchResultPending for each one.  Intended to run every
// 15 minutes.
func (j *Jobs) AdminMatchResultPending(ctx context.Context) error {
	matches, err := j.store.ListFinishedMatchesMissingResult(ctx)
	if err != nil {
		return fmt.Errorf("scheduler: match result pending: query: %w", err)
	}

	now := time.Now().UTC()
	for _, m := range matches {
		elapsed := int(now.Sub(m.KickoffAt).Minutes())
		payload := notification.AdminMatchResultPayload{
			MatchID:        m.ID,
			HomeTeam:       m.HomeTeam,
			AwayTeam:       m.AwayTeam,
			FinishedAt:     m.KickoffAt,
			MinutesElapsed: elapsed,
		}
		if err := j.writer.Write(ctx,
			notification.EventAdminMatchResultPending,
			"match",
			fmt.Sprintf("%d", m.ID),
			payload,
		); err != nil {
			j.log.Warn("scheduler: match result pending: write outbox",
				zap.Int("match_id", m.ID),
				zap.Error(err),
			)
		}
	}
	return nil
}
