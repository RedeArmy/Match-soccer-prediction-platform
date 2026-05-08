-- Down: restore system_params to the pre-migration-051 state.
--
-- migration 051 only updates existing rows (via ON CONFLICT DO UPDATE). The
-- canonical values from migration 049 + 050 are restored here. Note that any
-- operator overrides that existed before running migration 051 are NOT
-- automatically restored — they must be re-applied manually.

-- Restore group.min_members_for_active to the value set by migration 050 (no-op,
-- same value). Included for completeness.
UPDATE system_params SET value = '5', updated_at = NOW()
 WHERE key = 'group.min_members_for_active';

-- Restore any values that migration 049 set and 051 overwrote.
-- In practice the defaults are identical, so this is a no-op on fresh schemas.
-- Listed here as documentation of the pre-051 canonical defaults.
UPDATE system_params SET value = '5',  updated_at = NOW() WHERE key = 'audit.write_timeout_seconds';
UPDATE system_params SET value = '5',  updated_at = NOW() WHERE key = 'auth.validation_timeout_seconds';
