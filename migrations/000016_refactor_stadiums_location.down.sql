-- Down: restore stadiums.city and stadiums.country text columns
--
-- Reverses the up migration: re-adds the denormalized city/country text
-- columns, populates them from the cities/states/countries join, then drops
-- city_id. The country names are mapped back to the legacy abbreviations
-- ('USA', 'Mexico', 'Canada') used by the original schema.

-- Re-add the text columns as nullable during the transition
ALTER TABLE stadiums ADD COLUMN city    TEXT;
ALTER TABLE stadiums ADD COLUMN country TEXT;

-- Restore values from the normalized tables
UPDATE stadiums s
SET city    = ci.name,
    country = CASE co.code
        WHEN 'US' THEN 'USA'
        WHEN 'MX' THEN 'Mexico'
        WHEN 'CA' THEN 'Canada'
        ELSE co.name
    END
FROM cities ci
JOIN states    st ON ci.state_id   = st.id
JOIN countries co ON st.country_id = co.id
WHERE ci.id = s.city_id;

-- Enforce NOT NULL after backfill
ALTER TABLE stadiums ALTER COLUMN city    SET NOT NULL;
ALTER TABLE stadiums ALTER COLUMN country SET NOT NULL;

-- Remove forward-only additions
DROP INDEX   IF EXISTS idx_stadiums_city_id;
ALTER TABLE stadiums DROP CONSTRAINT IF EXISTS uq_stadiums_name;
ALTER TABLE stadiums DROP COLUMN city_id;
