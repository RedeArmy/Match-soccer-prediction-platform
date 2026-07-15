package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresExtraPredictionRepository is the PostgreSQL-backed implementation of
// ExtraPredictionRepository.
type PostgresExtraPredictionRepository struct {
	db *pgxpool.Pool
}

// NewPostgresExtraPredictionRepository constructs a PostgresExtraPredictionRepository.
func NewPostgresExtraPredictionRepository(db *pgxpool.Pool) *PostgresExtraPredictionRepository {
	return &PostgresExtraPredictionRepository{db: db}
}

const extraPredictionColumns = "id, user_id, match_id, extra_type, answer, points, scored_at, created_at, updated_at"

func collectExtraPredictions(rows pgx.Rows) ([]*domain.ExtraPrediction, error) {
	return collectRows(rows, func(r pgx.Rows) (*domain.ExtraPrediction, error) {
		p := &domain.ExtraPrediction{}
		return p, r.Scan(&p.ID, &p.UserID, &p.MatchID, &p.ExtraType, &p.Answer, &p.Points, &p.ScoredAt, &p.CreatedAt, &p.UpdatedAt)
	})
}

// Upsert inserts the extra prediction or, on (user_id, match_id, extra_type)
// conflict, executes a no-op UPDATE that allows RETURNING to yield the
// existing row. Mirrors PredictionRepository.Upsert's xmax idiom so POST
// /extras is safe to retry without the client receiving a 409.
func (r *PostgresExtraPredictionRepository) Upsert(ctx context.Context, p *domain.ExtraPrediction) (created bool, err error) {
	var wasInserted bool
	row := r.db.QueryRow(ctx,
		`INSERT INTO extra_predictions (user_id, match_id, extra_type, answer)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, match_id, extra_type) DO UPDATE
		     SET answer = EXCLUDED.answer, updated_at = NOW()
		 RETURNING `+extraPredictionColumns+`, (xmax = 0) AS was_inserted`,
		p.UserID, p.MatchID, string(p.ExtraType), p.Answer,
	)
	result := &domain.ExtraPrediction{}
	if scanErr := row.Scan(
		&result.ID, &result.UserID, &result.MatchID, &result.ExtraType, &result.Answer,
		&result.Points, &result.ScoredAt, &result.CreatedAt, &result.UpdatedAt,
		&wasInserted,
	); scanErr != nil {
		return false, apperrors.Internal(scanErr)
	}
	*p = *result
	return wasInserted, nil
}

// GetByUserAndMatch returns every extra guess (across all extra types)
// submitted by userID for matchID.
func (r *PostgresExtraPredictionRepository) GetByUserAndMatch(ctx context.Context, userID, matchID int) ([]*domain.ExtraPrediction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+extraPredictionColumns+` FROM extra_predictions WHERE user_id=$1 AND match_id=$2 ORDER BY extra_type`,
		userID, matchID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectExtraPredictions(rows)
}

// ListByUserAndMatches bulk-fetches every extra guess submitted by userID
// across all of matchIDs in one round-trip. Returns nil when matchIDs is empty.
func (r *PostgresExtraPredictionRepository) ListByUserAndMatches(ctx context.Context, userID int, matchIDs []int) ([]*domain.ExtraPrediction, error) {
	if len(matchIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+extraPredictionColumns+`
		   FROM extra_predictions
		  WHERE user_id = $1
		    AND match_id = ANY($2::int[])
		  ORDER BY match_id ASC, extra_type ASC`,
		userID, matchIDs,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectExtraPredictions(rows)
}

// ScoreMatchBatch reads all extra predictions for matchID, calls scorer with
// the slice, and writes the returned point map back — all inside a single
// transaction. Mirrors PredictionRepository.ScoreMatchBatch's atomicity
// guarantee and idempotent scored_at IS NULL guard.
func (r *PostgresExtraPredictionRepository) ScoreMatchBatch(ctx context.Context, matchID int, scorer func([]*domain.ExtraPrediction) (map[int]int, error), chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = domain.DefaultScoringUpdateChunkSize
	}
	return withRetryTx(ctx, r.db, "ExtraPredictionRepository.ScoreMatchBatch", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+extraPredictionColumns+` FROM extra_predictions WHERE match_id=$1 ORDER BY created_at ASC`, matchID,
		)
		if err != nil {
			return apperrors.Internal(err)
		}
		predictions, err := collectExtraPredictions(rows)
		if err != nil {
			return err
		}

		points, err := scorer(predictions)
		if err != nil {
			return err
		}
		return applyExtraPointsUpdate(ctx, tx, points, chunkSize)
	})
}

// applyExtraPointsUpdate converts points into parallel id/pts slices and
// executes the UPDATE in chunkSize-row UNNEST batches, all inside the
// caller's open transaction. An empty points map is a no-op.
func applyExtraPointsUpdate(ctx context.Context, tx pgx.Tx, points map[int]int, chunkSize int) error {
	if len(points) == 0 {
		return nil
	}

	ids := make([]int, 0, len(points))
	pts := make([]int, 0, len(points))
	for id, p := range points {
		ids = append(ids, id)
		pts = append(pts, p)
	}

	for i := 0; i < len(ids); i += chunkSize {
		end := min(i+chunkSize, len(ids))
		if _, err := tx.Exec(ctx,
			`UPDATE extra_predictions
			    SET points     = v.points,
			        scored_at  = NOW(),
			        updated_at = NOW()
			   FROM UNNEST($1::int[], $2::int[]) AS v(id, points)
			  WHERE extra_predictions.id = v.id
			    AND extra_predictions.scored_at IS NULL`,
			ids[i:end], pts[i:end],
		); err != nil {
			return apperrors.Internal(err)
		}
	}
	return nil
}

var _ ExtraPredictionRepository = (*PostgresExtraPredictionRepository)(nil)
