-- Migration 000132: add prizes_distributed_at to quinielas for idempotent distribution.
--
-- Without this column, calling DistributePrizes twice (double-click or load-balancer
-- retry) credits every winner twice with no recovery path. The UPDATE ... WHERE
-- prizes_distributed_at IS NULL guard used in service code provides an atomic
-- compare-and-set that prevents double-distribution.

ALTER TABLE quinielas
  ADD COLUMN IF NOT EXISTS prizes_distributed_at TIMESTAMPTZ;
