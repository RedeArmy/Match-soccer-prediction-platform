package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// cachedDashboard holds a GetDashboardStats result with its expiry.
type cachedDashboard struct {
	stats     *domain.DashboardStats
	expiresAt time.Time
}

// TiebreakerSubmissionView pairs a tiebreaker prediction with its author's
// display name. Used by the admin tiebreaker submissions endpoint to avoid
// N+1 user lookups in the handler layer.
type TiebreakerSubmissionView struct {
	Submission *domain.Tiebreaker
	UserName   string
}

// AdminReadService handles cross-domain read queries used by admin panel
// endpoints that cannot be satisfied by a single existing service.
type AdminReadService interface {
	// GlobalLeaderboard returns the top limit users ranked by total scored
	// points across all quinielas.
	GlobalLeaderboard(ctx context.Context, limit int) ([]*domain.GlobalLeaderboardEntry, error)
	// ListPredictions returns predictions matching the given admin filters
	// with pagination.
	ListPredictions(ctx context.Context, f repository.PredictionAdminFilters, p repository.Pagination) ([]*domain.Prediction, error)
	// ListTiebreakerSubmissions returns all tiebreaker predictions with user
	// names resolved, paginated.
	ListTiebreakerSubmissions(ctx context.Context, p repository.Pagination) ([]TiebreakerSubmissionView, error)
	// ListSnapshotHistory returns the most recent limit snapshots for a quiniela.
	ListSnapshotHistory(ctx context.Context, quinielaID, limit int) ([]*domain.LeaderboardSnapshot, error)
	// GetDashboardStats returns aggregate counts for groups, users, and payments.
	// Intended for the admin dashboard home screen to populate summary widgets.
	GetDashboardStats(ctx context.Context) (*domain.DashboardStats, error)
}

// adminReadService is the concrete implementation of AdminReadService.
type adminReadService struct {
	predRepo       repository.PredictionRepository
	userRepo       repository.UserRepository
	quinielaRepo   repository.QuinielaRepository
	paymentRepo    repository.PaymentRecordRepository
	tiebreakerRepo repository.TiebreakerRepository
	snapRepo       repository.LeaderboardSnapshotRepository
	params         SystemParamService
	globalCache    cache.Store // optional; nil disables GlobalLeaderboard caching
	log            *zap.Logger
	mu             sync.RWMutex
	dashCache      *cachedDashboard
}

// AdminReadRepos groups the repository and optional cache dependencies for
// NewAdminReadService, keeping the constructor within the parameter-count limit.
type AdminReadRepos struct {
	Pred       repository.PredictionRepository
	User       repository.UserRepository
	Quiniela   repository.QuinielaRepository
	Payment    repository.PaymentRecordRepository
	Tiebreaker repository.TiebreakerRepository
	Snapshot   repository.LeaderboardSnapshotRepository
	// GlobalCache is optional. When non-nil, GlobalLeaderboard results are
	// cached in Redis using ParamKeyCacheLeaderboardTTL as the TTL.
	GlobalCache cache.Store
}

// NewAdminReadService constructs an adminReadService.
func NewAdminReadService(repos AdminReadRepos, params SystemParamService, log *zap.Logger) AdminReadService {
	return &adminReadService{
		predRepo:       repos.Pred,
		userRepo:       repos.User,
		quinielaRepo:   repos.Quiniela,
		paymentRepo:    repos.Payment,
		tiebreakerRepo: repos.Tiebreaker,
		snapRepo:       repos.Snapshot,
		params:         params,
		globalCache:    repos.GlobalCache,
		log:            log,
	}
}

// GlobalLeaderboard returns the top-N users by total scored points. Results are
// cached in Redis (when globalCache is set) using ParamKeyCacheLeaderboardTTL
// as the TTL. The scoring worker flushes the "global_leaderboard:" prefix via
// PostScoringCacheFlush after each MatchFinished event so the cache does not
// serve stale totals for longer than one TTL period after a match is scored.
func (s *adminReadService) GlobalLeaderboard(ctx context.Context, limit int) ([]*domain.GlobalLeaderboardEntry, error) {
	if s.globalCache != nil {
		key := cacheKeyGlobalLeaderboard(limit)
		if cached, ok := cacheGet[[]*domain.GlobalLeaderboardEntry](ctx, s.globalCache, key, s.log); ok {
			return cached, nil
		}
	}
	entries, err := s.predRepo.GlobalLeaderboard(ctx, limit)
	if err != nil {
		return nil, err
	}
	if s.globalCache != nil && len(entries) > 0 {
		ttlSecs := domain.DefaultCacheLeaderboardTTLSeconds
		if s.params != nil {
			ttlSecs = s.params.GetInt(ctx, domain.ParamKeyCacheLeaderboardTTL, domain.DefaultCacheLeaderboardTTLSeconds)
		}
		cacheSet(ctx, s.globalCache, cacheKeyGlobalLeaderboard(limit), entries,
			time.Duration(ttlSecs)*time.Second, s.log)
	}
	return entries, nil
}

func (s *adminReadService) ListPredictions(ctx context.Context, f repository.PredictionAdminFilters, p repository.Pagination) ([]*domain.Prediction, error) {
	return s.predRepo.ListAdmin(ctx, f, p)
}

// ListTiebreakerSubmissions returns all tiebreaker submissions with user names resolved.
// User names are fetched in a single batched query to avoid N+1 round-trips.
func (s *adminReadService) ListTiebreakerSubmissions(ctx context.Context, p repository.Pagination) ([]TiebreakerSubmissionView, error) {
	submissions, err := s.tiebreakerRepo.ListAll(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(submissions) == 0 {
		return []TiebreakerSubmissionView{}, nil
	}

	ids := make([]int, len(submissions))
	for i, tb := range submissions {
		ids[i] = tb.UserID
	}

	users, err := s.userRepo.ListByIDs(ctx, ids)
	if err != nil {
		s.log.Warn("admin_read: failed to resolve user names for tiebreaker submissions", zap.Error(err))
	}

	nameByID := make(map[int]string, len(users))
	for _, u := range users {
		nameByID[u.ID] = u.Name
	}

	views := make([]TiebreakerSubmissionView, len(submissions))
	for i, tb := range submissions {
		views[i] = TiebreakerSubmissionView{
			Submission: tb,
			UserName:   nameByID[tb.UserID],
		}
	}
	return views, nil
}

func (s *adminReadService) ListSnapshotHistory(ctx context.Context, quinielaID, limit int) ([]*domain.LeaderboardSnapshot, error) {
	return s.snapRepo.ListByQuiniela(ctx, quinielaID, limit)
}

// GetDashboardStats aggregates group, user, and payment counts. Results are
// cached for the duration of cache.dashboard_ttl_seconds (default 30 s) so
// repeated dashboard loads do not hammer the database. The three underlying
// aggregate queries run concurrently via errgroup on a cache miss.
func (s *adminReadService) GetDashboardStats(ctx context.Context) (*domain.DashboardStats, error) {
	if cached := s.dashboardFromCache(); cached != nil {
		return cached, nil
	}

	var (
		groupCounts   repository.QuinielaStatusCounts
		userCounts    repository.UserStatusCounts
		paymentCounts repository.PaymentStatusCounts
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		groupCounts, err = s.quinielaRepo.GetStatusCounts(gctx)
		return err
	})
	g.Go(func() error {
		var err error
		userCounts, err = s.userRepo.GetStatusCounts(gctx)
		return err
	})
	g.Go(func() error {
		var err error
		paymentCounts, err = s.paymentRepo.GetStatusCounts(gctx)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("load dashboard stats: %w", err)
	}

	stats := &domain.DashboardStats{
		Groups: domain.GroupDashboardStats{
			Total:    groupCounts.Total,
			Active:   groupCounts.Active,
			Inactive: groupCounts.Inactive,
			Deleted:  groupCounts.Deleted,
		},
		Users: domain.UserDashboardStats{
			Total:  userCounts.Total,
			Active: userCounts.Active,
			Banned: userCounts.Banned,
		},
		Payments: domain.PaymentDashboardStats{
			Pending:        paymentCounts.Pending,
			Confirmed:      paymentCounts.Confirmed,
			Rejected:       paymentCounts.Rejected,
			TotalCollected: paymentCounts.TotalCollected,
		},
	}
	s.setDashboardCache(ctx, stats)
	return stats, nil
}

func (s *adminReadService) dashboardFromCache() *domain.DashboardStats {
	s.mu.RLock()
	c := s.dashCache
	s.mu.RUnlock()
	if c != nil && time.Now().Before(c.expiresAt) {
		return c.stats
	}
	return nil
}

func (s *adminReadService) setDashboardCache(ctx context.Context, stats *domain.DashboardStats) {
	secs := domain.DefaultCacheDashboardTTLSeconds
	if s.params != nil {
		secs = s.params.GetInt(ctx, domain.ParamKeyCacheDashboardTTLSeconds, domain.DefaultCacheDashboardTTLSeconds)
	}
	if secs <= 0 {
		return // TTL of 0 disables caching
	}
	ttl := time.Duration(secs) * time.Second
	s.mu.Lock()
	s.dashCache = &cachedDashboard{stats: stats, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
}

var _ AdminReadService = (*adminReadService)(nil)
