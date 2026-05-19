package outbox_test

import (
	"context"
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
	for range 2 {
		if err := writer.Write(ctx,
			notification.EventPredictionScored,
			"prediction", "worker_happy",
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
