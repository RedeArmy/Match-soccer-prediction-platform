// Package escalation implements admin alert escalation for stale financial
// operations. It is intentionally narrow: it knows about bank transfers and
// withdrawal requests, and nothing else. Scheduler jobs delegate to this
// package so the escalation logic is independently testable without standing
// up a full scheduler harness.
package escalation

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
)

// Store is the read interface that StaleOps requires. It is satisfied by the
// production scheduler.Store; tests may supply a lightweight stub.
type Store interface {
	// ListStaleBankTransfers returns pending bank transfer proofs whose
	// created_at is strictly before the given cutoff, ordered by created_at ASC.
	ListStaleBankTransfers(ctx context.Context, before time.Time) ([]*domain.BankTransferProof, error)
	// ListStaleWithdrawals returns pending withdrawal requests whose created_at
	// is strictly before the given cutoff, ordered by created_at ASC.
	ListStaleWithdrawals(ctx context.Context, before time.Time) ([]*domain.WithdrawalRequest, error)
}

// Writer is the outbox write interface required by StaleOps.
type Writer interface {
	Write(ctx context.Context, eventType notification.EventType, aggregateType, aggregateID string, payload any) error
}

// Config carries the threshold configuration for one escalation cycle.
// Both durations must be positive; zero values disable the respective check.
type Config struct {
	// BankTransferStale is how long a bank transfer proof may stay pending
	// before an admin escalation alert is emitted.
	BankTransferStale time.Duration
	// WithdrawalStale is how long a withdrawal request may stay pending
	// before an admin escalation alert is emitted.
	WithdrawalStale time.Duration
	// Now returns the current time. Defaults to time.Now when nil, allowing
	// tests to inject a deterministic clock.
	Now func() time.Time
}

func (c *Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// StaleOps emits admin escalation alerts for bank transfers and withdrawal
// requests that have been pending longer than the configured thresholds.
//
// Run executes one full escalation cycle synchronously; it is designed to be
// called from a scheduler interval job every 30 minutes. Errors from
// individual item writes are logged and swallowed so that a single bad payload
// does not abort the remaining alerts.
type StaleOps struct {
	store  Store
	writer Writer
	cfg    Config
	log    *zap.Logger
}

// NewStaleOps constructs a StaleOps escalator.
func NewStaleOps(store Store, writer Writer, cfg Config, log *zap.Logger) *StaleOps {
	return &StaleOps{store: store, writer: writer, cfg: cfg, log: log}
}

// Run queries for stale operations and emits one outbox event per stale item.
// It returns the first fatal query error; write failures per-item are logged
// and skipped so the rest of the batch always completes.
func (e *StaleOps) Run(ctx context.Context) error {
	now := e.cfg.now()

	if e.cfg.BankTransferStale > 0 {
		if err := e.escalateBankTransfers(ctx, now.Add(-e.cfg.BankTransferStale)); err != nil {
			return err
		}
	}

	if e.cfg.WithdrawalStale > 0 {
		if err := e.escalateWithdrawals(ctx, now.Add(-e.cfg.WithdrawalStale)); err != nil {
			return err
		}
	}

	return nil
}

func (e *StaleOps) escalateBankTransfers(ctx context.Context, before time.Time) error {
	proofs, err := e.store.ListStaleBankTransfers(ctx, before)
	if err != nil {
		return fmt.Errorf("escalation: list stale bank transfers: %w", err)
	}
	for _, proof := range proofs {
		payload := notification.AdminBankTransferPayload{
			ProofID:      proof.ID,
			UserID:       proof.UserID,
			AmountCents:  proof.AmountCents,
			Currency:     proof.Currency,
			PendingSince: proof.CreatedAt.UTC().Format(time.RFC3339),
		}
		if err := e.writer.Write(ctx,
			notification.EventAdminBankTransferStale,
			"bank_transfer_proof",
			fmt.Sprintf("%d", proof.ID),
			payload,
		); err != nil {
			e.log.Warn("escalation: bank transfer write failed",
				zap.Int64("proof_id", proof.ID),
				zap.Error(err),
			)
		}
	}
	return nil
}

func (e *StaleOps) escalateWithdrawals(ctx context.Context, before time.Time) error {
	reqs, err := e.store.ListStaleWithdrawals(ctx, before)
	if err != nil {
		return fmt.Errorf("escalation: list stale withdrawals: %w", err)
	}
	for _, req := range reqs {
		payload := notification.AdminWithdrawalPayload{
			RequestID:    req.ID,
			UserID:       req.UserID,
			AmountCents:  req.AmountCents,
			Currency:     req.Currency,
			PendingSince: req.CreatedAt.UTC().Format(time.RFC3339),
		}
		if err := e.writer.Write(ctx,
			notification.EventAdminWithdrawalStale,
			"withdrawal_request",
			fmt.Sprintf("%d", req.ID),
			payload,
		); err != nil {
			e.log.Warn("escalation: withdrawal write failed",
				zap.Int("request_id", req.ID),
				zap.Error(err),
			)
		}
	}
	return nil
}
