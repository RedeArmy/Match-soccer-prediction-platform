DELETE FROM system_params
WHERE key IN ('fx.buy_margin_bps', 'fx.sell_margin_bps', 'fx.display_decimals', 'fx.stale_threshold_h');
