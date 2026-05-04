package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// zeroIntParamService returns 0 for all GetInt calls - used to disable the
// dashboard cache (TTL of 0 means skip caching entirely).
type zeroIntParamService struct{ noopSystemParamService }

func (*zeroIntParamService) GetInt(_ context.Context, _ string, _ int) int { return 0 }

const (
	adminReadUnexpectedErr = "unexpected error: %v"
	adminReadDBError       = "db error"
	adminReadExpectErr     = "expected error, got nil"
)

func newAdminReadSvc(
	predRepo *stubTotalPointsPredRepo,
	userRepo *stubUserRepo,
	tbRepo *stubTiebreakerRepo,
	snapRepo *stubSnapshotRepo,
) AdminReadService {
	return NewAdminReadService(
		AdminReadRepos{Pred: predRepo, User: userRepo, Quiniela: &stubQuinielaRepo{}, Payment: &stubPaymentRepo{}, Tiebreaker: tbRepo, Snapshot: snapRepo},
		&noopSystemParamService{}, zap.NewNop(),
	)
}

func newAdminReadSvcWithRepos(
	quinielaRepo *stubQuinielaRepo,
	paymentRepo *stubPaymentRepo,
	userRepo *stubUserRepo,
) AdminReadService {
	return NewAdminReadService(
		AdminReadRepos{Pred: &stubTotalPointsPredRepo{}, User: userRepo, Quiniela: quinielaRepo, Payment: paymentRepo, Tiebreaker: &stubTiebreakerRepo{}, Snapshot: &stubSnapshotRepo{}},
		&noopSystemParamService{}, zap.NewNop(),
	)
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

func newAdminReadSvcWithCache(predRepo *stubTotalPointsPredRepo, store *stubCacheStore) AdminReadService {
	return NewAdminReadService(
		AdminReadRepos{
			Pred:        predRepo,
			User:        &stubUserRepo{},
			Quiniela:    &stubQuinielaRepo{},
			Payment:     &stubPaymentRepo{},
			Tiebreaker:  &stubTiebreakerRepo{},
			Snapshot:    &stubSnapshotRepo{},
			GlobalCache: store,
		},
		&noopSystemParamService{}, zap.NewNop(),
	)
}

func TestAdminReadService_GlobalLeaderboard_CacheHit_NoDbCall(t *testing.T) {
	st := newStubCache()
	entries := []*domain.GlobalLeaderboardEntry{{Rank: 1, UserID: 1, TotalPoints: 30}}
	st.seed(cacheKeyGlobalLeaderboard(10), entries)

	predRepo := &stubTotalPointsPredRepo{}
	predRepo.globalErr = errors.New("should not reach db on cache hit")
	svc := newAdminReadSvcWithCache(predRepo, st)

	got, err := svc.GlobalLeaderboard(context.Background(), 10)
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 entry from cache, got %d", len(got))
	}
}

func TestAdminReadService_GlobalLeaderboard_CacheMiss_SetsCache(t *testing.T) {
	st := newStubCache()
	entries := []*domain.GlobalLeaderboardEntry{{Rank: 1, UserID: 2, TotalPoints: 20}}
	predRepo := &stubTotalPointsPredRepo{globalEntries: entries}
	svc := newAdminReadSvcWithCache(predRepo, st)

	_, err := svc.GlobalLeaderboard(context.Background(), 5)
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if st.setCalls != 1 {
		t.Errorf("expected 1 cache Set call on miss, got %d", st.setCalls)
	}
}

func TestAdminReadService_GlobalLeaderboard_EmptyResult_NotCached(t *testing.T) {
	st := newStubCache()
	predRepo := &stubTotalPointsPredRepo{globalEntries: []*domain.GlobalLeaderboardEntry{}}
	svc := newAdminReadSvcWithCache(predRepo, st)

	_, err := svc.GlobalLeaderboard(context.Background(), 5)
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if st.setCalls != 0 {
		t.Errorf("empty results must not be cached, got %d Set calls", st.setCalls)
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

	got, err := svc.ListTiebreakerSubmissions(context.Background(), repository.Unbounded())
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

	got, err := svc.ListTiebreakerSubmissions(context.Background(), repository.Unbounded())
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

	got, err := svc.ListTiebreakerSubmissions(context.Background(), repository.Unbounded())
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

	_, err := svc.ListTiebreakerSubmissions(context.Background(), repository.Unbounded())
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

// ── GetDashboardStats ─────────────────────────────────────────────────────────

func TestAdminReadService_GetDashboardStats_PropagatesAllCounts(t *testing.T) {
	paymentRepo := &stubPaymentRepo{
		statusCounts: repository.PaymentStatusCounts{
			Pending:        2,
			Confirmed:      5,
			Rejected:       1,
			TotalCollected: 15000,
		},
	}
	svc := newAdminReadSvcWithRepos(&stubQuinielaRepo{}, paymentRepo, &stubUserRepo{})

	got, err := svc.GetDashboardStats(context.Background())
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if got.Payments.TotalCollected != 15000 {
		t.Errorf("expected TotalCollected=15000, got %d", got.Payments.TotalCollected)
	}
	if got.Payments.Pending != 2 {
		t.Errorf("expected Pending=2, got %d", got.Payments.Pending)
	}
	if got.Payments.Confirmed != 5 {
		t.Errorf("expected Confirmed=5, got %d", got.Payments.Confirmed)
	}
}

func TestAdminReadService_GetDashboardStats_QuinielaRepoError_Propagates(t *testing.T) {
	svc := newAdminReadSvcWithRepos(
		&stubQuinielaRepo{err: errors.New(adminReadDBError)},
		&stubPaymentRepo{},
		&stubUserRepo{},
	)
	_, err := svc.GetDashboardStats(context.Background())
	if err == nil {
		t.Fatal(adminReadExpectErr)
	}
}

func TestAdminReadService_GetDashboardStats_UserRepoError_Propagates(t *testing.T) {
	svc := newAdminReadSvcWithRepos(
		&stubQuinielaRepo{},
		&stubPaymentRepo{},
		&stubUserRepo{err: errors.New(adminReadDBError)},
	)
	_, err := svc.GetDashboardStats(context.Background())
	if err == nil {
		t.Fatal(adminReadExpectErr)
	}
}

func TestAdminReadService_GetDashboardStats_PaymentRepoError_Propagates(t *testing.T) {
	svc := newAdminReadSvcWithRepos(
		&stubQuinielaRepo{},
		&stubPaymentRepo{err: errors.New(adminReadDBError)},
		&stubUserRepo{},
	)
	_, err := svc.GetDashboardStats(context.Background())
	if err == nil {
		t.Fatal(adminReadExpectErr)
	}
}

func TestAdminReadService_GetDashboardStats_Cache_SecondCallDoesNotHitRepo(t *testing.T) {
	qr := &stubQuinielaRepo{}
	svc := newAdminReadSvcWithRepos(qr, &stubPaymentRepo{}, &stubUserRepo{})

	first, err := svc.GetDashboardStats(context.Background())
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	// Poison the repo so a second DB hit would fail.
	qr.err = errors.New("should not be called")

	second, err := svc.GetDashboardStats(context.Background())
	if err != nil {
		t.Fatalf(adminReadUnexpectedErr, err)
	}
	if first != second {
		t.Error("expected second call to return cached stats pointer")
	}
}

func TestAdminReadService_GetDashboardStats_Cache_ZeroTTL_DisablesCache(t *testing.T) {
	qr := &stubQuinielaRepo{}
	// noopSystemParamService returns 0 for GetInt -> cache disabled.
	svc := NewAdminReadService(
		AdminReadRepos{Pred: &stubTotalPointsPredRepo{}, User: &stubUserRepo{}, Quiniela: qr, Payment: &stubPaymentRepo{}, Tiebreaker: &stubTiebreakerRepo{}, Snapshot: &stubSnapshotRepo{}},
		&zeroIntParamService{}, zap.NewNop(),
	)

	_, _ = svc.GetDashboardStats(context.Background())
	qr.err = errors.New("repo called on second request - cache should be disabled")
	_, err := svc.GetDashboardStats(context.Background())
	if err == nil {
		t.Error("expected error on second call when cache is disabled")
	}
}
