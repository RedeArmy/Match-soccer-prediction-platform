package domain

import "time"

// ── Payment record ────────────────────────────────────────────────────────────

// PaymentStatus tracks the lifecycle of a payment transaction.
type PaymentStatus string

// Allowed values for PaymentStatus.
const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusConfirmed PaymentStatus = "confirmed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
	// PaymentStatusRejected indicates the payment was reviewed and denied
	// before any funds were captured. Distinct from refunded, which implies
	// money was received and subsequently returned.
	PaymentStatusRejected PaymentStatus = "rejected"
)

// PaymentRecord tracks a single entry-fee payment for one member of a
// Quiniela. Amount is stored in the minor unit of Currency (e.g. centavos
// for MXN) to avoid floating-point representation issues.
//
// Reference is the opaque identifier returned by the external payment
// provider; it is nil until the payment provider issues a transaction ID.
// ConfirmedAt is nil while the payment is pending or rejected.
// ReviewedBy and Notes are populated by the admin who called Validate or
// Reject; both are nil / empty until a review action is taken.
type PaymentRecord struct {
	ID          int
	QuinielaID  int
	UserID      int
	Amount      int // in minor units (e.g. centavos)
	Currency    string
	Status      PaymentStatus
	Reference   *string    // nil until the payment provider assigns a transaction ID
	ReviewedBy  *int       // nil until validated or rejected by an admin
	Notes       string     // empty until an admin adds review notes
	ConfirmedAt *time.Time // nil for pending / rejected payments
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ── Balance ledger ────────────────────────────────────────────────────────────

// BalanceLedgerKind identifies the business operation that caused a balance
// mutation. Each row in balance_ledger carries exactly one kind so that the
// full history of an account can be audited and categorised without joining
// to multiple source tables.
type BalanceLedgerKind string

// Balance ledger kind constants enumerate every operation that can produce a
// balance_ledger row.
const (
	LedgerKindWebhookRecurrente BalanceLedgerKind = "webhook_recurrente"
	LedgerKindWebhookPayPal     BalanceLedgerKind = "webhook_paypal"
	LedgerKindBankTransfer      BalanceLedgerKind = "bank_transfer"
	LedgerKindEntryFee          BalanceLedgerKind = "entry_fee"
	LedgerKindPrize             BalanceLedgerKind = "prize"
	// LedgerKindWithdrawalReserve and LedgerKindWithdrawalRelease are part of a
	// balance-hold model that is not currently wired into the withdrawal flow
	// (see the WithdrawalRequest doc comment above) — no Go code writes these
	// kinds today. They remain defined because both the balance_ledger.kind
	// CHECK constraint (migrations/000069) and the frontend ledger renderer
	// (frontend/src/lib/utils.ts, isVisibleLedgerKind/ledgerKindKey) already
	// treat them as valid values; removing them here would desync the enum
	// from schema/frontend without actually removing the feature anywhere.
	LedgerKindWithdrawalReserve BalanceLedgerKind = "withdrawal_reserve"
	LedgerKindWithdrawalRelease BalanceLedgerKind = "withdrawal_release"
	LedgerKindWithdrawalDeduct  BalanceLedgerKind = "withdrawal_deduct"
)

// BalanceLedger is a single, immutable row recording one atomic balance change.
// Rows are append-only; they are never updated or deleted.
type BalanceLedger struct {
	ID           int64
	UserID       int
	DeltaCents   int // positive = credit, negative = debit/reserve; always in GTQ cents
	Kind         BalanceLedgerKind
	BalanceAfter int     // users.balance_cents after the mutation
	RefID        *int64  // primary key of the originating record
	RefType      *string // "payment_record" | "bank_transfer_proof" | "withdrawal_request"
	CreatedBy    *int    // nil = system / webhook
	IPAddress    *string // originating client IP; nil for system/webhook-initiated entries
	CreatedAt    time.Time
	// SourceCurrency and SourceAmountCents are set only for non-GTQ deposits
	// (e.g. USD via Recurrente). They preserve the original payment amount so
	// the frontend can display the exact deposited value without back-conversion.
	SourceCurrency    string // "" for GTQ or legacy rows
	SourceAmountCents int    // 0 for GTQ or legacy rows
}

// ── Bank transfer proofs ──────────────────────────────────────────────────────

// BankTransferStatus is the lifecycle state of a bank transfer proof.
type BankTransferStatus string

// Bank transfer proof lifecycle states.
const (
	BankTransferPending  BankTransferStatus = "pending"
	BankTransferApproved BankTransferStatus = "approved"
	BankTransferRejected BankTransferStatus = "rejected"
)

// BankTransferProof records a user-uploaded payment receipt for a Guatemalan
// bank transfer. An admin reviews the proof, verifies the declared amount, and
// either approves (crediting the user's balance) or rejects it.
//
// StorageKey is an opaque reference to the file inside the configured FileStore;
// raw file bytes are never stored in the database.
type BankTransferProof struct {
	ID                  int64
	UserID              int
	AmountCents         int
	Currency            string
	StorageKey          string
	ContentType         string
	FileSize            int
	Status              BankTransferStatus
	ReviewedBy          *int
	Notes               string
	ApprovedAmountCents *int
	ApprovedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ── Withdrawal requests ───────────────────────────────────────────────────────

// WithdrawalMethod specifies the channel through which funds are paid out.
type WithdrawalMethod string

// Supported payout channels for withdrawal requests.
const (
	WithdrawalMethodBankGT WithdrawalMethod = "bank_gt" // Guatemalan bank account
	WithdrawalMethodPayPal WithdrawalMethod = "paypal"  // international PayPal
)

// WithdrawalStatus is the lifecycle state of a withdrawal request.
type WithdrawalStatus string

// Withdrawal request lifecycle states.
const (
	WithdrawalPending   WithdrawalStatus = "pending"
	WithdrawalApproved  WithdrawalStatus = "approved"
	WithdrawalRejected  WithdrawalStatus = "rejected"
	WithdrawalProcessed WithdrawalStatus = "processed"
)

// WithdrawalRequest is a user-initiated payout request.
//
// No persistent balance hold is placed between creation and approval/rejection
// — GTQReservedCents is never written to users.reserved_cents (that column,
// and BalanceLedgerRepository.Reserve/ReleaseReservation/CommitReservation,
// are unused by this flow). Instead, availability is checked twice:
//
//   - On creation: PostgresWithdrawalRequestRepository.Create verifies
//     balance_cents - reserved_cents >= GTQReservedCents. This is advisory —
//     it prevents an obviously-unaffordable request from being filed, but does
//     not hold the funds.
//   - On approval: ApproveAndDebit re-checks the same condition atomically as
//     part of the same UPDATE that debits balance_cents, and returns Conflict
//     if the user's balance dropped below GTQReservedCents in the meantime
//     (e.g. they spent it on a group entry fee while the request was pending).
//     This guard — not a reservation — is what actually prevents overdraft.
//
// On rejection, nothing is released because nothing was held. A double
// pending request per user is prevented separately, by a unique constraint
// on withdrawal_requests (one pending row per user_id).
//
// AmountCents is the user-facing requested amount in the specified Currency
// (e.g. USD cents for PayPal withdrawals). GTQReservedCents is the GTQ
// centavos used for the balance checks above — always equal to AmountCents
// for GTQ requests and computed via payment.usd_gtq_rate for USD requests.
//
// PayoutDetails holds method-specific fields:
//   - bank_gt : {"account_number":"…","bank_name":"…"}
//   - paypal  : {"paypal_email":"…"}
type WithdrawalRequest struct {
	ID               int
	UserID           int
	AmountCents      int
	Currency         string
	GTQReservedCents int
	Method           WithdrawalMethod
	PayoutDetails    map[string]string
	Status           WithdrawalStatus
	ReviewedBy       *int
	Notes            string
	ProcessedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
