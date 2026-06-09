package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// adminBankStore is the full read-write interface used by the admin bank handler.
type adminBankStore interface {
	ListActive(ctx context.Context) ([]repository.Bank, error)
	ListAll(ctx context.Context) ([]repository.Bank, error)
	Create(ctx context.Context, name string) (repository.Bank, error)
	SetActive(ctx context.Context, id int, active bool) (repository.Bank, error)
}

// adminAccountTypeStore is the full read-write interface for account types.
type adminAccountTypeStore interface {
	ListActive(ctx context.Context) ([]repository.BankAccountType, error)
	ListAll(ctx context.Context) ([]repository.BankAccountType, error)
	Create(ctx context.Context, name string) (repository.BankAccountType, error)
	SetActive(ctx context.Context, id int, active bool) (repository.BankAccountType, error)
}

// AdminBankHandler serves admin endpoints for gt_banks and bank_account_types.
type AdminBankHandler struct {
	banks    adminBankStore
	accTypes adminAccountTypeStore
	log      *zap.Logger
}

// NewAdminBankHandler constructs an AdminBankHandler.
func NewAdminBankHandler(banks adminBankStore, accTypes adminAccountTypeStore, log *zap.Logger) *AdminBankHandler {
	return &AdminBankHandler{banks: banks, accTypes: accTypes, log: log}
}

type adminBankResponse struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ── Banks ─────────────────────────────────────────────────────────────────────

// ListBanks handles GET /api/v1/admin/banks.
// Accepts optional query param ?active=true to return only active banks.
func (h *AdminBankHandler) ListBanks(w http.ResponseWriter, r *http.Request) {
	var (
		banks []repository.Bank
		err   error
	)
	if r.URL.Query().Get("active") == "true" {
		banks, err = h.banks.ListActive(r.Context())
	} else {
		banks, err = h.banks.ListAll(r.Context())
	}
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]adminBankResponse, 0, len(banks))
	for _, b := range banks {
		resp = append(resp, adminBankResponse{ID: b.ID, Name: b.Name, Active: b.Active})
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateBank handles POST /api/v1/admin/banks.
func (h *AdminBankHandler) CreateBank(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[struct {
		Name string `json:"name"`
	}](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Name == "" {
		writeError(w, r, h.log, apperrors.Validation("name is required"))
		return
	}
	bank, err := h.banks.Create(r.Context(), req.Name)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminBankResponse{ID: bank.ID, Name: bank.Name, Active: bank.Active})
}

// SetBankActive handles PATCH /api/v1/admin/banks/{id}/active.
func (h *AdminBankHandler) SetBankActive(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid bank id"))
		return
	}
	req, err := decodeJSON[struct {
		Active bool `json:"active"`
	}](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	bank, err := h.banks.SetActive(r.Context(), id, req.Active)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, adminBankResponse{ID: bank.ID, Name: bank.Name, Active: bank.Active})
}

// ── Account types ─────────────────────────────────────────────────────────────

// ListAccountTypes handles GET /api/v1/admin/bank-account-types.
// Accepts optional query param ?active=true to return only active types.
func (h *AdminBankHandler) ListAccountTypes(w http.ResponseWriter, r *http.Request) {
	var (
		types []repository.BankAccountType
		err   error
	)
	if r.URL.Query().Get("active") == "true" {
		types, err = h.accTypes.ListActive(r.Context())
	} else {
		types, err = h.accTypes.ListAll(r.Context())
	}
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]adminBankResponse, 0, len(types))
	for _, t := range types {
		resp = append(resp, adminBankResponse{ID: t.ID, Name: t.Name, Active: t.Active})
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateAccountType handles POST /api/v1/admin/bank-account-types.
func (h *AdminBankHandler) CreateAccountType(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[struct {
		Name string `json:"name"`
	}](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Name == "" {
		writeError(w, r, h.log, apperrors.Validation("name is required"))
		return
	}
	t, err := h.accTypes.Create(r.Context(), req.Name)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminBankResponse{ID: t.ID, Name: t.Name, Active: t.Active})
}

// SetAccountTypeActive handles PATCH /api/v1/admin/bank-account-types/{id}/active.
func (h *AdminBankHandler) SetAccountTypeActive(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid account type id"))
		return
	}
	req, err := decodeJSON[struct {
		Active bool `json:"active"`
	}](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	t, err := h.accTypes.SetActive(r.Context(), id, req.Active)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, adminBankResponse{ID: t.ID, Name: t.Name, Active: t.Active})
}
