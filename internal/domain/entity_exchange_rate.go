package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// ExchangeRates is the in-memory representation of the current USD/GTQ rates
// after margin computation. Used by the service cache and API response types.
// All arithmetic uses decimal.Decimal to avoid float64 precision loss.
type ExchangeRates struct {
	// ReferenceRate is the raw sell rate published by the provider (GTQ per USD).
	// Example: 7.7500
	ReferenceRate decimal.Decimal
	// BuyRate is what the platform pays to acquire USD from users (below reference).
	// BuyRate = ReferenceRate × (1 − BuyMarginPct)
	// Example: 7.7500 × 0.985 = 7.6338
	BuyRate decimal.Decimal
	// SellRate is what the platform charges users for USD (above reference).
	// SellRate = ReferenceRate × (1 + SellMarginPct)
	// Example: 7.7500 × 1.020 = 7.9050
	SellRate decimal.Decimal
	// BuyMarginPct is the applied buy margin fraction (e.g., 0.0150 = 1.5%).
	BuyMarginPct decimal.Decimal
	// SellMarginPct is the applied sell margin fraction (e.g., 0.0200 = 2.0%).
	SellMarginPct decimal.Decimal
	// Source identifies the data provider: "banguat", "exchangerate-api",
	// "openexchangerates", "stale", or "override".
	Source string
	// Stale is true when all external sources failed and the last known rate is
	// being used. Payment operations remain available; an alert fires via n8n.
	Stale bool
	// IsOverride is true when an admin manually set the reference rate.
	IsOverride bool
	// EffectiveAt is when the rate became active (fetched_at or override time).
	EffectiveAt time.Time
}

// ExchangeRateRecord is the persisted form stored in exchange_rate_history.
// One row is inserted per RefreshRate or OverrideRate call.
type ExchangeRateRecord struct {
	ID             int64
	ReferenceRate  decimal.Decimal
	BuyRate        decimal.Decimal
	SellRate       decimal.Decimal
	BuyMarginPct   decimal.Decimal
	SellMarginPct  decimal.Decimal
	Source         string
	Stale          bool
	IsOverride     bool
	OverrideReason string // non-empty only when IsOverride=true
	OverrideBy     *int   // admin user ID; nil for automated fetches
	FetchedAt      time.Time
	EffectiveAt    time.Time
	CreatedAt      time.Time
}

// ToRates converts a persisted record to the in-memory ExchangeRates shape.
func (r *ExchangeRateRecord) ToRates() *ExchangeRates {
	return &ExchangeRates{
		ReferenceRate: r.ReferenceRate,
		BuyRate:       r.BuyRate,
		SellRate:      r.SellRate,
		BuyMarginPct:  r.BuyMarginPct,
		SellMarginPct: r.SellMarginPct,
		Source:        r.Source,
		Stale:         r.Stale,
		IsOverride:    r.IsOverride,
		EffectiveAt:   r.EffectiveAt,
	}
}
