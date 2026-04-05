package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresPredictionRepository is the PostgreSQL-backed implementation of PredictionRepository.
type PostgresPredictionRepository struct {
	db *pgxpool.Pool
}

// NewPostgresPredictionRepository constructs a PostgresPredictionRepository.
func NewPostgresPredictionRepository(db *pgxpool.Pool) *PostgresPredictionRepository {
	return &PostgresPredictionRepository{db: db}
}

const predictionColumns = "id, user_id, match_id, home_score, away_score, points, created_at, updated_at"

func scanPrediction(row pgx.Row) (*domain.Prediction, error) {
	p := &domain.Prediction{}
	err := row.Scan(&p.ID, &p.UserID, &p.MatchID, &p.HomeScore, &p.AwayScore, &p.Points, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return p, nil
}

func (r *PostgresPredictionRepository) Create(ctx context.Context, p *domain.Prediction) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO predictions (user_id, match_id, home_score, away_score)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+predictionColumns,
		p.UserID, p.MatchID, p.HomeScore, p.AwayScore,
	)
	result, err := scanPrediction(row)
	if err != nil {
		return err
	}
	*p = *result
	return nil
}

func (r *PostgresPredictionRepository) GetByID(ctx context.Context, id int) (*domain.Prediction, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+predictionColumns+` FROM predictions WHERE id=$1`, id,
	)
	return scanPrediction(row)
}

func (r *PostgresPredictionRepository) Update(ctx context.Context, p *domain.Prediction) error {
	row := r.db.QueryRow(ctx,
		`UPDATE predictions SET home_score=$1, away_score=$2, points=$3, updated_at=NOW()
		 WHERE id=$4 RETURNING `+predictionColumns,
		p.HomeScore, p.AwayScore, p.Points, p.ID,
	)
	result, err := scanPrediction(row)
	if err != nil {
		return err
	}
	if result == nil {
		return apperrors.NotFound("prediction not found")
	}
	*p = *result
	return nil
}

func (r *PostgresPredictionRepository) GetByUserAndMatch(ctx context.Context, userID, matchID int) (*domain.Prediction, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+predictionColumns+` FROM predictions WHERE user_id=$1 AND match_id=$2`,
		userID, matchID,
	)
	return scanPrediction(row)
}

func (r *PostgresPredictionRepository) ListByUser(ctx context.Context, userID int) ([]*domain.Prediction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+predictionColumns+` FROM predictions WHERE user_id=$1 ORDER BY created_at ASC`, userID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectPredictions(rows)
}

func (r *PostgresPredictionRepository) ListByMatch(ctx context.Context, matchID int) ([]*domain.Prediction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+predictionColumns+` FROM predictions WHERE match_id=$1 ORDER BY created_at ASC`, matchID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectPredictions(rows)
}

func collectPredictions(rows pgx.Rows) ([]*domain.Prediction, error) {
	var preds []*domain.Prediction
	for rows.Next() {
		p := &domain.Prediction{}
		if err := rows.Scan(&p.ID, &p.UserID, &p.MatchID, &p.HomeScore, &p.AwayScore, &p.Points, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		preds = append(preds, p)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return preds, nil
}
