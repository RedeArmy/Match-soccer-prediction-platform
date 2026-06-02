-- Migration 000161: seed HTTP client timeout params for the three FX data sources.
--
-- Promotes three previously hardcoded timeouts to system_params so that
-- operators can tune failover speed without a worker rebuild:
--
--   fx.banguat_timeout_sec           (was banguatTimeout = 10s)
--   fx.exchange_rate_api_timeout_sec (was exchangeRateAPITimeout = 5s)
--   fx.open_exchange_timeout_sec     (was openExchangeTimeout = 5s)
--
-- All three are is_runtime=FALSE: the HTTP clients are constructed once at
-- worker startup. A worker restart is required to apply changed values.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'fx.banguat_timeout_sec',
        '10',
        '10',
        'int',
        'fx',
        FALSE,
        'HTTP request timeout in seconds for the Banguat XML feed (the primary USD/GTQ source). Raise when the feed is slow; lower to fail over to the next source faster. Requires worker restart.'
    ),
    (
        'fx.exchange_rate_api_timeout_sec',
        '5',
        '5',
        'int',
        'fx',
        FALSE,
        'HTTP request timeout in seconds for v6.exchangerate-api.com (the first fallback USD/GTQ source). Requires worker restart.'
    ),
    (
        'fx.open_exchange_timeout_sec',
        '5',
        '5',
        'int',
        'fx',
        FALSE,
        'HTTP request timeout in seconds for openexchangerates.org (the second fallback USD/GTQ source). Requires worker restart.'
    )
ON CONFLICT (key) DO NOTHING;
