package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminExchangeRateHandler handles admin endpoints for exchange rate management.
type AdminExchangeRateHandler struct {
	svc    service.ExchangeRateService
	fxRepo repository.ExchangeRateRepository
	audit  service.AuditLogger
	log    *zap.Logger
}

// NewAdminExchangeRateHandler constructs an AdminExchangeRateHandler.
func NewAdminExchangeRateHandler(
	svc service.ExchangeRateService,
	fxRepo repository.ExchangeRateRepository,
	audit service.AuditLogger,
	log *zap.Logger,
) *AdminExchangeRateHandler {
	return &AdminExchangeRateHandler{svc: svc, fxRepo: fxRepo, audit: audit, log: log}
}

// ── Response types ────────────────────────────────────────────────────────────

// ExchangeRatesResponse is the JSON representation of domain.ExchangeRates.
type ExchangeRatesResponse struct {
	ReferenceRate string `json:"reference_rate"`
	BuyRate       string `json:"buy_rate"`
	SellRate      string `json:"sell_rate"`
	BuyMarginPct  string `json:"buy_margin_pct"`
	SellMarginPct string `json:"sell_margin_pct"`
	Source        string `json:"source"`
	Stale         bool   `json:"stale"`
	IsOverride    bool   `json:"is_override"`
	EffectiveAt   string `json:"effective_at"`
}

// ExchangeRateRecordResponse is the JSON representation of a history row.
type ExchangeRateRecordResponse struct {
	ID             int64  `json:"id"`
	ReferenceRate  string `json:"reference_rate"`
	BuyRate        string `json:"buy_rate"`
	SellRate       string `json:"sell_rate"`
	BuyMarginPct   string `json:"buy_margin_pct"`
	SellMarginPct  string `json:"sell_margin_pct"`
	Source         string `json:"source"`
	Stale          bool   `json:"stale"`
	IsOverride     bool   `json:"is_override"`
	OverrideReason string `json:"override_reason,omitempty"`
	OverrideBy     *int   `json:"override_by,omitempty"`
	FetchedAt      string `json:"fetched_at"`
	EffectiveAt    string `json:"effective_at"`
}

func ratesToResponse(r *domain.ExchangeRates) ExchangeRatesResponse {
	return ExchangeRatesResponse{
		ReferenceRate: r.ReferenceRate.String(),
		BuyRate:       r.BuyRate.String(),
		SellRate:      r.SellRate.String(),
		BuyMarginPct:  r.BuyMarginPct.String(),
		SellMarginPct: r.SellMarginPct.String(),
		Source:        r.Source,
		Stale:         r.Stale,
		IsOverride:    r.IsOverride,
		EffectiveAt:   r.EffectiveAt.UTC().Format(time.RFC3339),
	}
}

func rateRecordToResponse(r *domain.ExchangeRateRecord) ExchangeRateRecordResponse {
	resp := ExchangeRateRecordResponse{
		ID:             r.ID,
		ReferenceRate:  r.ReferenceRate.String(),
		BuyRate:        r.BuyRate.String(),
		SellRate:       r.SellRate.String(),
		BuyMarginPct:   r.BuyMarginPct.String(),
		SellMarginPct:  r.SellMarginPct.String(),
		Source:         r.Source,
		Stale:          r.Stale,
		IsOverride:     r.IsOverride,
		OverrideReason: r.OverrideReason,
		OverrideBy:     r.OverrideBy,
		FetchedAt:      r.FetchedAt.UTC().Format(time.RFC3339),
		EffectiveAt:    r.EffectiveAt.UTC().Format(time.RFC3339),
	}
	return resp
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GetCurrent handles GET /api/v1/admin/exchange-rate/current.
//
// @Summary      Get current exchange rate (admin)
// @Description  Returns the current GTQ/USD rates with full detail: reference
//
//	rate, buy/sell rates, margin percentages, source, staleness flag,
//	and whether the current rate is an admin override. Requires admin role.
//
// @Tags         admin-exchange-rate
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.ExchangeRatesResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/exchange-rate/current [get]
func (h *AdminExchangeRateHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	rates, err := h.svc.GetCurrentRates(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, ratesToResponse(rates))
}

// GetHistory handles GET /api/v1/admin/exchange-rate/history.
//
// @Summary      List exchange rate history (admin)
// @Description  Returns a paginated list of all GTQ/USD rate records, newest
//
//	first. Each row captures the rates and margins at the time of the
//	automated refresh or admin override. Requires admin role.
//
// @Tags         admin-exchange-rate
// @Produce      json
// @Security     BearerAuth
// @Param        limit   query     int  false  "Maximum records to return (1–200, default 50)"
// @Param        offset  query     int  false  "Number of records to skip (default 0)"
// @Success      200  {array}   handler.ExchangeRateRecordResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/exchange-rate/history [get]
func (h *AdminExchangeRateHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePaginationParams(r)
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	records, err := h.fxRepo.GetHistory(r.Context(), limit, offset)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	resp := make([]ExchangeRateRecordResponse, len(records))
	for i, rec := range records {
		resp[i] = rateRecordToResponse(rec)
	}
	writeJSON(w, http.StatusOK, resp)
}

// overrideRateRequest is the request body for POST /admin/exchange-rate/override.
type overrideRateRequest struct {
	// ReferenceRate is the new reference rate as a string to avoid float64
	// precision loss at the JSON boundary.  Parsed as decimal.Decimal.
	// Must be in the range (0, 20] — reasonable GTQ/USD bounds.
	ReferenceRate string `json:"reference_rate"`
	// Reason is required (minimum 10 characters) and is stored in
	// exchange_rate_history.override_reason for audit purposes.
	Reason string `json:"reason"`
}

// Override handles POST /api/v1/admin/exchange-rate/override.
//
// @Summary      Override exchange rate (admin)
// @Description  Sets a manual GTQ/USD reference rate, bypassing the automated
//
//	daily fetch. The provided rate is stored with is_override=true in
//	exchange_rate_history. A reason of at least 10 characters is
//	required and recorded for audit purposes. The cache is invalidated
//	immediately so subsequent requests use the new rate. Requires admin role.
//
// @Tags         admin-exchange-rate
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.overrideRateRequest  true  "New reference rate and reason"
// @Success      200  {object}  handler.ExchangeRatesResponse
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid rate or reason too short"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/exchange-rate/override [post]
func (h *AdminExchangeRateHandler) Override(w http.ResponseWriter, r *http.Request) {
	var req overrideRateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid JSON: "+err.Error()))
		return
	}
	if len(req.Reason) < 10 {
		writeError(w, r, h.log, apperrors.Validation("reason must be at least 10 characters"))
		return
	}
	ref, err := decimal.NewFromString(req.ReferenceRate)
	if err != nil || ref.LessThanOrEqual(decimal.Zero) || ref.GreaterThan(decimal.NewFromInt(20)) {
		writeError(w, r, h.log, apperrors.Validation(
			"reference_rate must be a valid decimal in range (0, 20]"))
		return
	}

	admin, ok := middleware.UserFromContext(r.Context())
	if !ok || admin == nil {
		writeError(w, r, h.log, apperrors.Unauthorised("admin identity required"))
		return
	}

	rates, err := h.svc.OverrideRate(r.Context(), ref, req.Reason, admin.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	resType := "exchange_rate"
	h.audit.Log(r.Context(), &admin.ID, &admin.Role,
		domain.AuditActionFXRateOverride, &resType, nil,
		map[string]any{
			"reference_rate": req.ReferenceRate,
			"sell_rate":      rates.SellRate.String(),
			"reason":         req.Reason,
		},
	)

	writeJSON(w, http.StatusOK, ratesToResponse(rates))
}

// Refresh handles POST /api/v1/admin/exchange-rate/refresh.
//
// @Summary      Refresh exchange rate (admin)
// @Description  Triggers an immediate GTQ/USD rate fetch from the configured
//
//	external sources (Banguat → ExchangeRateAPI → OpenExchange fallback
//	chain), outside the normal daily scheduled window. Useful for manual
//	correction after a data outage or when the stale flag is set.
//	Requires admin role.
//
// @Tags         admin-exchange-rate
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.ExchangeRatesResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse  "All external sources failed"
// @Router       /api/v1/admin/exchange-rate/refresh [post]
func (h *AdminExchangeRateHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	rates, err := h.svc.RefreshRate(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, ratesToResponse(rates))
}
