package handler

import (
	"net/http"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// SessionConfigHandler serves the public session-config endpoint.
// No authentication required — exposes only the max-age seconds value so
// the frontend can read the runtime value of auth.session_max_age_seconds
// without a redeploy. Subject to L1 IP rate limiting on the root router.
type SessionConfigHandler struct {
	params service.SystemParamService
}

// NewSessionConfigHandler constructs a SessionConfigHandler.
func NewSessionConfigHandler(svc service.SystemParamService) *SessionConfigHandler {
	return &SessionConfigHandler{params: svc}
}

// GetConfig handles GET /api/session-config.
// Returns {"session_max_age_seconds": N} where N is the current runtime value
// of auth.session_max_age_seconds (cached ~30 s by SystemParamService).
func (h *SessionConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	secs := h.params.GetInt(r.Context(), domain.ParamKeyAuthSessionMaxAgeSecs, domain.DefaultAuthSessionMaxAgeSecs)
	writeJSON(w, http.StatusOK, map[string]int{"session_max_age_seconds": secs})
}
