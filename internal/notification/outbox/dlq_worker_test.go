package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
)

// ── Test doubles ─────────────────────────────────────────────────────────────

type stubDLQRepo struct {
	batch      []*domain.NotificationDLQEntry
	claimErr   error
	countTotal int64
	countErr   error
	resolved   []int64
	resolveErr error
	failures   []int64
	failureErr error
}

func (r *stubDLQRepo) ClaimBatch(_ context.Context, _, _ int) ([]*domain.NotificationDLQEntry, error) {
	return r.batch, r.claimErr
}
func (r *stubDLQRepo) CountUnresolved(_ context.Context) (int64, error) {
	return r.countTotal, r.countErr
}
func (r *stubDLQRepo) MarkResolved(_ context.Context, id int64) error {
	if r.resolveErr != nil {
		return r.resolveErr
	}
	r.resolved = append(r.resolved, id)
	return nil
}
func (r *stubDLQRepo) RecordFailure(_ context.Context, id int64, _ string) error {
	if r.failureErr != nil {
		return r.failureErr
	}
	r.failures = append(r.failures, id)
	return nil
}

type stubDLQWriter struct {
	written  []notification.EventType
	writeErr error
}

func (w *stubDLQWriter) Write(_ context.Context, et notification.EventType, _, _ string, _ any) error {
	if w.writeErr != nil {
		return w.writeErr
	}
	w.written = append(w.written, et)
	return nil
}

// makeEntry creates a minimal DLQ entry for testing.
func makeEntry(id int64, eventType string) *domain.NotificationDLQEntry {
	return &domain.NotificationDLQEntry{
		ID:        id,
		EventType: eventType,
		Channel:   "email",
		Payload:   []byte(`{"user_id":1}`),
		CreatedAt: time.Now().Add(-time.Hour),
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestDLQWorker_Process_SuccessfulReplay_MarksResolved(t *testing.T) {
	t.Parallel()

	entry := makeEntry(42, string(notification.EventAdminBankTransferStale))
	repo := &stubDLQRepo{batch: []*domain.NotificationDLQEntry{entry}}
	writer := &stubDLQWriter{}

	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(time.Hour), // prevent auto-tick
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // run one poll cycle via Run; the ticker won't fire so we call poll directly

	// Exercise through a single poll cycle using a cancelable context.
	// We need to trigger poll indirectly — use a very short poll interval and
	// short-lived context so exactly one tick fires.
	repo2 := &stubDLQRepo{batch: []*domain.NotificationDLQEntry{entry}}
	writer2 := &stubDLQWriter{}
	w2 := outbox.NewDLQWorker(repo2, writer2, zap.NewNop(),
		outbox.WithDLQPollInterval(10*time.Millisecond),
	)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	w2.Run(ctx2)

	if len(writer2.written) == 0 {
		t.Fatal("expected at least one write to outbox")
	}
	if len(repo2.resolved) == 0 {
		t.Fatal("expected entry to be marked resolved")
	}
	if repo2.resolved[0] != 42 {
		t.Errorf("resolved id: got %d; want 42", repo2.resolved[0])
	}
	_ = w
	_ = ctx
}

func TestDLQWorker_Process_WriteError_RecordsFailure(t *testing.T) {
	t.Parallel()

	entry := makeEntry(99, string(notification.EventAdminWithdrawalStale))
	repo := &stubDLQRepo{batch: []*domain.NotificationDLQEntry{entry}}
	writer := &stubDLQWriter{writeErr: errors.New("smtp timeout")}

	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(10*time.Millisecond),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	if len(repo.failures) == 0 {
		t.Fatal("expected failure to be recorded")
	}
	if repo.failures[0] != 99 {
		t.Errorf("failure id: got %d; want 99", repo.failures[0])
	}
	if len(repo.resolved) != 0 {
		t.Errorf("resolved: got %d; want 0", len(repo.resolved))
	}
}

func TestDLQWorker_ClaimError_DoesNotPanic(t *testing.T) {
	t.Parallel()

	repo := &stubDLQRepo{claimErr: errors.New("db down")}
	writer := &stubDLQWriter{}

	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(10*time.Millisecond),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx) // must not panic

	if len(writer.written) != 0 {
		t.Errorf("written: got %d; want 0", len(writer.written))
	}
}

func TestDLQWorker_AboveAlertThreshold_DoesNotPanic(t *testing.T) {
	t.Parallel()

	// countTotal > alertThreshold should log an error but not stop the worker.
	repo := &stubDLQRepo{countTotal: 200}
	writer := &stubDLQWriter{}

	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(10*time.Millisecond),
		outbox.WithDLQAlertThreshold(50),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx) // must not panic
}

func TestDLQWorker_AboveWarningThreshold_DoesNotPanic(t *testing.T) {
	t.Parallel()

	// countTotal (15) above warning threshold (10) but below alert threshold (50)
	// — exercises the Warn-level log branch added by WithDLQWarningThreshold.
	repo := &stubDLQRepo{countTotal: 15}
	writer := &stubDLQWriter{}

	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(10*time.Millisecond),
		outbox.WithDLQWarningThreshold(10),
		outbox.WithDLQAlertThreshold(50),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx) // must not panic
}

func TestDLQWorker_BelowWarningThreshold_NeitherBranchFires(t *testing.T) {
	t.Parallel()

	// countTotal (5) is below both thresholds — clean path through poll().
	repo := &stubDLQRepo{countTotal: 5}
	writer := &stubDLQWriter{}

	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(10*time.Millisecond),
		outbox.WithDLQWarningThreshold(10),
		outbox.WithDLQAlertThreshold(50),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx) // must not panic
}

func TestDLQWorker_Run_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	repo := &stubDLQRepo{}
	writer := &stubDLQWriter{}
	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(time.Hour),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DLQWorker.Run did not stop after context cancellation")
	}
}

// TestDLQWorker_Options_Applied verifies that WithDLQBatchSize and
// WithDLQMaxAttempts are accepted by NewDLQWorker without panicking.
func TestDLQWorker_Options_Applied(t *testing.T) {
	t.Parallel()

	repo := &stubDLQRepo{}
	writer := &stubDLQWriter{}
	// Exercise both option closures; fields are unexported so we can only
	// verify that construction succeeds and the worker runs without error.
	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQBatchSize(7),
		outbox.WithDLQMaxAttempts(3),
		outbox.WithDLQPollInterval(time.Hour),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // stop immediately; we just want construction + Run init path
	w.Run(ctx)
}

// TestDLQWorker_Process_MarkResolvedError_DoesNotResolve verifies that when
// MarkResolved returns an error after a successful write, the entry is not
// counted as resolved and the worker continues.
func TestDLQWorker_Process_MarkResolvedError_DoesNotResolve(t *testing.T) {
	t.Parallel()

	entry := makeEntry(77, string(notification.EventAdminBankTransferStale))
	repo := &stubDLQRepo{
		batch:      []*domain.NotificationDLQEntry{entry},
		resolveErr: errors.New("db constraint"),
	}
	writer := &stubDLQWriter{}

	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(10*time.Millisecond),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	// Write reached outbox but resolved list must be empty due to the error.
	if len(writer.written) == 0 {
		t.Fatal("expected entry to be written to outbox")
	}
	if len(repo.resolved) != 0 {
		t.Errorf("resolved: got %d; want 0 (MarkResolved failed)", len(repo.resolved))
	}
}

// TestDLQWorker_Process_RecordFailureError_DoesNotPanic verifies that when
// both the outbox write and the subsequent RecordFailure call fail, the worker
// logs the error and continues without panicking.
func TestDLQWorker_Process_RecordFailureError_DoesNotPanic(t *testing.T) {
	t.Parallel()

	entry := makeEntry(88, string(notification.EventAdminWithdrawalStale))
	repo := &stubDLQRepo{
		batch:      []*domain.NotificationDLQEntry{entry},
		failureErr: errors.New("record failure db error"),
	}
	writer := &stubDLQWriter{writeErr: errors.New("smtp down")}

	w := outbox.NewDLQWorker(repo, writer, zap.NewNop(),
		outbox.WithDLQPollInterval(10*time.Millisecond),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx) // must not panic
}
