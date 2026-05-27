package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// KYCHandler serves user-facing KYC endpoints.
type KYCHandler struct {
	svc            service.KYCService
	fileStore      storage.FileStore
	maxUploadBytes int64
	log            *zap.Logger
}

// NewKYCHandler constructs a KYCHandler.
// fileStore and maxUploadBytes are required for the document upload endpoint.
// Pass maxUploadBytes = domain.DefaultKYCMaxDocUploadBytes when not configured at startup.
func NewKYCHandler(svc service.KYCService, fileStore storage.FileStore, maxUploadBytes int64, log *zap.Logger) *KYCHandler {
	if maxUploadBytes <= 0 {
		maxUploadBytes = int64(domain.DefaultKYCMaxDocUploadBytes)
	}
	return &KYCHandler{
		svc:            svc,
		fileStore:      fileStore,
		maxUploadBytes: maxUploadBytes,
		log:            log,
	}
}

// GetStatus handles GET /api/v1/kyc/status.
// Returns the authenticated user's KYC profile, or a placeholder when none exists.
func (h *KYCHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	profile, err := h.svc.GetProfile(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if profile == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": string(domain.KYCStatusUnverified),
			"tier":   0,
		})
		return
	}
	writeJSON(w, http.StatusOK, kycProfileToResponse(profile))
}

// Submit handles POST /api/v1/kyc/submit.
// Creates or updates the user's KYC profile.
type submitKYCRequest struct {
	FullName       string `json:"full_name"`
	DateOfBirth    string `json:"date_of_birth"` // YYYY-MM-DD
	Nationality    string `json:"nationality"`
	DocumentType   string `json:"document_type"`
	DocumentNumber string `json:"document_number"`
	AddressLine    string `json:"address_line"`
	City           string `json:"city"`
	Country        string `json:"country"`
	PostalCode     string `json:"postal_code"`
}

// Submit handles POST /api/v1/kyc/submit.
func (h *KYCHandler) Submit(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, moneyJSONBodyLimit)
	req, err := decodeJSON[submitKYCRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	svcReq := service.SubmitKYCRequest{
		FullName:       req.FullName,
		Nationality:    req.Nationality,
		DocumentType:   domain.KYCDocumentType(req.DocumentType),
		DocumentNumber: req.DocumentNumber,
		AddressLine:    req.AddressLine,
		City:           req.City,
		Country:        req.Country,
		PostalCode:     req.PostalCode,
		SubmissionIP:   repository.ClientIPFromContext(r.Context()),
	}
	if req.DateOfBirth != "" {
		dob, err := time.Parse("2006-01-02", req.DateOfBirth)
		if err != nil {
			writeError(w, r, h.log, apperrors.Validation("date_of_birth must be in YYYY-MM-DD format"))
			return
		}
		svcReq.DateOfBirth = &dob
	}
	profile, err := h.svc.Submit(r.Context(), caller.ID, svcReq)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, kycProfileToResponse(profile))
}

// GetRequirements handles GET /api/v1/kyc/requirements.
func (h *KYCHandler) GetRequirements(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	reqs, err := h.svc.GetRequirements(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, kycRequirementsToResponse(reqs))
}

// ListDocuments handles GET /api/v1/kyc/documents.
func (h *KYCHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	docs, err := h.svc.ListDocuments(r.Context(), caller.ID)
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

// ListEvents handles GET /api/v1/kyc/events.
func (h *KYCHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	profile, err := h.svc.GetProfile(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if profile == nil {
		writeJSON(w, http.StatusOK, map[string]any{"events": []any{}, "next_cursor": ""})
		return
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	events, next, err := h.svc.ListEvents(r.Context(), profile.ID, domain.KYCProfileTypeUser,
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

// UploadDocument handles POST /api/v1/kyc/documents.
// Accepts a multipart/form-data upload with fields:
//
//	document_type (string) — one of: gov_id, selfie, proof_of_address, proof_of_funds
//	file          (binary) — the identity document image or PDF
func (h *KYCHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	// Guard: the user must have submitted a KYC profile before uploading.
	profile, err := h.svc.GetProfile(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if profile == nil {
		writeError(w, r, h.log, apperrors.Forbidden("ERR_KYC_PROFILE_REQUIRED: submit a KYC profile before uploading documents"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		writeError(w, r, h.log, apperrors.RequestBodyTooLarge())
		return
	}

	rawDocType := r.FormValue("document_type")
	docType := domain.KYCDocumentType(rawDocType)
	switch docType {
	case domain.KYCDocGovID, domain.KYCDocSelfie, domain.KYCDocProofOfAddress, domain.KYCDocProofOfFunds:
		// valid
	default:
		writeError(w, r, h.log, apperrors.Validation(
			"document_type must be one of: gov_id, selfie, proof_of_address, proof_of_funds",
		))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("file is required"))
		return
	}
	defer func() { _ = file.Close() }()

	contentType := header.Header.Get("Content-Type")
	if !domain.KYCAllowedContentTypes[contentType] {
		writeError(w, r, h.log, apperrors.Validation(
			"unsupported file type; allowed: image/jpeg, image/png, image/webp, application/pdf",
		))
		return
	}

	ext := extensionForContentType(contentType)
	storageKey := fmt.Sprintf("kyc/%d/%s%s", caller.ID, generateID(), ext)

	// Compute SHA-256 while streaming to the FileStore — single pass, no temp buffer.
	hasher := sha256.New()
	tee := io.TeeReader(file, hasher)
	if err := h.fileStore.Put(r.Context(), storageKey, contentType, tee, header.Size); err != nil {
		writeError(w, r, h.log, apperrors.Internal(err))
		return
	}
	fileHash := hex.EncodeToString(hasher.Sum(nil))

	doc, err := h.svc.UploadDocument(r.Context(), caller.ID, service.UploadDocRequest{
		ProfileID:    profile.ID,
		ProfileType:  domain.KYCProfileTypeUser,
		DocumentType: docType,
		StorageKey:   storageKey,
		ContentType:  contentType,
		FileSize:     int(header.Size),
		FileHash:     fileHash,
	})
	if err != nil {
		_ = h.fileStore.Delete(r.Context(), storageKey) // best-effort cleanup
		writeError(w, r, h.log, err)
		return
	}

	writeJSON(w, http.StatusCreated, kycDocumentToResponse(doc))
}
