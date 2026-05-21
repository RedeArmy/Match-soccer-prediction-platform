package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification/scheduler"
)

// PostgresSchedulerStore implements scheduler.Store backed by PostgreSQL.
type PostgresSchedulerStore struct {
	db *pgxpool.Pool
}

// NewPostgresSchedulerStore constructs a PostgresSchedulerStore.
func NewPostgresSchedulerStore(db *pgxpool.Pool) *PostgresSchedulerStore {
	return &PostgresSchedulerStore{db: db}
}

// CountPendingTransfers returns the number of bank transfer proofs awaiting review.
func (s *PostgresSchedulerStore) CountPendingTransfers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM bank_transfer_proofs WHERE status = 'pending'`).Scan(&n)
	return n, err
}

// CountPendingWithdrawals returns the number of withdrawal requests awaiting approval.
func (s *PostgresSchedulerStore) CountPendingWithdrawals(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM withdrawal_requests WHERE status = 'pending'`).Scan(&n)
	return n, err
}

// OldestPendingTransferSince returns the created_at of the oldest pending proof.
func (s *PostgresSchedulerStore) OldestPendingTransferSince(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRow(ctx,
		`SELECT COALESCE(MIN(created_at), '0001-01-01') FROM bank_transfer_proofs WHERE status = 'pending'`,
	).Scan(&t)
	return t, err
}

// DailySummary returns aggregated operational metrics for the 24-hour window
// starting at since.
func (s *PostgresSchedulerStore) DailySummary(ctx context.Context, since time.Time) (scheduler.DailySummaryRow, error) {
	until := since.Add(24 * time.Hour)

	var row scheduler.DailySummaryRow

	// New users registered in the window.
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE created_at >= $1 AND created_at < $2`,
		since, until,
	).Scan(&row.NewUsers); err != nil {
		return row, err
	}

	// Bank transfer activity.
	if err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE created_at >= $1 AND created_at < $2),
			COUNT(*) FILTER (WHERE status = 'approved' AND updated_at >= $1 AND updated_at < $2),
			COALESCE(SUM(amount_cents) FILTER (WHERE status = 'approved' AND updated_at >= $1 AND updated_at < $2), 0)
		FROM bank_transfer_proofs`,
		since, until,
	).Scan(&row.NewTransfers, &row.ApprovedTransfers, &row.TotalCreditedCents); err != nil {
		return row, err
	}

	// Withdrawal activity.
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM withdrawal_requests WHERE created_at >= $1 AND created_at < $2`,
		since, until,
	).Scan(&row.NewWithdrawals); err != nil {
		return row, err
	}

	// Current pending counts (point-in-time, not windowed).
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM bank_transfer_proofs WHERE status = 'pending'`,
	).Scan(&row.PendingTransfers); err != nil {
		return row, err
	}
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM withdrawal_requests WHERE status = 'pending'`,
	).Scan(&row.PendingWithdrawals); err != nil {
		return row, err
	}

	return row, nil
}

// WeeklySummary returns aggregated metrics for the 7-day window starting at since.
func (s *PostgresSchedulerStore) WeeklySummary(ctx context.Context, since time.Time) (scheduler.WeeklySummaryRow, error) {
	until := since.Add(7 * 24 * time.Hour)

	var row scheduler.WeeklySummaryRow

	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE created_at >= $1 AND created_at < $2`,
		since, until,
	).Scan(&row.NewUsers); err != nil {
		return row, err
	}

	if err := s.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount_cents) FILTER (WHERE status = 'approved'), 0),
			COUNT(*) FILTER (WHERE status != 'pending')
		FROM withdrawal_requests WHERE created_at >= $1 AND created_at < $2`,
		since, until,
	).Scan(&row.WithdrawalCents, &row.TotalWithdrawals); err != nil {
		return row, err
	}

	// Total credited revenue = sum of approved bank transfers.
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_cents), 0)
		FROM bank_transfer_proofs
		WHERE status = 'approved' AND updated_at >= $1 AND updated_at < $2`,
		since, until,
	).Scan(&row.TotalRevenueCents); err != nil {
		return row, err
	}

	// Active quinielas: those whose members submitted at least one prediction in the window.
	if err := s.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT gm.quiniela_id)
		FROM predictions p
		JOIN group_memberships gm ON gm.user_id = p.user_id AND gm.status = 'active'
		WHERE p.created_at >= $1 AND p.created_at < $2`,
		since, until,
	).Scan(&row.ActiveQuinielas); err != nil {
		return row, err
	}

	// Top leaderboard group: highest total points scored in the window.
	_ = s.db.QueryRow(ctx, `
		SELECT q.name, COALESCE(SUM(p.points), 0) AS pts
		FROM quinielas q
		JOIN group_memberships gm ON gm.quiniela_id = q.id AND gm.status = 'active'
		JOIN predictions p ON p.user_id = gm.user_id
		WHERE p.updated_at >= $1 AND p.updated_at < $2
		GROUP BY q.id, q.name
		ORDER BY pts DESC
		LIMIT 1`,
		since, until,
	).Scan(&row.TopGroupName, &row.TopGroupPoints)

	return row, nil
}

// ListFinishedMatchesMissingResult returns matches in "finished" status that
// have no score recorded.
func (s *PostgresSchedulerStore) ListFinishedMatchesMissingResult(ctx context.Context) ([]*domain.Match, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, home_team, away_team, kickoff_at
		FROM matches
		WHERE status = 'finished'
		  AND (home_score IS NULL OR away_score IS NULL)
		ORDER BY kickoff_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []*domain.Match
	for rows.Next() {
		m := &domain.Match{}
		if err := rows.Scan(&m.ID, &m.HomeTeam, &m.AwayTeam, &m.KickoffAt); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// ListStaleBankTransfers returns pending bank transfer proofs whose created_at
// is strictly before before, ordered by created_at ascending.
func (s *PostgresSchedulerStore) ListStaleBankTransfers(ctx context.Context, before time.Time) ([]*domain.BankTransferProof, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, amount_cents, currency, created_at
		FROM bank_transfer_proofs
		WHERE status = 'pending' AND created_at < $1
		ORDER BY created_at ASC`,
		before,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var proofs []*domain.BankTransferProof
	for rows.Next() {
		p := &domain.BankTransferProof{}
		if err := rows.Scan(&p.ID, &p.UserID, &p.AmountCents, &p.Currency, &p.CreatedAt); err != nil {
			return nil, err
		}
		proofs = append(proofs, p)
	}
	return proofs, rows.Err()
}

// ListStaleWithdrawals returns pending withdrawal requests whose created_at
// is strictly before before, ordered by created_at ascending.
func (s *PostgresSchedulerStore) ListStaleWithdrawals(ctx context.Context, before time.Time) ([]*domain.WithdrawalRequest, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, amount_cents, currency, created_at
		FROM withdrawal_requests
		WHERE status = 'pending' AND created_at < $1
		ORDER BY created_at ASC`,
		before,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []*domain.WithdrawalRequest
	for rows.Next() {
		r := &domain.WithdrawalRequest{}
		if err := rows.Scan(&r.ID, &r.UserID, &r.AmountCents, &r.Currency, &r.CreatedAt); err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	return reqs, rows.Err()
}

// ListUpcomingMatchesWithDeadline returns matches whose kickoff is within
// deadlineWindow and that have users with no prediction submitted.
func (s *PostgresSchedulerStore) ListUpcomingMatchesWithDeadline(ctx context.Context, deadlineWindow time.Duration) ([]scheduler.DeadlineMatch, error) {
	now := time.Now().UTC()
	cutoff := now.Add(deadlineWindow)

	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT m.id, m.home_team, m.away_team, m.kickoff_at,
		       u.id AS user_id
		FROM matches m
		JOIN quinielas q ON TRUE
		JOIN group_memberships gm ON gm.quiniela_id = q.id AND gm.status = 'active'
		JOIN users u ON u.id = gm.user_id
		LEFT JOIN predictions p ON p.match_id = m.id AND p.user_id = u.id
		WHERE m.status = 'scheduled'
		  AND m.kickoff_at > $1
		  AND m.kickoff_at <= $2
		  AND p.id IS NULL
		ORDER BY m.kickoff_at ASC, u.id ASC`,
		now, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byMatch := make(map[int]*scheduler.DeadlineMatch)
	var order []int

	for rows.Next() {
		var matchID, userID int
		var homeTeam, awayTeam string
		var kickoffAt time.Time
		if err := rows.Scan(&matchID, &homeTeam, &awayTeam, &kickoffAt, &userID); err != nil {
			return nil, err
		}

		if _, ok := byMatch[matchID]; !ok {
			byMatch[matchID] = &scheduler.DeadlineMatch{
				Match: &domain.Match{
					ID:        matchID,
					HomeTeam:  homeTeam,
					AwayTeam:  awayTeam,
					KickoffAt: kickoffAt,
				},
				MinutesLeft: int(time.Until(kickoffAt).Minutes()),
			}
			order = append(order, matchID)
		}
		byMatch[matchID].MissingUserIDs = append(byMatch[matchID].MissingUserIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]scheduler.DeadlineMatch, 0, len(order))
	for _, id := range order {
		result = append(result, *byMatch[id])
	}
	return result, nil
}
