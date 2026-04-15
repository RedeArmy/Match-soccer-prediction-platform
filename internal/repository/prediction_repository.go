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

// ListByUserAndQuiniela returns all predictions for userID where the user is
// an active member of quinielaID. A single EXISTS subquery verifies membership
// so the call is one database round-trip. If the user is not an active member
// (or the quiniela does not exist) the result is an empty slice, not an error.
func (r *PostgresPredictionRepository) ListByUserAndQuiniela(ctx context.Context, userID, quinielaID int) ([]*domain.Prediction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+predictionColumns+`
		   FROM predictions
		  WHERE user_id = $1
		    AND EXISTS (
		          SELECT 1 FROM group_memberships gm
		           WHERE gm.quiniela_id = $2
		             AND gm.user_id     = $1
		             AND gm.status      = 'active'
		        )
		  ORDER BY created_at ASC`,
		userID, quinielaID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectPredictions(rows)
}

// TotalPointsByQuiniela returns a map of userID → total scored points for
// every active, paid member of the given quiniela. It uses a single SQL
// query with a LEFT JOIN between group_memberships and predictions so the
// ranking service never issues N+1 queries when computing a leaderboard.
//
// Predictions with NULL points (match not yet scored) are excluded from the
// sum. A member with no scored predictions appears in the map with value 0.
func (r *PostgresPredictionRepository) TotalPointsByQuiniela(ctx context.Context, quinielaID int) (map[int]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT gm.user_id, COALESCE(SUM(p.points), 0)
		   FROM group_memberships gm
		   LEFT JOIN predictions p
		          ON p.user_id = gm.user_id
		         AND p.points IS NOT NULL
		  WHERE gm.quiniela_id = $1
		    AND gm.status = 'active'
		    AND gm.paid = TRUE
		  GROUP BY gm.user_id`,
		quinielaID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	result := make(map[int]int)
	for rows.Next() {
		var userID, totalPoints int
		if err := rows.Scan(&userID, &totalPoints); err != nil {
			return nil, apperrors.Internal(err)
		}
		result[userID] = totalPoints
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return result, nil
}

// TotalPointsByQuinielaAndPhase returns a map of userID → total scored points
// for every active, paid member of the given quiniela, restricted to matches
// in the provided phase.
//
// A member with no scored predictions in the requested phase appears in the map
// with value 0, preserving the same semantics as TotalPointsByQuiniela.
//
// Query design — derived table instead of correlated EXISTS:
// The phase filter is applied inside a derived table (phase_pred) that joins
// predictions with matches before the outer LEFT JOIN executes. This lets the
// planner use a hash join between group_memberships and the pre-filtered
// prediction set rather than evaluating a correlated subquery once per
// predictions row. At 500 members × 64 matches the correlated form issues up
// to 32 000 subquery evaluations per leaderboard request; the derived-table
// form reduces this to two sequential scans and one hash join.
func (r *PostgresPredictionRepository) TotalPointsByQuinielaAndPhase(ctx context.Context, quinielaID int, phase domain.MatchPhase) (map[int]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT gm.user_id, COALESCE(SUM(phase_pred.points), 0)
		   FROM group_memberships gm
		   LEFT JOIN (
		       SELECT p.user_id, p.points
		         FROM predictions p
		         JOIN matches m ON m.id = p.match_id AND m.phase = $2
		        WHERE p.points IS NOT NULL
		   ) AS phase_pred ON phase_pred.user_id = gm.user_id
		  WHERE gm.quiniela_id = $1
		    AND gm.status = 'active'
		    AND gm.paid = TRUE
		  GROUP BY gm.user_id`,
		quinielaID, string(phase),
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	result := make(map[int]int)
	for rows.Next() {
		var userID, totalPoints int
		if err := rows.Scan(&userID, &totalPoints); err != nil {
			return nil, apperrors.Internal(err)
		}
		result[userID] = totalPoints
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return result, nil
}

// UpdateManyPoints atomically updates the points column for every prediction
// ID in the provided map. A single UPDATE … FROM UNNEST statement is used so
// the operation is one round-trip and one lock acquisition regardless of how
// many predictions are being scored.
//
// Atomicity guarantee: a single SQL statement is atomic in PostgreSQL without
// an explicit transaction. The match is either fully scored or not scored at
// all; there is no partial state.
//
// IDs that do not exist in the predictions table are silently skipped; the
// caller (scoring service) is responsible for ensuring the map contains only
// valid prediction IDs.
func (r *PostgresPredictionRepository) UpdateManyPoints(ctx context.Context, points map[int]int) error {
	if len(points) == 0 {
		return nil
	}

	ids := make([]int, 0, len(points))
	pts := make([]int, 0, len(points))
	for id, p := range points {
		ids = append(ids, id)
		pts = append(pts, p)
	}

	if _, err := r.db.Exec(ctx,
		`UPDATE predictions
		    SET points     = v.points,
		        updated_at = NOW()
		   FROM UNNEST($1::int[], $2::int[]) AS v(id, points)
		  WHERE predictions.id = v.id`,
		ids, pts,
	); err != nil {
		return apperrors.Internal(err)
	}
	return nil
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
