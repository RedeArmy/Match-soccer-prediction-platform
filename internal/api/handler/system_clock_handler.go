package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/clock"
)

// SystemClockHandler exposes the application clock so clients can determine the
// current "system time" even when a dev override is active via system.date.
type SystemClockHandler struct {
	clk clock.Nower
	log *zap.Logger
}

// NewSystemClockHandler constructs a SystemClockHandler. clk should be the same
// ParamClock (or Real{} in production) used by the rest of the application.
func NewSystemClockHandler(clk clock.Nower, log *zap.Logger) *SystemClockHandler {
	return &SystemClockHandler{clk: clk, log: log}
}

type systemClockResponse struct {
	Now string `json:"now"`
}

// GetClock returns the current application time as an RFC 3339 string.
// In production this is always the real wall-clock time. In development it
// reflects the system.date param override if one is configured.
//
// GET /api/v1/system/clock
func (h *SystemClockHandler) GetClock(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, systemClockResponse{
		Now: h.clk.Now().Format("2006-01-02T15:04:05Z07:00"),
	})
}
