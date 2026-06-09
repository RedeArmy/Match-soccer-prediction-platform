package handler

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

// BankLister is the narrow repository interface for gt_banks.
type BankLister interface {
	ListActive(ctx context.Context) ([]repository.Bank, error)
}

// BankAccountTypeLister is the narrow repository interface for bank_account_types.
type BankAccountTypeLister interface {
	ListActive(ctx context.Context) ([]repository.BankAccountType, error)
}

// BankHandler serves the bank and account-type lookup endpoints.
type BankHandler struct {
	repo     BankLister
	typeRepo BankAccountTypeLister
	log      *zap.Logger
}

// NewBankHandler constructs a BankHandler.
func NewBankHandler(repo BankLister, typeRepo BankAccountTypeLister, log *zap.Logger) *BankHandler {
	return &BankHandler{repo: repo, typeRepo: typeRepo, log: log}
}

type bankResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// List handles GET /api/v1/banks.
func (h *BankHandler) List(w http.ResponseWriter, r *http.Request) {
	banks, err := h.repo.ListActive(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]bankResponse, 0, len(banks))
	for _, b := range banks {
		resp = append(resp, bankResponse{ID: b.ID, Name: b.Name})
	}
	writeJSON(w, http.StatusOK, resp)
}

// ListAccountTypes handles GET /api/v1/bank-account-types.
func (h *BankHandler) ListAccountTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.typeRepo.ListActive(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]bankResponse, 0, len(types))
	for _, t := range types {
		resp = append(resp, bankResponse{ID: t.ID, Name: t.Name})
	}
	writeJSON(w, http.StatusOK, resp)
}
