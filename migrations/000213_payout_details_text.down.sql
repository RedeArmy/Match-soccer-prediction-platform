-- Reverting to JSONB will fail if any encrypted (non-JSON) rows exist.
-- Ensure all payout_details values are valid JSON before running this.

BEGIN;
ALTER TABLE withdrawal_requests
    ALTER COLUMN payout_details TYPE JSONB
    USING payout_details::jsonb;
COMMIT;
