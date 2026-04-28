package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// adminReadService is the concrete implementation of AdminReadService.
type adminReadService struct {
	predRepo       repository.PredictionRepository
	userRepo       repository.UserRepository
	quinielaRepo   repository.QuinielaRepository
	paymentRepo    repository.PaymentRecordRepository
	tiebreakerRepo repository.TiebreakerRepository
	snapRepo       repository.LeaderboardSnapshotRepository
	log            *zap.Logger
}

// NewAdminReadService constructs an adminReadService.
func NewAdminReadService(
	predRepo repository.PredictionRepository,
	userRepo repository.UserRepository,
	quinielaRepo repository.QuinielaRepository,
	paymentRepo repository.PaymentRecordRepository,
	tiebreakerRepo repository.TiebreakerRepository,
	snapRepo repository.LeaderboardSnapshotRepository,
	log *zap.Logger,
) AdminReadService {
	return &adminReadService{
		predRepo:       predRepo,
		userRepo:       userRepo,
		quinielaRepo:   quinielaRepo,
		paymentRepo:    paymentRepo,
		tiebreakerRepo: tiebreakerRepo,
		snapRepo:       snapRepo,
		log:            log,
	}
}

func (s *adminReadService) GlobalLeaderboard(ctx context.Context, limit int) ([]*domain.GlobalLeaderboardEntry, error) {
	return s.predRepo.GlobalLeaderboard(ctx, limit)
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

// GetDashboardStats aggregates group, user, and payment counts in three
// sequential queries and returns them as a single DashboardStats value.
func (s *adminReadService) GetDashboardStats(ctx context.Context) (*domain.DashboardStats, error) {
	groupCounts, err := s.quinielaRepo.GetStatusCounts(ctx)
	if err != nil {
		return nil, err
	}
	userCounts, err := s.userRepo.GetStatusCounts(ctx)
	if err != nil {
		return nil, err
	}
	paymentCounts, err := s.paymentRepo.GetStatusCounts(ctx)
	if err != nil {
		return nil, err
	}
	return &domain.DashboardStats{
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
	}, nil
}

var _ AdminReadService = (*adminReadService)(nil)
