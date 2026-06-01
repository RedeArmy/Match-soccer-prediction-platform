package handler

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/service"
)

// ExchangeRateHandler serves the public exchange rate display endpoint.
// No authentication required — rate display is public information.
// Subject to L1 IP rate limiting (applied globally on the root router).
type ExchangeRateHandler struct {
	svc service.ExchangeRateService
	log *zap.Logger
}

// NewExchangeRateHandler constructs an ExchangeRateHandler.
func NewExchangeRateHandler(svc service.ExchangeRateService, log *zap.Logger) *ExchangeRateHandler {
	return &ExchangeRateHandler{svc: svc, log: log}
}

// publicExchangeRateResponse is the public-facing rate response.
// Reference rate and margin percentages are intentionally omitted —
// they are internal business parameters.
type publicExchangeRateResponse struct {
	BuyRate     string `json:"buy_rate"`
	SellRate    string `json:"sell_rate"`
	EffectiveAt string `json:"effective_at"`
	Stale       bool   `json:"stale"`
}

// GetCurrent handles GET /api/exchange-rate.
// Returns the current buy/sell rates for frontend display.
// Never calls an external API — reads from the service cache.
func (h *ExchangeRateHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	rates, err := h.svc.GetCurrentRates(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, publicExchangeRateResponse{
		BuyRate:     rates.BuyRate.String(),
		SellRate:    rates.SellRate.String(),
		EffectiveAt: rates.EffectiveAt.UTC().Format(time.RFC3339),
		Stale:       rates.Stale,
	})
}
