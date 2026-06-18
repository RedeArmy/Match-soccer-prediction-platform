DROP INDEX IF EXISTS payment_intents_provider_status_idx;

ALTER TABLE payment_intents
    DROP CONSTRAINT IF EXISTS payment_intents_status_check;
ALTER TABLE payment_intents
    ADD CONSTRAINT payment_intents_status_check
    CHECK (status IN ('pending', 'captured', 'expired'));

ALTER TABLE payment_intents
    DROP COLUMN IF EXISTS rejected_at,
    DROP COLUMN IF EXISTS review_notes,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS comprobante_file_size,
    DROP COLUMN IF EXISTS comprobante_content_type,
    DROP COLUMN IF EXISTS comprobante_key,
    DROP COLUMN IF EXISTS provider;
