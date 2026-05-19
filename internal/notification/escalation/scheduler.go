// Package escalation implements the periodic stale-alert scanner that emits
// EventAdminBankTransferStale and EventAdminWithdrawalStale outbox events when
// pending operations exceed their configured review thresholds.
//
// The scheduler writes directly to the domain_outbox table via outbox.Writer.
// The outbox worker then claims those entries and dispatches them through the
// AdminDispatcher with the same reliability guarantees (retry, DLQ) as every
// other admin notification.
package escalation

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ParamReader is the subset of SystemParamService used by the scheduler.
type ParamReader interface {
	GetInt(ctx context.Context, key string, defaultVal int) int
}

// OutboxWriter is the write-side contract consumed by the Scheduler.
// The production implementation is *outbox.Writer; tests may supply a stub.
type OutboxWriter interface {
	Write(ctx context.Context, eventType notification.EventType, aggregateType, aggregateID string, payload any) error
}

// Scheduler polls every interval for bank-transfer proofs and withdrawal
// requests whose age has exceeded the configured stale threshold, and emits
// outbox events so the AdminDispatcher can alert the operations team.
type Scheduler struct {
	params       ParamReader
	transferRepo repository.BankTransferProofRepository
	withdrawRepo repository.WithdrawalRequestRepository
	writer       OutboxWriter
	interval     time.Duration
	log          *zap.Logger
}

// NewScheduler constructs an EscalationScheduler.  interval is the poll
// cadence; 30 minutes is the default used in production.
func NewScheduler(
	params ParamReader,
	transferRepo repository.BankTransferProofRepository,
	withdrawRepo repository.WithdrawalRequestRepository,
	writer OutboxWriter,
	interval time.Duration,
	log *zap.Logger,
) *Scheduler {
	return &Scheduler{
		params:       params,
		transferRepo: transferRepo,
		withdrawRepo: withdrawRepo,
		writer:       writer,
		interval:     interval,
		log:          log,
	}
}

// Run blocks until ctx is cancelled, running one escalation cycle per
// interval.  Errors within a cycle are logged and swallowed — a transient
// database hiccup must not stop the scheduler.
func (s *Scheduler) Run(ctx context.Context) {
	s.log.Info("escalation scheduler started", zap.Duration("interval", s.interval))
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run immediately on start so the first alert is not delayed by the full interval.
	s.runCycle(ctx)

	for {
		select {
		case <-ctx.Done():
			s.log.Info("escalation scheduler stopped")
			return
		case <-ticker.C:
			s.runCycle(ctx)
		}
	}
}

func (s *Scheduler) runCycle(ctx context.Context) {
	bankStaleSec := s.params.GetInt(ctx, domain.ParamKeyNotifyBankTransferStaleSec, domain.DefaultNotifyBankTransferStaleSec)
	bankThreshold := time.Duration(bankStaleSec) * time.Second
	s.escalateStaleTransfers(ctx, bankThreshold)

	withdrawStaleSec := s.params.GetInt(ctx, domain.ParamKeyNotifyWithdrawalStaleSec, domain.DefaultNotifyWithdrawalStaleSec)
	withdrawThreshold := time.Duration(withdrawStaleSec) * time.Second
	s.escalateStaleWithdrawals(ctx, withdrawThreshold)
}

func (s *Scheduler) escalateStaleTransfers(ctx context.Context, threshold time.Duration) {
	proofs, err := s.transferRepo.ListPending(ctx)
	if err != nil {
		s.log.Warn("escalation: list pending bank transfers failed", zap.Error(err))
		return
	}

	cutoff := time.Now().Add(-threshold)
	staleCount := 0
	for _, proof := range proofs {
		if !proof.CreatedAt.Before(cutoff) {
			continue
		}
		staleCount++
		payload := notification.AdminBankTransferPayload{
			ProofID:      proof.ID,
			UserID:       proof.UserID,
			AmountCents:  proof.AmountCents,
			Currency:     proof.Currency,
			PendingSince: proof.CreatedAt.UTC().Format(time.RFC3339),
		}
		if err := s.writer.Write(ctx,
			notification.EventAdminBankTransferStale,
			"bank_transfer_proof",
			fmt.Sprintf("%d", proof.ID),
			payload,
		); err != nil {
			s.log.Warn("escalation: failed to write stale bank-transfer outbox event",
				zap.Int64("proof_id", proof.ID),
				zap.Error(err),
			)
		}
	}

	if staleCount > 0 {
		s.log.Info("escalation: stale bank transfers queued",
			zap.Int("count", staleCount),
			zap.Duration("threshold", threshold),
		)
	}
}

func (s *Scheduler) escalateStaleWithdrawals(ctx context.Context, threshold time.Duration) {
	reqs, err := s.withdrawRepo.ListPending(ctx)
	if err != nil {
		s.log.Warn("escalation: list pending withdrawals failed", zap.Error(err))
		return
	}

	cutoff := time.Now().Add(-threshold)
	staleCount := 0
	for _, req := range reqs {
		if !req.CreatedAt.Before(cutoff) {
			continue
		}
		staleCount++
		payload := notification.AdminWithdrawalPayload{
			RequestID:    req.ID,
			UserID:       req.UserID,
			AmountCents:  req.AmountCents,
			Currency:     req.Currency,
			PendingSince: req.CreatedAt.UTC().Format(time.RFC3339),
		}
		if err := s.writer.Write(ctx,
			notification.EventAdminWithdrawalStale,
			"withdrawal_request",
			fmt.Sprintf("%d", req.ID),
			payload,
		); err != nil {
			s.log.Warn("escalation: failed to write stale withdrawal outbox event",
				zap.Int("request_id", req.ID),
				zap.Error(err),
			)
		}
	}

	if staleCount > 0 {
		s.log.Info("escalation: stale withdrawals queued",
			zap.Int("count", staleCount),
			zap.Duration("threshold", threshold),
		)
	}
}
