-- Rollback: restore previous dynamic trigger and system param values.

-- 1. Restore trigger to read max_members from quinielas table.
CREATE OR REPLACE FUNCTION enforce_max_members()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    max_m        INT;
    active_count INT;
BEGIN
    IF NEW.status != 'active' THEN
        RETURN NEW;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.status = 'active' THEN
        RETURN NEW;
    END IF;

    SELECT max_members INTO max_m FROM quinielas WHERE id = NEW.quiniela_id;
    IF max_m IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT COUNT(*) INTO active_count
      FROM group_memberships
     WHERE quiniela_id = NEW.quiniela_id
       AND status = 'active';

    IF active_count >= max_m THEN
        RAISE EXCEPTION 'max_members_exceeded';
    END IF;

    RETURN NEW;
END;
$$;

-- 2. Restore group.min_members_for_active to 3.
UPDATE system_params
   SET value      = '3',
       updated_at = NOW()
 WHERE key = 'group.min_members_for_active';

-- 3. Re-insert group.default_prize_threshold.
INSERT INTO system_params (key, value, type, category, is_runtime)
VALUES ('group.default_prize_threshold', '3', 'int', 'group', TRUE)
ON CONFLICT (key) DO NOTHING;
