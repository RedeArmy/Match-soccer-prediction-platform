package service

import (
	"context"
	"errors"
	"math"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const rankingUnexpectedErrorFmt = "unexpected error: %v"

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

// stubTiebreakerRepo implements repository.TiebreakerRepository for service tests.
type stubTiebreakerRepo struct {
	tiebreakers []*domain.Tiebreaker
	err         error
}

func (r *stubTiebreakerRepo) Create(_ context.Context, _ *domain.Tiebreaker) error { return r.err }
func (r *stubTiebreakerRepo) GetByUserAndQuiniela(_ context.Context, _, _ int) (*domain.Tiebreaker, error) {
	return nil, r.err
}
func (r *stubTiebreakerRepo) Update(_ context.Context, _ *domain.Tiebreaker) error { return r.err }
func (r *stubTiebreakerRepo) ListByQuiniela(_ context.Context, _ int) ([]*domain.Tiebreaker, error) {
	return r.tiebreakers, r.err
}

// stubTotalPointsPredRepo extends stubPredRepo with configurable
// TotalPointsByQuiniela and TotalPointsByQuinielaAndPhase responses for
// ranking service tests.
type stubTotalPointsPredRepo struct {
	stubPredRepo
	pointsByUser      map[int]int
	pointsErr         error
	phasePointsByUser map[int]int
	phasePointsErr    error
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

// ── RankingService tests ──────────────────────────────────────────────────────

func TestGetLeaderboard_QuinielaNotFound_ReturnsNotFoundError(t *testing.T) {
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: nil},
		&stubPredRepo{},
		&stubUserRepo{},
		&stubTiebreakerRepo{},
		zap.NewNop(),
	)

	_, err := svc.GetLeaderboard(context.Background(), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetLeaderboard_NoActivePaidMembers_ReturnsNil(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test", PrizeThreshold: 3}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{}, // empty: no active+paid members
	}
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: q},
		predRepo,
		&stubUserRepo{},
		&stubTiebreakerRepo{},
		zap.NewNop(),
	)

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries != nil {
		t.Errorf("expected nil for empty leaderboard, got %v", entries)
	}
}

func TestGetLeaderboard_SortedByPoints(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test", PrizeThreshold: 3}
	userA := &domain.User{ID: 1, Name: "Alice"}
	userB := &domain.User{ID: 2, Name: "Bob"}
	userC := &domain.User{ID: 3, Name: "Carlos"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{
			1: 10,
			2: 25,
			3: 25, // tie with Bob
		},
	}
	userRepo := &stubUserRepo{users: []*domain.User{userA, userB, userC}}

	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: q},
		predRepo,
		userRepo,
		&stubTiebreakerRepo{},
		zap.NewNop(),
	)

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Top two should be tied at rank 1 with 25 points.
	if entries[0].TotalPoints != 25 || entries[0].Rank != 1 {
		t.Errorf("entry[0]: want rank 1 pts 25, got rank %d pts %d", entries[0].Rank, entries[0].TotalPoints)
	}
	if entries[1].TotalPoints != 25 || entries[1].Rank != 1 {
		t.Errorf("entry[1]: want rank 1 pts 25, got rank %d pts %d", entries[1].Rank, entries[1].TotalPoints)
	}
	// Third should be rank 3 (1224 competition ranking).
	if entries[2].TotalPoints != 10 || entries[2].Rank != 3 {
		t.Errorf("entry[2]: want rank 3 pts 10, got rank %d pts %d", entries[2].Rank, entries[2].TotalPoints)
	}
}

func TestGetLeaderboard_DatabaseError_PropagatesError(t *testing.T) {
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	predRepo := &stubTotalPointsPredRepo{
		pointsErr: apperrors.Internal(errors.New("db error")),
	}
	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, &stubUserRepo{}, &stubTiebreakerRepo{}, zap.NewNop())

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Error("expected error from TotalPointsByQuiniela, got nil")
	}
}

func TestGetLeaderboard_DeletedUser_SkippedSilently(t *testing.T) {
	// pointsByUser has user IDs 1 and 2, but ListByIDs only returns user 2.
	// User 1's entry must be skipped without error; the leaderboard has 1 entry.
	q := &domain.Quiniela{ID: 1, Name: "Test", PrizeThreshold: 3}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{
			1: 10, // user 1 has points but will not be returned by ListByIDs
			2: 20,
		},
	}
	// Only user 2 is returned — user 1 has been deleted.
	userRepo := &stubUserRepo{users: []*domain.User{userB}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, &stubTiebreakerRepo{}, zap.NewNop())

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (deleted user skipped), got %d", len(entries))
	}
	if entries[0].User.ID != 2 {
		t.Errorf("expected entry for user 2, got user %d", entries[0].User.ID)
	}
}

func TestGetLeaderboard_ListByIDsError_Propagated(t *testing.T) {
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 10}, // non-empty so we reach ListByIDs
	}
	userRepo := &stubUserRepo{err: errors.New("db error")}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, &stubTiebreakerRepo{}, zap.NewNop())

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from ListByIDs, got nil")
	}
}

// ── assignPrizes ──────────────────────────────────────────────────────────────

func TestAssignPrizes_Threshold3_NineMembers_ThreeWinners(t *testing.T) {
	// 9 members / threshold 3 = 3 winners
	entries := make([]*domain.LeaderboardEntry, 9)
	for i := range entries {
		entries[i] = &domain.LeaderboardEntry{User: &domain.User{ID: i + 1}, TotalPoints: 10 - i, Rank: i + 1}
	}
	assignPrizes(entries, 3)

	for i, e := range entries {
		want := i < 3
		if e.PrizeWinner != want {
			t.Errorf("entry[%d] PrizeWinner=%v, want %v", i, e.PrizeWinner, want)
		}
	}
}

func TestAssignPrizes_TwoMembers_AlwaysOneWinner(t *testing.T) {
	// 2 members / threshold 3 = 0 → clamped to 1
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1}, TotalPoints: 5, Rank: 1},
		{User: &domain.User{ID: 2}, TotalPoints: 2, Rank: 2},
	}
	assignPrizes(entries, 3)

	if !entries[0].PrizeWinner {
		t.Error("first place should always be a prize winner")
	}
	if entries[1].PrizeWinner {
		t.Error("second place should not be a prize winner")
	}
}

func TestAssignPrizes_TiedAtCutoff_AllTiedEntriesWin(t *testing.T) {
	// 6 members / threshold 3 = 2 winners; entries[1] and entries[2] are tied at rank 2
	// so all three (rank 1 + both rank-2 entries) receive PrizeWinner = true.
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1}, TotalPoints: 10, Rank: 1},
		{User: &domain.User{ID: 2}, TotalPoints: 5, Rank: 2},
		{User: &domain.User{ID: 3}, TotalPoints: 5, Rank: 2}, // tied with entry[1]
		{User: &domain.User{ID: 4}, TotalPoints: 3, Rank: 4},
		{User: &domain.User{ID: 5}, TotalPoints: 2, Rank: 5},
		{User: &domain.User{ID: 6}, TotalPoints: 1, Rank: 6},
	}
	assignPrizes(entries, 3)

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

func TestAssignPrizes_ZeroThreshold_UsesDefault(t *testing.T) {
	// prizeThreshold=0 must not panic; DefaultPrizeThreshold (3) is used instead.
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1}, TotalPoints: 10, Rank: 1},
	}
	// Should not panic.
	assignPrizes(entries, 0)
	if !entries[0].PrizeWinner {
		t.Error("sole member should be a prize winner")
	}
}

// ── GetPhaseLeaderboard ───────────────────────────────────────────────────────

func TestGetPhaseLeaderboard_QuinielaNotFound_ReturnsNotFoundError(t *testing.T) {
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: nil},
		&stubTotalPointsPredRepo{},
		&stubUserRepo{},
		&stubTiebreakerRepo{},
		zap.NewNop(),
	)

	_, err := svc.GetPhaseLeaderboard(context.Background(), 99, domain.PhaseGroupStage)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPhaseLeaderboard_NoEntries_ReturnsNil(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test", PrizeThreshold: 3}
	predRepo := &stubTotalPointsPredRepo{
		phasePointsByUser: map[int]int{},
	}
	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, &stubUserRepo{}, &stubTiebreakerRepo{}, zap.NewNop())

	entries, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries != nil {
		t.Errorf("expected nil for empty phase leaderboard, got %v", entries)
	}
}

func TestGetPhaseLeaderboard_SortedByPoints_WithPrizeWinner(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test", PrizeThreshold: 3}
	userA := &domain.User{ID: 1, Name: "Alice"}
	userB := &domain.User{ID: 2, Name: "Bob"}
	userC := &domain.User{ID: 3, Name: "Carlos"}

	predRepo := &stubTotalPointsPredRepo{
		phasePointsByUser: map[int]int{1: 10, 2: 25, 3: 5},
	}
	userRepo := &stubUserRepo{users: []*domain.User{userA, userB, userC}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, &stubTiebreakerRepo{}, zap.NewNop())

	entries, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Bob has the most points → rank 1 → prize winner (3 members / threshold 3 = 1 winner).
	if entries[0].User.ID != 2 || entries[0].Rank != 1 {
		t.Errorf("entry[0]: expected Bob rank 1, got user %d rank %d", entries[0].User.ID, entries[0].Rank)
	}
	if !entries[0].PrizeWinner {
		t.Error("rank-1 entry should be a prize winner")
	}
	if entries[1].PrizeWinner || entries[2].PrizeWinner {
		t.Error("rank 2+ entries should not be prize winners when threshold=3 and N=3")
	}
}

// ── tiebreakerDistance unit tests ────────────────────────────────────────────

func TestTiebreakerDistance_NilTiebreaker_ReturnsMaxInt(t *testing.T) {
	if got := tiebreakerDistance(nil); got != math.MaxInt {
		t.Errorf("expected math.MaxInt for nil tiebreaker, got %d", got)
	}
}

func TestTiebreakerDistance_NilResult_ReturnsMaxInt(t *testing.T) {
	tb := &domain.Tiebreaker{Prediction: 40, Result: nil}
	if got := tiebreakerDistance(tb); got != math.MaxInt {
		t.Errorf("expected math.MaxInt when result is nil, got %d", got)
	}
}

func TestTiebreakerDistance_ExceedsResult_ReturnsMaxInt(t *testing.T) {
	result := 50
	tb := &domain.Tiebreaker{Prediction: 55, Result: &result}
	if got := tiebreakerDistance(tb); got != math.MaxInt {
		t.Errorf("expected math.MaxInt for exceeded prediction, got %d", got)
	}
}

func TestTiebreakerDistance_ExactMatch_ReturnsZero(t *testing.T) {
	result := 50
	tb := &domain.Tiebreaker{Prediction: 50, Result: &result}
	if got := tiebreakerDistance(tb); got != 0 {
		t.Errorf("expected 0 for exact match, got %d", got)
	}
}

func TestTiebreakerDistance_ValidGuess_ReturnsPositiveDistance(t *testing.T) {
	result := 50
	tb := &domain.Tiebreaker{Prediction: 47, Result: &result}
	if got := tiebreakerDistance(tb); got != 3 {
		t.Errorf("expected distance 3, got %d", got)
	}
}

// ── sortAndRank tiebreaker integration ───────────────────────────────────────

func TestGetLeaderboard_TiebreakerResolvesEqualPoints(t *testing.T) {
	// Bob (distance 2) should rank above Alice (distance 5) despite equal TotalPoints.
	result := 50
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	userA := &domain.User{ID: 1, Name: "Alice"}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
	}
	tbRepo := &stubTiebreakerRepo{
		tiebreakers: []*domain.Tiebreaker{
			{UserID: 1, QuinielaID: 1, Prediction: 45, Result: &result}, // distance 5
			{UserID: 2, QuinielaID: 1, Prediction: 48, Result: &result}, // distance 2 — closer
		},
	}
	userRepo := &stubUserRepo{users: []*domain.User{userA, userB}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, tbRepo, zap.NewNop())

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].User.ID != 2 {
		t.Errorf("expected Bob (closer tiebreaker) at rank 1, got user %d", entries[0].User.ID)
	}
	// Different tiebreaker distances → different ranks; no shared rank.
	if entries[0].Rank != 1 || entries[1].Rank != 2 {
		t.Errorf("expected distinct ranks 1 and 2, got %d and %d", entries[0].Rank, entries[1].Rank)
	}
}

func TestGetLeaderboard_TiebreakerExceeded_DisqualifiesEntry(t *testing.T) {
	// Alice exceeds the result and is disqualified; Bob has a valid guess and ranks higher.
	result := 50
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	userA := &domain.User{ID: 1, Name: "Alice"}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 15, 2: 15},
	}
	tbRepo := &stubTiebreakerRepo{
		tiebreakers: []*domain.Tiebreaker{
			{UserID: 1, QuinielaID: 1, Prediction: 55, Result: &result}, // 55 > 50: disqualified
			{UserID: 2, QuinielaID: 1, Prediction: 49, Result: &result}, // valid, distance 1
		},
	}
	userRepo := &stubUserRepo{users: []*domain.User{userA, userB}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, tbRepo, zap.NewNop())

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries[0].User.ID != 2 {
		t.Errorf("expected Bob (valid tiebreaker) at rank 1, got user %d", entries[0].User.ID)
	}
	if entries[0].Rank != 1 || entries[1].Rank != 2 {
		t.Errorf("expected distinct ranks 1 and 2, got %d and %d", entries[0].Rank, entries[1].Rank)
	}
}

func TestGetLeaderboard_TiebreakerNoResult_SameRankFallsBackToUserID(t *testing.T) {
	// No tiebreaker result yet (Result == nil): both distances are MaxInt → same rank;
	// stable ordering falls back to user ID (Alice < Bob).
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	userA := &domain.User{ID: 1, Name: "Alice"}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
	}
	tbRepo := &stubTiebreakerRepo{
		tiebreakers: []*domain.Tiebreaker{
			{UserID: 1, QuinielaID: 1, Prediction: 48, Result: nil},
			{UserID: 2, QuinielaID: 1, Prediction: 45, Result: nil},
		},
	}
	userRepo := &stubUserRepo{users: []*domain.User{userA, userB}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, tbRepo, zap.NewNop())

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	// Both entries share rank 1 — tiebreaker result is not confirmed yet.
	if entries[0].Rank != 1 || entries[1].Rank != 1 {
		t.Errorf("expected both to share rank 1 (result nil), got %d and %d", entries[0].Rank, entries[1].Rank)
	}
	// Stable fallback: Alice (ID 1) is listed before Bob (ID 2).
	if entries[0].User.ID != 1 {
		t.Errorf("expected Alice (lower user ID) first when tiebreaker result is nil, got user %d", entries[0].User.ID)
	}
}

func TestGetLeaderboard_TiebreakerNoSubmission_ValidGuessBeatsMissing(t *testing.T) {
	// Bob submitted a tiebreaker; Alice did not. Bob ranks higher despite equal TotalPoints.
	result := 50
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	userA := &domain.User{ID: 1, Name: "Alice"}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 20, 2: 20},
	}
	tbRepo := &stubTiebreakerRepo{
		tiebreakers: []*domain.Tiebreaker{
			// Alice has no entry in the slice — distance will be MaxInt.
			{UserID: 2, QuinielaID: 1, Prediction: 48, Result: &result}, // distance 2
		},
	}
	userRepo := &stubUserRepo{users: []*domain.User{userA, userB}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, tbRepo, zap.NewNop())

	entries, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(rankingUnexpectedErrorFmt, err)
	}
	if entries[0].User.ID != 2 {
		t.Errorf("expected Bob (submitted tiebreaker) at rank 1, got user %d", entries[0].User.ID)
	}
	if entries[0].Rank != 1 || entries[1].Rank != 2 {
		t.Errorf("expected distinct ranks 1 and 2, got %d and %d", entries[0].Rank, entries[1].Rank)
	}
}

func TestGetLeaderboard_TiebreakerRepoError_Propagated(t *testing.T) {
	// tiebreakerRepo.ListByQuiniela returns an error after buildEntries succeeds;
	// the error must propagate to the caller.
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 10},
	}
	tbRepo := &stubTiebreakerRepo{err: errors.New("tiebreaker db error")}
	userRepo := &stubUserRepo{users: []*domain.User{{ID: 1, Name: "Alice"}}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, tbRepo, zap.NewNop())

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from tiebreakerRepo.ListByQuiniela, got nil")
	}
}

func TestGetPhaseLeaderboard_DatabaseError_PropagatesError(t *testing.T) {
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	predRepo := &stubTotalPointsPredRepo{
		phasePointsErr: apperrors.Internal(errors.New("db error")),
	}
	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, &stubUserRepo{}, &stubTiebreakerRepo{}, zap.NewNop())

	_, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err == nil {
		t.Error("expected error from TotalPointsByQuinielaAndPhase, got nil")
	}
}

func TestGetPhaseLeaderboard_TiebreakerRepoError_Propagated(t *testing.T) {
	q := &domain.Quiniela{ID: 1, PrizeThreshold: 3}
	predRepo := &stubTotalPointsPredRepo{
		phasePointsByUser: map[int]int{1: 10},
	}
	tbRepo := &stubTiebreakerRepo{err: errors.New("tiebreaker db error")}
	userRepo := &stubUserRepo{users: []*domain.User{{ID: 1, Name: "Alice"}}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo, tbRepo, zap.NewNop())

	_, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err == nil {
		t.Fatal("expected error from tiebreakerRepo.ListByQuiniela, got nil")
	}
}

// ── PrizeThreshold default in QuinielaService ─────────────────────────────────

// stubQuinielaRepo is also used from ranking_service_test.go; ensure it satisfies
// repository.QuinielaRepository including PrizeThreshold hydration in the stub.
// (No changes needed to the stub itself — it just returns the configured quiniela.)
