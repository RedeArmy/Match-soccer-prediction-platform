-- Enforce reference uniqueness for non-empty payment references.
--
-- A partial index is used so that:
--   • NULL references (free groups with no bank transfer) are never constrained.
--   • Empty-string references (legacy or manual records) are also exempt.
--   • Any non-null, non-empty reference is globally unique, enabling idempotent
--     INSERT ... ON CONFLICT for duplicate webhook deliveries or retried requests.
CREATE UNIQUE INDEX idx_payment_records_reference_unique
    ON payment_records (reference)
    WHERE reference IS NOT NULL AND reference <> '';
