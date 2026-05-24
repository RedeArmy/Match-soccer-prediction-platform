package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// decodeJSONOptional decodes the request body into v, silently ignoring JSON
// parse errors. Returns a non-nil error only when the body exceeds the reader
// limit set by MaxBytesReader — callers must propagate that as 413.
func decodeJSONOptional(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			return apperrors.RequestBodyTooLarge()
		}
	}
	return nil
}

// bankTransferObservabilityNotifier is the narrow slice of ObservabilityNotifier
// consumed by BankTransferHandler.
type bankTransferObservabilityNotifier interface {
	NotifyTransferUploaded(ctx context.Context, userID, amountClaimedCents int, fileURL string)
}

// BankTransferHandler handles bank transfer proof upload and admin review.
type BankTransferHandler struct {
	svc       service.BankTransferService
	fileStore storage.FileStore
	maxUpload int64 // bytes; read from system_params at construction
	minAmount int   // cents; payment.bank_transfer_min_amount_cents
	maxAmount int   // cents; payment.bank_transfer_max_amount_cents
	log       *zap.Logger
	notifier  bankTransferObservabilityNotifier // nil = disabled
}

// NewBankTransferHandler constructs a BankTransferHandler.
// minAmountCents and maxAmountCents bound the declared transfer amount;
// both are read from system_params at startup and fall back to domain defaults
// when zero or negative.
func NewBankTransferHandler(
	svc service.BankTransferService,
	fileStore storage.FileStore,
	maxUploadBytes int64,
	minAmountCents, maxAmountCents int,
	log *zap.Logger,
) *BankTransferHandler {
	if maxUploadBytes <= 0 {
		maxUploadBytes = int64(domain.DefaultPaymentMaxUploadBytes)
	}
	if minAmountCents <= 0 {
		minAmountCents = domain.DefaultBankTransferMinAmountCents
	}
	if maxAmountCents <= 0 {
		maxAmountCents = domain.DefaultBankTransferMaxAmountCents
	}
	return &BankTransferHandler{
		svc:       svc,
		fileStore: fileStore,
		maxUpload: maxUploadBytes,
		minAmount: minAmountCents,
		maxAmount: maxAmountCents,
		log:       log,
	}
}

// SetNotifier wires an ObservabilityNotifier for transfer-uploaded events.
// Call at composition time (buildHandlers) before any requests are served.
func (h *BankTransferHandler) SetNotifier(n bankTransferObservabilityNotifier) {
	h.notifier = n
}

// Upload handles POST /api/v1/bank-transfers.
//
// @Summary      Upload bank transfer proof
// @Description  Accepts a multipart/form-data upload containing the payment screenshot or PDF.
//
//	Fields: amount_cents (int), currency (string, default "GTQ"), file (binary).
//
// @Tags         balance
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        amount_cents  formData  int     true   "Declared amount in minor currency units"
// @Param        currency      formData  string  false  "Currency code (default GTQ)"
// @Param        file          formData  file    true   "Proof image or PDF"
// @Success      201  {object}  handler.BankTransferResponse
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      413  {object}  handler.ErrorResponse  "File too large"
// @Failure      422  {object}  handler.ErrorResponse
// @Router       /api/v1/bank-transfers [post]
func (h *BankTransferHandler) Upload(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	if err := r.ParseMultipartForm(h.maxUpload); err != nil { //nolint:gosec // G120: body size already enforced by RequestBodyLimit middleware via http.MaxBytesReader
		writeError(w, r, h.log, apperrors.RequestBodyTooLarge())
		return
	}

	amountCents, err := strconv.Atoi(r.FormValue("amount_cents")) //nolint:gosec // G120: body size already enforced by ParseMultipartForm limit above
	if err != nil || amountCents <= 0 {
		writeError(w, r, h.log, apperrors.Validation("amount_cents must be a positive integer"))
		return
	}
	if amountCents < h.minAmount {
		writeError(w, r, h.log, apperrors.Validation(
			fmt.Sprintf("amount_cents must be at least %d", h.minAmount),
		))
		return
	}
	if amountCents > h.maxAmount {
		writeError(w, r, h.log, apperrors.Validation(
			fmt.Sprintf("amount_cents must be at most %d", h.maxAmount),
		))
		return
	}
	currency := r.FormValue("currency") //nolint:gosec // G120: body already parsed and bounded by ParseMultipartForm above
	if currency == "" {
		currency = "GTQ"
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("file is required"))
		return
	}
	defer func() { _ = file.Close() }()

	if header.Size > h.maxUpload {
		writeError(w, r, h.log, apperrors.RequestBodyTooLarge())
		return
	}

	contentType := header.Header.Get("Content-Type")
	if !allowedProofContentType(contentType) {
		writeError(w, r, h.log, apperrors.Validation("unsupported file type; allowed: image/jpeg, image/png, image/webp, application/pdf"))
		return
	}

	ext := extensionForContentType(contentType)
	storageKey := fmt.Sprintf("bank-transfers/%d/%s%s", caller.ID, generateID(), ext) //nolint:perfsprint

	if err := h.fileStore.Put(r.Context(), storageKey, contentType, file, header.Size); err != nil {
		writeError(w, r, h.log, apperrors.Internal(err))
		return
	}

	proof, err := h.svc.Upload(r.Context(), caller.ID, amountCents, currency, storageKey, contentType, int(header.Size))
	if err != nil {
		_ = h.fileStore.Delete(r.Context(), storageKey) // best-effort cleanup
		writeError(w, r, h.log, err)
		return
	}

	if h.notifier != nil {
		h.notifier.NotifyTransferUploaded(r.Context(), caller.ID, amountCents, storageKey)
	}
	writeJSON(w, http.StatusCreated, bankTransferToResponse(proof))
}

// ListMine handles GET /api/v1/bank-transfers.
//
// @Summary      List my bank transfers
// @Description  Returns all bank transfer proofs submitted by the authenticated user.
// @Tags         balance
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.BankTransferResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Router       /api/v1/bank-transfers [get]
func (h *BankTransferHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	proofs, err := h.svc.ListByUser(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	data := make([]BankTransferResponse, len(proofs))
	for i, p := range proofs {
		data[i] = bankTransferToResponse(p)
	}
	writeJSON(w, http.StatusOK, data)
}

// AdminListPending handles GET /admin/bank-transfers/pending.
//
// @Summary      List pending bank transfers
// @Description  Returns all bank transfer proofs awaiting admin review. Requires admin role.
// @Tags         admin-payments
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.BankTransferResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/bank-transfers/pending [get]
func (h *BankTransferHandler) AdminListPending(w http.ResponseWriter, r *http.Request) {
	proofs, err := h.svc.ListPending(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	data := make([]BankTransferResponse, len(proofs))
	for i, p := range proofs {
		data[i] = bankTransferToResponse(p)
	}
	writeJSON(w, http.StatusOK, data)
}

type reviewBankTransferRequest struct {
	Notes string `json:"notes"`
}

// AdminApprove handles POST /admin/bank-transfers/{id}/approve.
//
// @Summary      Approve bank transfer
// @Description  Approves a pending bank transfer proof and credits the user's balance. Requires admin role.
// @Tags         admin-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                                 true  "Proof ID"
// @Param        body  body      handler.reviewBankTransferRequest   false "Optional notes"
// @Success      200  {object}  handler.BankTransferResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/bank-transfers/{id}/approve [post]
func (h *BankTransferHandler) AdminApprove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation("invalid bank transfer id"))
		return
	}
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, moneyJSONBodyLimit)
	var req reviewBankTransferRequest
	if err := decodeJSONOptional(r, &req); err != nil {
		writeError(w, r, h.log, err)
		return
	}

	proof, err := h.svc.ApproveTransfer(r.Context(), id, caller.ID, req.Notes)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, bankTransferToResponse(proof))
}

// AdminReject handles POST /admin/bank-transfers/{id}/reject.
//
// @Summary      Reject bank transfer
// @Description  Rejects a pending bank transfer proof. Requires admin role.
// @Tags         admin-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                               true  "Proof ID"
// @Param        body  body      handler.reviewBankTransferRequest true  "Rejection notes"
// @Success      200  {object}  handler.BankTransferResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/bank-transfers/{id}/reject [post]
func (h *BankTransferHandler) AdminReject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation("invalid bank transfer id"))
		return
	}
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, moneyJSONBodyLimit)
	req, err := decodeJSON[reviewBankTransferRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Notes == "" {
		writeError(w, r, h.log, apperrors.Validation("notes are required"))
		return
	}

	proof, err := h.svc.RejectTransfer(r.Context(), id, caller.ID, req.Notes)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, bankTransferToResponse(proof))
}

func allowedProofContentType(ct string) bool {
	switch ct {
	case "image/jpeg", "image/png", "image/webp", "application/pdf":
		return true
	}
	return false
}

func extensionForContentType(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}
