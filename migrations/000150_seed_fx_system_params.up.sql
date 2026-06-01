-- Migration 000150: seed fx.* system parameters for the competitive margin engine.
--
-- fx.buy_margin_bps  — platform buys USD from users below reference rate.
--   150 bps = 1.5%: example 7.75 × 0.985 = 7.6338 GTQ/USD.
--
-- fx.sell_margin_bps — platform sells USD to users above reference rate.
--   200 bps = 2.0%: example 7.75 × 1.020 = 7.9050 GTQ/USD.
--   Note: payment.usd_gtq_rate is derived from the sell rate (GTQ reserved
--   for USD withdrawals), so sell_margin_bps supersedes exchange_rate_margin_bps
--   from migration 000148 for the ExchangeRateService computation.
--
-- fx.display_decimals — decimal places shown to users in rate display endpoints.
-- fx.stale_threshold_h — hours before an unrefreshed rate triggers an n8n alert.
--
-- All four params are is_runtime=TRUE so operators can adjust them via the
-- admin API and the next scheduled RefreshRate picks up the new values.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'fx.buy_margin_bps',
        '150',
        '150',
        'int',
        'fx',
        TRUE,
        'Buy-side FX margin in basis points (100 bps = 1%). Platform buys USD '
        'from users at ReferenceRate × (1 - margin). Default: 150 (1.5%). '
        'Valid range: 0–500. Takes effect on the next daily RefreshRate call.'
    ),
    (
        'fx.sell_margin_bps',
        '200',
        '200',
        'int',
        'fx',
        TRUE,
        'Sell-side FX margin in basis points (100 bps = 1%). Platform sells USD '
        'to users at ReferenceRate × (1 + margin). Default: 200 (2.0%). '
        'Also drives payment.usd_gtq_rate (GTQ centavos per USD for withdrawal '
        'balance reservation). Valid range: 0–500.'
    ),
    (
        'fx.display_decimals',
        '4',
        '4',
        'int',
        'fx',
        TRUE,
        'Decimal places shown to users when displaying buy/sell FX rates '
        '(e.g. 4 → "7.6338"). Valid range: 2–8.'
    ),
    (
        'fx.stale_threshold_h',
        '26',
        '26',
        'int',
        'fx',
        TRUE,
        'Hours after the last successful RefreshRate call before the cached '
        'rate is declared stale and an n8n /webhook/fx-rate-stale alert fires. '
        'Default: 26 h (2 h grace window past the next daily scheduled run). '
        'Valid range: 1–72.'
    )
ON CONFLICT (key) DO NOTHING;
