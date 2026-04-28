-- Correct the category for DLQ system params.
--
-- Migration 000040 inserted dlq.sample_size and dlq.replay_default_limit under
-- category 'messaging' but documented 'dlq' as a distinct category in its
-- header. This mismatch causes GetByCategory("dlq") to return an empty slice
-- and GetByCategory("messaging") to return params that belong to a different
-- concern. Correcting to 'dlq' aligns keys, category values, and the
-- documented category list.
UPDATE system_params
   SET category = 'dlq'
 WHERE key IN ('dlq.sample_size', 'dlq.replay_default_limit');
