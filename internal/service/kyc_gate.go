package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// KYCGate enforces KYC tier-based limits on money-movement operations.
//
// CheckWithdrawal and CheckDeposit return apperrors.Forbidden when the
// user's current tier does not permit the requested operation. The caller
// should surface this as HTTP 403 with the reason embedded in the response.
//
// CheckWinFreeze evaluates whether a prize credit should be held in escrow
// pending KYC completion. The caller (prize-credit path) is responsible
// for calling KYCService.FreezeBalance when this returns shouldFreeze=true.
//
// ExceedsAMLThreshold is a non-blocking check: it returns true when the
// amount meets or exceeds the Guatemalan UAF mandatory reporting threshold.
// The caller must record an AML audit event but must not reject the transaction.
type KYCGate interface {
	// CheckWithdrawal returns nil when userID may withdraw amountCents.
	// Returns apperrors.Forbidden with an explanation when blocked by KYC tier.
	CheckWithdrawal(ctx context.Context, userID, amountCents int) error
	// CheckDeposit returns nil when userID may receive a deposit of amountCents.
	// Tier 0 and Tier 1 share the Tier-1 per-transaction cap; higher tiers have
	// their own caps; Tier 3 is unlimited. No tier is fully blocked from depositing.
	CheckDeposit(ctx context.Context, userID, amountCents int) error
	// CheckWinFreeze reports whether a prize credit should be frozen.
	// Returns (true, reason, nil) for any prize amount when the user is below
	// Tier 2. Returns (false, "", nil) when the prize can be credited freely.
	CheckWinFreeze(ctx context.Context, userID, prizeCents int) (bool, string, error)
	// ExceedsAMLThreshold returns true when amountCents meets or exceeds the
	// kyc.aml_threshold_cents system parameter (default Q25,000). The caller
	// must write an audit event; the transaction itself is never blocked.
	ExceedsAMLThreshold(ctx context.Context, amountCents int) (bool, error)
	// ExceedsCumulativeAMLThreshold returns true when the user's cumulative
	// credit transactions in the last 24 hours plus amountCents meet or exceed
	// the AML threshold. Non-blocking: caller must write an audit event.
	ExceedsCumulativeAMLThreshold(ctx context.Context, userID, amountCents int) (bool, error)
	// CheckDepositVelocity returns apperrors.Forbidden when userID has exceeded
	// the rolling 24-hour deposit limit for their KYC tier. Blocking check.
	CheckDepositVelocity(ctx context.Context, userID, amountCents int) error
	// CheckWithdrawalVelocity returns apperrors.Forbidden when userID has exceeded
	// the rolling 24-hour withdrawal limit for their KYC tier. Blocking check.
	CheckWithdrawalVelocity(ctx context.Context, userID, amountCents int) error
}

const msgKYCUpgradeForHigherLimits = "Completa la verificación KYC completa para acceder a límites mayores."

// kycGate is the production implementation of KYCGate.
//
// It reads the user's kyc_tier from the users table (denormalised by
// KYCService.Approve) and compares it against limits read from system_params.
// The param fallbacks match the migration 000121 seed values so the gate is
// always functional even before the first admin KYC configuration change.
type kycGate struct {
	userRepo repository.UserRepository
	params   SystemParamService
	metrics  *KYCMetrics
	ledger   repository.BalanceLedgerRepository // optional; nil disables cumulative checks
}

// NewKYCGate constructs a KYCGate backed by the given repositories.
func NewKYCGate(userRepo repository.UserRepository, params SystemParamService) KYCGate {
	return &kycGate{userRepo: userRepo, params: params}
}

// SetLedger wires the ledger repository for cumulative AML checks.
// Called once at startup; nil disables cumulative checking (safe in tests).
func (g *kycGate) SetLedger(ledger repository.BalanceLedgerRepository) { g.ledger = ledger }

// SetMetrics wires the OTel instruments into the gate. Called once at startup
// after RegisterKYCMetrics; safe to skip in tests (nil metrics is a no-op).
func (g *kycGate) SetMetrics(m *KYCMetrics) { g.metrics = m }

func (g *kycGate) CheckWithdrawal(ctx context.Context, userID, amountCents int) error {
	tier, err := g.tierFor(ctx, userID)
	if err != nil {
		return err
	}
	// Withdrawals require a government-issued photo ID verified by an admin
	// (Tier 2+). Tier 0 and Tier 1 (phone only) are insufficient because AML
	// regulations require identity confirmation before any payout is processed.
	if tier < domain.KYCTierTwo {
		if g.metrics != nil {
			g.metrics.RecordGateBlock(ctx, "withdrawal", "tier_insufficient")
		}
		return apperrors.Forbidden(
			"Para retirar fondos debes completar la verificación de identidad. " +
				"Los residentes en Guatemala deben enviar su DPI vigente; " +
				"los extranjeros deben enviar un documento de identidad oficial vigente " +
				"(pasaporte, cédula de residencia u equivalente). " +
				"Sube tus documentos en la sección Verificación de Identidad y espera la aprobación del equipo de cumplimiento.",
		)
	}
	if tier == domain.KYCTierTwo {
		cap := g.intParam(ctx, domain.ParamKeyKYCTier2PayoutLimitCents, domain.DefaultKYCTier2PayoutLimitCents)
		if amountCents > cap {
			if g.metrics != nil {
				g.metrics.RecordGateBlock(ctx, "withdrawal", "cap_exceeded")
			}
			return apperrors.Forbidden(fmt.Sprintf(
				"El monto de retiro (Q%.2f) supera el límite de tu nivel de verificación actual (Q%.2f). "+
					msgKYCUpgradeForHigherLimits,
				float64(amountCents)/100,
				float64(cap)/100,
			))
		}
	}
	// KYCTierThree: no limit
	return nil
}

func (g *kycGate) CheckDeposit(ctx context.Context, userID, amountCents int) error {
	tier, err := g.tierFor(ctx, userID)
	if err != nil {
		return err
	}
	// Deposits never require identity verification — any user may send funds.
	// Tier 0 and Tier 1 share the Tier-1 per-transaction cap. Tier 2 has a
	// higher cap. Tier 3 is unlimited.
	var cap int
	switch tier {
	case domain.KYCTierUnverified, domain.KYCTierOne:
		cap = g.intParam(ctx, domain.ParamKeyKYCTier1DepositLimitCents, domain.DefaultKYCTier1DepositLimitCents)
	case domain.KYCTierTwo:
		cap = g.intParam(ctx, domain.ParamKeyKYCTier2DepositLimitCents, domain.DefaultKYCTier2DepositLimitCents)
	default:
		return nil // Tier 3: no limit
	}
	if amountCents > cap {
		if g.metrics != nil {
			g.metrics.RecordGateBlock(ctx, "deposit", "cap_exceeded")
		}
		return apperrors.Forbidden(fmt.Sprintf(
			"El monto del depósito (Q%.2f) supera el límite permitido para tu nivel actual (Q%.2f). "+
				"Completa la verificación de identidad para acceder a límites mayores.",
			float64(amountCents)/100,
			float64(cap)/100,
		))
	}
	return nil
}

func (g *kycGate) CheckWinFreeze(ctx context.Context, userID, prizeCents int) (bool, string, error) {
	tier, err := g.tierFor(ctx, userID)
	if err != nil {
		return false, "", err
	}
	if tier >= domain.KYCTierTwo {
		return false, "", nil
	}
	// Any prize amount is frozen for unverified users (Tier 0/1). They must
	// submit a government-issued ID before the balance can be released.
	reason := fmt.Sprintf(
		"Has ganado un premio de Q%.2f. Para recibir tus fondos debes completar la verificación de identidad: "+
			"residentes en Guatemala deben subir su DPI vigente; extranjeros deben subir un documento oficial vigente "+
			"(pasaporte, cédula de residencia u equivalente). Tu saldo ha sido retenido hasta que el equipo de cumplimiento apruebe tu solicitud.",
		float64(prizeCents)/100,
	)
	return true, reason, nil
}

// tierFor returns the KYC tier for the given user without loading the full profile.
// Reading from users.kyc_tier (denormalised) avoids a JOIN to kyc_profiles on
// every money-movement call.
func (g *kycGate) tierFor(ctx context.Context, userID int) (domain.KYCTier, error) {
	u, err := g.userRepo.GetByID(ctx, userID)
	if err != nil {
		return 0, err
	}
	if u == nil {
		return 0, apperrors.NotFound("user not found")
	}
	return u.KYCTier, nil
}

// intParam reads a system param as an integer, falling back to defaultVal when
// the key is absent or the stored string cannot be parsed. This mirrors the
// pattern used by WithdrawalService and other param-consuming services.
func (g *kycGate) intParam(ctx context.Context, key string, defaultVal int) int {
	p, err := g.params.Get(ctx, key)
	if err != nil || p == nil {
		return defaultVal
	}
	v, err := strconv.Atoi(p.Value)
	if err != nil {
		return defaultVal
	}
	return v
}

func (g *kycGate) ExceedsAMLThreshold(ctx context.Context, amountCents int) (bool, error) {
	threshold := g.intParam(ctx, domain.ParamKeyKYCAMLThresholdCents, domain.DefaultKYCAMLThresholdCents)
	return amountCents >= threshold, nil
}

func (g *kycGate) CheckDepositVelocity(ctx context.Context, userID, amountCents int) error {
	if g.ledger == nil {
		return nil
	}
	tier, err := g.tierFor(ctx, userID)
	if err != nil {
		return err
	}
	var cap int
	switch tier {
	case domain.KYCTierUnverified, domain.KYCTierOne:
		cap = g.intParam(ctx, domain.ParamKeyKYCTier1DepositVelocityCents, domain.DefaultKYCTier1DepositVelocityCents)
	case domain.KYCTierTwo:
		cap = g.intParam(ctx, domain.ParamKeyKYCTier2DepositVelocityCents, domain.DefaultKYCTier2DepositVelocityCents)
	default:
		return nil // Tier 3: no velocity limit
	}
	creditKinds := []domain.BalanceLedgerKind{
		domain.LedgerKindBankTransfer,
		domain.LedgerKindWebhookRecurrente,
		domain.LedgerKindWebhookPayPal,
	}
	sum, err := g.ledger.SumTransactionsByUserAndPeriod(ctx, userID, creditKinds, time.Now().Add(-24*time.Hour))
	if err != nil {
		return err
	}
	if (sum + int64(amountCents)) > int64(cap) {
		if g.metrics != nil {
			g.metrics.RecordGateBlock(ctx, "deposit", "velocity_exceeded")
		}
		return apperrors.Forbidden(fmt.Sprintf(
			"El depósito supera el límite de velocidad de 24 horas para tu nivel de verificación actual (Q%.2f/día). "+
				msgKYCUpgradeForHigherLimits,
			float64(cap)/100,
		))
	}
	return nil
}

func (g *kycGate) CheckWithdrawalVelocity(ctx context.Context, userID, amountCents int) error {
	if g.ledger == nil {
		return nil
	}
	tier, err := g.tierFor(ctx, userID)
	if err != nil {
		return err
	}
	var cap int
	switch tier {
	case domain.KYCTierUnverified, domain.KYCTierOne:
		cap = g.intParam(ctx, domain.ParamKeyKYCTier1WithdrawalVelocityCents, domain.DefaultKYCTier1WithdrawalVelocityCents)
	case domain.KYCTierTwo:
		cap = g.intParam(ctx, domain.ParamKeyKYCTier2WithdrawalVelocityCents, domain.DefaultKYCTier2WithdrawalVelocityCents)
	default:
		return nil // Tier 3: no velocity limit
	}
	if cap == 0 {
		if g.metrics != nil {
			g.metrics.RecordGateBlock(ctx, "withdrawal", "velocity_no_allowance")
		}
		return apperrors.Forbidden(
			"Tu nivel de verificación actual no permite retiros. Completa la verificación de identidad para habilitar retiros.",
		)
	}
	withdrawalKinds := []domain.BalanceLedgerKind{
		domain.LedgerKindWithdrawalDeduct,
	}
	sum, err := g.ledger.SumTransactionsByUserAndPeriod(ctx, userID, withdrawalKinds, time.Now().Add(-24*time.Hour))
	if err != nil {
		return err
	}
	if (sum + int64(amountCents)) > int64(cap) {
		if g.metrics != nil {
			g.metrics.RecordGateBlock(ctx, "withdrawal", "velocity_exceeded")
		}
		return apperrors.Forbidden(fmt.Sprintf(
			"El retiro supera el límite de velocidad de 24 horas para tu nivel de verificación actual (Q%.2f/día). "+
				msgKYCUpgradeForHigherLimits,
			float64(cap)/100,
		))
	}
	return nil
}

func (g *kycGate) ExceedsCumulativeAMLThreshold(ctx context.Context, userID, amountCents int) (bool, error) {
	threshold := g.intParam(ctx, domain.ParamKeyKYCAMLThresholdCents, domain.DefaultKYCAMLThresholdCents)
	if amountCents >= threshold {
		return true, nil
	}
	if g.ledger == nil {
		return false, nil
	}
	creditKinds := []domain.BalanceLedgerKind{
		domain.LedgerKindBankTransfer,
		domain.LedgerKindWebhookRecurrente,
		domain.LedgerKindWebhookPayPal,
		domain.LedgerKindPrize,
	}
	since := time.Now().Add(-24 * time.Hour)
	sum, err := g.ledger.SumTransactionsByUserAndPeriod(ctx, userID, creditKinds, since)
	if err != nil {
		return false, err
	}
	return (sum + int64(amountCents)) >= int64(threshold), nil
}

var _ KYCGate = (*kycGate)(nil)
