-- Rollback: restore the enforce_max_members trigger and remove the
-- group.max_size system param added in the up migration.

CREATE OR REPLACE FUNCTION enforce_max_members()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    active_count INT;
BEGIN
    IF NEW.status != 'active' THEN
        RETURN NEW;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.status = 'active' THEN
        RETURN NEW;
    END IF;

    SELECT COUNT(*) INTO active_count
      FROM group_memberships
     WHERE quiniela_id = NEW.quiniela_id
       AND status = 'active';

    IF active_count >= 20 THEN
        RAISE EXCEPTION 'max_members_exceeded';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_enforce_max_members
BEFORE INSERT OR UPDATE ON group_memberships
FOR EACH ROW EXECUTE FUNCTION enforce_max_members();

DELETE FROM system_params WHERE key = 'group.max_size';
