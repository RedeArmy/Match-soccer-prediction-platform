-- Migration: replace stadiums.city/country text columns with a city_id FK
--
-- The stadiums table previously stored location as two denormalized TEXT
-- columns (city, country). This migration replaces them with a foreign key to
-- the normalized cities table added in migration 000015, enabling the full
-- hierarchy: stadium → city → state → country.
--
-- Steps:
--   1. Add city_id as nullable so the column can be added before data migration.
--   2. Populate city_id for any existing rows using the city name already stored
--      in the row, joining through the location tables inserted in 000015.
--      The CASE maps the legacy country abbreviations ('USA', 'Mexico',
--      'Canada') to the canonical country names in the countries table.
--   3. Drop the now-redundant city and country text columns.
--   4. Enforce NOT NULL on city_id; on a fresh database (no pre-existing rows)
--      this is a no-op, and for migrated rows all 16 venues are guaranteed to
--      match because the city names in 000015 are taken verbatim from the seed.
--   5. Add a unique constraint on stadiums.name so the seed can use
--      ON CONFLICT (name) DO NOTHING safely on repeated runs.

-- Step 1: add nullable column
ALTER TABLE stadiums
    ADD COLUMN city_id INTEGER REFERENCES cities(id);

-- Step 2: backfill for any pre-existing seeded rows
UPDATE stadiums s
SET city_id = ci.id
FROM cities ci
JOIN states  st ON ci.state_id   = st.id
JOIN countries co ON st.country_id = co.id
WHERE ci.name = s.city
  AND co.name = CASE s.country
      WHEN 'USA'    THEN 'United States'
      WHEN 'Mexico' THEN 'Mexico'
      WHEN 'Canada' THEN 'Canada'
      ELSE s.country
  END;

-- Step 3: remove legacy text columns
ALTER TABLE stadiums DROP COLUMN city;
ALTER TABLE stadiums DROP COLUMN country;

-- Step 4: enforce NOT NULL (all existing rows now have city_id populated)
ALTER TABLE stadiums ALTER COLUMN city_id SET NOT NULL;

-- Step 5: unique constraint on name for idempotent seed runs
ALTER TABLE stadiums ADD CONSTRAINT uq_stadiums_name UNIQUE (name);

CREATE INDEX IF NOT EXISTS idx_stadiums_city_id ON stadiums (city_id);
