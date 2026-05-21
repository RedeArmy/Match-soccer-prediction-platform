-- Migration: seed system parameters that exist in domain/constants.go and
-- cmd/validate-params/allParams but were never inserted into system_params.
--
-- These 18 rows were previously absent from every seed migration, meaning the
-- application always fell back to the hard-coded domain constant at runtime.
-- Adding them here makes them visible and tunable via the admin system_params
-- API without a code deploy, and allows cmd/validate-params to confirm the DB
-- matches the canonical defaults.
--
-- All rows use ON CONFLICT (key) DO NOTHING so re-running this migration or
-- applying it against an instance that already has manual operator overrides
-- is safe and idempotent.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES

    -- ── Conflict detection ────────────────────────────────────────────────────
    -- is_runtime=TRUE: read per-request by ConflictService.ConflictSummary.
    (
        'conflict.max_scan',
        '5000', '5000',
        'int', 'conflict',
        TRUE,
        'Maximum number of conflicts loaded into memory during ConflictService.ConflictSummary scans. Prevents unbounded memory growth when invoked by background jobs or dashboard widgets. Default: 5000. Changeable at runtime.'
    ),

    -- ── Messaging / Redis Streams ─────────────────────────────────────────────
    -- Both are is_runtime=FALSE: the consumer goroutine pool is sized at startup;
    -- a worker restart is required for changes to take effect.
    (
        'messaging.stream_worker_count',
        '8', '8',
        'int', 'messaging',
        FALSE,
        'Size of the per-event-type goroutine pool that processes Redis Stream messages concurrently. Increase for higher-throughput deployments; decrease to limit DB connection saturation. Default: 8. Worker restart required.'
    ),
    (
        'messaging.stream_read_block_sec',
        '5', '5',
        'int', 'messaging',
        FALSE,
        'XREADGROUP block timeout in seconds. A smaller value makes the consumer loop react faster to shutdown signals at the cost of more idle Redis round-trips. Default: 5. Worker restart required.'
    ),

    -- ── Audit retry policy ────────────────────────────────────────────────────
    -- is_runtime=FALSE: the audit goroutine pool is constructed at startup.
    (
        'audit.max_retries',
        '2', '2',
        'int', 'system',
        FALSE,
        'Number of write attempts before an in-flight audit log entry is permanently discarded; emits audit_lost=true on exhaustion. Default: 2. Process restart required.'
    ),
    (
        'audit.retry_delay_ms',
        '250', '250',
        'int', 'system',
        FALSE,
        'Delay in milliseconds between audit-log write retries to allow transient DB failures to clear. Default: 250 ms. Process restart required.'
    ),

    -- ── Worker: leaderboard snapshot generation ───────────────────────────────
    -- All three are is_runtime=FALSE: worker restart required.
    (
        'worker.snapshot_concurrency',
        '4', '4',
        'int', 'worker',
        FALSE,
        'Maximum number of quiniela snapshots generated concurrently per MatchFinished event. Sized for a shared-CPU machine (256 MB RAM); raise on larger instances. Default: 4. Worker restart required.'
    ),
    (
        'worker.snapshot_retry_base_ms',
        '100', '100',
        'int', 'worker',
        FALSE,
        'Initial backoff delay in milliseconds for snapshot write retries (exponential: doubles each attempt). Default: 100 ms. Worker restart required.'
    ),
    (
        'worker.snapshot_max_attempts',
        '3', '3',
        'int', 'worker',
        FALSE,
        'Maximum number of snapshot write attempts per quiniela per match event before the snapshot is skipped. Default: 3. Worker restart required.'
    ),

    -- ── Worker: background maintenance jobs ───────────────────────────────────
    (
        'worker.dlq_monitor_interval_sec',
        '300', '300',
        'int', 'worker',
        FALSE,
        'Interval in seconds between DLQ size log events emitted by the background DLQ monitor goroutine. Default: 300 (5 minutes). Worker restart required.'
    ),
    (
        'worker.purge_interval_hours',
        '24', '24',
        'int', 'worker',
        FALSE,
        'Hours between permanent purge runs that hard-delete soft-deleted users and quinielas older than system.purge_retention_days. Default: 24 hours. Worker restart required.'
    ),

    -- ── System: soft-delete retention ─────────────────────────────────────────
    (
        'system.purge_retention_days',
        '30', '30',
        'int', 'system',
        FALSE,
        'Age in days after which soft-deleted users and quinielas are permanently removed by the worker purge goroutine. Default: 30 days. Worker restart required.'
    ),

    -- ── API: request body size limit ──────────────────────────────────────────
    -- is_runtime=FALSE: MaxBytesReader is applied at startup via the middleware chain.
    (
        'api.body_size_limit_bytes',
        '65536', '65536',
        'int', 'api',
        FALSE,
        'Maximum request body size in bytes for standard API endpoints. Requests exceeding this limit are rejected with 413 to prevent DoS. Default: 65536 (64 KB). Process restart required.'
    ),

    -- ── Snapshot: per-quiniela retention count ────────────────────────────────
    -- Category "worker" (not "snapshot") because the daily purge goroutine is the
    -- sole consumer; the key prefix differs from the DB category by convention.
    -- is_runtime=FALSE: the purge goroutine reads this once at startup.
    (
        'snapshot.keep_latest_count',
        '5', '5',
        'int', 'worker',
        FALSE,
        'Number of most-recent leaderboard snapshots to retain per quiniela. The daily purge job deletes every snapshot beyond this count, bounding table growth. Default: 5. Worker restart required.'
    ),

    -- ── Payment / balance ─────────────────────────────────────────────────────
    -- is_runtime=TRUE for file-upload size (handler reads it at request time via
    -- BankTransferHandler) and is_runtime=TRUE for all amount bounds (WithdrawalService
    -- reads them per-request via SystemParamService).
    (
        'payment.max_upload_bytes',
        '5242880', '5242880',
        'int', 'payment',
        TRUE,
        'Maximum size in bytes for bank-transfer proof file uploads. Requests with larger multipart bodies are rejected with 413. Default: 5242880 (5 MB). Changeable at runtime.'
    ),
    (
        'payment.withdrawal_min_cents',
        '5000', '5000',
        'int', 'payment',
        TRUE,
        'Minimum withdrawal amount in minor currency units (centavos). Requests below this threshold are rejected with 422. Default: 5000 (Q50). Changeable at runtime.'
    ),
    (
        'payment.withdrawal_max_cents',
        '500000', '500000',
        'int', 'payment',
        TRUE,
        'Maximum withdrawal amount in minor currency units (centavos). Requests above this threshold are rejected with 422. Default: 500000 (Q5000). Changeable at runtime.'
    ),
    (
        'payment.bank_transfer_min_amount_cents',
        '1000', '1000',
        'int', 'payment',
        TRUE,
        'Minimum declared amount in minor currency units for a bank-transfer proof submission. Claims below this threshold are rejected with 422 at upload time. Default: 1000 (Q10). Changeable at runtime.'
    ),
    (
        'payment.bank_transfer_max_amount_cents',
        '10000000', '10000000',
        'int', 'payment',
        TRUE,
        'Maximum declared amount in minor currency units for a bank-transfer proof submission. Claims above this threshold are rejected with 422 at upload time. Default: 10000000 (Q100 000). Changeable at runtime.'
    )

ON CONFLICT (key) DO NOTHING;
