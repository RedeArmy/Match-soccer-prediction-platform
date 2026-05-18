DELETE FROM system_params
WHERE key IN (
    'api.idempotency_ttl_hours',
    'api.idempotency_key_max_len',
    'breaker.paypal_cert_max_fails',
    'breaker.paypal_cert_cooldown_sec',
    'breaker.file_store_max_fails',
    'breaker.file_store_cooldown_sec',
    'repository.tx_retry_max_attempts',
    'repository.tx_retry_base_delay_ms',
    'repository.tx_retry_max_delay_ms'
);
