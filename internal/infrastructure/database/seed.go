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

func seedMatches(ctx context.Context, db *pgxpool.Pool) error {
	kickoff := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
	_, err := db.Exec(ctx, `
		INSERT INTO matches (home_team, away_team, status, kickoff_at) VALUES
			('Brazil',    'Argentina', 'scheduled', $1),
			('France',    'Germany',   'scheduled', $2),
			('Spain',     'England',   'scheduled', $3)
		ON CONFLICT DO NOTHING
	`, kickoff, kickoff.Add(2*time.Hour), kickoff.Add(4*time.Hour))
	return err
}
