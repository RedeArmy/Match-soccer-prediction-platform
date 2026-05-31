package handler

import (
	"strings"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// BalanceResponse is returned by GET /api/v1/users/me/balance.
type BalanceResponse struct {
	BalanceCents   int `json:"balance_cents"`
	ReservedCents  int `json:"reserved_cents"`
	AvailableCents int `json:"available_cents"`
}

// LedgerEntryResponse is a single immutable row from the balance ledger.
type LedgerEntryResponse struct {
	ID           int64   `json:"id"`
	DeltaCents   int     `json:"delta_cents"`
	Kind         string  `json:"kind"`
	BalanceAfter int     `json:"balance_after"`
	RefID        *int64  `json:"ref_id,omitempty"`
	RefType      *string `json:"ref_type,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

// BankTransferResponse is returned by bank transfer proof endpoints.
// storage_key is intentionally absent: it is an internal FileStore path that
// must never be handed to API consumers.  Use GET /bank-transfers/{id}/download
// to retrieve the file content through the application layer.
type BankTransferResponse struct {
	ID          int64   `json:"id"`
	UserID      int     `json:"user_id"`
	AmountCents int     `json:"amount_cents"`
	Currency    string  `json:"currency"`
	ContentType string  `json:"content_type"`
	FileSize    int     `json:"file_size"`
	Status      string  `json:"status"`
	ReviewedBy  *int    `json:"reviewed_by,omitempty"`
	Notes       string  `json:"notes,omitempty"`
	ApprovedAt  *string `json:"approved_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// WithdrawalResponse is returned by withdrawal request endpoints.
type WithdrawalResponse struct {
	ID            int               `json:"id"`
	UserID        int               `json:"user_id"`
	AmountCents   int               `json:"amount_cents"`
	Currency      string            `json:"currency"`
	Method        string            `json:"method"`
	PayoutDetails map[string]string `json:"payout_details,omitempty"`
	Status        string            `json:"status"`
	ReviewedBy    *int              `json:"reviewed_by,omitempty"`
	Notes         string            `json:"notes,omitempty"`
	ProcessedAt   *string           `json:"processed_at,omitempty"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
}

func balanceToResponse(balanceCents, reservedCents int) BalanceResponse {
	return BalanceResponse{
		BalanceCents:   balanceCents,
		ReservedCents:  reservedCents,
		AvailableCents: balanceCents - reservedCents,
	}
}

func ledgerEntryToResponse(e *domain.BalanceLedger) LedgerEntryResponse {
	return LedgerEntryResponse{
		ID:           e.ID,
		DeltaCents:   e.DeltaCents,
		Kind:         string(e.Kind),
		BalanceAfter: e.BalanceAfter,
		RefID:        e.RefID,
		RefType:      e.RefType,
		CreatedAt:    e.CreatedAt.Format(timeFormat),
	}
}

func bankTransferToResponse(p *domain.BankTransferProof) BankTransferResponse {
	r := BankTransferResponse{
		ID:          p.ID,
		UserID:      p.UserID,
		AmountCents: p.AmountCents,
		Currency:    p.Currency,
		ContentType: p.ContentType,
		FileSize:    p.FileSize,
		Status:      string(p.Status),
		ReviewedBy:  p.ReviewedBy,
		Notes:       p.Notes,
		CreatedAt:   p.CreatedAt.Format(timeFormat),
		UpdatedAt:   p.UpdatedAt.Format(timeFormat),
	}
	if p.ApprovedAt != nil {
		s := p.ApprovedAt.Format(timeFormat)
		r.ApprovedAt = &s
	}
	return r
}

// maskTailChars redacts all but the last n characters of s with asterisks.
// Returns all asterisks when s is shorter than or equal to n chars.
func maskTailChars(s string, n int) string {
	if len(s) <= n {
		return strings.Repeat("*", len(s))
	}
	return strings.Repeat("*", len(s)-n) + s[len(s)-n:]
}

// maskEmailAddress returns the first character plus "***@" plus the domain,
// e.g. "user@example.com" → "u***@example.com".  Returns "***" for
// addresses that cannot be split at "@".
func maskEmailAddress(s string) string {
	at := strings.IndexByte(s, '@')
	if at <= 0 {
		return "***"
	}
	return string(s[0]) + "***" + s[at:]
}

// maskPayoutDetails returns a copy of details with sensitive routing fields
// redacted.  bank_name is left unmasked: it is non-sensitive routing metadata
// that admins require to identify the destination bank when processing a
// transfer.  Only the fields that could uniquely identify a user's financial
// account are masked.
func maskPayoutDetails(details map[string]string) map[string]string {
	if len(details) == 0 {
		return details
	}
	masked := make(map[string]string, len(details))
	for k, v := range details {
		switch k {
		case "account_number":
			masked[k] = maskTailChars(v, 4) // show last 4 digits only
		case "paypal_email":
			masked[k] = maskEmailAddress(v)
		default:
			masked[k] = v
		}
	}
	return masked
}

// withdrawalToResponseMasked builds a WithdrawalResponse with sensitive
// payout_details fields redacted.  Use this for admin list endpoints where a
// single response would otherwise expose the raw financial details of every
// user in the result set.  Single-item admin responses (approve / reject /
// process) use the unmasked withdrawalToResponse so the processing admin can
// read the full destination account.
func withdrawalToResponseMasked(w *domain.WithdrawalRequest) WithdrawalResponse {
	r := withdrawalToResponse(w)
	r.PayoutDetails = maskPayoutDetails(w.PayoutDetails)
	return r
}

func withdrawalToResponse(w *domain.WithdrawalRequest) WithdrawalResponse {
	r := WithdrawalResponse{
		ID:            w.ID,
		UserID:        w.UserID,
		AmountCents:   w.AmountCents,
		Currency:      w.Currency,
		Method:        string(w.Method),
		PayoutDetails: w.PayoutDetails,
		Status:        string(w.Status),
		ReviewedBy:    w.ReviewedBy,
		Notes:         w.Notes,
		CreatedAt:     w.CreatedAt.Format(timeFormat),
		UpdatedAt:     w.UpdatedAt.Format(timeFormat),
	}
	if w.ProcessedAt != nil {
		s := w.ProcessedAt.Format(timeFormat)
		r.ProcessedAt = &s
	}
	return r
}
