package handler

import (
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ExtraPredictionHandler handles HTTP requests for the /api/v1/extras resource
// (match extras: bonus predictions beyond the scoreline).
type ExtraPredictionHandler struct {
	svc service.ExtraPredictionService
	log *zap.Logger
}

// NewExtraPredictionHandler constructs an ExtraPredictionHandler.
func NewExtraPredictionHandler(svc service.ExtraPredictionService, log *zap.Logger) *ExtraPredictionHandler {
	return &ExtraPredictionHandler{svc: svc, log: log}
}

// submitExtraPredictionRequest is the JSON body accepted by POST /api/v1/extras.
// extra_type must be one of: "first_scorer", "halftime_result". The allowed
// values for answer depend on extra_type — see domain.ValidateExtraAnswer.
type submitExtraPredictionRequest struct {
	MatchID   int    `json:"match_id"`
	ExtraType string `json:"extra_type"`
	Answer    string `json:"answer"`
}

// Submit handles POST /api/v1/extras.
//
// @Summary      Submit a match extra guess
// @Description  Submits (or updates) a guess for one match extra — a bonus
//
//	prediction beyond the scoreline. The user identity is taken from
//	the Bearer token. Only accepted while the match is still scheduled
//	and before the prediction deadline, the same lock rule enforced on
//	POST /api/v1/predictions.
//
// @Tags         extras
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.submitExtraPredictionRequest  true  "Extra guess"
// @Success      200   {object}  handler.ExtraPredictionResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      404   {object}  handler.ErrorResponse  "Match not found"
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/extras [post]
func (h *ExtraPredictionHandler) Submit(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[submitExtraPredictionRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	pred, err := h.svc.Submit(r.Context(), caller.ID, req.MatchID, domain.ExtraType(req.ExtraType), req.Answer)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, extraPredictionToResponse(pred))
}

// GetMine handles GET /api/v1/extras/me?match_ids=1,2,3.
//
// @Summary      List my match extra guesses
// @Description  Returns the authenticated caller's extra guesses for the given
//
//	comma-separated list of match IDs. Used by the match-list view to
//	bulk-fetch every extra without an N+1 query per match.
//
// @Tags         extras
// @Produce      json
// @Security     BearerAuth
// @Param        match_ids  query     string  true  "Comma-separated match IDs"
// @Success      200        {array}   handler.ExtraPredictionResponse
// @Failure      401        {object}  handler.ErrorResponse
// @Failure      422        {object}  handler.ErrorResponse  "Invalid match_ids"
// @Failure      500        {object}  handler.ErrorResponse
// @Router       /api/v1/extras/me [get]
func (h *ExtraPredictionHandler) GetMine(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	matchIDs, err := parseCommaSeparatedIDs(r.URL.Query().Get("match_ids"))
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	preds, err := h.svc.ListByUserAndMatches(r.Context(), caller.ID, matchIDs)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	out := make([]ExtraPredictionResponse, len(preds))
	for i, p := range preds {
		out[i] = extraPredictionToResponse(p)
	}
	writeJSON(w, http.StatusOK, out)
}

// parseCommaSeparatedIDs parses a comma-separated list of positive integers.
// An empty input string returns an empty (nil) slice, not an error.
func parseCommaSeparatedIDs(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 {
			return nil, apperrors.Validation("invalid ID in match_ids")
		}
		ids = append(ids, n)
	}
	return ids, nil
}
