package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/footballprovider"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubSyncMatchRepo struct {
	candidates    []*domain.Match
	linkCalled    bool
	unlinkCalled  bool
	updateSyncAt  int
	linkErr       error
	candidatesErr error
}

func (r *stubSyncMatchRepo) Create(_ context.Context, _ *domain.Match) error { return nil }
func (r *stubSyncMatchRepo) GetByID(_ context.Context, _ int) (*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) Update(_ context.Context, _ *domain.Match) error { return nil }
func (r *stubSyncMatchRepo) List(_ context.Context) ([]*domain.Match, error) { return nil, nil }
func (r *stubSyncMatchRepo) ListByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) ListByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) LinkExternal(_ context.Context, _ int, _ string, _ int64) error {
	r.linkCalled = true
	return r.linkErr
}
func (r *stubSyncMatchRepo) UnlinkExternal(_ context.Context, _ int) error {
	r.unlinkCalled = true
	return nil
}
func (r *stubSyncMatchRepo) ListSyncCandidates(_ context.Context, _ int) ([]*domain.Match, error) {
	return r.candidates, r.candidatesErr
}
func (r *stubSyncMatchRepo) FindByTeams(_ context.Context, _, _ string) (*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) UpdateKickoff(_ context.Context, _ int, _ time.Time) error {
	return nil
}
func (r *stubSyncMatchRepo) UpdateSyncState(_ context.Context, _ int) error {
	r.updateSyncAt++
	return nil
}

type stubSyncMatchSvc struct {
	started   int
	finished  int
	startErr  error
	finishErr error
}

func (s *stubSyncMatchSvc) CreateMatch(_ context.Context, _ *domain.Match) error { return nil }
func (s *stubSyncMatchSvc) GetMatch(_ context.Context, _ int) (*domain.Match, error) {
	return nil, nil
}
func (s *stubSyncMatchSvc) ListMatches(_ context.Context) ([]*domain.Match, error) { return nil, nil }
func (s *stubSyncMatchSvc) ListMatchesByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	return nil, nil
}
func (s *stubSyncMatchSvc) ListMatchesByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return nil, nil
}
func (s *stubSyncMatchSvc) StartMatch(_ context.Context, _ int) (*domain.Match, error) {
	s.started++
	return &domain.Match{Status: domain.MatchStatusLive}, s.startErr
}
func (s *stubSyncMatchSvc) UpdateResult(_ context.Context, _ int, _, _ int, _ *domain.WinMethod) (*domain.Match, error) {
	s.finished++
	return &domain.Match{Status: domain.MatchStatusFinished}, s.finishErr
}
func (s *stubSyncMatchSvc) CorrectResult(_ context.Context, _ int, _, _ int, _ *domain.WinMethod) (*domain.Match, error) {
	return &domain.Match{Status: domain.MatchStatusFinished}, nil
}
func (s *stubSyncMatchSvc) CancelMatch(_ context.Context, _ int) (*domain.Match, error) {
	return &domain.Match{Status: domain.MatchStatusCancelled}, nil
}

type stubProvider struct {
	fixture    *footballprovider.Fixture
	fetchErr   error
	liveCalled bool
}

func (p *stubProvider) GetFixture(_ context.Context, _ int64) (*footballprovider.Fixture, error) {
	return p.fixture, p.fetchErr
}
func (p *stubProvider) GetLiveFixtures(_ context.Context, _, _ int) ([]*footballprovider.Fixture, error) {
	p.liveCalled = true
	return nil, nil
}
func (p *stubProvider) GetFixturesByDate(_ context.Context, _, _ int, _ string) ([]*footballprovider.Fixture, error) {
	return nil, nil
}

func extID(n int64) *int64 { return &n }

func buildSyncSvc(repo *stubSyncMatchRepo, matchSvc *stubSyncMatchSvc, provider footballprovider.Client) service.MatchSyncer {
	return service.NewMatchSyncService(repo, matchSvc, provider, zap.NewNop())
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestMatchSync_PollAndApply_NoCandidates_ReturnsZeroLive(t *testing.T) {
	repo := &stubSyncMatchRepo{}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, &stubProvider{})

	live, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if live != 0 {
		t.Errorf("live: got %d, want 0", live)
	}
	if repo.updateSyncAt != 0 {
		t.Errorf("UpdateSyncState called unexpectedly")
	}
}

func TestMatchSync_PollAndApply_ScheduledGoesLive_CallsStartMatch(t *testing.T) {
	extMatchID := int64(867234)
	candidate := &domain.Match{
		ID: 1, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extMatchID,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{ExternalID: extMatchID, Status: footballprovider.StatusFirstHalf},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	live, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if live != 1 {
		t.Errorf("live: got %d, want 1", live)
	}
	if matchSvc.started != 1 {
		t.Errorf("StartMatch calls: got %d, want 1", matchSvc.started)
	}
	if repo.updateSyncAt != 1 {
		t.Errorf("UpdateSyncState calls: got %d, want 1", repo.updateSyncAt)
	}
}

func TestMatchSync_PollAndApply_LiveGoesFullTime_CallsUpdateResult(t *testing.T) {
	extMatchID := int64(100)
	candidate := &domain.Match{
		ID: 2, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extMatchID,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{
			ExternalID: extMatchID, Status: footballprovider.StatusFullTime,
			HomeScore: 2, AwayScore: 1,
		},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	live, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if live != 0 {
		t.Errorf("live: got %d, want 0 (match is finished)", live)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: got %d, want 1", matchSvc.finished)
	}
}

func TestMatchSync_PollAndApply_AfterPenalties_CallsUpdateResultWithPenalties(t *testing.T) {
	extMatchID := int64(200)
	candidate := &domain.Match{
		ID: 3, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extMatchID,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{
			ExternalID: extMatchID, Status: footballprovider.StatusAfterPEN,
			HomeScore: 1, AwayScore: 1,
		},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: got %d, want 1", matchSvc.finished)
	}
}

func TestMatchSync_PollAndApply_StartMatchValidationError_IsIdempotent(t *testing.T) {
	extMatchID := int64(300)
	candidate := &domain.Match{
		ID: 4, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extMatchID,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{
		startErr: apperrors.Validation("match can only be started from scheduled status"),
	}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{ExternalID: extMatchID, Status: footballprovider.StatusFirstHalf},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not propagate as an error — idempotent case logged and swallowed.
}

func TestMatchSync_PollAndApply_ProviderError_SkipsMatch_NoOverallError(t *testing.T) {
	extMatchID := int64(400)
	candidate := &domain.Match{
		ID: 5, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extMatchID,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	provider := &stubProvider{fetchErr: errors.New("provider unavailable")}
	svc := buildSyncSvc(repo, matchSvc, provider)

	live, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected overall error: %v", err)
	}
	if live != 0 {
		t.Errorf("live count: got %d, want 0", live)
	}
	if matchSvc.started != 0 {
		t.Errorf("StartMatch should not have been called on provider error")
	}
}

func TestMatchSync_PollAndApply_NoProvider_ReturnsError(t *testing.T) {
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{{ID: 1}}}
	svc := service.NewMatchSyncService(repo, &stubSyncMatchSvc{}, nil, zap.NewNop())

	_, err := svc.PollAndApply(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error when provider is nil, got nil")
	}
}

func TestMatchSync_LinkExternal_InvalidProvider_ReturnsValidationError(t *testing.T) {
	svc := buildSyncSvc(&stubSyncMatchRepo{}, &stubSyncMatchSvc{}, &stubProvider{})

	err := svc.LinkExternal(context.Background(), 1, "unknown-provider", 123)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestMatchSync_LinkExternal_ZeroExternalID_ReturnsValidationError(t *testing.T) {
	svc := buildSyncSvc(&stubSyncMatchRepo{}, &stubSyncMatchSvc{}, &stubProvider{})

	err := svc.LinkExternal(context.Background(), 1, "api-football", 0)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestMatchSync_LinkExternal_ValidInput_DelegatesToRepo(t *testing.T) {
	repo := &stubSyncMatchRepo{}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, &stubProvider{})

	err := svc.LinkExternal(context.Background(), 1, "api-football", 867234)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.linkCalled {
		t.Error("expected repo.LinkExternal to be called")
	}
}

func TestMatchSync_UnlinkExternal_DelegatesToRepo(t *testing.T) {
	repo := &stubSyncMatchRepo{}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, &stubProvider{})

	if err := svc.UnlinkExternal(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.unlinkCalled {
		t.Error("expected repo.UnlinkExternal to be called")
	}
}

func TestMatchSync_ReconcileDate_FinishedMismatch_ReturnsDiff(t *testing.T) {
	extMatchID := int64(500)
	home, away := 2, 1
	candidate := &domain.Match{
		ID: 6, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extMatchID,
		HomeScore:        &home, AwayScore: &away,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{
			ExternalID: extMatchID, Status: footballprovider.StatusFullTime,
			HomeScore: 2, AwayScore: 1,
		},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	diffs, err := svc.ReconcileDate(context.Background(), 1, 2026)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	if diffs[0].MatchID != 6 {
		t.Errorf("diff.MatchID: got %d, want 6", diffs[0].MatchID)
	}
}

func TestMatchSync_PollAndApply_CancelledFixture_DoesNotTransition(t *testing.T) {
	extMatchID := int64(600)
	candidate := &domain.Match{
		ID: 7, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extMatchID,
		KickoffAt:        time.Now().Add(2 * time.Hour),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{ExternalID: extMatchID, Status: footballprovider.StatusCancelled},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.started != 0 || matchSvc.finished != 0 {
		t.Error("no transitions should occur for a cancelled fixture")
	}
}

func strPtr(s string) *string { return &s }

// ── DailyFixtureSync ──────────────────────────────────────────────────────────

func TestMatchSync_DailyFixtureSync_NoProvider_ReturnsError(t *testing.T) {
	svc := service.NewMatchSyncService(&stubSyncMatchRepo{}, &stubSyncMatchSvc{}, nil, zap.NewNop())
	_, err := svc.DailyFixtureSync(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when provider is nil")
	}
}

func TestMatchSync_DailyFixtureSync_NoCandidates_ReturnsEmptyResult(t *testing.T) {
	repo := &stubSyncMatchRepo{candidates: nil}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, &stubProvider{})

	result, err := svc.DailyFixtureSync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total: want 0, got %d", result.Total)
	}
}

func TestMatchSync_DailyFixtureSync_UpdatesKickoffWhenDiffers(t *testing.T) {
	extID := int64(77)
	kickoff := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	candidate := &domain.Match{
		ID: 1, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID,
		KickoffAt:        kickoff,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	newKickoff := kickoff.Add(10 * time.Minute)
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{
			ExternalID: extID,
			KickoffUTC: newKickoff,
			Status:     footballprovider.StatusNotStarted,
		},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	result, err := svc.DailyFixtureSync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total: want 1, got %d", result.Total)
	}
	if result.Updated != 1 {
		t.Errorf("updated: want 1, got %d", result.Updated)
	}
}

func TestMatchSync_DailyFixtureSync_DateRangeFilter_ExcludesOutOfRange(t *testing.T) {
	extID1, extID2 := int64(10), int64(11)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	inRange := &domain.Match{
		ID: 1, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID1,
		KickoffAt:        today.Add(2 * time.Hour),
	}
	outOfRange := &domain.Match{
		ID: 2, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID2,
		KickoffAt:        today.Add(-48 * time.Hour),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{inRange, outOfRange}}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{Status: footballprovider.StatusNotStarted},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	start := today
	result, err := svc.DailyFixtureSync(context.Background(), &start, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total after filter: want 1, got %d", result.Total)
	}
}

func TestMatchSync_DailyFixtureSync_FinishedFixture_TransitionsLiveMatch(t *testing.T) {
	extID := int64(88)
	candidate := &domain.Match{
		ID: 3, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID,
		KickoffAt:        time.Now().Add(-2 * time.Hour),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{
			ExternalID: extID,
			Status:     footballprovider.StatusFullTime,
			HomeScore:  2,
			AwayScore:  1,
		},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: want 1, got %d", matchSvc.finished)
	}
}

func TestMatchSync_DailyFixtureSync_RepoError_ReturnsError(t *testing.T) {
	repo := &stubSyncMatchRepo{candidatesErr: errors.New("db down")}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, &stubProvider{})

	_, err := svc.DailyFixtureSync(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when repo fails")
	}
}

func TestMatchSync_DailyFixtureSync_EndDateFilter_ExcludesFutureMatch(t *testing.T) {
	extID1, extID2 := int64(20), int64(21)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	inRange := &domain.Match{
		ID: 1, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID1,
		KickoffAt:        today.Add(-1 * time.Hour),
	}
	tooFuture := &domain.Match{
		ID: 2, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID2,
		KickoffAt:        today.Add(48 * time.Hour),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{inRange, tooFuture}}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{Status: footballprovider.StatusNotStarted},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	end := today
	result, err := svc.DailyFixtureSync(context.Background(), nil, &end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total after end filter: want 1, got %d", result.Total)
	}
}

func TestMatchSync_DailyFixtureSync_GetFixtureError_ContinuesOtherMatches(t *testing.T) {
	extID1, extID2 := int64(30), int64(31)
	ok := &domain.Match{
		ID: 1, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID1,
		KickoffAt:        time.Now().Add(1 * time.Hour),
	}
	fail := &domain.Match{
		ID: 2, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID2,
		KickoffAt:        time.Now().Add(-1 * time.Hour),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{ok, fail}}
	matchSvc := &stubSyncMatchSvc{}
	callCount := 0
	provider := &stubProviderFn{fn: func(_ context.Context, id int64) (*footballprovider.Fixture, error) {
		callCount++
		if id == extID2 {
			return nil, errors.New("upstream error")
		}
		return &footballprovider.Fixture{ExternalID: id, Status: footballprovider.StatusFullTime, HomeScore: 1}, nil
	}}
	svc := service.NewMatchSyncService(repo, matchSvc, provider, zap.NewNop())

	result, err := svc.DailyFixtureSync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total: want 2, got %d", result.Total)
	}
	if callCount != 2 {
		t.Errorf("GetFixture calls: want 2, got %d", callCount)
	}
}

func TestMatchSync_DailyFixtureSync_LiveFixture_StartsScheduledMatch(t *testing.T) {
	extID := int64(99)
	candidate := &domain.Match{
		ID: 5, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extID,
		KickoffAt:        time.Now().Add(-5 * time.Minute),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{
			ExternalID: extID,
			Status:     footballprovider.StatusFirstHalf,
		},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.started != 1 {
		t.Errorf("StartMatch calls: want 1, got %d", matchSvc.started)
	}
}

// stubProviderFn allows injecting a custom GetFixture implementation.
type stubProviderFn struct {
	fn func(context.Context, int64) (*footballprovider.Fixture, error)
}

func (p *stubProviderFn) GetFixture(ctx context.Context, id int64) (*footballprovider.Fixture, error) {
	return p.fn(ctx, id)
}
func (p *stubProviderFn) GetLiveFixtures(_ context.Context, _, _ int) ([]*footballprovider.Fixture, error) {
	return nil, nil
}
func (p *stubProviderFn) GetFixturesByDate(_ context.Context, _, _ int, _ string) ([]*footballprovider.Fixture, error) {
	return nil, nil
}
