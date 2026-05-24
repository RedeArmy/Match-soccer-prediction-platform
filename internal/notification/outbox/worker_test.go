package outbox_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
)

// -- stub dispatcher --------------------------------------------------------

type stubDispatcher struct {
	calls   atomic.Int64
	failFor map[string]int // eventType → remaining failures
}

func (s *stubDispatcher) Dispatch(_ context.Context, entry *notification.OutboxEntry) error {
	s.calls.Add(1)
	if rem, ok := s.failFor[string(entry.EventType)]; ok && rem > 0 {
		s.failFor[string(entry.EventType)] = rem - 1
		return errStubFail
	}
	return nil
}

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const errStubFail sentinelError = "stub dispatch failure"

// truncateOutbox removes all rows from domain_outbox for test isolation.
// Worker tests share a single database, so each test must clean up the table
// it writes to avoid cross-test interference.
func truncateOutbox(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"TRUNCATE domain_outbox, notification_dlq RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate domain_outbox: %v", err)
	}
}

// ---------------------------------------------------------------------------

// TestWorker_PollAndDispatch covers the happy path: seed outbox rows, run the
// worker, verify they are marked done.
// Not parallel: shares the database table with other worker tests.
func TestWorker_PollAndDispatch(t *testing.T) {
	truncateOutbox(t)
	ctx := context.Background()

	log := zaptest.NewLogger(t)
	repo := outbox.NewPostgresRepository(testPool)
	dispatcher := &stubDispatcher{}

	w := outbox.NewWorker(repo, dispatcher, log,
		outbox.WithBatchSize(10),
		outbox.WithPollInterval(50*time.Millisecond),
		outbox.WithLockDuration(30*time.Second),
	)

	writer := outbox.NewWriter(testPool)
	for i := range 2 {
		if err := writer.Write(ctx,
			notification.EventPredictionScored,
			"prediction", fmt.Sprintf("worker_happy_%d", i),
			notification.PredictionScoredPayload{UserID: 1, MatchID: 1, PointsEarned: 3},
		); err != nil {
			t.Fatalf("seed write: %v", err)
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	w.Run(runCtx)

	var doneCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM domain_outbox WHERE status = 'done'`,
	).Scan(&doneCount); err != nil {
		t.Fatalf("count done: %v", err)
	}
	if doneCount < 2 {
		t.Errorf("expected 2 done entries; got %d", doneCount)
	}
	if dispatcher.calls.Load() < 2 {
		t.Errorf("expected ≥ 2 dispatcher calls; got %d", dispatcher.calls.Load())
	}
}

// TestWorker_RetryOnDispatchError verifies that a transient dispatch failure
// causes rescheduling and eventual success on the second attempt.
// Not parallel: shares the database table.
func TestWorker_RetryOnDispatchError(t *testing.T) {
	truncateOutbox(t)
	ctx := context.Background()

	log := zaptest.NewLogger(t)
	repo := outbox.NewPostgresRepository(testPool)
	dispatcher := &stubDispatcher{
		failFor: map[string]int{string(notification.EventPaymentFailed): 1},
	}

	// Use a very short poll interval so the retry happens quickly.
	w := outbox.NewWorker(repo, dispatcher, log,
		outbox.WithBatchSize(10),
		outbox.WithPollInterval(100*time.Millisecond),
		outbox.WithLockDuration(30*time.Second),
	)

	writer := outbox.NewWriter(testPool)
	if err := writer.Write(ctx,
		notification.EventPaymentFailed,
		"payment_record", "retry_1",
		notification.PaymentPayload{UserID: 2, PaymentID: 9},
	); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	// Force the scheduled_at to be in the past after the first reschedule by
	// running long enough for the backoff (≤ 1.5 s for attempt=1) to expire.
	runCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	w.Run(runCtx)

	var status string
	if err := testPool.QueryRow(ctx,
		`SELECT status FROM domain_outbox LIMIT 1`,
	).Scan(&status); err != nil {
		t.Fatalf("fetch status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected status 'done'; got %q", status)
	}
	if dispatcher.calls.Load() < 2 {
		t.Errorf("expected ≥ 2 dispatcher calls (1 fail + 1 success); got %d",
			dispatcher.calls.Load())
	}
}

// TestWorker_MarkFailedAfterMaxAttempts verifies that an entry is marked
// 'failed' once all retry attempts are exhausted.
// Not parallel: shares the database table.
func TestWorker_MarkFailedAfterMaxAttempts(t *testing.T) {
	truncateOutbox(t)
	ctx := context.Background()

	log := zaptest.NewLogger(t)
	repo := outbox.NewPostgresRepository(testPool)

	// Always fail — never succeeds.
	alwaysFail := &stubDispatcher{
		failFor: map[string]int{string(notification.EventPaymentFailed): 999},
	}

	w := outbox.NewWorker(repo, alwaysFail, log,
		outbox.WithBatchSize(10),
		outbox.WithPollInterval(20*time.Millisecond),
		outbox.WithLockDuration(30*time.Second),
	)

	writer := outbox.NewWriter(testPool)
	if err := writer.Write(ctx,
		notification.EventPaymentFailed,
		"payment_record", "exhaust_1",
		notification.PaymentPayload{UserID: 3, PaymentID: 10},
	); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	// Manually bump attempts to 4 so the next claim (which increments to 5)
	// triggers the max-attempts path immediately without waiting for backoffs.
	if _, err := testPool.Exec(ctx,
		"UPDATE domain_outbox SET attempts = 4",
	); err != nil {
		t.Fatalf("bump attempts: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	w.Run(runCtx)

	var status string
	if err := testPool.QueryRow(ctx,
		`SELECT status FROM domain_outbox LIMIT 1`,
	).Scan(&status); err != nil {
		t.Fatalf("fetch status: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status 'failed'; got %q", status)
	}
}

// TestRepository_MarkFailed directly exercises the repository MarkFailed path.
func TestRepository_MarkFailed(t *testing.T) {
	truncateOutbox(t)
	ctx := context.Background()

	writer := outbox.NewWriter(testPool)
	if err := writer.Write(ctx,
		notification.EventPaymentFailed,
		"payment_record", "markfailed_1",
		notification.PaymentPayload{UserID: 5, PaymentID: 20},
	); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	var id int64
	if err := testPool.QueryRow(ctx,
		`SELECT id FROM domain_outbox LIMIT 1`,
	).Scan(&id); err != nil {
		t.Fatalf("get id: %v", err)
	}

	repo := outbox.NewPostgresRepository(testPool)
	if err := repo.MarkFailed(ctx, id, "test failure reason"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	var status, detail string
	if err := testPool.QueryRow(ctx,
		`SELECT status, error_detail FROM domain_outbox WHERE id = $1`, id,
	).Scan(&status, &detail); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "failed" {
		t.Errorf("status: got %q; want 'failed'", status)
	}
	if detail != "test failure reason" {
		t.Errorf("error_detail: got %q; want 'test failure reason'", detail)
	}
}

// TestRepository_Schedule directly exercises the repository Schedule path.
func TestRepository_Schedule(t *testing.T) {
	truncateOutbox(t)
	ctx := context.Background()

	writer := outbox.NewWriter(testPool)
	if err := writer.Write(ctx,
		notification.EventAccountWelcome,
		"user", "schedule_1",
		notification.AccountWelcomePayload{UserID: 7, UserName: "tester"},
	); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	var id int64
	if err := testPool.QueryRow(ctx,
		`SELECT id FROM domain_outbox LIMIT 1`,
	).Scan(&id); err != nil {
		t.Fatalf("get id: %v", err)
	}

	repo := outbox.NewPostgresRepository(testPool)
	futureTime := time.Now().Add(1 * time.Hour)
	if err := repo.Schedule(ctx, id, futureTime); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	var status string
	var scheduledAt time.Time
	if err := testPool.QueryRow(ctx,
		`SELECT status, scheduled_at FROM domain_outbox WHERE id = $1`, id,
	).Scan(&status, &scheduledAt); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if status != "pending" {
		t.Errorf("status: got %q; want 'pending'", status)
	}
	if scheduledAt.Before(time.Now()) {
		t.Errorf("scheduled_at: %v should be in the future", scheduledAt)
	}
}

// TestWorker_NoopDispatcher verifies the exported NoopDispatcher satisfies
// the Dispatcher interface and never returns an error.
func TestWorker_NoopDispatcher(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	d := outbox.NewNoopDispatcher(log)

	entry := &notification.OutboxEntry{
		EventType:     notification.EventAccountWelcome,
		AggregateType: "user",
		AggregateID:   "1",
	}
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Errorf("NoopDispatcher.Dispatch: %v", err)
	}
}

// TestNewPostgresRepository verifies that NewPostgresRepository returns a
// non-nil Repository.
func TestNewPostgresRepository(t *testing.T) {
	t.Parallel()
	if outbox.NewPostgresRepository(testPool) == nil {
		t.Fatal("expected non-nil Repository")
	}
}

// TestWorker_WithMaxAttempts verifies that WithMaxAttempts(1) causes an entry
// to be marked failed on the very first dispatch failure (no retries).
// Not parallel: shares the database table.
func TestWorker_WithMaxAttempts(t *testing.T) {
	truncateOutbox(t)
	ctx := context.Background()

	log := zaptest.NewLogger(t)
	repo := outbox.NewPostgresRepository(testPool)
	alwaysFail := &stubDispatcher{
		failFor: map[string]int{string(notification.EventPaymentFailed): 999},
	}

	w := outbox.NewWorker(repo, alwaysFail, log,
		outbox.WithBatchSize(10),
		outbox.WithPollInterval(20*time.Millisecond),
		outbox.WithLockDuration(30*time.Second),
		outbox.WithMaxAttempts(1), // fail immediately on first error
	)

	writer := outbox.NewWriter(testPool)
	if err := writer.Write(ctx,
		notification.EventPaymentFailed,
		"payment_record", "max1_1",
		notification.PaymentPayload{UserID: 4, PaymentID: 11},
	); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	w.Run(runCtx)

	var status string
	if err := testPool.QueryRow(ctx,
		`SELECT status FROM domain_outbox LIMIT 1`,
	).Scan(&status); err != nil {
		t.Fatalf("fetch status: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status 'failed' after 1 attempt; got %q", status)
	}
}

// stubLagNotifier records NotifyOutboxLag calls for testing.
type stubLagNotifier struct {
	calls atomic.Int64
}

func (s *stubLagNotifier) NotifyOutboxLag(_ context.Context, _ float64, _ int64) {
	s.calls.Add(1)
}

// TestWorker_LagBelowThreshold_NoAlert verifies that NotifyOutboxLag is NOT
// called when the entry lag is below the configured threshold.
// Not parallel: shares the database table.
func TestWorker_LagBelowThreshold_NoAlert(t *testing.T) {
	truncateOutbox(t)
	ctx := context.Background()

	log := zaptest.NewLogger(t)
	repo := outbox.NewPostgresRepository(testPool)
	notifier := &stubLagNotifier{}
	dispatcher := &stubDispatcher{}

	// Threshold of 1 h: a freshly inserted entry will never exceed this.
	w := outbox.NewWorker(repo, dispatcher, log,
		outbox.WithBatchSize(10),
		outbox.WithPollInterval(50*time.Millisecond),
		outbox.WithLockDuration(30*time.Second),
		outbox.WithOutboxNotifier(notifier),
		outbox.WithOutboxLagThreshold(1*time.Hour),
	)

	writer := outbox.NewWriter(testPool)
	if err := writer.Write(ctx,
		notification.EventAccountWelcome,
		"user", "lag_below_1",
		notification.AccountWelcomePayload{UserID: 11, UserName: "tester2"},
	); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	w.Run(runCtx)

	if notifier.calls.Load() != 0 {
		t.Errorf("expected 0 NotifyOutboxLag calls below threshold; got %d", notifier.calls.Load())
	}
}

// TestWorker_WithOutboxNotifier_TriggersLagAlert verifies that WithOutboxNotifier
// wires the notifier and that WithOutboxLagThreshold(0) fires the alert on the
// next poll when there is a pending entry. It also seeds two entries with
// differing created_at values so checkLag's oldest-finding loop is exercised.
// Not parallel: shares the database table.
func TestWorker_WithOutboxNotifier_TriggersLagAlert(t *testing.T) {
	truncateOutbox(t)
	ctx := context.Background()

	log := zaptest.NewLogger(t)
	repo := outbox.NewPostgresRepository(testPool)
	notifier := &stubLagNotifier{}
	dispatcher := &stubDispatcher{}

	w := outbox.NewWorker(repo, dispatcher, log,
		outbox.WithBatchSize(10),
		outbox.WithPollInterval(50*time.Millisecond),
		outbox.WithLockDuration(30*time.Second),
		outbox.WithOutboxNotifier(notifier),
		outbox.WithOutboxLagThreshold(0), // any entry triggers the alert
	)

	writer := outbox.NewWriter(testPool)
	for _, id := range []string{"lag_alert_1", "lag_alert_2"} {
		if err := writer.Write(ctx,
			notification.EventAccountWelcome,
			"user", id,
			notification.AccountWelcomePayload{UserID: 10, UserName: "tester"},
		); err != nil {
			t.Fatalf("seed write %s: %v", id, err)
		}
	}
	// Make the second entry older so that when ClaimBatch returns rows in id order
	// (lag_alert_1 first, lag_alert_2 second), the inner oldest-check triggers
	// e.CreatedAt.Before(oldest.CreatedAt) = true and the assignment is covered.
	if _, err := testPool.Exec(ctx,
		"UPDATE domain_outbox SET created_at = created_at - interval '1 minute' WHERE aggregate_id = 'lag_alert_2'",
	); err != nil {
		t.Fatalf("backdate created_at: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	w.Run(runCtx)

	if notifier.calls.Load() == 0 {
		t.Error("expected NotifyOutboxLag to be called at least once; got 0 calls")
	}
}
