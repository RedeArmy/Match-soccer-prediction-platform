package service

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// KYCWinnerFreezeNotifier is the narrow interface used by PrizeService to fire
// the non-blocking n8n kyc-winner-freeze webhook. A nil implementation is safe
// (no-op). Wire the real Notifier at composition time when n8n is configured.
type KYCWinnerFreezeNotifier interface {
	NotifyKYCWinnerFreeze(ctx context.Context, userID, amountCents int, traceID string)
}

// PrizeCrediter credits prize winnings to a user's balance, inserting the KYC
// freeze check mandated by Guatemalan SIB/UAF regulations before any credit.
//
// CreditPrize is the single authorised call site for LedgerKindPrize credits.
// All prize-disbursement paths (quiniela finalisation, scoring completion jobs)
// must route through this service so that the freeze guard can never be bypassed.
type PrizeCrediter interface {
	// CreditPrize attempts to credit prizeCents to userID.
	//
	// When the user is below KYCTierTwo the credit is withheld: the balance is
	// frozen via KYCService.FreezeBalance, an audit event is emitted, and the
	// n8n kyc-winner-freeze webhook is fired asynchronously. The caller receives
	// (false, nil) — the prize is not lost, it is held in escrow.
	//
	// When the user is at KYCTierTwo or above the ledger is credited immediately
	// and the caller receives (true, nil).
	//
	// refID and refType identify the originating record (e.g. a quiniela ID and
	// "quiniela") and are stored on the balance_ledger row for audit traceability.
	CreditPrize(ctx context.Context, userID, prizeCents int, refID int64, refType string) (credited bool, err error)
}

type prizeService struct {
	ledger   repository.BalanceLedgerRepository
	kycGate  KYCGate
	kycSvc   KYCService
	notifier KYCWinnerFreezeNotifier // may be nil
	log      *zap.Logger
}

// NewPrizeService constructs a PrizeCrediter.
// notifier may be nil when n8n is not configured.
func NewPrizeService(
	ledger repository.BalanceLedgerRepository,
	kycGate KYCGate,
	kycSvc KYCService,
	notifier KYCWinnerFreezeNotifier,
	log *zap.Logger,
) PrizeCrediter {
	return &prizeService{
		ledger:   ledger,
		kycGate:  kycGate,
		kycSvc:   kycSvc,
		notifier: notifier,
		log:      log,
	}
}

func (s *prizeService) CreditPrize(ctx context.Context, userID, prizeCents int, refID int64, refType string) (bool, error) {
	shouldFreeze, reason, err := s.kycGate.CheckWinFreeze(ctx, userID, prizeCents)
	if err != nil {
		return false, err
	}

	if shouldFreeze {
		if err := s.kycSvc.FreezeBalance(ctx, userID, prizeCents, reason); err != nil {
			return false, err
		}

		traceID := kycTraceID(ctx)

		if s.notifier != nil {
			s.notifier.NotifyKYCWinnerFreeze(ctx, userID, prizeCents, traceID)
		}

		s.log.Info("prize.freeze: balance frozen pending KYC verification",
			zap.Int("user_id", userID),
			zap.Int("prize_cents", prizeCents),
			zap.String("trace_id", traceID),
		)
		return false, nil
	}

	if err := s.ledger.Credit(ctx, userID, prizeCents, domain.LedgerKindPrize, refID, refType, 0); err != nil {
		return false, err
	}
	return true, nil
}

// kycTraceID extracts the W3C trace ID from the context span, or returns an
// empty string when no valid span is present.
func kycTraceID(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
