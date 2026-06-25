package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresTournamentRepository is the PostgreSQL-backed implementation of
// TournamentRepository.
type PostgresTournamentRepository struct {
	db *pgxpool.Pool
}

// NewPostgresTournamentRepository constructs a PostgresTournamentRepository.
func NewPostgresTournamentRepository(db *pgxpool.Pool) *PostgresTournamentRepository {
	return &PostgresTournamentRepository{db: db}
}

const slotColumns = "id, label, description, team, auto_source, confirmed_at, confirmed_by_user_id, created_at, updated_at"

func scanSlot(row pgx.Row) (*domain.TournamentSlot, error) {
	s := &domain.TournamentSlot{}
	err := row.Scan(&s.ID, &s.Label, &s.Description, &s.Team, &s.AutoSource, &s.ConfirmedAt, &s.ConfirmedByUserID, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return s, nil
}

// CreateSlot inserts a new bracket position slot. Label must be unique.
func (r *PostgresTournamentRepository) CreateSlot(ctx context.Context, label, description string) (*domain.TournamentSlot, error) {
	row := r.db.QueryRow(ctx,
		`INSERT INTO tournament_slots (label, description) VALUES ($1, $2) RETURNING `+slotColumns,
		label, description,
	)
	slot, err := scanSlot(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, apperrors.Conflict("a tournament slot with this label already exists")
		}
		return nil, err
	}
	return slot, nil
}

// GetSlot returns a single slot by ID. Returns nil, nil when not found.
func (r *PostgresTournamentRepository) GetSlot(ctx context.Context, id int) (*domain.TournamentSlot, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+slotColumns+` FROM tournament_slots WHERE id=$1`, id,
	)
	return scanSlot(row)
}

// ListSlots returns all bracket position slots ordered by id.
func (r *PostgresTournamentRepository) ListSlots(ctx context.Context) ([]*domain.TournamentSlot, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+slotColumns+` FROM tournament_slots ORDER BY id`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var slots []*domain.TournamentSlot
	for rows.Next() {
		s := &domain.TournamentSlot{}
		if err := rows.Scan(&s.ID, &s.Label, &s.Description, &s.Team, &s.ConfirmedAt, &s.ConfirmedByUserID, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		slots = append(slots, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return slots, nil
}

// ConfirmSlot sets the team for a slot and records who confirmed it.
// Propagates the team name to any matches that reference this slot via
// home_slot_id or away_slot_id. Returns NotFound when the slot does not exist.
func (r *PostgresTournamentRepository) ConfirmSlot(ctx context.Context, id, confirmedByUserID int, team string) (*domain.TournamentSlot, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx,
		`UPDATE tournament_slots
		 SET team=$1, confirmed_at=NOW(), confirmed_by_user_id=$2, updated_at=NOW()
		 WHERE id=$3
		 RETURNING `+slotColumns,
		team, confirmedByUserID, id,
	)
	slot, err := scanSlot(row)
	if err != nil {
		return nil, err
	}
	if slot == nil {
		return nil, apperrors.NotFound("tournament slot not found")
	}

	if _, err := tx.Exec(ctx,
		`UPDATE matches SET home_team = $1, updated_at = NOW() WHERE home_slot_id = $2`,
		team, id,
	); err != nil {
		return nil, apperrors.Internal(err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE matches SET away_team = $1, updated_at = NOW() WHERE away_slot_id = $2`,
		team, id,
	); err != nil {
		return nil, apperrors.Internal(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperrors.Internal(err)
	}
	return slot, nil
}

// FindSlotByAutoSource returns the slot with the given auto_source code.
// Returns nil, nil when no matching slot exists.
func (r *PostgresTournamentRepository) FindSlotByAutoSource(ctx context.Context, autoSource string) (*domain.TournamentSlot, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+slotColumns+` FROM tournament_slots WHERE auto_source = $1`, autoSource,
	)
	return scanSlot(row)
}

// AutoConfirmSlot sets the advancing team on a slot and propagates the name to
// any knockout matches that reference it via home_slot_id or away_slot_id.
// Unlike ConfirmSlot, no admin actor is required — this is system-driven.
func (r *PostgresTournamentRepository) AutoConfirmSlot(ctx context.Context, slotID int, team string) (*domain.TournamentSlot, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx,
		`UPDATE tournament_slots
		 SET team=$1, confirmed_at=NOW(), updated_at=NOW()
		 WHERE id=$2 AND team IS NULL
		 RETURNING `+slotColumns,
		team, slotID,
	)
	slot, err := scanSlot(row)
	if err != nil {
		return nil, err
	}
	if slot == nil {
		// Either the slot does not exist or it was already confirmed — look it up.
		row2 := tx.QueryRow(ctx, `SELECT `+slotColumns+` FROM tournament_slots WHERE id=$1`, slotID)
		slot, err = scanSlot(row2)
		if err != nil {
			return nil, err
		}
		if slot == nil {
			return nil, apperrors.NotFound("tournament slot not found")
		}
		// Already confirmed — nothing to propagate.
		return slot, tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE matches SET home_team=$1, updated_at=NOW() WHERE home_slot_id=$2`,
		team, slotID,
	); err != nil {
		return nil, apperrors.Internal(err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE matches SET away_team=$1, updated_at=NOW() WHERE away_slot_id=$2`,
		team, slotID,
	); err != nil {
		return nil, apperrors.Internal(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperrors.Internal(err)
	}
	return slot, nil
}

var _ TournamentRepository = (*PostgresTournamentRepository)(nil)
