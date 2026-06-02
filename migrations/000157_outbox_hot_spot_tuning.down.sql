DROP INDEX IF EXISTS idx_outbox_completed_created_at;

ALTER TABLE domain_outbox RESET (
    fillfactor,
    autovacuum_vacuum_scale_factor,
    autovacuum_analyze_scale_factor,
    autovacuum_vacuum_cost_delay
);
