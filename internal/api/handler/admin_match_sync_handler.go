package handler

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/service"
)

// AdminMatchSyncHandler handles admin operations for the automated match-sync
// subsystem: linking fixtures to external IDs, triggering manual sync cycles,
// and querying reconciliation diffs.
type AdminMatchSyncHandler struct {
	svc service.MatchSyncer
	log *zap.Logger
}

// NewAdminMatchSyncHandler constructs an AdminMatchSyncHandler.
func NewAdminMatchSyncHandler(svc service.MatchSyncer, log *zap.Logger) *AdminMatchSyncHandler {
	return &AdminMatchSyncHandler{svc: svc, log: log}
}

// ── Request / response types ──────────────────────────────────────────────────

type linkExternalRequest struct {
	Provider   string `json:"provider"`
	ExternalID int64  `json:"external_id"`
}

type linkExternalResponse struct {
	MatchID    int    `json:"match_id"`
	Provider   string `json:"provider"`
	ExternalID int64  `json:"external_id"`
}

type pollResponse struct {
	LiveCount int `json:"live_count"`
}

type syncDiffItem struct {
	MatchID        int    `json:"match_id"`
	InternalStatus string `json:"internal_status"`
	ExternalStatus string `json:"external_status"`
	InternalHome   *int   `json:"internal_home"`
	InternalAway   *int   `json:"internal_away"`
	ExternalHome   int    `json:"external_home"`
	ExternalAway   int    `json:"external_away"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// LinkExternal handles POST /api/v1/admin/matches/{id}/external-link
//
// @Summary      Link match to external provider fixture
// @Description  Admin only. Associates the internal match with a provider fixture ID so
//               the sync worker can poll and automatically apply results.
// @Tags         admin-match-sync
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                     true  "Internal match ID"
// @Param        body  body      handler.linkExternalRequest  true  "Provider and external fixture ID"
// @Success      200   {object}  handler.linkExternalResponse
// @Failure      404   {object}  handler.ErrorResponse
// @Failure      422   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/matches/{id}/external-link [post]
func (h *AdminMatchSyncHandler) LinkExternal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	req, err := decodeJSON[linkExternalRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if err := h.svc.LinkExternal(r.Context(), id, req.Provider, req.ExternalID); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, linkExternalResponse{
		MatchID:    id,
		Provider:   req.Provider,
		ExternalID: req.ExternalID,
	})
}

// UnlinkExternal handles DELETE /api/v1/admin/matches/{id}/external-link
//
// @Summary      Remove external provider link from a match
// @Description  Admin only. Reverts the match to manual-only management.
// @Tags         admin-match-sync
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Internal match ID"
// @Success      204
// @Failure      404   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/matches/{id}/external-link [delete]
func (h *AdminMatchSyncHandler) UnlinkExternal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if err := h.svc.UnlinkExternal(r.Context(), id); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TriggerPoll handles POST /api/v1/admin/match-sync/poll
//
// @Summary      Trigger a manual match-sync poll cycle
// @Description  Admin only. Immediately runs PollAndApply outside the scheduler
//               interval, useful for ad-hoc result ingestion or smoke tests.
// @Tags         admin-match-sync
// @Produce      json
// @Security     BearerAuth
// @Success      200   {object}  handler.pollResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/match-sync/poll [post]
func (h *AdminMatchSyncHandler) TriggerPoll(w http.ResponseWriter, r *http.Request) {
	liveCount, err := h.svc.PollAndApply(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, pollResponse{LiveCount: liveCount})
}

// Reconcile handles GET /api/v1/admin/match-sync/reconcile
//
// @Summary      Reconcile local match state against external provider
// @Description  Admin only. Compares every linked, non-finished match against the provider
//               and returns a list of discrepancies. No writes are performed.
// @Tags         admin-match-sync
// @Produce      json
// @Security     BearerAuth
// @Param        league_id  query     int  false  "API-Football league ID (default: 1)"
// @Param        season     query     int  false  "Season year (default: 2026)"
// @Success      200   {array}   handler.syncDiffItem
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/match-sync/reconcile [get]
func (h *AdminMatchSyncHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	leagueID := queryIntDefault(r, "league_id", 1)
	season := queryIntDefault(r, "season", 2026)

	diffs, err := h.svc.ReconcileDate(r.Context(), leagueID, season)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	items := make([]syncDiffItem, 0, len(diffs))
	for _, d := range diffs {
		items = append(items, syncDiffItem{
			MatchID:        d.MatchID,
			InternalStatus: string(d.InternalStatus),
			ExternalStatus: string(d.ExternalStatus),
			InternalHome:   d.InternalHome,
			InternalAway:   d.InternalAway,
			ExternalHome:   d.ExternalHome,
			ExternalAway:   d.ExternalAway,
		})
	}
	writeJSON(w, http.StatusOK, items)
}

// queryIntDefault reads a query parameter as an int, returning defaultVal when
// the parameter is absent or unparseable.
func queryIntDefault(r *http.Request, param string, defaultVal int) int {
	raw := r.URL.Query().Get(param)
	if raw == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return n
}
