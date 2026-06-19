package handler

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/service"
)

// FeatureFlagHandler serves the public feature-flag endpoint.
// No authentication required — flags control frontend visibility only.
// Subject to L1 IP rate limiting applied globally on the root router.
type FeatureFlagHandler struct {
	svc service.SystemParamService
	log *zap.Logger
}

// NewFeatureFlagHandler constructs a FeatureFlagHandler.
func NewFeatureFlagHandler(svc service.SystemParamService, log *zap.Logger) *FeatureFlagHandler {
	return &FeatureFlagHandler{svc: svc, log: log}
}

// GetAll handles GET /api/feature-flags.
// Returns all system params whose key begins with "featflag." as a flat
// JSON object keyed by param name (without the "featflag." prefix).
func (h *FeatureFlagHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	const prefix = "featflag."

	params, err := h.svc.GetAll(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	flags := make(map[string]string)
	for _, p := range params {
		if strings.HasPrefix(p.Key, prefix) {
			flags[strings.TrimPrefix(p.Key, prefix)] = p.Value
		}
	}

	writeJSON(w, http.StatusOK, flags)
}
