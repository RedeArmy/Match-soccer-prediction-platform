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
)
