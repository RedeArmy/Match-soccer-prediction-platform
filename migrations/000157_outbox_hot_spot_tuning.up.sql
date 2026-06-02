-- Migration 000157: domain_outbox storage and maintenance tuning (ATD-007).
--
-- Context
-- -------
-- domain_outbox is a high-churn table: every dispatched notification touches
-- a row 3–4 times (INSERT pending → UPDATE processing → UPDATE done/failed).
-- Two partial indexes already exist from earlier migrations:
--   idx_outbox_poll              (m000081) — poll hot path  WHERE status='pending'
--   idx_outbox_pending_event_dedup (m000093) — dedup unique  WHERE status='pending'
-- This migration adds the complementary storage-level tuning.
--
-- Why NOT partition by created_at
-- --------------------------------
-- FIFA 2026 is a bounded event (≈64 matches, 30-day window).  At 50K DAU and
-- 4 outbox events per match the peak table size is ≈12.8M rows — comfortably
-- within Postgres B-tree range given the existing partial indexes.  Partitioning
-- would add DDL lock risk on the parent table, complicate the SELECT FOR UPDATE
-- SKIP LOCKED worker query, and require ongoing partition management; none of
-- those costs are justified at this scale.
--
-- Changes
-- -------
-- 1. FILLFACTOR = 70
--    Reserves 30 % of each page for in-place updates.  Rows cycling
--    pending → processing → done stay on the same physical page (Heap-Only
--    Tuple updates), avoiding index-entry rewrites and halving dead-tuple
--    accumulation per row lifecycle.
--
-- 2. Aggressive autovacuum
--    The default 20 % scale factor delays vacuum until 20 K dead tuples
--    accumulate on a 100 K-row table.  Setting 1 % triggers vacuum after
--    ~1 K dead tuples, keeping the table lean under continuous churn.
--    autovacuum_vacuum_cost_delay is lowered from the default 20 ms to 2 ms
--    so the daemon processes pages quickly on a hot table without I/O
--    throttling that would let dead tuples pile up between passes.
--
-- 3. Partial index on created_at for terminal rows
--    Supports the purge DELETE that removes old done/failed rows.  The
--    partial predicate limits the index to purge candidates only; pending and
--    processing rows are never eligible for deletion.

ALTER TABLE domain_outbox SET (fillfactor = 70);

ALTER TABLE domain_outbox SET (
    autovacuum_vacuum_scale_factor  = 0.01,
    autovacuum_analyze_scale_factor = 0.005,
    autovacuum_vacuum_cost_delay    = 2
);

CREATE INDEX idx_outbox_completed_created_at
    ON domain_outbox (created_at)
    WHERE status IN ('done', 'failed');
