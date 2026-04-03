package service

// TODO: implement RankingService.
//
// GetLeaderboard returns the ranked list of players for a given Quiniela,
// sorted by total points descending. When two or more players are tied on
// points, the Tiebreaker forecast is used to determine final order:
// the player whose forecast is closest to the actual result (without exceeding
// it) ranks higher. Players who did not submit a tiebreaker rank below those
// who did, regardless of points.
