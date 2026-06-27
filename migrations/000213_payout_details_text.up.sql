-- Migration 000213: change payout_details column type from JSONB to TEXT.
--
-- AES-256-GCM ciphertext produced by payoutenc is an opaque byte sequence,
-- not valid JSON. Storing it in a JSONB column forces the database to parse
-- the value as JSON on every write, and exposes the raw structure to DB-level
-- operators (pg_dump, SELECT, RLS bypass) when the Noop encrypter is active.
--
-- TEXT is the correct type for opaque ciphertext. Existing rows (which may
-- contain plaintext JSON written before encryption was mandated) are preserved
-- verbatim as their text representation. The application-layer encrypter
-- handles the mixed state on reads via payoutenc.Unmarshal.
--
-- NOTE: rows inserted before WCQ_PAYMENT_PAYOUTENCRYPTIONKEY was set in
-- production contain plaintext JSON. This migration does not retroactively
-- encrypt them; a separate data migration is required to remediate those rows.

BEGIN;
ALTER TABLE withdrawal_requests
    ALTER COLUMN payout_details TYPE TEXT
    USING payout_details::text;
COMMIT;
