package service

import (
	"context"
	"errors"
	"math"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	rankingUnexpectedErrorFmt = "unexpected error: %v"
	rankingAlice              = "Alice"
	rankingCarlos             = "Carlos"
	rankingTestQuiniela       = "Test"
	rankingDistinctRanksFmt   = "expected distinct ranks, got %d and %d"
	rankingDBError            = "db error"
	ranking3EntriesFmt        = "expected 3 entries, got %d"
)

// stubUserRepo implements repository.UserRepository for service tests.
type stubUserRepo struct {
	user  *domain.User
	users []*domain.User
	err   error
}

func (r *stubUserRepo) Create(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) Update(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepo) Delete(_ context.Context, _ int) error          { return r.err }
func (r *stubUserRepo) List(_ context.Context) ([]*domain.User, error) { return r.users, r.err }
func (r *stubUserRepo) ListByIDs(_ context.Context, _ []int) ([]*domain.User, error) {
	return r.users, r.err
}
func (r *stubUserRepo) Ban(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) Unban(_ context.Context, _ int) error                 { return r.err }
func (r *stubUserRepo) ListBanned(_ context.Context) ([]*domain.User, error) { return nil, r.err }
func (r *stubUserRepo) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.Pagination) ([]*domain.User, error) {
	return r.users, r.err
}
func (r *stubUserRepo) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, r.err
}

// stubTotalPointsPredRepo extends stubPredRepo with configurable
// TotalPointsByQuiniela, TotalPointsByQuinielaAndPhase, and
// PredictionStatsByQuiniela responses for ranking service tests.
type stubTotalPointsPredRepo struct {
	stubPredRepo
	pointsByUser      map[int]int
	pointsErr         error
	phasePointsByUser map[int]int
	phasePointsErr    error
	statsByUser       map[int]*domain.UserPredictionStats
	statsErr          error
	globalEntries     []*domain.GlobalLeaderboardEntry
	globalErr         error
	adminList         []*domain.Prediction
	adminListErr      error
}

func (r *stubTotalPointsPredRepo) TotalPointsByQuiniela(_ context.Context, _ int) (map[int]int, error) {
	return r.pointsByUser, r.pointsErr
}

func (r *stubTotalPointsPredRepo) TotalPointsByQuinielaAndPhase(_ context.Context, _ int, _ domain.MatchPhase) (map[int]int, error) {
	if r.phasePointsByUser != nil || r.phasePointsErr != nil {
		return r.phasePointsByUser, r.phasePointsErr
	}
	return r.pointsByUser, r.pointsErr
}

func (r *stubTotalPointsPredRepo) PredictionStatsByQuiniela(_ context.Context, _ int) (map[int]*domain.UserPredictionStats, error) {
	return r.statsByUser, r.statsErr
}

func (r *stubTotalPointsPredRepo) GlobalLeaderboard(_ context.Context, _ int) ([]*domain.GlobalLeaderboardEntry, error) {
	return r.globalEntries, r.globalErr
}

func (r *stubTotalPointsPredRepo) ListAdmin(_ context.Context, _ repository.PredictionAdminFilters, _ repository.Pagination) ([]*domain.Prediction, error) {
	return r.adminList, r.adminListErr
}

// stubTiebreakerRepo implements repository.TiebreakerRepository for ranking tests.
type stubTiebreakerRepo struct {
	tbs []*domain.Tiebreaker
	err error
}

func (r *stubTiebreakerRepo) Upsert(_ context.Context, _ *domain.Tiebreaker) error { return r.err }
func (r *stubTiebreakerRepo) GetByUser(_ context.Context, _, _ int) (*domain.Tiebreaker, error) {
	return nil, r.err
}
func (r *stubTiebreakerRepo) Update(_ context.Context, _ *domain.Tiebreaker) error { return r.err }
func (r *stubTiebreakerRepo) ListByUserIDs(_ context.Context, _ []int) ([]*domain.Tiebreaker, error) {
	return r.tbs, r.err
}
func (r *stubTiebreakerRepo) ListByUserIDsForConfig(_ context.Context, _ []int, _ int) ([]*domain.Tiebreaker, error) {
	return r.tbs, r.err
}
func (r *stubTiebreakerRepo) ListAll(_ context.Context, _ repository.Pagination) ([]*domain.Tiebreaker, error) {
	return r.tbs, r.err
}

// stubTiebreakerCfgRepo implements repository.TiebreakerConfigRepository for ranking tests.
type stubTiebreakerCfgRepo struct {
	cfg *domain.TiebreakerConfig
	err error
}

func (r *stubTiebreakerCfgRepo) Get(_ context.Context) (*domain.TiebreakerConfig, error) {
	return r.cfg, r.err
}
func (r *stubTiebreakerCfgRepo) GetByPhase(_ context.Context, _ domain.MatchPhase) (*domain.TiebreakerConfig, error) {
	return nil, r.err
}
func (r *stubTiebreakerCfgRepo) GetByQuiniela(_ context.Context, _ int) (*domain.TiebreakerConfig, error) {
	return nil, r.err
}
func (r *stubTiebreakerCfgRepo) Upsert(_ context.Context, _ string) (*domain.TiebreakerConfig, error) {
	return r.cfg, r.err
}
func (r *stubTiebreakerCfgRepo) UpsertForPhase(_ context.Context, phase domain.MatchPhase, _ string) (*domain.TiebreakerConfig, error) {
	return r.cfg, r.err
}
func (r *stubTiebreakerCfgRepo) UpsertForQuiniela(_ context.Context, _ int, _ string) (*domain.TiebreakerConfig, error) {
	return r.cfg, r.err
}
func (r *stubTiebreakerCfgRepo) SetResult(_ context.Context, _ int) error        { return r.err }
func (r *stubTiebreakerCfgRepo) SetResultByID(_ context.Context, _, _ int) error { return r.err }

// ── helpers ───────────────────────────────────────────────────────────────────

func newRankingSvc(q *domain.Quiniela, predRepo *stubTotalPointsPredRepo, users []*domain.User) Ranker {
	return NewRankingService(
		&stubQuinielaRepo{quiniela: q},
		predRepo,
		&stubUserRepo{users: users},
		&stubMemberRepo{activeCount: len(users)},
		&stubTiebreakerRepo{},
		&stubTiebreakerCfgRepo{},
		zap.NewNop(),
	)
}

// ── GetLeaderboard basic cases ────────────────────────────────────────────────

func TestGetLeaderboard_QuinielaNotFound_ReturnsNotFoundError(t *testing.T) {
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: nil},
		&stubPredRepo{},
		&stubUserRepo{},
		&stubMemberRepo{},
		&stubTiebreakerRepo{},
		&stubTiebreakerCfgRepo{},
		zap.NewNop(),
	)

	_, err := svc.GetLeaderboard(context.Background(), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetLeaderboard_NoActivePaidMembers_ReturnsNil(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: rankingTestQuiniela}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{},
	}
	svc := newRankingSvc(q, predRepo, nil)

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries.Entries != nil {
		t.Errorf("expected nil for empty leaderboard, got %v", entries.Entries)
	}
}

func TestGetLeaderboard_SortedByPoints(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: rankingTestQuiniela}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}
	userC := &domain.User{ID: 3, Name: rankingCarlos}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 10, 2: 25, 3: 25},
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA, userB, userC})

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries.Entries) != 3 {
		t.Fatalf(ranking3EntriesFmt, len(entries.Entries))
	}

	if entries.Entries[0].TotalPoints != 25 || entries.Entries[0].Rank != 1 {
		t.Errorf("entry[0]: want rank 1 pts 25, got rank %d pts %d", entries.Entries[0].Rank, entries.Entries[0].TotalPoints)
	}
	if entries.Entries[1].TotalPoints != 25 || entries.Entries[1].Rank != 1 {
		t.Errorf("entry[1]: want rank 1 pts 25, got rank %d pts %d", entries.Entries[1].Rank, entries.Entries[1].TotalPoints)
	}
	if entries.Entries[2].TotalPoints != 10 || entries.Entries[2].Rank != 3 {
		t.Errorf("entry[2]: want rank 3 pts 10, got rank %d pts %d", entries.Entries[2].Rank, entries.Entries[2].TotalPoints)
	}
}

func TestGetLeaderboard_DatabaseError_PropagatesError(t *testing.T) {
	q := &domain.Quiniela{ID: 1}
	predRepo := &stubTotalPointsPredRepo{
		pointsErr: apperrors.Internal(errors.New(rankingDBError)),
	}
	svc := newRankingSvc(q, predRepo, nil)

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Error("expected error from TotalPointsByQuiniela, got nil")
	}
}

func TestGetLeaderboard_DeletedUser_SkippedSilently(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: rankingTestQuiniela}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 10, 2: 20},
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userB})

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries.Entries) != 1 {
		t.Fatalf("expected 1 entry (deleted user skipped), got %d", len(entries.Entries))
	}
	if entries.Entries[0].User.ID != 2 {
		t.Errorf("expected entry for user 2, got user %d", entries.Entries[0].User.ID)
	}
}

func TestGetLeaderboard_ListByIDsError_Propagated(t *testing.T) {
	q := &domain.Quiniela{ID: 1}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 10},
	}
	userRepo := &stubUserRepo{err: errors.New(rankingDBError)}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, &stubMemberRepo{}, &stubTiebreakerRepo{}, &stubTiebreakerCfgRepo{}, zap.NewNop())

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from ListByIDs, got nil")
	}
}

func TestGetLeaderboard_PredictionStatsError_Propagated(t *testing.T) {
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 10},
		statsErr:     errors.New("stats db error"),
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA})

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from PredictionStatsByQuiniela, got nil")
	}
}

// ── Tiebreaker rule 1: CorrectCount DESC ──────────────────────────────────────

func TestGetLeaderboard_CorrectCountBreaksTie(t *testing.T) {
	// Alice 8 correct, Bob 5 correct - same total points -> Alice ranks higher.
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
		statsByUser: map[int]*domain.UserPredictionStats{
			1: {CorrectCount: 8, TotalCount: 10, ExactCount: 2},
			2: {CorrectCount: 5, TotalCount: 10, ExactCount: 2},
		},
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA, userB})

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries.Entries))
	}
	if entries.Entries[0].User.ID != 1 {
		t.Errorf("expected Alice (more correct) at rank 1, got user %d", entries.Entries[0].User.ID)
	}
	if entries.Entries[0].Rank != 1 || entries.Entries[1].Rank != 2 {
		t.Errorf(rankingDistinctRanksFmt, entries.Entries[0].Rank, entries.Entries[1].Rank)
	}
}

// ── Tiebreaker rule 2: TotalCount ASC ────────────────────────────────────────

func TestGetLeaderboard_TotalCountBreaksTie_WhenCorrectCountEqual(t *testing.T) {
	// Alice 5 total, Bob 7 total - same correct count -> Alice ranks higher (fewer is better).
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
		statsByUser: map[int]*domain.UserPredictionStats{
			1: {CorrectCount: 4, TotalCount: 5, ExactCount: 1},
			2: {CorrectCount: 4, TotalCount: 7, ExactCount: 1},
		},
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA, userB})

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries.Entries[0].User.ID != 1 {
		t.Errorf("expected Alice (fewer predictions) at rank 1, got user %d", entries.Entries[0].User.ID)
	}
	if entries.Entries[0].Rank != 1 || entries.Entries[1].Rank != 2 {
		t.Errorf(rankingDistinctRanksFmt, entries.Entries[0].Rank, entries.Entries[1].Rank)
	}
}

// ── Tiebreaker rule 3: ExactCount DESC ────────────────────────────────────────

func TestGetLeaderboard_ExactCountBreaksTie_WhenFirstTwoRulesEqual(t *testing.T) {
	// Alice 3 exact, Bob 1 exact - same correct and total counts -> Alice ranks higher.
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
		statsByUser: map[int]*domain.UserPredictionStats{
			1: {CorrectCount: 4, TotalCount: 6, ExactCount: 3},
			2: {CorrectCount: 4, TotalCount: 6, ExactCount: 1},
		},
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA, userB})

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries.Entries[0].User.ID != 1 {
		t.Errorf("expected Alice (more exact hits) at rank 1, got user %d", entries.Entries[0].User.ID)
	}
	if entries.Entries[0].Rank != 1 || entries.Entries[1].Rank != 2 {
		t.Errorf(rankingDistinctRanksFmt, entries.Entries[0].Rank, entries.Entries[1].Rank)
	}
}

// ── All three rules equal -> shared rank, stable fallback ──────────────────────

func TestGetLeaderboard_AllStatsEqual_SameRank_FallsBackToUserID(t *testing.T) {
	// All stats identical -> both share rank 1; stable fallback sorts by user ID.
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
		statsByUser: map[int]*domain.UserPredictionStats{
			1: {CorrectCount: 4, TotalCount: 6, ExactCount: 2},
			2: {CorrectCount: 4, TotalCount: 6, ExactCount: 2},
		},
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA, userB})

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries.Entries[0].Rank != 1 || entries.Entries[1].Rank != 1 {
		t.Errorf("expected both to share rank 1, got %d and %d", entries.Entries[0].Rank, entries.Entries[1].Rank)
	}
	if entries.Entries[0].User.ID != 1 {
		t.Errorf("expected Alice (lower user ID) listed first when all stats equal, got user %d", entries.Entries[0].User.ID)
	}
}

func TestGetLeaderboard_EmptyStats_SameRank_FallsBackToUserID(t *testing.T) {
	// Stats map is nil -> statsFor returns zero values for all users -> same rank.
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
		statsByUser:  nil,
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA, userB})

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries.Entries[0].Rank != 1 || entries.Entries[1].Rank != 1 {
		t.Errorf("expected both to share rank 1 with empty stats, got %d and %d", entries.Entries[0].Rank, entries.Entries[1].Rank)
	}
}

// ── Tiebreaker chain: all three rules fire in sequence ────────────────────────

func TestGetLeaderboard_TiebreakerChain_CorrectCountFirst(t *testing.T) {
	// Three-way tie on points; rule 1 separates Carlos, rules 2&3 still needed for Alice/Bob.
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}
	userC := &domain.User{ID: 3, Name: rankingCarlos}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20, 3: 20},
		statsByUser: map[int]*domain.UserPredictionStats{
			1: {CorrectCount: 4, TotalCount: 5, ExactCount: 3}, // rule2 wins vs Bob
			2: {CorrectCount: 4, TotalCount: 7, ExactCount: 3}, // rule2 loses vs Alice
			3: {CorrectCount: 2, TotalCount: 4, ExactCount: 1}, // rule1 loses to all
		},
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA, userB, userC})

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries.Entries) != 3 {
		t.Fatalf(ranking3EntriesFmt, len(entries.Entries))
	}

	// Alice rank 1, Bob rank 2, Carlos rank 3
	if entries.Entries[0].User.ID != 1 || entries.Entries[0].Rank != 1 {
		t.Errorf("entry[0]: expected Alice rank 1, got user %d rank %d", entries.Entries[0].User.ID, entries.Entries[0].Rank)
	}
	if entries.Entries[1].User.ID != 2 || entries.Entries[1].Rank != 2 {
		t.Errorf("entry[1]: expected Bob rank 2, got user %d rank %d", entries.Entries[1].User.ID, entries.Entries[1].Rank)
	}
	if entries.Entries[2].User.ID != 3 || entries.Entries[2].Rank != 3 {
		t.Errorf("entry[2]: expected Carlos rank 3, got user %d rank %d", entries.Entries[2].User.ID, entries.Entries[2].Rank)
	}
}

// ── assignPrizes ──────────────────────────────────────────────────────────────

func makeRankedEntries(n int) []*domain.LeaderboardEntry {
	entries := make([]*domain.LeaderboardEntry, n)
	for i := range entries {
		entries[i] = &domain.LeaderboardEntry{User: &domain.User{ID: i + 1}, TotalPoints: n - i, Rank: i + 1}
	}
	return entries
}

func TestAssignPrizes_FiveMembers_TwoWinners(t *testing.T) {
	entries := makeRankedEntries(5)
	assignPrizes(entries, 5)
	for i, e := range entries {
		want := i < 2 // WinnerCount(5) = 2
		if e.PrizeWinner != want {
			t.Errorf("entry[%d] PrizeWinner=%v, want %v", i, e.PrizeWinner, want)
		}
	}
}

func TestAssignPrizes_NineMembers_ThreeWinners(t *testing.T) {
	entries := makeRankedEntries(9)
	assignPrizes(entries, 9)
	for i, e := range entries {
		want := i < 3 // WinnerCount(9) = 3
		if e.PrizeWinner != want {
			t.Errorf("entry[%d] PrizeWinner=%v, want %v", i, e.PrizeWinner, want)
		}
	}
}

func TestAssignPrizes_FourMembers_NoPrizes(t *testing.T) {
	// Below MinMembersPerGroup: no one wins.
	entries := makeRankedEntries(4)
	assignPrizes(entries, 4)
	for _, e := range entries {
		if e.PrizeWinner {
			t.Error("no entry should be a prize winner in a sub-minimum group")
		}
	}
}

func TestAssignPrizes_TiedAtCutoff_AllTiedEntriesWin(t *testing.T) {
	// 6 members → WinnerCount(6) = 3. Entries at rank 2 are tied.
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1}, TotalPoints: 10, Rank: 1},
		{User: &domain.User{ID: 2}, TotalPoints: 5, Rank: 2},
		{User: &domain.User{ID: 3}, TotalPoints: 5, Rank: 2},
		{User: &domain.User{ID: 4}, TotalPoints: 3, Rank: 4},
		{User: &domain.User{ID: 5}, TotalPoints: 2, Rank: 5},
		{User: &domain.User{ID: 6}, TotalPoints: 1, Rank: 6},
	}
	assignPrizes(entries, 6)

	// Entries 0–2 are rank 1 and rank 2 (both tied rank-2s included).
	for i := 0; i < 3; i++ {
		if !entries[i].PrizeWinner {
			t.Errorf("entry[%d] should be prize winner (tied at cutoff rank)", i)
		}
	}
	for i := 3; i < 6; i++ {
		if entries[i].PrizeWinner {
			t.Errorf("entry[%d] should not be a prize winner", i)
		}
	}
}

// ── GetPhaseLeaderboard ───────────────────────────────────────────────────────

func TestGetPhaseLeaderboard_QuinielaNotFound_ReturnsNotFoundError(t *testing.T) {
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: nil},
		&stubTotalPointsPredRepo{},
		&stubUserRepo{},
		&stubMemberRepo{},
		&stubTiebreakerRepo{},
		&stubTiebreakerCfgRepo{},
		zap.NewNop(),
	)

	_, err := svc.GetPhaseLeaderboard(context.Background(), 99, domain.PhaseGroupStage)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPhaseLeaderboard_NoEntries_ReturnsNil(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: rankingTestQuiniela}
	predRepo := &stubTotalPointsPredRepo{
		phasePointsByUser: map[int]int{},
	}
	svc := newRankingSvc(q, predRepo, nil)

	entries, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries.Entries != nil {
		t.Errorf("expected nil for empty phase leaderboard, got %v", entries.Entries)
	}
}

func TestGetPhaseLeaderboard_SortedByPoints_WithPrizeWinner(t *testing.T) {
	// 5 members → tier awards prizes to top 2 (WinnerCount(5) = 2).
	q := &domain.Quiniela{ID: 1, Name: rankingTestQuiniela}
	users := []*domain.User{
		{ID: 1, Name: rankingAlice},
		{ID: 2, Name: "Bob"},
		{ID: 3, Name: rankingCarlos},
		{ID: 4, Name: "Diana"},
		{ID: 5, Name: "Eve"},
	}
	predRepo := &stubTotalPointsPredRepo{
		phasePointsByUser: map[int]int{1: 10, 2: 25, 3: 5, 4: 20, 5: 15},
	}
	svc := newRankingSvc(q, predRepo, users)

	entries, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries.Entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries.Entries))
	}
	// Rank 1: Bob (25pts), Rank 2: Diana (20pts) — both are prize winners.
	if entries.Entries[0].User.ID != 2 || entries.Entries[0].Rank != 1 {
		t.Errorf("entry[0]: expected Bob rank 1, got user %d rank %d", entries.Entries[0].User.ID, entries.Entries[0].Rank)
	}
	if !entries.Entries[0].PrizeWinner {
		t.Error("rank-1 entry should be a prize winner")
	}
	if !entries.Entries[1].PrizeWinner {
		t.Error("rank-2 entry should be a prize winner for a 5-member group")
	}
	for _, e := range entries.Entries[2:] {
		if e.PrizeWinner {
			t.Errorf("rank %d entry should not be a prize winner (only top 2 win in a 5-member group)", e.Rank)
		}
	}
}

func TestGetPhaseLeaderboard_DatabaseError_PropagatesError(t *testing.T) {
	q := &domain.Quiniela{ID: 1}
	predRepo := &stubTotalPointsPredRepo{
		phasePointsErr: apperrors.Internal(errors.New(rankingDBError)),
	}
	svc := newRankingSvc(q, predRepo, nil)

	_, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err == nil {
		t.Error("expected error from TotalPointsByQuinielaAndPhase, got nil")
	}
}

func TestGetPhaseLeaderboard_StatsError_Propagated(t *testing.T) {
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	predRepo := &stubTotalPointsPredRepo{
		phasePointsByUser: map[int]int{1: 10},
		statsErr:          errors.New("stats db error"),
	}
	svc := newRankingSvc(q, predRepo, []*domain.User{userA})

	_, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err == nil {
		t.Fatal("expected error from PredictionStatsByQuiniela, got nil")
	}
}

// ── statsFor unit tests ───────────────────────────────────────────────────────

func TestStatsFor_NilMap_ReturnsZeroValue(t *testing.T) {
	s := statsFor(nil, 42)
	if s.CorrectCount != 0 || s.TotalCount != 0 || s.ExactCount != 0 {
		t.Errorf("expected zero-value stats for nil map, got %+v", s)
	}
}

func TestStatsFor_MissingKey_ReturnsZeroValue(t *testing.T) {
	m := map[int]*domain.UserPredictionStats{1: {CorrectCount: 3}}
	s := statsFor(m, 99)
	if s.CorrectCount != 0 || s.TotalCount != 0 || s.ExactCount != 0 {
		t.Errorf("expected zero-value stats for missing key, got %+v", s)
	}
}

func TestStatsFor_NilEntry_ReturnsZeroValue(t *testing.T) {
	m := map[int]*domain.UserPredictionStats{1: nil}
	s := statsFor(m, 1)
	if s.CorrectCount != 0 || s.TotalCount != 0 || s.ExactCount != 0 {
		t.Errorf("expected zero-value stats for nil entry, got %+v", s)
	}
}

func TestStatsFor_ValidEntry_ReturnsStats(t *testing.T) {
	want := domain.UserPredictionStats{CorrectCount: 5, TotalCount: 8, ExactCount: 2}
	m := map[int]*domain.UserPredictionStats{1: &want}
	got := statsFor(m, 1)
	if got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}

// ── sameRank unit tests ───────────────────────────────────────────────────────

func TestSameRank_DifferentPoints_ReturnsFalse(t *testing.T) {
	a := &domain.LeaderboardEntry{User: &domain.User{ID: 1}, TotalPoints: 10}
	b := &domain.LeaderboardEntry{User: &domain.User{ID: 2}, TotalPoints: 5}
	if sameRank(a, b, nil, nil) {
		t.Error("entries with different TotalPoints must not share a rank")
	}
}

func TestSameRank_SamePointsDifferentCorrectCount_ReturnsFalse(t *testing.T) {
	stats := map[int]*domain.UserPredictionStats{
		1: {CorrectCount: 5, TotalCount: 8, ExactCount: 2},
		2: {CorrectCount: 3, TotalCount: 8, ExactCount: 2},
	}
	a := &domain.LeaderboardEntry{User: &domain.User{ID: 1}, TotalPoints: 20}
	b := &domain.LeaderboardEntry{User: &domain.User{ID: 2}, TotalPoints: 20}
	if sameRank(a, b, stats, nil) {
		t.Error("entries with different CorrectCount must not share a rank")
	}
}

func TestSameRank_SamePointsDifferentTotalCount_ReturnsFalse(t *testing.T) {
	stats := map[int]*domain.UserPredictionStats{
		1: {CorrectCount: 4, TotalCount: 5, ExactCount: 2},
		2: {CorrectCount: 4, TotalCount: 9, ExactCount: 2},
	}
	a := &domain.LeaderboardEntry{User: &domain.User{ID: 1}, TotalPoints: 20}
	b := &domain.LeaderboardEntry{User: &domain.User{ID: 2}, TotalPoints: 20}
	if sameRank(a, b, stats, nil) {
		t.Error("entries with different TotalCount must not share a rank")
	}
}

func TestSameRank_SamePointsDifferentExactCount_ReturnsFalse(t *testing.T) {
	stats := map[int]*domain.UserPredictionStats{
		1: {CorrectCount: 4, TotalCount: 6, ExactCount: 3},
		2: {CorrectCount: 4, TotalCount: 6, ExactCount: 1},
	}
	a := &domain.LeaderboardEntry{User: &domain.User{ID: 1}, TotalPoints: 20}
	b := &domain.LeaderboardEntry{User: &domain.User{ID: 2}, TotalPoints: 20}
	if sameRank(a, b, stats, nil) {
		t.Error("entries with different ExactCount must not share a rank")
	}
}

func TestSameRank_AllDimensionsEqual_ReturnsTrue(t *testing.T) {
	stats := map[int]*domain.UserPredictionStats{
		1: {CorrectCount: 4, TotalCount: 6, ExactCount: 2},
		2: {CorrectCount: 4, TotalCount: 6, ExactCount: 2},
	}
	a := &domain.LeaderboardEntry{User: &domain.User{ID: 1}, TotalPoints: 20}
	b := &domain.LeaderboardEntry{User: &domain.User{ID: 2}, TotalPoints: 20}
	if !sameRank(a, b, stats, nil) {
		t.Error("entries identical on all dimensions must share a rank")
	}
}

// ── tiebreakerDistance unit tests ────────────────────────────────────────────

func TestTiebreakerDistance_NoEntry_ReturnsMathMaxInt(t *testing.T) {
	d := tiebreakerDistance(nil, 1)
	if d != math.MaxInt {
		t.Errorf("expected math.MaxInt for nil map, got %d", d)
	}
}

func TestTiebreakerDistance_UserAbsent_ReturnsMathMaxInt(t *testing.T) {
	distances := map[int]int{2: 3}
	d := tiebreakerDistance(distances, 1)
	if d != math.MaxInt {
		t.Errorf("expected math.MaxInt when user absent from map, got %d", d)
	}
}

func TestTiebreakerDistance_ExactMatch_ReturnsZero(t *testing.T) {
	distances := map[int]int{1: 0}
	d := tiebreakerDistance(distances, 1)
	if d != 0 {
		t.Errorf("expected 0 for exact match, got %d", d)
	}
}

func TestTiebreakerDistance_AbsoluteDifference(t *testing.T) {
	distances := map[int]int{1: 3}
	d := tiebreakerDistance(distances, 1)
	if d != 3 {
		t.Errorf("expected 3, got %d", d)
	}
}

// ── Tiebreaker rule 4: TiebreakerDistance ASC ────────────────────────────────

func TestGetLeaderboard_TiebreakerDistanceBreaksTie_WhenAllStatsEqual(t *testing.T) {
	// Alice predicts 8, Bob predicts 15; result is 10.
	// Alice distance=2, Bob distance=5 -> Alice ranks higher.
	result := 10
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
		statsByUser: map[int]*domain.UserPredictionStats{
			1: {CorrectCount: 4, TotalCount: 6, ExactCount: 2},
			2: {CorrectCount: 4, TotalCount: 6, ExactCount: 2},
		},
	}
	tbRepo := &stubTiebreakerRepo{
		tbs: []*domain.Tiebreaker{
			{UserID: 1, Prediction: 8},
			{UserID: 2, Prediction: 15},
		},
	}
	cfgRepo := &stubTiebreakerCfgRepo{cfg: &domain.TiebreakerConfig{Result: &result}}
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: q},
		predRepo,
		&stubUserRepo{users: []*domain.User{userA, userB}},
		&stubMemberRepo{},
		tbRepo,
		cfgRepo,
		zap.NewNop(),
	)

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries.Entries))
	}
	if entries.Entries[0].User.ID != 1 {
		t.Errorf("expected Alice (closer tiebreaker) at rank 1, got user %d", entries.Entries[0].User.ID)
	}
	if entries.Entries[0].Rank != 1 || entries.Entries[1].Rank != 2 {
		t.Errorf(rankingDistinctRanksFmt, entries.Entries[0].Rank, entries.Entries[1].Rank)
	}
}

func TestGetLeaderboard_TiebreakerDistanceEqual_SameRank(t *testing.T) {
	// Both predict exactly the result -> distance=0 for both -> shared rank.
	result := 10
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
		statsByUser: map[int]*domain.UserPredictionStats{
			1: {CorrectCount: 4, TotalCount: 6, ExactCount: 2},
			2: {CorrectCount: 4, TotalCount: 6, ExactCount: 2},
		},
	}
	tbRepo := &stubTiebreakerRepo{
		tbs: []*domain.Tiebreaker{
			{UserID: 1, Prediction: 10},
			{UserID: 2, Prediction: 10},
		},
	}
	cfgRepo := &stubTiebreakerCfgRepo{cfg: &domain.TiebreakerConfig{Result: &result}}
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: q},
		predRepo,
		&stubUserRepo{users: []*domain.User{userA, userB}},
		&stubMemberRepo{},
		tbRepo,
		cfgRepo,
		zap.NewNop(),
	)

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries.Entries[0].Rank != 1 || entries.Entries[1].Rank != 1 {
		t.Errorf("expected both to share rank 1 with equal tiebreaker distance, got %d and %d",
			entries.Entries[0].Rank, entries.Entries[1].Rank)
	}
}

func TestGetLeaderboard_TiebreakerRepoError_Propagated(t *testing.T) {
	q := &domain.Quiniela{ID: 1}
	userA := &domain.User{ID: 1, Name: rankingAlice}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 10},
	}
	result := 10
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: q},
		predRepo,
		&stubUserRepo{users: []*domain.User{userA}},
		&stubMemberRepo{},
		&stubTiebreakerRepo{err: errors.New(rankingDBError)},
		&stubTiebreakerCfgRepo{cfg: &domain.TiebreakerConfig{Result: &result}},
		zap.NewNop(),
	)

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from tiebreaker repo, got nil")
	}
}

// stubQuinielaRepo is also used from ranking_service_test.go; ensure it satisfies
// repository.QuinielaRepository.
