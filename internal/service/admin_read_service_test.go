package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

const (
	adminReadUnexpectedErr = "unexpected error: %v"
	adminReadDBError       = "db error"
)

func newAdminReadSvc(
	predRepo *stubTotalPointsPredRepo,
	userRepo *stubUserRepo,
	tbRepo *stubTiebreakerRepo,
	snapRepo *stubSnapshotRepo,
) AdminReadService {
	return NewAdminReadService(predRepo, userRepo, tbRepo, snapRepo, zap.NewNop())
}

// ── GlobalLeaderboard ─────────────────────────────────────────────────────────

func TestAdminReadService_GlobalLeaderboard_ReturnsEntries(t *testing.T) {
	entries := []*domain.GlobalLeaderboardEntry{{Rank: 1, UserID: 1, TotalPoints: 30}}
	predRepo := &stubTotalPointsPredRepo{}
	predRepo.globalEntries = entries
	svc := newAdminReadSvc(predRepo, &stubUserRepo{}, &stubTiebreakerRepo{}, &stubSnapshotRepo{})

	got, err := svc.GlobalLeaderboard(context.Background(), 10)
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 entry, got %d", len(got))
	}
}

func TestAdminReadService_GlobalLeaderboard_PropagatesError(t *testing.T) {
	predRepo := &stubTotalPointsPredRepo{}
	predRepo.globalErr = errors.New(adminReadDBError)
	svc := newAdminReadSvc(predRepo, &stubUserRepo{}, &stubTiebreakerRepo{}, &stubSnapshotRepo{})

	_, err := svc.GlobalLeaderboard(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── ListPredictions ───────────────────────────────────────────────────────────

func TestAdminReadService_ListPredictions_ReturnsSlice(t *testing.T) {
	preds := []*domain.Prediction{{ID: 1}, {ID: 2}}
	predRepo := &stubTotalPointsPredRepo{}
	predRepo.adminList = preds
	svc := newAdminReadSvc(predRepo, &stubUserRepo{}, &stubTiebreakerRepo{}, &stubSnapshotRepo{})

	got, err := svc.ListPredictions(context.Background(), repository.PredictionAdminFilters{}, repository.Pagination{Limit: 10})
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
}

// ── ListTiebreakerSubmissions ─────────────────────────────────────────────────

func TestAdminReadService_ListTiebreakerSubmissions_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newAdminReadSvc(&stubTotalPointsPredRepo{}, &stubUserRepo{}, &stubTiebreakerRepo{}, &stubSnapshotRepo{})

	got, err := svc.ListTiebreakerSubmissions(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(got))
	}
}

func TestAdminReadService_ListTiebreakerSubmissions_ResolvesUserNames(t *testing.T) {
	tbs := []*domain.Tiebreaker{{ID: 1, UserID: 10, Prediction: 3}}
	users := []*domain.User{{ID: 10, Name: "alice"}}
	tbRepo := &stubTiebreakerRepo{tbs: tbs}
	userRepo := &stubUserRepo{users: users}
	svc := newAdminReadSvc(&stubTotalPointsPredRepo{}, userRepo, tbRepo, &stubSnapshotRepo{})

	got, err := svc.ListTiebreakerSubmissions(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 view, got %d", len(got))
	}
	if got[0].UserName != "alice" {
		t.Errorf("expected UserName 'alice', got %q", got[0].UserName)
	}
}

func TestAdminReadService_ListTiebreakerSubmissions_UserRepoError_StillReturns(t *testing.T) {
	tbs := []*domain.Tiebreaker{{ID: 1, UserID: 10, Prediction: 3}}
	tbRepo := &stubTiebreakerRepo{tbs: tbs}
	userRepo := &stubUserRepo{err: errors.New(adminReadDBError)}
	svc := newAdminReadSvc(&stubTotalPointsPredRepo{}, userRepo, tbRepo, &stubSnapshotRepo{})

	got, err := svc.ListTiebreakerSubmissions(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf("unexpected error from ListTiebreakerSubmissions when user lookup fails: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 view even when user names are unresolvable, got %d", len(got))
	}
}

func TestAdminReadService_ListTiebreakerSubmissions_TiebreakerRepoError_PropagatesError(t *testing.T) {
	tbRepo := &stubTiebreakerRepo{err: errors.New(adminReadDBError)}
	svc := newAdminReadSvc(&stubTotalPointsPredRepo{}, &stubUserRepo{}, tbRepo, &stubSnapshotRepo{})

	_, err := svc.ListTiebreakerSubmissions(context.Background(), repository.Pagination{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── ListSnapshotHistory ───────────────────────────────────────────────────────

func TestAdminReadService_ListSnapshotHistory_ReturnsSnapshots(t *testing.T) {
	snaps := []*domain.LeaderboardSnapshot{{ID: 1, QuinielaID: 5}}
	snapRepo := &stubSnapshotRepo{snapshots: snaps}
	svc := newAdminReadSvc(&stubTotalPointsPredRepo{}, &stubUserRepo{}, &stubTiebreakerRepo{}, snapRepo)

	got, err := svc.ListSnapshotHistory(context.Background(), 5, 10)
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(got))
	}
}
