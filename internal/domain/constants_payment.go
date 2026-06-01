package domain

// Default values for payment, withdrawal, and bank-transfer system parameters.
const (
	// DefaultPaymentMaxUploadBytes is the maximum size in bytes for bank transfer
	// proof uploads (5 MB).
	DefaultPaymentMaxUploadBytes = 5_242_880 // payment.max_upload_bytes

	// DefaultWithdrawalMinCents is the minimum withdrawal amount in minor units (50 GTQ).
	DefaultWithdrawalMinCents = 5_000 // payment.withdrawal_min_cents
	// DefaultWithdrawalMaxCents is the maximum withdrawal amount in minor units (5 000 GTQ).
	DefaultWithdrawalMaxCents = 500_000 // payment.withdrawal_max_cents

	// Bank transfer amount bounds. The declared amount on a bank transfer proof is
	// validated against these before the proof is accepted for admin review.
	DefaultBankTransferMinAmountCents = 1_000      // payment.bank_transfer_min_amount_cents (10 GTQ)
	DefaultBankTransferMaxAmountCents = 10_000_000 // payment.bank_transfer_max_amount_cents (100 000 GTQ)

	// DefaultPaymentIntentTTLMinutes is the number of minutes a pending PayPal
	// payment intent remains valid. After expiry the customer must restart checkout.
	DefaultPaymentIntentTTLMinutes = 60 // payment.intent_ttl_minutes

	// DefaultUSDGTQRate is the GTQ centavos per USD dollar used as the fallback
	// exchange rate when the payment.usd_gtq_rate system param is absent.
	// 790 = Q7.90 per $1 USD (approximate market rate at quiniela launch).
	// Formula: gtq_reserved_cents = usd_cents * DefaultUSDGTQRate / 100.
	DefaultUSDGTQRate = 790 // payment.usd_gtq_rate

	// DefaultExchangeRateMarginBPS is the unified safety markup used by the
	// simple single-rate model (Phase 4 prior implementation, kept for migration
	// compatibility). Superseded by DefaultFXBuyMarginBPS / DefaultFXSellMarginBPS.
	DefaultExchangeRateMarginBPS = 200 // payment.exchange_rate_margin_bps

	// DefaultFXBuyMarginBPS is the platform's buy-side margin in basis points.
	// The platform buys USD from users at ReferenceRate × (1 − margin).
	// 150 bps = 1.5% — example: 7.75 × 0.985 = 7.6338 GTQ/USD.
	// is_runtime=TRUE: adjustable via admin API without a worker restart.
	DefaultFXBuyMarginBPS = 150 // fx.buy_margin_bps

	// DefaultFXSellMarginBPS is the platform's sell-side margin in basis points.
	// The platform sells USD to users at ReferenceRate × (1 + margin).
	// 200 bps = 2.0% — example: 7.75 × 1.020 = 7.9050 GTQ/USD.
	// is_runtime=TRUE: adjustable via admin API without a worker restart.
	DefaultFXSellMarginBPS = 200 // fx.sell_margin_bps

	// DefaultFXDisplayDecimals is the number of decimal places shown to users
	// when displaying buy/sell rates (e.g. 4 → "7.6338").
	DefaultFXDisplayDecimals = 4 // fx.display_decimals

	// DefaultFXStaleThresholdH is the number of hours after which a rate is
	// considered stale and an n8n alert fires.
	// 26 h gives a 2-hour grace window past the next scheduled daily refresh.
	DefaultFXStaleThresholdH = 26 // fx.stale_threshold_h
)

// Payment, withdrawal, and bank-transfer system parameter keys.
const (
	// ParamKeyPaymentMaxUploadBytes is the maximum size in bytes for bank transfer
	// proof uploads.
	ParamKeyPaymentMaxUploadBytes = "payment.max_upload_bytes"
	// ParamKeyWithdrawalMinCents is the minimum withdrawal amount in minor units.
	ParamKeyWithdrawalMinCents = "payment.withdrawal_min_cents"
	// ParamKeyWithdrawalMaxCents is the maximum withdrawal amount in minor units.
	ParamKeyWithdrawalMaxCents = "payment.withdrawal_max_cents"
	// ParamKeyBankTransferMinAmountCents is the minimum declared amount in minor
	// units for a bank transfer proof submission.
	// Defaults to DefaultBankTransferMinAmountCents (1 000 = 10 GTQ).
	ParamKeyBankTransferMinAmountCents = "payment.bank_transfer_min_amount_cents"
	// ParamKeyBankTransferMaxAmountCents is the maximum declared amount in minor
	// units for a bank transfer proof submission.
	// Defaults to DefaultBankTransferMaxAmountCents (10 000 000 = 100 000 GTQ).
	ParamKeyBankTransferMaxAmountCents = "payment.bank_transfer_max_amount_cents"
	// ParamKeyPaymentIntentTTLMinutes is the number of minutes a pending PayPal
	// payment intent remains valid. is_runtime=TRUE: tunable without restart.
	ParamKeyPaymentIntentTTLMinutes = "payment.intent_ttl_minutes"

	// ParamKeyUSDGTQRate is the GTQ centavos per USD dollar used to convert
	// USD PayPal withdrawal amounts into GTQ centavos for balance reservation.
	// is_runtime=TRUE: tunable via admin API; propagates within 30 s.
	ParamKeyUSDGTQRate = "payment.usd_gtq_rate"

	// ParamKeyExchangeRateMarginBPS is the unified margin from the Phase 4
	// implementation. Superseded by ParamKeyFXBuyMarginBPS / ParamKeyFXSellMarginBPS
	// but kept in system_params for migration continuity.
	ParamKeyExchangeRateMarginBPS = "payment.exchange_rate_margin_bps"

	// ParamKeyFXBuyMarginBPS is the buy-side margin in basis points (100 = 1%).
	// Platform buys USD from users at ReferenceRate × (1 − margin).
	// is_runtime=TRUE: takes effect on the next RefreshRate call.
	ParamKeyFXBuyMarginBPS = "fx.buy_margin_bps"

	// ParamKeyFXSellMarginBPS is the sell-side margin in basis points (100 = 1%).
	// Platform sells USD to users at ReferenceRate × (1 + margin).
	// is_runtime=TRUE: takes effect on the next RefreshRate call.
	ParamKeyFXSellMarginBPS = "fx.sell_margin_bps"

	// ParamKeyFXDisplayDecimals controls the decimal places shown in rate
	// display endpoints (e.g. 4 → "7.6338").
	// is_runtime=TRUE.
	ParamKeyFXDisplayDecimals = "fx.display_decimals"

	// ParamKeyFXStaleThresholdH is the age in hours after which a cached rate
	// triggers an n8n stale-rate alert.  Default: 26 h.
	// is_runtime=TRUE.
	ParamKeyFXStaleThresholdH = "fx.stale_threshold_h"
)
