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

// SessionConfigResponse is the body returned by GET /api/session-config.
type SessionConfigResponse struct {
	SessionMaxAgeSecs int `json:"session_max_age_seconds"`
}

// GetConfig handles GET /api/session-config.
//
// @Summary      Get session configuration
// @Description  Returns the current runtime value of auth.session_max_age_seconds.
// @Description  No authentication required. Used by the frontend to schedule
// @Description  a proactive client-side session expiry timer without a redeploy.
// @Tags         session
// @Produce      json
// @Success      200  {object}  handler.SessionConfigResponse
// @Failure      429  {object}  handler.ErrorResponse  "Rate limit exceeded"
// @Router       /api/session-config [get]
func (h *SessionConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	secs := h.params.GetInt(r.Context(), domain.ParamKeyAuthSessionMaxAgeSecs, domain.DefaultAuthSessionMaxAgeSecs)
	writeJSON(w, http.StatusOK, SessionConfigResponse{SessionMaxAgeSecs: secs})
}
