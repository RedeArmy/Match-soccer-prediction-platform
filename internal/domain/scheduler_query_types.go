package domain

import "time"

// DailyReminderMatch carries the minimal details about one upcoming match for
// which a user has not yet submitted a prediction. Returned by the scheduler
// store and consumed by the daily-reminder notification job.
//
// Defined here (domain) so that both the repository layer (which queries the
// DB to build the result) and the notification layer (which renders it into
// emails and push payloads) can reference it without creating a cross-layer
// import between repository and notification.
type DailyReminderMatch struct {
	MatchID   int       `json:"match_id"`
	HomeTeam  string    `json:"home_team"`
	AwayTeam  string    `json:"away_team"`
	KickoffAt time.Time `json:"kickoff_at"`
}

// DailySummaryMatchRow is one row in the per-day results table included in
// daily-summary notification emails. PredHome/PredAway are nil when the user
// did not submit a prediction for this match.
type DailySummaryMatchRow struct {
	MatchID      int       `json:"match_id"`
	HomeTeam     string    `json:"home_team"`
	AwayTeam     string    `json:"away_team"`
	KickoffAt    time.Time `json:"kickoff_at"`
	HomeScore    int       `json:"home_score"`
	AwayScore    int       `json:"away_score"`
	PredHome     *int      `json:"pred_home"`
	PredAway     *int      `json:"pred_away"`
	PointsEarned int       `json:"points_earned"`
}

// DailySummaryPayload carries all data needed to render a per-user daily
// summary email without additional DB queries. One payload per user per day,
// independent of quiniela membership.
type DailySummaryPayload struct {
	UserID      int                    `json:"user_id"`
	MatchDate   string                 `json:"match_date"` // "2026-06-22" in the user's local timezone
	Timezone    string                 `json:"timezone"`   // IANA timezone used to compute MatchDate
	Matches     []DailySummaryMatchRow `json:"matches"`
	PointsToday int                    `json:"points_today"`
}

// DailySummaryRow carries aggregated operational metrics for a 24-hour window.
// Returned by the scheduler store and consumed by the admin daily-summary job.
type DailySummaryRow struct {
	NewUsers           int
	NewTransfers       int
	ApprovedTransfers  int
	TotalCreditedCents int
	NewWithdrawals     int
	PendingTransfers   int
	PendingWithdrawals int
}

// WeeklySummaryRow carries aggregated metrics for a 7-day window.
// Returned by the scheduler store and consumed by the admin weekly-summary job.
type WeeklySummaryRow struct {
	TotalRevenueCents int
	NewUsers          int
	ActiveQuinielas   int
	TopGroupName      string
	TopGroupPoints    int
	TotalWithdrawals  int
	WithdrawalCents   int
}

// DeadlineMatch pairs a match with the user IDs that have not yet submitted a
// prediction for it, and the minutes remaining until kickoff. Used by the
// prediction-deadline notification job.
type DeadlineMatch struct {
	Match          *Match
	MissingUserIDs []int
	MinutesLeft    int
}
