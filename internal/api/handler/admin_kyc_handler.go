package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminKYCHandler serves admin KYC review endpoints.
type AdminKYCHandler struct {
	svc service.KYCService
	log *zap.Logger
}

// NewAdminKYCHandler constructs an AdminKYCHandler.
func NewAdminKYCHandler(svc service.KYCService, log *zap.Logger) *AdminKYCHandler {
	return &AdminKYCHandler{svc: svc, log: log}
}

// ListQueue handles GET /api/v1/admin/kyc/queue.
// Returns profiles in pending, under_review, or escalated state.
func (h *AdminKYCHandler) ListQueue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	f := repository.KYCProfileFilters{}
	if s := q.Get("status"); s != "" {
		st := domain.KYCStatus(s)
		f.Status = &st
	}
	profiles, err := h.svc.ListQueue(r.Context(), f, repository.Pagination{Limit: limit, Offset: offset})
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]KYCProfileResponse, 0, len(profiles))
	for _, p := range profiles {
		resp = append(resp, kycProfileToResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetProfile handles GET /api/v1/admin/kyc/profiles/{profileID}.
func (h *AdminKYCHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := strconv.Atoi(chi.URLParam(r, "profileID"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	profile, err := h.svc.GetProfileByID(r.Context(), profileID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if profile == nil {
		writeError(w, r, h.log, apperrors.NotFound("kyc profile not found"))
		return
	}
	writeJSON(w, http.StatusOK, kycProfileToResponse(profile))
}

// Approve handles POST /api/v1/admin/kyc/profiles/{profileID}/approve.
type approveKYCRequest struct {
	Tier int `json:"tier"` // 1, 2, or 3
}

func (h *AdminKYCHandler) Approve(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	profileID, err := strconv.Atoi(chi.URLParam(r, "profileID"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	req, err := decodeJSON[approveKYCRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Tier < 1 || req.Tier > 3 {
		writeError(w, r, h.log, apperrors.Validation("tier must be 1, 2, or 3"))
		return
	}
	if err := h.svc.Approve(r.Context(), profileID, admin.ID, domain.KYCTier(req.Tier)); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// Reject handles POST /api/v1/admin/kyc/profiles/{profileID}/reject.
type rejectKYCRequest struct {
	Reason string `json:"reason"`
}

func (h *AdminKYCHandler) Reject(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	profileID, err := strconv.Atoi(chi.URLParam(r, "profileID"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	req, err := decodeJSON[rejectKYCRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if err := h.svc.Reject(r.Context(), profileID, admin.ID, req.Reason); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// Escalate handles POST /api/v1/admin/kyc/profiles/{profileID}/escalate.
type escalateKYCRequest struct {
	Reason string `json:"reason"`
}

func (h *AdminKYCHandler) Escalate(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	profileID, err := strconv.Atoi(chi.URLParam(r, "profileID"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	req, err := decodeJSON[escalateKYCRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if err := h.svc.Escalate(r.Context(), profileID, admin.ID, req.Reason); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "escalated"})
}

// RequestDocument handles POST /api/v1/admin/kyc/profiles/{profileID}/request-doc.
type requestDocKYCRequest struct {
	DocumentType string `json:"document_type"`
	Reason       string `json:"reason"`
}

func (h *AdminKYCHandler) RequestDocument(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	profileID, err := strconv.Atoi(chi.URLParam(r, "profileID"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	req, err := decodeJSON[requestDocKYCRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if err := h.svc.RequestDocument(r.Context(), profileID, admin.ID,
		domain.KYCDocumentType(req.DocumentType), req.Reason); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "requested"})
}

// VerifyDocument handles POST /api/v1/admin/kyc/documents/{docID}/verify.
func (h *AdminKYCHandler) VerifyDocument(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	docID, err := strconv.ParseInt(chi.URLParam(r, "docID"), 10, 64)
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid document ID"))
		return
	}
	if err := h.svc.VerifyDocument(r.Context(), docID, admin.ID); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

// ListDocumentsForProfile handles GET /api/v1/admin/kyc/profiles/{profileID}/documents.
func (h *AdminKYCHandler) ListDocumentsForProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := strconv.Atoi(chi.URLParam(r, "profileID"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	profile, err := h.svc.GetProfileByID(r.Context(), profileID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if profile == nil {
		writeError(w, r, h.log, apperrors.NotFound("kyc profile not found"))
		return
	}
	// Reuse the service's ListDocuments via the profile's user ID — the service
	// resolves profile→docs internally, so we go through a dedicated admin path
	// that scopes by profile ID directly.
	docs, err := h.svc.ListDocuments(r.Context(), profile.UserID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]KYCDocumentResponse, 0, len(docs))
	for _, d := range docs {
		resp = append(resp, kycDocumentToResponse(d))
	}
	writeJSON(w, http.StatusOK, resp)
}

// ListFrozenBalances handles GET /api/v1/admin/kyc/frozen-balances.
func (h *AdminKYCHandler) ListFrozenBalances(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.svc.ListFrozenBalances(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]FrozenBalanceResponse, 0, len(summaries))
	for _, s := range summaries {
		resp = append(resp, frozenBalanceToResponse(s))
	}
	writeJSON(w, http.StatusOK, resp)
}

// ReleaseFrozenBalance handles POST /api/v1/admin/kyc/users/{userID}/release-freeze.
func (h *AdminKYCHandler) ReleaseFrozenBalance(w http.ResponseWriter, r *http.Request) {
	admin, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	userID, err := strconv.Atoi(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid user ID"))
		return
	}
	if err := h.svc.ReleaseFrozenBalance(r.Context(), userID, admin.ID); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "released"})
}

// ListProfileEvents handles GET /api/v1/admin/kyc/profiles/{profileID}/events.
func (h *AdminKYCHandler) ListProfileEvents(w http.ResponseWriter, r *http.Request) {
	profileID, err := strconv.Atoi(chi.URLParam(r, "profileID"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	events, next, err := h.svc.ListEvents(r.Context(), profileID, domain.KYCProfileTypeUser,
		repository.CursorPage{Limit: limit, Cursor: r.URL.Query().Get("cursor")})
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]KYCEventResponse, 0, len(events))
	for _, e := range events {
		resp = append(resp, kycEventToResponse(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": resp, "next_cursor": next})
}
