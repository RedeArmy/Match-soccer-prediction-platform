package service

import (
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// Match-list cache keys. All are invalidated together on any match mutation
// so that every variant of the list stays consistent.

const cacheKeyMatchesAll = "matches:all"

func cacheKeyMatchesByPhase(phase domain.MatchPhase) string {
	return "matches:phase:" + string(phase)
}

func cacheKeyMatchesByStatus(status domain.MatchStatus) string {
	return "matches:status:" + string(status)
}

// Leaderboard cache keys.

func cacheKeyLeaderboard(quinielaID int) string {
	return fmt.Sprintf("leaderboard:%d", quinielaID)
}

func cacheKeyPhaseLeaderboard(quinielaID int, phase domain.MatchPhase) string {
	return fmt.Sprintf("leaderboard:%d:phase:%s", quinielaID, phase)
}
