package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// KYBHandler serves organiser-facing KYB endpoints.
type KYBHandler struct {
	svc service.KYBService
	log *zap.Logger
}

// NewKYBHandler constructs a KYBHandler.
func NewKYBHandler(svc service.KYBService, log *zap.Logger) *KYBHandler {
	return &KYBHandler{svc: svc, log: log}
}

// GetStatus handles GET /api/v1/kyb/status.
func (h *KYBHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	profile, err := h.svc.GetStatus(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if profile == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "unverified"})
		return
	}
	writeJSON(w, http.StatusOK, kybProfileToResponse(profile))
}

// Submit handles POST /api/v1/kyb/submit.
type submitKYBRequest struct {
	LegalName          string `json:"legal_name"`
	TaxID              string `json:"tax_id"`
	RegistrationNumber string `json:"registration_number"`
	Jurisdiction       string `json:"jurisdiction"`
	IncorporationDate  string `json:"incorporation_date"` // YYYY-MM-DD
	UBOName            string `json:"ubo_name"`
	UBODocumentNumber  string `json:"ubo_document_number"`
}

// Submit handles POST /api/v1/kyb/submit.
func (h *KYBHandler) Submit(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, moneyJSONBodyLimit)
	req, err := decodeJSON[submitKYBRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	input := service.KYBSubmitInput{
		LegalName:          req.LegalName,
		TaxID:              req.TaxID,
		RegistrationNumber: req.RegistrationNumber,
		Jurisdiction:       req.Jurisdiction,
		UBOName:            req.UBOName,
		UBODocumentNumber:  req.UBODocumentNumber,
	}
	if req.IncorporationDate != "" {
		d, err := time.Parse("2006-01-02", req.IncorporationDate)
		if err != nil {
			writeError(w, r, h.log, apperrors.Validation("incorporation_date must be in YYYY-MM-DD format"))
			return
		}
		input.IncorporationDate = &d
	}
	profile, err := h.svc.Submit(r.Context(), caller.ID, input)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, kybProfileToResponse(profile))
}

// AdminKYBHandler serves admin KYB review endpoints.
type AdminKYBHandler struct {
	svc service.KYBService
	log *zap.Logger
}

// NewAdminKYBHandler constructs an AdminKYBHandler.
func NewAdminKYBHandler(svc service.KYBService, log *zap.Logger) *AdminKYBHandler {
	return &AdminKYBHandler{svc: svc, log: log}
}

// ListQueue handles GET /api/v1/admin/kyb/queue.
func (h *AdminKYBHandler) ListQueue(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	profiles, err := h.svc.ListPending(r.Context(), limit, offset)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]KYBProfileResponse, 0, len(profiles))
	for _, p := range profiles {
		resp = append(resp, kybProfileToResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetProfile handles GET /api/v1/admin/kyb/profiles/{id}.
func (h *AdminKYBHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	profile, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if profile == nil {
		writeError(w, r, h.log, apperrors.NotFound("kyb profile not found"))
		return
	}
	writeJSON(w, http.StatusOK, kybProfileToResponse(profile))
}

// Approve handles POST /api/v1/admin/kyb/profiles/{id}/approve.
func (h *AdminKYBHandler) Approve(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	if err := h.svc.Approve(r.Context(), id, caller.ID); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

type rejectKYBRequest struct {
	Reason string `json:"reason"`
}

// Reject handles POST /api/v1/admin/kyb/profiles/{id}/reject.
func (h *AdminKYBHandler) Reject(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid profile ID"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, moneyJSONBodyLimit)
	req, err := decodeJSON[rejectKYBRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Reason == "" {
		writeError(w, r, h.log, apperrors.Validation("reason is required for rejection"))
		return
	}
	if err := h.svc.Reject(r.Context(), id, caller.ID, req.Reason); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}
