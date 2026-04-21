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

const slotColumns = "id, label, team, confirmed_at, confirmed_by_user_id, created_at, updated_at"

func scanSlot(row pgx.Row) (*domain.TournamentSlot, error) {
	s := &domain.TournamentSlot{}
	err := row.Scan(&s.ID, &s.Label, &s.Team, &s.ConfirmedAt, &s.ConfirmedByUserID, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return s, nil
}

// CreateSlot inserts a new bracket position slot. Label must be unique.
func (r *PostgresTournamentRepository) CreateSlot(ctx context.Context, label string) (*domain.TournamentSlot, error) {
	row := r.db.QueryRow(ctx,
		`INSERT INTO tournament_slots (label) VALUES ($1) RETURNING `+slotColumns,
		label,
	)
	slot, err := scanSlot(row)
	if err != nil {
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
		if err := rows.Scan(&s.ID, &s.Label, &s.Team, &s.ConfirmedAt, &s.ConfirmedByUserID, &s.CreatedAt, &s.UpdatedAt); err != nil {
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
// Returns NotFound when the slot does not exist.
func (r *PostgresTournamentRepository) ConfirmSlot(ctx context.Context, id, confirmedByUserID int, team string) (*domain.TournamentSlot, error) {
	row := r.db.QueryRow(ctx,
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
	return slot, nil
}
