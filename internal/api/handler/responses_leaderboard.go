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
type LeaderboardResponse struct {
	QuinielaID int                        `json:"quiniela_id"`
	Phase      string                     `json:"phase,omitempty"` // empty string omitted for the overall leaderboard
	Entries    []LeaderboardEntryResponse `json:"entries"`
}
