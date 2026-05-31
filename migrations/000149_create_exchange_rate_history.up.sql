-- Migration 000149: exchange_rate_history table.
--
-- Immutable append-only record of every USD/GTQ rate determination: automated
-- daily fetches and admin overrides.  One row is inserted per RefreshRate or
-- OverrideRate call.  The most recent row is the authoritative current rate.
--
-- Rates are stored as NUMERIC to avoid float64 precision loss at the boundary
-- between the Go decimal.Decimal type and PostgreSQL.
--
-- buy_margin_pct / sell_margin_pct store the margin fractions at the time of
-- computation (e.g. 0.015000 = 1.5%) so historical rows remain self-contained
-- even if the fx.buy_margin_bps / fx.sell_margin_bps params are later changed.

CREATE TABLE exchange_rate_history (
    id              BIGSERIAL       PRIMARY KEY,
    reference_rate  NUMERIC(12,6)   NOT NULL,
    buy_rate        NUMERIC(12,6)   NOT NULL,
    sell_rate       NUMERIC(12,6)   NOT NULL,
    buy_margin_pct  NUMERIC(8,6)    NOT NULL,
    sell_margin_pct NUMERIC(8,6)    NOT NULL,
    source          VARCHAR(50)     NOT NULL,
    stale           BOOLEAN         NOT NULL DEFAULT FALSE,
    is_override     BOOLEAN         NOT NULL DEFAULT FALSE,
    override_reason TEXT,
    override_by     INT REFERENCES users(id) ON DELETE SET NULL,
    fetched_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    effective_at    TIMESTAMPTZ     NOT NULL,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_exchange_rate_history_effective_at
    ON exchange_rate_history (effective_at DESC);

CREATE INDEX idx_exchange_rate_history_is_override
    ON exchange_rate_history (is_override)
    WHERE is_override = TRUE;
