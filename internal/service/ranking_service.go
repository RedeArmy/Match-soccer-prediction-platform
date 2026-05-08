package service

import (
	"context"
	"fmt"
	"math"
	"sort"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// rankingService is the concrete implementation of Ranker.
type rankingService struct {
	quinielaRepo      repository.QuinielaRepository
	predRepo          repository.PredictionRepository
	userRepo          repository.UserRepository
	memberRepo        repository.GroupMembershipRepository
	tiebreakerRepo    repository.TiebreakerRepository
	tiebreakerCfgRepo repository.TiebreakerConfigRepository
	log               *zap.Logger
}

// NewRankingService constructs a rankingService with the given dependencies.
func NewRankingService(
	quinielaRepo repository.QuinielaRepository,
	predRepo repository.PredictionRepository,
	userRepo repository.UserRepository,
	memberRepo repository.GroupMembershipRepository,
	tiebreakerRepo repository.TiebreakerRepository,
	tiebreakerCfgRepo repository.TiebreakerConfigRepository,
	log *zap.Logger,
) Ranker {
	return &rankingService{
		quinielaRepo:      quinielaRepo,
		predRepo:          predRepo,
		userRepo:          userRepo,
		memberRepo:        memberRepo,
		tiebreakerRepo:    tiebreakerRepo,
		tiebreakerCfgRepo: tiebreakerCfgRepo,
		log:               log,
	}
}

// GetLeaderboard returns the overall ranked standings for the given quiniela.
//
// Only active, paid members are included. Predictions with nil points (match
// not yet scored) are excluded from the aggregation. Members with no scored
// predictions appear with TotalPoints = 0. PrizeWinner is set to true on
// entries within the prize positions determined by domain.WinnerCount.
//
// The returned LeaderboardResult includes ActivePaidMembers, WinnerCount, and
// EligibleForPrizes so the handler layer never needs a second DB round-trip
// to determine whether prizes apply. ActivePaidMembers is fetched from the
// database independently of the prediction aggregation so that members who
// have not yet submitted any predictions are still counted for prize-tier
// purposes.
//
// Ranking algorithm - standard competition ranking (1224…):
// Two members with equal points receive the same rank. The rank after a tie
// of N members at position P is P+N (not P+1). This is the most common
// expectation in tournament contexts.
//
// Tie-breaking proceeds through a four-rule chain applied in order:
//  1. Most correct predictions (CorrectCount DESC).
//  2. Fewest predictions submitted (TotalCount ASC).
//  3. Most exact-score hits (ExactCount DESC).
//  4. Closest tiebreaker forecast (TiebreakerDistance ASC); math.MaxInt when
//     the result has not been confirmed yet or the member never submitted.
//
// If all four rules still yield a tie, user ID is used as a last-resort
// stable sort key so the output order is always reproducible.
//
// The implementation is four database round-trips:
//  1. QuinielaRepository.GetByID — verify the group exists.
//  2. GroupMembershipRepository.CountActivePaid — authoritative prize count.
//  3. PredictionRepository.TotalPointsByQuiniela — O(members) LEFT JOIN.
//  4. UserRepository.ListByIDs + TiebreakerRepository.ListByUserIDs — O(members).
//
// No N+1 queries regardless of group size.
func (s *rankingService) GetLeaderboard(ctx context.Context, quinielaID int) (*LeaderboardResult, error) {
	q, err := s.quinielaRepo.GetByID(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", quinielaID))
	}

	pointsByUser, err := s.predRepo.TotalPointsByQuiniela(ctx, quinielaID)
	if err != nil {
		return nil, err
	}

	return s.buildLeaderboard(ctx, quinielaID, pointsByUser)
}

// GetPhaseLeaderboard returns standings restricted to predictions on matches in
// the specified tournament phase. The algorithm is identical to GetLeaderboard
// but delegates point aggregation to TotalPointsByQuinielaAndPhase so only
// phase-relevant predictions are counted.
//
// Note: ActivePaidMembers and prize metadata always reflect the full group
// membership, not just members who have predictions in the given phase. This
// is intentional: prize eligibility is a group-level property, not a
// phase-level one.
func (s *rankingService) GetPhaseLeaderboard(ctx context.Context, quinielaID int, phase domain.MatchPhase) (*LeaderboardResult, error) {
	q, err := s.quinielaRepo.GetByID(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", quinielaID))
	}

	pointsByUser, err := s.predRepo.TotalPointsByQuinielaAndPhase(ctx, quinielaID, phase)
	if err != nil {
		return nil, err
	}

	return s.buildLeaderboard(ctx, quinielaID, pointsByUser)
}

// buildLeaderboard is the shared core of GetLeaderboard and GetPhaseLeaderboard.
// It fetches the authoritative active-paid count, hydrates entries from
// pointsByUser, attaches tiebreaker distances and prediction stats, then sorts,
// ranks, and marks prize winners.
//
// pointsByUser may be empty (no scored predictions yet); in that case the
// result is returned with a nil Entries slice and correct prize metadata.
func (s *rankingService) buildLeaderboard(ctx context.Context, quinielaID int, pointsByUser map[int]int) (*LeaderboardResult, error) {
	// Fetch the authoritative active-paid count independently of the prediction
	// aggregation. This ensures members who have not submitted any predictions
	// are still counted for prize-tier purposes.
	activePaid, err := s.memberRepo.CountActivePaid(ctx, quinielaID)
	if err != nil {
		return nil, err
	}

	result := &LeaderboardResult{
		ActivePaidMembers: activePaid,
		WinnerCount:       domain.WinnerCount(activePaid),
		EligibleForPrizes: domain.EligibleForPayments(activePaid),
	}

	if len(pointsByUser) == 0 {
		result.Entries = nil
		return result, nil
	}

	entries, err := s.buildEntries(ctx, quinielaID, pointsByUser)
	if err != nil {
		return nil, err
	}

	stats, err := s.fetchPredictionStats(ctx, quinielaID)
	if err != nil {
		return nil, err
	}

	userIDs := extractUserIDs(entries)
	distances, err := s.fetchTiebreakerDistances(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	sortAndRank(entries, stats, distances)
	assignPrizes(entries, activePaid)

	result.Entries = entries
	return result, nil
}

// buildEntries hydrates LeaderboardEntry values from a userID->points map.
// It fetches all user objects in a single batch query and logs a warning for
// any user ID that is absent from the users table (soft-deleted users).
//
// Go map iteration order is randomised by the runtime on every execution, so
// the slice returned here has non-deterministic element ordering. This is safe
// because sortAndRank operates on the complete slice after construction and
// imposes a fully deterministic final order. Do not change this function to
// stream entries incrementally (e.g. write into a channel before the slice is
// complete) without ensuring sortAndRank still receives the full set - partial
// construction would surface the map non-determinism in the final output.
func (s *rankingService) buildEntries(ctx context.Context, quinielaID int, pointsByUser map[int]int) ([]*domain.LeaderboardEntry, error) {
	userIDs := make([]int, 0, len(pointsByUser))
	for id := range pointsByUser {
		userIDs = append(userIDs, id)
	}
	users, err := s.userRepo.ListByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	userByID := make(map[int]*domain.User, len(users))
	for _, u := range users {
		userByID[u.ID] = u
	}

	entries := make([]*domain.LeaderboardEntry, 0, len(pointsByUser))
	for userID, pts := range pointsByUser {
		u, ok := userByID[userID]
		if !ok {
			s.log.Warn("leaderboard: skipping member absent from users table - likely soft-deleted",
				zap.Int("user_id", userID),
				zap.Int("quiniela_id", quinielaID),
			)
			continue
		}
		entries = append(entries, &domain.LeaderboardEntry{
			User:        u,
			TotalPoints: pts,
		})
	}
	return entries, nil
}

// fetchPredictionStats returns a userID-keyed map of prediction statistics for
// the given quiniela. An empty map is returned when the repository returns nil
// so the ranking logic always operates on a valid (possibly empty) map.
func (s *rankingService) fetchPredictionStats(ctx context.Context, quinielaID int) (map[int]*domain.UserPredictionStats, error) {
	m, err := s.predRepo.PredictionStatsByQuiniela(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return make(map[int]*domain.UserPredictionStats), nil
	}
	return m, nil
}

// extractUserIDs returns the user IDs from a slice of leaderboard entries.
func extractUserIDs(entries []*domain.LeaderboardEntry) []int {
	ids := make([]int, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.User.ID)
	}
	return ids
}

// fetchTiebreakerDistances loads the global tiebreaker config and each user's
// prediction, then pre-computes the absolute distance for every user ID.
// Returns a map[userID]distance. When the global result has not been confirmed
// yet, all distances are math.MaxInt.
func (s *rankingService) fetchTiebreakerDistances(ctx context.Context, userIDs []int) (map[int]int, error) {
	cfg, err := s.tiebreakerCfgRepo.Get(ctx)
	if err != nil {
		return nil, err
	}

	distances := make(map[int]int, len(userIDs))
	for _, id := range userIDs {
		distances[id] = math.MaxInt
	}

	if cfg == nil || cfg.Result == nil {
		return distances, nil
	}

	tbs, err := s.tiebreakerRepo.ListByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, tb := range tbs {
		d := tb.Prediction - *cfg.Result
		if d < 0 {
			d = -d
		}
		distances[tb.UserID] = d
	}
	return distances, nil
}

// tiebreakerDistance looks up the pre-computed distance for userID.
// Returns math.MaxInt when userID is absent from the map.
func tiebreakerDistance(distances map[int]int, userID int) int {
	if d, ok := distances[userID]; ok {
		return d
	}
	return math.MaxInt
}

// statsFor returns the prediction stats for userID, or a zero-value struct when
// the user has no entry in the map (e.g. no scored predictions yet). Using a
// zero-value rather than nil avoids nil-pointer guards throughout the comparator.
func statsFor(stats map[int]*domain.UserPredictionStats, userID int) domain.UserPredictionStats {
	if s, ok := stats[userID]; ok && s != nil {
		return *s
	}
	return domain.UserPredictionStats{}
}

// sortAndRank sorts entries descending by TotalPoints and applies standard
// competition ranks (1224…).
//
// Tie-breaking proceeds through a four-rule chain:
//  1. CorrectCount DESC - the member with more correct predictions ranks higher.
//  2. TotalCount ASC   - among equally-correct members, fewer submissions ranks higher.
//  3. ExactCount DESC  - among members with the same correct and total counts,
//     more exact-score hits (PointsExactScore = 5) ranks higher.
//  4. TiebreakerDistance ASC - closest numeric forecast to the confirmed result
//     ranks higher; math.MaxInt when result not yet confirmed or member did not
//     submit.
//
// If all four rules still cannot separate two entries, user ID is used as a
// final stable sort key. This last step is not a business rule - it exists
// solely to guarantee a reproducible output order across identical datasets.
// Do not replace it with an unstable comparator such as a hash or pointer address.
func sortAndRank(entries []*domain.LeaderboardEntry, stats map[int]*domain.UserPredictionStats, distances map[int]int) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].TotalPoints != entries[j].TotalPoints {
			return entries[i].TotalPoints > entries[j].TotalPoints
		}
		si := statsFor(stats, entries[i].User.ID)
		sj := statsFor(stats, entries[j].User.ID)
		if si.CorrectCount != sj.CorrectCount {
			return si.CorrectCount > sj.CorrectCount
		}
		if si.TotalCount != sj.TotalCount {
			return si.TotalCount < sj.TotalCount
		}
		if si.ExactCount != sj.ExactCount {
			return si.ExactCount > sj.ExactCount
		}
		di := tiebreakerDistance(distances, entries[i].User.ID)
		dj := tiebreakerDistance(distances, entries[j].User.ID)
		if di != dj {
			return di < dj
		}
		return entries[i].User.ID < entries[j].User.ID
	})
	assignRanks(entries, stats, distances)
}

// sameRank reports whether a and b are indistinguishable on all sort dimensions
// and therefore share the same competition rank. TotalPoints, all three
// prediction stats, and tiebreaker distance must all be equal.
func sameRank(a, b *domain.LeaderboardEntry, stats map[int]*domain.UserPredictionStats, distances map[int]int) bool {
	if a.TotalPoints != b.TotalPoints {
		return false
	}
	sa := statsFor(stats, a.User.ID)
	sb := statsFor(stats, b.User.ID)
	if sa.CorrectCount != sb.CorrectCount || sa.TotalCount != sb.TotalCount || sa.ExactCount != sb.ExactCount {
		return false
	}
	return tiebreakerDistance(distances, a.User.ID) == tiebreakerDistance(distances, b.User.ID)
}

// assignRanks applies standard competition ranking (1224…) to a pre-sorted
// slice of leaderboard entries.
//
// Two entries share a rank only when they are indistinguishable on all sort
// dimensions: equal TotalPoints, equal values on all three prediction stats,
// and equal tiebreaker distance. The next rank is skipped for the size of the
// tied group.
func assignRanks(entries []*domain.LeaderboardEntry, stats map[int]*domain.UserPredictionStats, distances map[int]int) {
	for i := 0; i < len(entries); i++ {
		if i == 0 || !sameRank(entries[i], entries[i-1], stats, distances) {
			entries[i].Rank = i + 1
		} else {
			entries[i].Rank = entries[i-1].Rank
		}
	}
}

// assignPrizes marks entries as PrizeWinner = true based on the fixed prize
// tiers defined by domain.WinnerCount(activePaidMembers).
//
// activePaidMembers is the authoritative count of active, paid members in the
// group — it must come from GroupMembershipRepository.CountActivePaid, not
// from len(entries). The two values differ when some paid members have not yet
// submitted any predictions: those members appear in the member roster but not
// in the leaderboard entries slice. Using len(entries) would under-count the
// group size and assign fewer prizes than the business rules require.
//
// All entries whose Rank is ≤ winnerCount are marked, including tied entries
// that share a rank at the boundary — so the actual number of winners can
// exceed winnerCount when a tie falls exactly on the cut-off rank.
// The entries slice must already be sorted and ranked before this is called.
func assignPrizes(entries []*domain.LeaderboardEntry, activePaidMembers int) {
	if len(entries) == 0 {
		return
	}
	winnerCount := domain.WinnerCount(activePaidMembers)
	if winnerCount == 0 {
		return // group below minimum; no prizes
	}
	if winnerCount > len(entries) {
		// Fewer entries than winners (some paid members have no predictions yet).
		// Mark all current entries as prize winners; the remaining prize slots
		// will be filled as those members submit and score predictions.
		for _, e := range entries {
			e.PrizeWinner = true
		}
		return
	}
	// Read the rank of the winnerCount-th entry (0-indexed) to capture all
	// members tied at the boundary rank.
	cutoffRank := entries[winnerCount-1].Rank
	for _, e := range entries {
		e.PrizeWinner = e.Rank <= cutoffRank
	}
}

var _ Ranker = (*rankingService)(nil)
