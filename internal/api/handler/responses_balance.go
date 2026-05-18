package handler

import (
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
type BankTransferResponse struct {
	ID          int64   `json:"id"`
	UserID      int     `json:"user_id"`
	AmountCents int     `json:"amount_cents"`
	Currency    string  `json:"currency"`
	StorageKey  string  `json:"storage_key"`
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
		StorageKey:  p.StorageKey,
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
