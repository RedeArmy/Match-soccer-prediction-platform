-- Trigger: enforce max_members capacity at the database level.
--
-- The application performs a check-then-insert for group capacity, but without
-- a database-level guard concurrent requests can race past the check and
-- overfill a group. This trigger fires BEFORE any INSERT or UPDATE that would
-- activate a new membership, and raises an exception when the active count
-- already equals max_members. The exception code 'P0001' (raise_exception)
-- with message 'max_members_exceeded' is caught by the repository layer and
-- translated into a 409 Conflict response.

CREATE OR REPLACE FUNCTION enforce_max_members()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    max_m        INT;
    active_count INT;
BEGIN
    -- Only enforce when a row becomes active.
    IF NEW.status != 'active' THEN
        RETURN NEW;
    END IF;
    -- For UPDATE active→active, the count does not change.
    IF TG_OP = 'UPDATE' AND OLD.status = 'active' THEN
        RETURN NEW;
    END IF;

    SELECT max_members INTO max_m FROM quinielas WHERE id = NEW.quiniela_id;
    IF max_m IS NULL THEN
        RETURN NEW;
    END IF;

    -- BEFORE trigger: the row being inserted/updated is not yet visible, so
    -- COUNT(*) reflects the current committed active members.
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

CREATE TRIGGER trg_enforce_max_members
BEFORE INSERT OR UPDATE ON group_memberships
FOR EACH ROW EXECUTE FUNCTION enforce_max_members();
