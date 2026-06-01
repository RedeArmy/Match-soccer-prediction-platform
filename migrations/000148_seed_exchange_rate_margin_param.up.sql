-- Migration 000148: seed payment.exchange_rate_margin_bps system parameter.
--
-- Safety markup applied by ExchangeRateService on top of the raw provider rate
-- before storing it as payment.usd_gtq_rate.  Expressed in basis points
-- (100 bps = 1 %, 200 bps = 2 %).
--
-- Default: 200 (2 % buffer against intraday rate movement).
-- is_runtime=TRUE: adjustable via admin API; the next daily refresh picks up
-- the new value without a worker restart.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'payment.exchange_rate_margin_bps',
    '200',
    '200',
    'int',
    'payment',
    TRUE,
    'Safety markup added by ExchangeRateService on top of the raw USD/GTQ '
    'provider rate before persisting payment.usd_gtq_rate.  Expressed in '
    'basis points (100 bps = 1 %, 200 bps = 2 %).  Valid range: 0–500.  '
    'Takes effect on the next daily exchange-rate refresh without restart.'
)
ON CONFLICT (key) DO NOTHING;
