package service

import (
	"context"
	"errors"
	"testing"

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

// stubTotalPointsPredRepo extends stubPredRepo with a configurable
// TotalPointsByQuiniela response for ranking service tests.
type stubTotalPointsPredRepo struct {
	stubPredRepo
	pointsByUser map[int]int
	pointsErr    error
}

func (r *stubTotalPointsPredRepo) TotalPointsByQuiniela(_ context.Context, _ int) (map[int]int, error) {
	return r.pointsByUser, r.pointsErr
}

// ── RankingService tests ──────────────────────────────────────────────────────

func TestGetLeaderboard_QuinielaNotFound_ReturnsNotFoundError(t *testing.T) {
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: nil},
		&stubPredRepo{},
		&stubUserRepo{},
	)

	_, err := svc.GetLeaderboard(context.Background(), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetLeaderboard_NoActivePaidMembers_ReturnsNil(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test"}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{}, // empty: no active+paid members
	}
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: q},
		predRepo,
		&stubUserRepo{},
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
	q := &domain.Quiniela{ID: 1, Name: "Test"}
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
	q := &domain.Quiniela{ID: 1}
	predRepo := &stubTotalPointsPredRepo{
		pointsErr: apperrors.Internal(errors.New("db error")),
	}
	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, &stubUserRepo{})

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Error("expected error from TotalPointsByQuiniela, got nil")
	}
}

func TestGetLeaderboard_DeletedUser_SkippedSilently(t *testing.T) {
	// pointsByUser has user IDs 1 and 2, but ListByIDs only returns user 2.
	// User 1's entry must be skipped without error; the leaderboard has 1 entry.
	q := &domain.Quiniela{ID: 1, Name: "Test"}
	userB := &domain.User{ID: 2, Name: "Bob"}

	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{
			1: 10, // user 1 has points but will not be returned by ListByIDs
			2: 20,
		},
	}
	// Only user 2 is returned — user 1 has been deleted.
	userRepo := &stubUserRepo{users: []*domain.User{userB}}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo)

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
	q := &domain.Quiniela{ID: 1}
	predRepo := &stubTotalPointsPredRepo{
		pointsByUser: map[int]int{1: 10}, // non-empty so we reach ListByIDs
	}
	userRepo := &stubUserRepo{err: errors.New("db error")}

	svc := NewRankingService(&stubQuinielaRepo{quiniela: q}, predRepo, userRepo)

	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from ListByIDs, got nil")
	}
}
