-- Down: remove location tables
--
-- Cities must be dropped before states (FK: cities.state_id → states.id),
-- and states before countries (FK: states.country_id → countries.id).
-- The stadiums table will have already been reverted by migration 000016 down,
-- which restores the city/country text columns and drops the city_id FK before
-- this migration removes the cities table.
DROP TABLE IF EXISTS cities;
DROP TABLE IF EXISTS states;
DROP TABLE IF EXISTS countries;