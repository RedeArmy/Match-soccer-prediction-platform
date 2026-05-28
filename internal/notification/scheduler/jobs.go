package scheduler

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/escalation"
)

// dateOnlyLayout is the Go reference time for YYYY-MM-DD date strings.
const dateOnlyLayout = "2006-01-02"

// ParamReader is the subset of SystemParamService consumed by Jobs.
type ParamReader interface {
	GetInt(ctx context.Context, key string, defaultVal int) int
}

// OutboxWriter is the write-side contract consumed by scheduler jobs.
// The production implementation is *outbox.PoolWriter; tests may supply a stub.
type OutboxWriter interface {
	Write(ctx context.Context, eventType notification.EventType, aggregateType, aggregateID string, payload any) error
	// WriteDedup inserts an outbox row only when no row with the same dedupKey
	// already exists.  written=false (nil error) means the insert was skipped.
	WriteDedup(ctx context.Context, dedupKey string, eventType notification.EventType, aggregateType, aggregateID string, payload any) (written bool, err error)
}

// bucketToleranceMin is the half-width (in minutes) of the bucket window
// around each configured lead time.  A match with MinutesLeft in
// [leadMin - bucketToleranceMin, leadMin + bucketToleranceMin] is considered
// to be in that bucket.  The value matches the scheduler's firing interval so
// a single tick always covers exactly one reminder opportunity.
const bucketToleranceMin = 5

// Store provides the aggregated query results that scheduler jobs need.
// Each method maps to a narrow, purpose-built repository query so the interface
// stays small and the production implementation can be incrementally built.
type Store interface {
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

	// ListStaleBankTransfers returns pending bank transfer proofs whose created_at
	// is strictly before the given cutoff, ordered by created_at ascending.
	ListStaleBankTransfers(ctx context.Context, before time.Time) ([]*domain.BankTransferProof, error)

	// ListStaleWithdrawals returns pending withdrawal requests whose created_at
	// is strictly before the given cutoff, ordered by created_at ascending.
	ListStaleWithdrawals(ctx context.Context, before time.Time) ([]*domain.WithdrawalRequest, error)
}

// DailySummaryRow carries the aggregated stats returned by Store.DailySummary.
type DailySummaryRow struct {
	NewUsers           int
	NewTransfers       int
	ApprovedTransfers  int
	TotalCreditedCents int
	NewWithdrawals     int
	PendingTransfers   int
	PendingWithdrawals int
}

// WeeklySummaryRow carries the aggregated stats returned by Store.WeeklySummary.
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

// JobsConfig bundles the dependencies for NewJobs.
type JobsConfig struct {
	Store  Store
	Writer OutboxWriter
	// Params is optional.  When nil, StaleEscalation is a no-op.
	Params ParamReader
	// Clock is optional.  When nil, defaults to RealClock (time.Now).
	Clock Nower
	Log   *zap.Logger
}

// Jobs holds the dependencies shared across all scheduler job functions and
// returns pre-built job functions that can be registered with a Scheduler.
type Jobs struct {
	store  Store
	writer OutboxWriter
	params ParamReader
	clock  Nower
	log    *zap.Logger
}

// NewJobs constructs a Jobs instance.  Clock defaults to RealClock when nil.
func NewJobs(cfg JobsConfig) *Jobs {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	return &Jobs{
		store:  cfg.Store,
		writer: cfg.Writer,
		params: cfg.Params,
		clock:  cfg.Clock,
		log:    cfg.Log,
	}
}

// getInt reads an integer param, returning def when params is nil.
func (j *Jobs) getInt(ctx context.Context, key string, def int) int {
	if j.params == nil {
		return def
	}
	return j.params.GetInt(ctx, key, def)
}

// inBucket reports whether minutesLeft falls within [leadMin±bucketToleranceMin].
func inBucket(minutesLeft, leadMin int) bool {
	return minutesLeft >= leadMin-bucketToleranceMin && minutesLeft <= leadMin+bucketToleranceMin
}

// resolveDeadlineBucket returns the lead-time bucket (in minutes) that
// minutesLeft falls into, checking all provided lead times in order.
// Returns (0, false) when minutesLeft is not within tolerance of any lead time.
func resolveDeadlineBucket(minutesLeft int, leadMins ...int) (int, bool) {
	for _, lm := range leadMins {
		if inBucket(minutesLeft, lm) {
			return lm, true
		}
	}
	return 0, false
}

// PredictionDeadlineApproaching queries upcoming matches and emits
// EventPredictionMissingReminder for users with no prediction, but only when
// the match is within one of three configured lead-time buckets.
//
// Three buckets gate emission so users receive at most three reminders per match:
//   - bucket 0: notify.prediction_missing_lead_min  (default 120 min — early warning)
//   - bucket 1: notify.prediction_deadline_lead_min_1 (default 60 min)
//   - bucket 2: notify.prediction_deadline_lead_min_2 (default 15 min)
//
// A dedup_key per (match, user, bucket) prevents duplicate outbox rows when
// the scheduler fires multiple times within the same bucket window.
//
// Intended to run every 5 minutes (bucketToleranceMin = 5).
func (j *Jobs) PredictionDeadlineApproaching(ctx context.Context) error {
	missingLeadMin := j.getInt(ctx, domain.ParamKeyNotifyPredictionMissingLeadMin, domain.DefaultNotifyPredictionMissingLeadMin)
	leadMin1 := j.getInt(ctx, domain.ParamKeyNotifyPredictionDeadlineLeadMin1, domain.DefaultNotifyPredictionDeadlineLeadMin1)
	leadMin2 := j.getInt(ctx, domain.ParamKeyNotifyPredictionDeadlineLeadMin2, domain.DefaultNotifyPredictionDeadlineLeadMin2)

	maxLead := missingLeadMin
	if leadMin1 > maxLead {
		maxLead = leadMin1
	}
	if leadMin2 > maxLead {
		maxLead = leadMin2
	}
	window := time.Duration(maxLead+bucketToleranceMin) * time.Minute

	upcoming, err := j.store.ListUpcomingMatchesWithDeadline(ctx, window)
	if err != nil {
		return fmt.Errorf("scheduler: prediction deadline: list upcoming: %w", err)
	}

	for _, dm := range upcoming {
		bucketMin, ok := resolveDeadlineBucket(dm.MinutesLeft, missingLeadMin, leadMin1, leadMin2)
		if !ok {
			continue // match is in the window but not at an alert boundary
		}

		for _, uid := range dm.MissingUserIDs {
			dedupKey := fmt.Sprintf("prediction.missing_reminder:match:%d:user:%d:b%d",
				dm.Match.ID, uid, bucketMin)
			payload := notification.PredictionDeadlinePayload{
				UserID:      uid,
				MatchID:     dm.Match.ID,
				HomeTeam:    dm.Match.HomeTeam,
				AwayTeam:    dm.Match.AwayTeam,
				DeadlineAt:  dm.Match.KickoffAt,
				MinutesLeft: dm.MinutesLeft,
			}
			written, err := j.writer.WriteDedup(ctx,
				dedupKey,
				notification.EventPredictionMissingReminder,
				"match",
				fmt.Sprintf("%d", dm.Match.ID),
				payload,
			)
			if err != nil {
				j.log.Warn("scheduler: prediction deadline: write outbox",
					zap.Int("match_id", dm.Match.ID),
					zap.Int("user_id", uid),
					zap.Error(err),
				)
			} else if !written {
				j.log.Debug("scheduler: prediction deadline: dedup skipped",
					zap.Int("match_id", dm.Match.ID),
					zap.Int("user_id", uid),
					zap.Int("bucket_min", bucketMin),
				)
			}
		}
	}
	return nil
}

// AdminPendingReminder counts pending bank transfers and withdrawals and emits
// EventAdminPendingReminder when either count is non-zero.
//
// Dedup: notify.pending_reminder_interval_sec (default 4 h) controls the
// minimum time between two successive alerts by acting as the WriteDedup
// window. Within the same interval window the event is emitted at most once,
// even if the scheduler tick fires more frequently.
//
// When the number of pending bank transfers reaches the configured
// notify.bank_transfer_queue_depth_threshold it also emits the P0 event
// EventAdminBankTransferQueueDepth so that operators are alerted to a
// growing backlog before it becomes critical.
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

	// Dedup key is keyed to the current interval window so that the reminder
	// fires at most once per notify.pending_reminder_interval_sec seconds.
	intervalSec := j.getInt(ctx,
		domain.ParamKeyNotifyPendingReminderIntervalSec,
		domain.DefaultNotifyPendingReminderIntervalSec,
	)
	windowStart := j.clock.Now().Unix() / int64(intervalSec)
	dedupKey := fmt.Sprintf("admin.pending_reminder:window:%d", windowStart)

	if _, err := j.writer.WriteDedup(ctx,
		dedupKey,
		notification.EventAdminPendingReminder,
		"scheduler",
		"pending_reminder",
		payload,
	); err != nil {
		return fmt.Errorf("scheduler: pending reminder: write outbox: %w", err)
	}

	// P0 queue-depth alert: emit when pending bank transfers exceed the threshold.
	threshold := j.getInt(ctx,
		domain.ParamKeyNotifyBankTransferQueueDepthThreshold,
		domain.DefaultNotifyBankTransferQueueDepthThreshold,
	)
	if transfers >= threshold {
		qdPayload := notification.AdminBankTransferPayload{QueueDepth: transfers}
		if err := j.writer.Write(ctx,
			notification.EventAdminBankTransferQueueDepth,
			"scheduler",
			"queue_depth",
			qdPayload,
		); err != nil {
			j.log.Warn("scheduler: pending reminder: write queue depth alert", zap.Error(err))
			// Best-effort: do not fail the whole job over the secondary alert.
		}
	}

	return nil
}

// AdminDailySummary collects 24-hour operational metrics and emits
// EventAdminDailySummary.  Intended to run once a day at 08:00 local time.
func (j *Jobs) AdminDailySummary(ctx context.Context) error {
	since := j.clock.Now().UTC().Truncate(24 * time.Hour)
	row, err := j.store.DailySummary(ctx, since)
	if err != nil {
		return fmt.Errorf("scheduler: daily summary: query: %w", err)
	}

	payload := notification.AdminDailySummaryPayload{
		Date:               since.Format(dateOnlyLayout),
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
		since.Format(dateOnlyLayout),
		payload,
	); err != nil {
		return fmt.Errorf("scheduler: daily summary: write outbox: %w", err)
	}
	return nil
}

// AdminWeeklyReport collects 7-day metrics and emits EventAdminWeeklyReport.
// Intended to run once per week on Monday at 08:00 local time.
func (j *Jobs) AdminWeeklyReport(ctx context.Context) error {
	now := j.clock.Now().UTC()
	since := now.AddDate(0, 0, -7).Truncate(24 * time.Hour)

	row, err := j.store.WeeklySummary(ctx, since)
	if err != nil {
		return fmt.Errorf("scheduler: weekly report: query: %w", err)
	}

	payload := notification.AdminWeeklyReportPayload{
		WeekStartDate:     since.Format(dateOnlyLayout),
		WeekEndDate:       now.Truncate(24 * time.Hour).Format(dateOnlyLayout),
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
		since.Format(dateOnlyLayout),
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

	now := j.clock.Now().UTC()
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

// StaleEscalation queries pending bank transfers and withdrawal requests older
// than their configured review thresholds and emits a stale alert for each one.
// Skips silently when Params is nil.  Intended to run every 30 minutes.
// Escalation logic is delegated to the escalation package for independent
// testability and extension.
func (j *Jobs) StaleEscalation(ctx context.Context) error {
	if j.params == nil {
		return nil
	}

	bankStaleSec := j.params.GetInt(ctx, domain.ParamKeyNotifyBankTransferStaleSec, domain.DefaultNotifyBankTransferStaleSec)
	withdrawStaleSec := j.params.GetInt(ctx, domain.ParamKeyNotifyWithdrawalStaleSec, domain.DefaultNotifyWithdrawalStaleSec)
	now := j.clock.Now()

	esc := escalation.NewStaleOps(j.store, j.writer, escalation.Config{
		BankTransferStale: time.Duration(bankStaleSec) * time.Second,
		WithdrawalStale:   time.Duration(withdrawStaleSec) * time.Second,
		Now:               func() time.Time { return now },
	}, j.log)

	if err := esc.Run(ctx); err != nil {
		return fmt.Errorf("scheduler: stale escalation: %w", err)
	}
	return nil
}
