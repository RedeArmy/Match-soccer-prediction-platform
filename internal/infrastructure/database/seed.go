package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Seed inserts a deterministic set of development fixtures into the database.
//
// It is safe to call on an already-seeded database because each INSERT uses
// ON CONFLICT DO NOTHING, preventing duplicate rows without returning an error.
//
// Guard the call site with a flag or environment variable - this function must
// never be invoked against a production database.
func Seed(ctx context.Context, db *pgxpool.Pool) error {
	if err := seedUsers(ctx, db); err != nil {
		return fmt.Errorf("seed users: %w", err)
	}
	if err := seedCountries(ctx, db); err != nil {
		return fmt.Errorf("seed countries: %w", err)
	}
	if err := seedStates(ctx, db); err != nil {
		return fmt.Errorf("seed states: %w", err)
	}
	if err := seedCities(ctx, db); err != nil {
		return fmt.Errorf("seed cities: %w", err)
	}
	if err := seedStadiums(ctx, db); err != nil {
		return fmt.Errorf("seed stadiums: %w", err)
	}
	if err := seedMatches(ctx, db); err != nil {
		return fmt.Errorf("seed matches: %w", err)
	}
	return nil
}

func seedUsers(ctx context.Context, db *pgxpool.Pool) error {
	// password_hash was removed in migration 000010: authentication is delegated
	// to Clerk and no credential is stored in the application database.
	_, err := db.Exec(ctx, `
		INSERT INTO users (name, email, role) VALUES
			('Admin User',  'admin@quiniela.local',  'admin'),
			('Player One',  'player1@quiniela.local', 'user'),
			('Player Two',  'player2@quiniela.local', 'user')
		ON CONFLICT (email) DO NOTHING
	`)
	return err
}

// seedCountries inserts the 3 FIFA World Cup 2026 host nations.
// These are also inserted by migration 000015; this seed function is provided
// so that the seed is self-contained and idempotent on any database state.
func seedCountries(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO countries (name, code) VALUES
			('United States', 'US'),
			('Mexico',        'MX'),
			('Canada',        'CA')
		ON CONFLICT (code) DO NOTHING
	`)
	return err
}

// seedStates inserts the 14 host states and provinces across the 3 host nations.
func seedStates(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO states (name, code, country_id) VALUES
			('New Jersey',       'NJ',   (SELECT id FROM countries WHERE code = 'US')),
			('Texas',            'TX',   (SELECT id FROM countries WHERE code = 'US')),
			('California',       'CA',   (SELECT id FROM countries WHERE code = 'US')),
			('Florida',          'FL',   (SELECT id FROM countries WHERE code = 'US')),
			('Pennsylvania',     'PA',   (SELECT id FROM countries WHERE code = 'US')),
			('Washington',       'WA',   (SELECT id FROM countries WHERE code = 'US')),
			('Missouri',         'MO',   (SELECT id FROM countries WHERE code = 'US')),
			('Georgia',          'GA',   (SELECT id FROM countries WHERE code = 'US')),
			('Massachusetts',    'MA',   (SELECT id FROM countries WHERE code = 'US')),
			('Ciudad de México', 'CDMX', (SELECT id FROM countries WHERE code = 'MX')),
			('Nuevo León',       'NL',   (SELECT id FROM countries WHERE code = 'MX')),
			('Jalisco',          'JAL',  (SELECT id FROM countries WHERE code = 'MX')),
			('British Columbia', 'BC',   (SELECT id FROM countries WHERE code = 'CA')),
			('Ontario',          'ON',   (SELECT id FROM countries WHERE code = 'CA'))
		ON CONFLICT (code, country_id) DO NOTHING
	`)
	return err
}

// seedCities inserts the 16 FIFA World Cup 2026 host cities.
// Each subquery uses (state_code, country_code) to resolve the correct state,
// avoiding ambiguity between identically-coded values in different countries
// (e.g. state code 'CA' for California vs. country code 'CA' for Canada).
func seedCities(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO cities (name, state_id) VALUES
			('East Rutherford',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'NJ'   AND c.code = 'US')),
			('Arlington',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'TX'   AND c.code = 'US')),
			('Inglewood',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'CA'   AND c.code = 'US')),
			('Miami Gardens',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'FL'   AND c.code = 'US')),
			('Santa Clara',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'CA'   AND c.code = 'US')),
			('Philadelphia',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'PA'   AND c.code = 'US')),
			('Seattle',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'WA'   AND c.code = 'US')),
			('Kansas City',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'MO'   AND c.code = 'US')),
			('Atlanta',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'GA'   AND c.code = 'US')),
			('Houston',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'TX'   AND c.code = 'US')),
			('Foxborough',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'MA'   AND c.code = 'US')),
			('Mexico City',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'CDMX' AND c.code = 'MX')),
			('Monterrey',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'NL'   AND c.code = 'MX')),
			('Guadalajara',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'JAL'  AND c.code = 'MX')),
			('Vancouver',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'BC'   AND c.code = 'CA')),
			('Toronto',
				(SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'ON'   AND c.code = 'CA'))
		ON CONFLICT (name, state_id) DO NOTHING
	`)
	return err
}

// seedStadiums inserts the 16 official FIFA World Cup 2026 venues.
// city_id is resolved via a subquery against the cities/states/countries tables
// seeded immediately before this call. Capacity figures are approximate and
// reflect announced configurations.
func seedStadiums(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO stadiums (name, city_id, capacity) VALUES
			('MetLife Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'East Rutherford' AND s.code = 'NJ'   AND c.code = 'US'), 82500),
			('AT&T Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Arlington'       AND s.code = 'TX'   AND c.code = 'US'), 80000),
			('SoFi Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Inglewood'       AND s.code = 'CA'   AND c.code = 'US'), 70240),
			('Hard Rock Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Miami Gardens'   AND s.code = 'FL'   AND c.code = 'US'), 65326),
			('Levi''s Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Santa Clara'     AND s.code = 'CA'   AND c.code = 'US'), 68500),
			('Lincoln Financial Field',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Philadelphia'    AND s.code = 'PA'   AND c.code = 'US'), 69000),
			('Lumen Field',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Seattle'         AND s.code = 'WA'   AND c.code = 'US'), 69000),
			('Arrowhead Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Kansas City'     AND s.code = 'MO'   AND c.code = 'US'), 76416),
			('Mercedes-Benz Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Atlanta'         AND s.code = 'GA'   AND c.code = 'US'), 71000),
			('NRG Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Houston'         AND s.code = 'TX'   AND c.code = 'US'), 72220),
			('Gillette Stadium',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Foxborough'      AND s.code = 'MA'   AND c.code = 'US'), 65878),
			('Estadio Azteca',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Mexico City'     AND s.code = 'CDMX' AND c.code = 'MX'), 87523),
			('Estadio BBVA',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Monterrey'       AND s.code = 'NL'   AND c.code = 'MX'), 53500),
			('Estadio Akron',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Guadalajara'     AND s.code = 'JAL'  AND c.code = 'MX'), 49850),
			('BC Place',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Vancouver'       AND s.code = 'BC'   AND c.code = 'CA'), 54500),
			('BMO Field',
				(SELECT ci.id FROM cities ci JOIN states s ON ci.state_id = s.id JOIN countries c ON s.country_id = c.id WHERE ci.name = 'Toronto'         AND s.code = 'ON'   AND c.code = 'CA'), 45736)
		ON CONFLICT (name) DO NOTHING
	`)
	return err
}

func seedMatches(ctx context.Context, db *pgxpool.Pool) error {
	kickoff := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
	_, err := db.Exec(ctx, `
		INSERT INTO matches (home_team, away_team, status, phase, stadium_id, kickoff_at) VALUES
			('Brazil',  'Argentina', 'scheduled', 'group_stage', (SELECT id FROM stadiums WHERE name = 'Estadio Azteca'  LIMIT 1), $1),
			('France',  'Germany',   'scheduled', 'group_stage', (SELECT id FROM stadiums WHERE name = 'MetLife Stadium' LIMIT 1), $2),
			('Spain',   'England',   'scheduled', 'group_stage', (SELECT id FROM stadiums WHERE name = 'SoFi Stadium'    LIMIT 1), $3)
		ON CONFLICT DO NOTHING
	`, kickoff, kickoff.Add(2*time.Hour), kickoff.Add(4*time.Hour))
	return err
}
