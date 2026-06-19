package service

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
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

	// NotifyFreezeOnly fires the KYC winner-freeze outbox notification for a
	// winner whose prize was already frozen atomically by DistributePrizesAtomically.
	// Unlike CreditPrize, it does NOT re-check the KYC tier or re-write the ledger.
	// It is called by DistributePrizes after the atomic distribution transaction
	// commits, solely to trigger the n8n notification workflow. Using this instead
	// of CreditPrize eliminates the race window where a KYC tier upgrade between
	// the atomic tx commit and the post-loop could cause an unintended balance credit.
	NotifyFreezeOnly(ctx context.Context, userID, prizeCents int, refID int64, refType string) error
}

type prizeService struct {
	ledger       repository.BalanceLedgerRepository
	userRepo     repository.UserRepository // nil ⇒ no USD prize conversion
	kycGate      KYCGate
	kycSvc       KYCService
	fxSvc        ExchangeRateService     // nil ⇒ no USD prize conversion
	outboxWriter outbox.Writer           // may be nil; nil falls back to legacy notifier
	notifier     KYCWinnerFreezeNotifier // legacy; used only when outboxWriter is nil
	log          *zap.Logger
}

// PrizeServiceOption configures optional behaviour of prizeService.
type PrizeServiceOption func(*prizeService)

// WithPrizeFxSvc enables automatic GTQ→USD prize conversion for users whose
// balance_currency is "USD". Both userRepo and fxSvc are required; when either
// is nil, conversion is skipped (safe for tests without a DB).
func WithPrizeFxSvc(userRepo repository.UserRepository, fxSvc ExchangeRateService) PrizeServiceOption {
	return func(s *prizeService) {
		s.userRepo = userRepo
		s.fxSvc = fxSvc
	}
}

// SetFxSvc wires the exchange-rate service and user repository for GTQ→USD
// prize conversion. Called once at startup, after the FX module is ready.
func (s *prizeService) SetFxSvc(userRepo repository.UserRepository, fxSvc ExchangeRateService) {
	s.userRepo = userRepo
	s.fxSvc = fxSvc
}

// NewPrizeService constructs a PrizeCrediter.
// outboxWriter should be the transactional outbox writer; pass nil to fall back
// to the legacy fire-and-forget notifier for backward compatibility.
// notifier may be nil when n8n is not configured.
func NewPrizeService(
	ledger repository.BalanceLedgerRepository,
	kycGate KYCGate,
	kycSvc KYCService,
	outboxWriter outbox.Writer,
	notifier KYCWinnerFreezeNotifier,
	log *zap.Logger,
	opts ...PrizeServiceOption,
) PrizeCrediter {
	svc := &prizeService{
		ledger:       ledger,
		kycGate:      kycGate,
		kycSvc:       kycSvc,
		outboxWriter: outboxWriter,
		notifier:     notifier,
		log:          log,
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

func (s *prizeService) CreditPrize(ctx context.Context, userID, prizeCents int, refID int64, refType string) (bool, error) {
	// Convert the GTQ prize amount to USD cents when the winner holds a USD balance.
	creditCents := prizeCents
	if s.userRepo != nil && s.fxSvc != nil {
		if balCurrency, err := s.userRepo.GetBalanceCurrency(ctx, userID); err == nil && balCurrency == "USD" {
			usdCents, _, convErr := s.fxSvc.ConvertGTQToUSD(ctx, int64(prizeCents))
			if convErr != nil {
				s.log.Warn("prize: GTQ→USD conversion failed, using GTQ amount",
					zap.Int("user_id", userID),
					zap.Int("prize_cents", prizeCents),
					zap.Error(convErr),
				)
			} else {
				creditCents = int(usdCents)
			}
		}
	}

	shouldFreeze, reason, err := s.kycGate.CheckWinFreeze(ctx, userID, creditCents)
	if err != nil {
		return false, err
	}

	if shouldFreeze {
		if err := s.kycSvc.FreezeBalance(ctx, userID, creditCents, reason); err != nil {
			return false, err
		}

		traceID := kycTraceID(ctx)
		s.notifyKYCWinnerFreeze(ctx, userID, creditCents, traceID)

		s.log.Info("prize.freeze: balance frozen pending KYC verification",
			zap.Int("user_id", userID),
			zap.Int("prize_cents", creditCents),
			zap.String("trace_id", traceID),
		)
		return false, nil
	}

	if err := s.ledger.Credit(ctx, userID, creditCents, domain.LedgerKindPrize, refID, refType, 0); err != nil {
		return false, err
	}
	return true, nil
}

// notifyKYCWinnerFreeze writes a kyc.winner_freeze event to the outbox (preferred)
// or falls back to the legacy fire-and-forget notifier when no outbox writer is set.
func (s *prizeService) notifyKYCWinnerFreeze(ctx context.Context, userID, prizeCents int, traceID string) {
	if s.outboxWriter != nil {
		payload := notification.KYCWinnerFreezePayload{
			UserID:      userID,
			AmountCents: prizeCents,
			TraceID:     traceID,
		}
		aggregateID := fmt.Sprintf("%d", userID)
		if err := s.outboxWriter.Write(ctx, notification.EventKYCWinnerFreeze, "user", aggregateID, payload); err != nil {
			s.log.Warn("prize.freeze: outbox write failed; winner freeze notification may be delayed",
				zap.Int("user_id", userID),
				zap.Error(err),
			)
		}
		return
	}
	if s.notifier != nil {
		s.notifier.NotifyKYCWinnerFreeze(ctx, userID, prizeCents, traceID)
	}
}

// NotifyFreezeOnly fires the outbox notification for a winner whose prize was
// already frozen atomically in DistributePrizesAtomically. It does not re-check
// the KYC tier, update the ledger, or re-write kyc_profiles — those were
// committed in the atomic transaction. This eliminates the race window where
// a tier upgrade between the atomic commit and the post-loop could otherwise
// cause CreditPrize to credit a balance that the transaction froze.
func (s *prizeService) NotifyFreezeOnly(ctx context.Context, userID, prizeCents int, _ int64, _ string) error {
	traceID := kycTraceID(ctx)
	s.notifyKYCWinnerFreeze(ctx, userID, prizeCents, traceID)
	return nil
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
