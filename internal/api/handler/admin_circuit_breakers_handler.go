package handler

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

// BreakerLister is the narrow interface the circuit breakers handler requires
// from the breaker registry. The concrete *breaker.Registry satisfies this.
type BreakerLister interface {
	All() []*breaker.Breaker
}

// AdminCircuitBreakersHandler exposes the current state of all registered
// circuit breakers to admin operators.
type AdminCircuitBreakersHandler struct {
	registry BreakerLister
	log      *zap.Logger
}

// NewAdminCircuitBreakersHandler constructs the handler.
func NewAdminCircuitBreakersHandler(registry BreakerLister, log *zap.Logger) *AdminCircuitBreakersHandler {
	return &AdminCircuitBreakersHandler{registry: registry, log: log}
}

type circuitBreakerEntry struct {
	Name     string     `json:"name"`
	State    string     `json:"state"`
	OpenedAt *time.Time `json:"opened_at,omitempty"`
}

type circuitBreakersResponse struct {
	Breakers []circuitBreakerEntry `json:"breakers"`
}

// List handles GET /admin/observability/circuit-breakers.
//
// @Summary      Circuit breaker states
// @Description  Returns the current state (closed/open/half-open) and, for open
//
//	breakers, the time they were last opened for every registered circuit
//	breaker in this replica.
//
// @Tags         admin-observability
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.circuitBreakersResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/observability/circuit-breakers [get]
func (h *AdminCircuitBreakersHandler) List(w http.ResponseWriter, _ *http.Request) {
	all := h.registry.All()
	entries := make([]circuitBreakerEntry, len(all))
	for i, b := range all {
		entry := circuitBreakerEntry{
			Name:  b.Name(),
			State: b.CurrentState().String(),
		}
		if b.CurrentState() != breaker.StateClosed {
			openedAt := b.OpenedAt()
			if !openedAt.IsZero() {
				entry.OpenedAt = &openedAt
			}
		}
		entries[i] = entry
	}
	writeJSON(w, http.StatusOK, circuitBreakersResponse{Breakers: entries})
}
