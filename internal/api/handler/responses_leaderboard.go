package handler

// LeaderboardEntryResponse is the JSON representation of a single leaderboard entry.
type LeaderboardEntryResponse struct {
	Rank        int    `json:"rank"`
	UserID      int    `json:"user_id"`
	UserName    string `json:"user_name"`
	TotalPoints int    `json:"total_points"`
	PrizeWinner bool   `json:"prize_winner"`
}

// LeaderboardResponse wraps the ranked entries returned by GET …/leaderboard.
//
// Prize metadata fields:
//   - active_paid_members: count of active members who have settled their entry
//     fee. This is the authoritative input to winner_count and eligible_for_prizes.
//   - winner_count: number of prize positions for this group size, derived from
//     the fixed tier table (5→2, 6-9→3, 10-14→4, 15-20→5). Zero when the group
//     has fewer than 5 active paid members.
//   - eligible_for_prizes: true when active_paid_members ≥ 5. When false, no
//     prizes will be distributed regardless of the entry_fee setting.
type LeaderboardResponse struct {
	QuinielaID        int                        `json:"quiniela_id"`
	Phase             string                     `json:"phase,omitempty"` // empty string omitted for the overall leaderboard
	ActivePaidMembers int                        `json:"active_paid_members"`
	WinnerCount       int                        `json:"winner_count"`
	EligibleForPrizes bool                       `json:"eligible_for_prizes"`
	Entries           []LeaderboardEntryResponse `json:"entries"`
}
