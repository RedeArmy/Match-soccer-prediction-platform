package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const errMsgDuplicatePrediction = "a prediction for this match has already been submitted"

// PostgresPredictionRepository is the PostgreSQL-backed implementation of PredictionRepository.
type PostgresPredictionRepository struct {
	db *pgxpool.Pool
}

// NewPostgresPredictionRepository constructs a PostgresPredictionRepository.
func NewPostgresPredictionRepository(db *pgxpool.Pool) *PostgresPredictionRepository {
	return &PostgresPredictionRepository{db: db}
}

const predictionColumns = "id, user_id, match_id, home_score, away_score, predicted_win_method, points, created_at, updated_at"

func scanPrediction(row pgx.Row) (*domain.Prediction, error) {
	p := &domain.Prediction{}
	if err := row.Scan(&p.ID, &p.UserID, &p.MatchID, &p.HomeScore, &p.AwayScore, &p.PredictedWinMethod, &p.Points, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, singleScanErr(err)
	}
	return p, nil
}

// Upsert inserts the prediction or, on (user_id, match_id) conflict, executes
// a no-op UPDATE that allows RETURNING to yield the existing row. The xmax
// system column distinguishes a true INSERT (xmax = 0) from a conflict-
// triggered UPDATE (xmax ≠ 0), giving the caller a reliable created flag
// without a second round-trip.
func (r *PostgresPredictionRepository) Upsert(ctx context.Context, p *domain.Prediction) (created bool, err error) {
	var wasInserted bool
	row := r.db.QueryRow(ctx,
		`INSERT INTO predictions (user_id, match_id, home_score, away_score, predicted_win_method)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, match_id) DO UPDATE
		     SET updated_at = predictions.updated_at
		 RETURNING `+predictionColumns+`, (xmax = 0) AS was_inserted`,
		p.UserID, p.MatchID, p.HomeScore, p.AwayScore, p.PredictedWinMethod,
	)
	result := &domain.Prediction{}
	if scanErr := row.Scan(
		&result.ID, &result.UserID, &result.MatchID,
		&result.HomeScore, &result.AwayScore, &result.PredictedWinMethod, &result.Points,
		&result.CreatedAt, &result.UpdatedAt,
		&wasInserted,
	); scanErr != nil {
		return false, apperrors.Internal(scanErr)
	}
	*p = *result
	return wasInserted, nil
}

func (r *PostgresPredictionRepository) Create(ctx context.Context, p *domain.Prediction) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO predictions (user_id, match_id, home_score, away_score, predicted_win_method)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+predictionColumns,
		p.UserID, p.MatchID, p.HomeScore, p.AwayScore, p.PredictedWinMethod,
	)
	result, err := scanPrediction(row)
	if err != nil {
		if isUniqueViolation(err) {
			return apperrors.Conflict(errMsgDuplicatePrediction)
		}
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
		`UPDATE predictions SET home_score=$1, away_score=$2, predicted_win_method=$3, points=$4, updated_at=NOW()
		 WHERE id=$5 RETURNING `+predictionColumns,
		p.HomeScore, p.AwayScore, p.PredictedWinMethod, p.Points, p.ID,
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

func (r *PostgresPredictionRepository) UpdateIfUnchanged(ctx context.Context, p *domain.Prediction, expectedUpdatedAt time.Time) error {
	row := r.db.QueryRow(ctx,
		`UPDATE predictions
		    SET home_score          = $1,
		        away_score          = $2,
		        predicted_win_method = $3,
		        points              = $4,
		        updated_at          = NOW()
		  WHERE id         = $5
		    AND updated_at = $6
		  RETURNING `+predictionColumns,
		p.HomeScore, p.AwayScore, p.PredictedWinMethod, p.Points, p.ID, expectedUpdatedAt,
	)
	result, err := scanPrediction(row)
	if err != nil {
		return err
	}
	if result == nil {
		return apperrors.Conflict("prediction was modified by another request; please retry")
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

// ListQuinielaIDsByMatch returns the distinct quiniela IDs for all active,
// paid members who have a prediction for matchID. One round-trip; the JOIN
// on group_memberships filters out inactive / unpaid members so that quinielas
// with no eligible participants are excluded from the snapshot run.
func (r *PostgresPredictionRepository) ListQuinielaIDsByMatch(ctx context.Context, matchID int) ([]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT DISTINCT gm.quiniela_id
		   FROM predictions p
		   JOIN group_memberships gm
		     ON gm.user_id = p.user_id
		    AND gm.status  = 'active'
		    AND gm.paid    = true
		  WHERE p.match_id = $1`,
		matchID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, apperrors.Internal(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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

// TotalPointsByQuiniela returns a map of userID -> total scored points for
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
	return collectUserPointTotals(rows)
}

// TotalPointsByQuinielaAndPhase returns a map of userID -> total scored points
// for every active, paid member of the given quiniela, restricted to matches
// in the provided phase.
//
// A member with no scored predictions in the requested phase appears in the map
// with value 0, preserving the same semantics as TotalPointsByQuiniela.
//
// Query design - derived table instead of correlated EXISTS:
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
	return collectUserPointTotals(rows)
}

func collectUserPointTotals(rows pgx.Rows) (map[int]int, error) {
	pairs, err := collectRows(rows, func(r pgx.Rows) ([2]int, error) {
		var pair [2]int
		return pair, r.Scan(&pair[0], &pair[1])
	})
	if err != nil {
		return nil, err
	}
	result := make(map[int]int, len(pairs))
	for _, p := range pairs {
		result[p[0]] = p[1]
	}
	return result, nil
}

// PredictionStatsByQuiniela returns per-user prediction statistics for every
// active, paid member of the given quiniela. A single SQL query counts
// correct, total, and exact-score predictions per member in one pass.
//
// The LEFT JOIN with the IS NOT NULL guard means only scored predictions
// (matches that have been played and evaluated) contribute to the counts.
// Members with no scored predictions appear in the result with all counts
// at zero so the ranking service can still list them in the leaderboard.
//
// FILTER aggregates are used rather than CASE expressions because they are
// clearer, and both compile to the same execution plan on PostgreSQL ≥ 9.4.
func (r *PostgresPredictionRepository) PredictionStatsByQuiniela(ctx context.Context, quinielaID int) (map[int]*domain.UserPredictionStats, error) {
	rows, err := r.db.Query(ctx,
		`SELECT
		     gm.user_id,
		     COUNT(p.id) FILTER (WHERE p.points > 0)     AS correct_count,
		     COUNT(p.id)                                  AS total_count,
		     COUNT(p.id) FILTER (WHERE p.points = $2)    AS exact_count
		   FROM group_memberships gm
		   LEFT JOIN predictions p
		          ON p.user_id = gm.user_id
		         AND p.points IS NOT NULL
		  WHERE gm.quiniela_id = $1
		    AND gm.status = 'active'
		    AND gm.paid = TRUE
		  GROUP BY gm.user_id`,
		quinielaID, domain.PointsExactScore,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	result := make(map[int]*domain.UserPredictionStats)
	for rows.Next() {
		var userID int
		s := &domain.UserPredictionStats{}
		if err := rows.Scan(&userID, &s.CorrectCount, &s.TotalCount, &s.ExactCount); err != nil {
			return nil, apperrors.Internal(err)
		}
		result[userID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return result, nil
}

// GetUserPredictionCounts returns aggregated prediction metrics for a single
// user across all quinielas in one database round-trip. A user with no
// predictions receives a zero-value struct with a nil LastPredictionAt.
//
// FILTER aggregates avoid scanning the table multiple times: PostgreSQL
// evaluates all FILTER conditions in a single pass over the matching rows.
func (r *PostgresPredictionRepository) GetUserPredictionCounts(ctx context.Context, userID int) (*domain.UserPredictionCounts, error) {
	row := r.db.QueryRow(ctx,
		`SELECT
		     COUNT(*)                                                   AS total_predictions,
		     COUNT(*) FILTER (WHERE points IS NOT NULL)                 AS scored_predictions,
		     COUNT(*) FILTER (WHERE points > 0)                        AS correct_predictions,
		     COUNT(*) FILTER (WHERE points = $2)                       AS exact_predictions,
		     COALESCE(SUM(points) FILTER (WHERE points IS NOT NULL), 0) AS total_points,
		     MAX(created_at)                                            AS last_prediction_at
		   FROM predictions
		  WHERE user_id = $1`,
		userID, domain.PointsExactScore,
	)
	c := &domain.UserPredictionCounts{}
	if err := row.Scan(
		&c.TotalPredictions,
		&c.ScoredPredictions,
		&c.CorrectPredictions,
		&c.ExactPredictions,
		&c.TotalPoints,
		&c.LastPredictionAt,
	); err != nil {
		return nil, apperrors.Internal(err)
	}
	return c, nil
}

// GetUserPointsByPhase returns a map of tournament phase to total scored
// points for a single user. Phases with no scored predictions are omitted.
// An empty map (not nil) is returned when the user has no scored predictions.
func (r *PostgresPredictionRepository) GetUserPointsByPhase(ctx context.Context, userID int) (map[domain.MatchPhase]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT m.phase, COALESCE(SUM(p.points), 0)
		   FROM predictions p
		   JOIN matches m ON m.id = p.match_id
		  WHERE p.user_id = $1
		    AND p.points IS NOT NULL
		  GROUP BY m.phase`,
		userID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	result := make(map[domain.MatchPhase]int)
	for rows.Next() {
		var phase domain.MatchPhase
		var pts int
		if err := rows.Scan(&phase, &pts); err != nil {
			return nil, apperrors.Internal(err)
		}
		result[phase] = pts
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return result, nil
}

// ListUserScoredPointsChronological returns the points values of all scored
// predictions for a user, ordered ascending by match kickoff time. Unscored
// predictions (points IS NULL) are excluded. The returned slice is consumed
// by UserStatsService to compute current and longest prediction streaks.
func (r *PostgresPredictionRepository) ListUserScoredPointsChronological(ctx context.Context, userID int) ([]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT p.points
		   FROM predictions p
		   JOIN matches m ON m.id = p.match_id
		  WHERE p.user_id = $1
		    AND p.points IS NOT NULL
		  ORDER BY m.kickoff_at ASC`,
		userID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var points []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, apperrors.Internal(err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return points, nil
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
	return collectRows(rows, func(r pgx.Rows) (*domain.Prediction, error) {
		p := &domain.Prediction{}
		return p, r.Scan(&p.ID, &p.UserID, &p.MatchID, &p.HomeScore, &p.AwayScore, &p.PredictedWinMethod, &p.Points, &p.CreatedAt, &p.UpdatedAt)
	})
}

// ListAdmin returns predictions matching the given admin filters with pagination.
func (r *PostgresPredictionRepository) ListAdmin(ctx context.Context, f PredictionAdminFilters, p Pagination) ([]*domain.Prediction, error) {
	wb := newWhereBuilder()
	if f.UserID != nil {
		wb.add("user_id = $%d", *f.UserID)
	}
	if f.MatchID != nil {
		wb.add("match_id = $%d", *f.MatchID)
	}
	if f.QuinielaID != nil {
		wb.add("user_id IN (SELECT user_id FROM group_memberships WHERE quiniela_id = $%d AND status = 'active')", *f.QuinielaID)
	}

	q := `SELECT ` + predictionColumns + ` FROM predictions` + wb.clause() + ` ORDER BY created_at DESC`
	q, pagedArgs, _, err := applyPagination(q, wb.args, wb.next(), p)
	if err != nil {
		return nil, apperrors.BadRequest(err.Error())
	}

	rows, err := r.db.Query(ctx, q, pagedArgs...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectPredictions(rows)
}

// GlobalLeaderboard returns the top limit users ranked by total scored points
// across all quinielas.
func (r *PostgresPredictionRepository) GlobalLeaderboard(ctx context.Context, limit int) ([]*domain.GlobalLeaderboardEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.name, COALESCE(SUM(p.points), 0) AS total_points
		FROM users u
		LEFT JOIN predictions p ON p.user_id = u.id AND p.points IS NOT NULL
		WHERE u.deleted_at IS NULL AND u.banned_at IS NULL
		GROUP BY u.id, u.name
		ORDER BY total_points DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var entries []*domain.GlobalLeaderboardEntry
	rank := 1
	for rows.Next() {
		e := &domain.GlobalLeaderboardEntry{Rank: rank}
		if err := rows.Scan(&e.UserID, &e.UserName, &e.TotalPoints); err != nil {
			return nil, apperrors.Internal(err)
		}
		entries = append(entries, e)
		rank++
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return entries, nil
}

// InsertScoringBatch writes one row to prediction_score_log per entry using a
// single UNNEST INSERT statement. The log is best-effort: callers must not
// treat an error here as a scoring failure.
func (r *PostgresPredictionRepository) InsertScoringBatch(ctx context.Context, entries []domain.PredictionScoreLog) error {
	if len(entries) == 0 {
		return nil
	}

	predIDs := make([]int, len(entries))
	matchIDs := make([]int, len(entries))
	userIDs := make([]int, len(entries))
	oldPts := make([]*int, len(entries))
	newPts := make([]int, len(entries))
	mHome := make([]int, len(entries))
	mAway := make([]int, len(entries))
	mWin := make([]*string, len(entries))
	mPhase := make([]string, len(entries))
	pHome := make([]int, len(entries))
	pAway := make([]int, len(entries))
	pWin := make([]*string, len(entries))
	cfgExact := make([]int, len(entries))
	cfgOutcome := make([]int, len(entries))
	cfgGoalDiff := make([]int, len(entries))
	cfgET := make([]int, len(entries))
	cfgPen := make([]int, len(entries))

	for i, e := range entries {
		predIDs[i] = e.PredictionID
		matchIDs[i] = e.MatchID
		userIDs[i] = e.UserID
		oldPts[i] = e.OldPoints
		newPts[i] = e.NewPoints
		mHome[i] = e.MatchHomeScore
		mAway[i] = e.MatchAwayScore
		if e.MatchWinMethod != nil {
			s := string(*e.MatchWinMethod)
			mWin[i] = &s
		}
		mPhase[i] = string(e.MatchPhase)
		pHome[i] = e.PredHomeScore
		pAway[i] = e.PredAwayScore
		if e.PredWinMethod != nil {
			s := string(*e.PredWinMethod)
			pWin[i] = &s
		}
		cfgExact[i] = e.CfgExactScore
		cfgOutcome[i] = e.CfgCorrectOutcome
		cfgGoalDiff[i] = e.CfgGoalDiff
		cfgET[i] = e.CfgExtraTimeBonus
		cfgPen[i] = e.CfgPenaltiesBonus
	}

	if _, err := r.db.Exec(ctx, `
		INSERT INTO prediction_score_log (
			prediction_id, match_id, user_id,
			old_points, new_points,
			match_home_score, match_away_score, match_win_method, match_phase,
			pred_home_score,  pred_away_score,  pred_win_method,
			cfg_exact_score,  cfg_correct_outcome, cfg_goal_diff,
			cfg_extra_time_bonus, cfg_penalties_bonus
		)
		SELECT * FROM UNNEST(
			$1::int[], $2::int[], $3::int[],
			$4::smallint[], $5::smallint[],
			$6::smallint[], $7::smallint[], $8::text[], $9::text[],
			$10::smallint[], $11::smallint[], $12::text[],
			$13::smallint[], $14::smallint[], $15::smallint[],
			$16::smallint[], $17::smallint[]
		)`,
		predIDs, matchIDs, userIDs,
		oldPts, newPts,
		mHome, mAway, mWin, mPhase,
		pHome, pAway, pWin,
		cfgExact, cfgOutcome, cfgGoalDiff,
		cfgET, cfgPen,
	); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

var _ PredictionRepository = (*PostgresPredictionRepository)(nil)
