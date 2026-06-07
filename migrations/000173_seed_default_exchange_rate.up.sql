-- Migration 000173: seed a default GTQ/USD exchange rate so the
-- public /api/exchange-rate endpoint has a value to return before
-- the automated daily fetch runs for the first time.
INSERT INTO exchange_rate_history (
    reference_rate,
    buy_rate,
    sell_rate,
    buy_margin_pct,
    sell_margin_pct,
    source,
    stale,
    is_override,
    override_reason,
    effective_at
)
SELECT
    7.750000,   -- reference GTQ per USD (Banguat mid-rate, June 2026)
    7.595000,   -- buy rate  (reference × 0.98, 2 % margin)
    7.905000,   -- sell rate (reference × 1.02, 2 % margin)
    0.020000,
    0.020000,
    'seed',
    FALSE,
    TRUE,
    'Initial seed rate — replaced automatically by the daily exchange-rate worker',
    NOW()
WHERE NOT EXISTS (SELECT 1 FROM exchange_rate_history LIMIT 1);
