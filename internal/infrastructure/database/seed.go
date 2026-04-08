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
// Guard the call site with a flag or environment variable — this function must
// never be invoked against a production database.
func Seed(ctx context.Context, db *pgxpool.Pool) error {
	if err := seedUsers(ctx, db); err != nil {
		return fmt.Errorf("seed users: %w", err)
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
	_, err := db.Exec(ctx, `
		INSERT INTO users (name, email, password_hash, role) VALUES
			('Admin User',  'admin@quiniela.local',  'not-a-real-hash', 'admin'),
			('Player One',  'player1@quiniela.local', 'not-a-real-hash', 'player'),
			('Player Two',  'player2@quiniela.local', 'not-a-real-hash', 'player')
		ON CONFLICT (email) DO NOTHING
	`)
	return err
}

// seedStadiums inserts the 16 official FIFA World Cup 2026 venues.
// Capacity figures are approximate and reflect announced configurations.
func seedStadiums(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		INSERT INTO stadiums (name, city, country, capacity) VALUES
			('MetLife Stadium',        'East Rutherford', 'USA',    82500),
			('AT&T Stadium',           'Arlington',       'USA',    80000),
			('SoFi Stadium',           'Inglewood',       'USA',    70240),
			('Hard Rock Stadium',      'Miami Gardens',   'USA',    65326),
			('Levi''s Stadium',        'Santa Clara',     'USA',    68500),
			('Lincoln Financial Field','Philadelphia',    'USA',    69000),
			('Lumen Field',            'Seattle',         'USA',    69000),
			('Arrowhead Stadium',      'Kansas City',     'USA',    76416),
			('Mercedes-Benz Stadium',  'Atlanta',         'USA',    71000),
			('NRG Stadium',            'Houston',         'USA',    72220),
			('Gillette Stadium',       'Foxborough',      'USA',    65878),
			('Estadio Azteca',         'Mexico City',     'Mexico', 87523),
			('Estadio BBVA',           'Monterrey',       'Mexico', 53500),
			('Estadio Akron',          'Guadalajara',     'Mexico', 49850),
			('BC Place',               'Vancouver',       'Canada', 54500),
			('BMO Field',              'Toronto',         'Canada', 45736)
		ON CONFLICT DO NOTHING
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
