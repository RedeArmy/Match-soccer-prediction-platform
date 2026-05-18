-- Migration: seed system parameters for the reliability layer added in Round 2.
--
-- Nine new parameters cover four operational areas:
--
--   api          — Idempotency-Key TTL and maximum accepted key length.
--   breaker      — Circuit-breaker thresholds for PayPal cert fetcher and
--                  the file-store (S3/GDrive/OneDrive).
--   repository   — DB transaction retry policy (max attempts + backoff).
--
-- All nine are is_runtime=FALSE: the server reads them once at startup and
-- passes the values to constructors. A process restart is required for any
-- change to take effect. Operators can safely change the values in system_params
-- and then perform a rolling restart without any data-migration step.
--
-- Idempotent: ON CONFLICT DO NOTHING means re-running this migration is safe.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    -- ── Idempotency middleware ───────────────────────────────────────────────
    (
        'api.idempotency_ttl_hours',
        '24', '24',
        'int', 'api',
        FALSE,
        'Hours a committed idempotency entry is retained. Clients may safely retry with the same Idempotency-Key for this duration after the original request. Restart required for changes.'
    ),
    (
        'api.idempotency_key_max_len',
        '255', '255',
        'int', 'api',
        FALSE,
        'Maximum byte length of a client-supplied Idempotency-Key header value. Requests with longer keys are rejected with 422. Restart required for changes.'
    ),

    -- ── Circuit breaker: PayPal certificate fetcher ──────────────────────────
    (
        'breaker.paypal_cert_max_fails',
        '3', '3',
        'int', 'breaker',
        FALSE,
        'Consecutive PayPal certificate-download failures that open the circuit breaker. While open, webhook deliveries return 500 immediately so PayPal retries later. Restart required.'
    ),
    (
        'breaker.paypal_cert_cooldown_sec',
        '60', '60',
        'int', 'breaker',
        FALSE,
        'Seconds the PayPal cert-fetcher circuit stays open before allowing a single trial request. A successful trial closes the circuit; a failed one resets the cooldown. Restart required.'
    ),

    -- ── Circuit breaker: file store (S3 / GDrive / OneDrive) ─────────────────
    (
        'breaker.file_store_max_fails',
        '5', '5',
        'int', 'breaker',
        FALSE,
        'Consecutive file-store errors (Put or Get) that open the circuit breaker. Put and Get return 500 immediately while open; Delete is a silent no-op (best-effort cleanup). Restart required.'
    ),
    (
        'breaker.file_store_cooldown_sec',
        '30', '30',
        'int', 'breaker',
        FALSE,
        'Seconds the file-store circuit stays open before allowing a single trial request. A successful trial closes the circuit; a failed one resets the cooldown. Restart required.'
    ),

    -- ── DB transaction retry policy ───────────────────────────────────────────
    (
        'repository.tx_retry_max_attempts',
        '3', '3',
        'int', 'repository',
        FALSE,
        'Total transaction attempts (including the first) before a transient serialization or deadlock error is returned to the caller. Restart required.'
    ),
    (
        'repository.tx_retry_base_delay_ms',
        '50', '50',
        'int', 'repository',
        FALSE,
        'Base backoff delay in milliseconds between transaction retry attempts. Actual delay uses equal-jitter: base/2 fixed + rand[0, base/2]. Doubles each attempt. Restart required.'
    ),
    (
        'repository.tx_retry_max_delay_ms',
        '1000', '1000',
        'int', 'repository',
        FALSE,
        'Maximum backoff delay cap in milliseconds for DB transaction retries. Prevents unbounded waits at high attempt counts. Restart required.'
    )
ON CONFLICT (key) DO NOTHING;
