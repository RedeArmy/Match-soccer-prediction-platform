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
	candidates                      []*domain.Match
	linkCalled                      bool
	unlinkCalled                    bool
	updateSyncAt                    int
	linkErr                         error
	candidatesErr                   error
	kickoffErr                      error
	updateErr                       error
	updateCount                     int
	updatedMatch                    *domain.Match
	findByTeamsMatch                *domain.Match
	findByTeamsErr                  error
	linkCalledCount                 int
	updateLiveProgressErr           error
	updateLiveProgressHits          int
	finishedPenaltyMissingWinner    []*domain.Match
	finishedPenaltyMissingWinnerErr error
	// findByTeamsResponses, when non-nil, is consumed sequentially: the first
	// call returns index 0, the second returns index 1, and so on.  When the
	// slice is exhausted the last entry is repeated.  This lets tests exercise
	// the swapped home/away retry in tryAutoLink.
	findByTeamsResponses []struct {
		match *domain.Match
		err   error
	}
	findByTeamsCallCount int
}

func (r *stubSyncMatchRepo) Create(_ context.Context, _ *domain.Match) error { return nil }
func (r *stubSyncMatchRepo) GetByID(_ context.Context, _ int) (*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) Update(_ context.Context, m *domain.Match) error {
	r.updateCount++
	r.updatedMatch = m
	return r.updateErr
}
func (r *stubSyncMatchRepo) List(_ context.Context) ([]*domain.Match, error) { return nil, nil }
func (r *stubSyncMatchRepo) ListByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) ListByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) LinkExternal(_ context.Context, _ int, _ string, _ int64) error {
	r.linkCalled = true
	r.linkCalledCount++
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
	r.findByTeamsCallCount++
	if len(r.findByTeamsResponses) > 0 {
		idx := r.findByTeamsCallCount - 1
		if idx >= len(r.findByTeamsResponses) {
			idx = len(r.findByTeamsResponses) - 1
		}
		resp := r.findByTeamsResponses[idx]
		return resp.match, resp.err
	}
	return r.findByTeamsMatch, r.findByTeamsErr
}
func (r *stubSyncMatchRepo) UpdateKickoff(_ context.Context, _ int, _ time.Time) error {
	return r.kickoffErr
}
func (r *stubSyncMatchRepo) UpdateSyncState(_ context.Context, _ int) error {
	r.updateSyncAt++
	return nil
}
func (r *stubSyncMatchRepo) UpdateSlots(_ context.Context, _ int, _, _ *int) (*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) ListByGroupLabel(_ context.Context, _ string) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubSyncMatchRepo) UpdateLiveProgress(_ context.Context, _ int, _ *string, _, _ *int) error {
	r.updateLiveProgressHits++
	return r.updateLiveProgressErr
}
func (r *stubSyncMatchRepo) ListFinishedPenaltyMatchesMissingWinner(_ context.Context) ([]*domain.Match, error) {
	return r.finishedPenaltyMissingWinner, r.finishedPenaltyMissingWinnerErr
}

type stubSyncMatchSvc struct {
	started            int
	finished           int
	corrected          int
	startErr           error
	finishErr          error
	correctErr         error
	lastPenaltyWinner  *string // last value passed to UpdateResult
	lastCorrectPWinner *string // last value passed to CorrectResult
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
func (s *stubSyncMatchSvc) UpdateResult(_ context.Context, _ int, score service.ScoreUpdate) (*domain.Match, error) {
	s.finished++
	s.lastPenaltyWinner = score.PenaltyWinner
	return &domain.Match{Status: domain.MatchStatusFinished}, s.finishErr
}
func (s *stubSyncMatchSvc) CorrectResult(_ context.Context, _ int, score service.ScoreUpdate) (*domain.Match, error) {
	s.corrected++
	s.lastCorrectPWinner = score.PenaltyWinner
	return &domain.Match{Status: domain.MatchStatusFinished}, s.correctErr
}
func (s *stubSyncMatchSvc) CancelMatch(_ context.Context, _ int) (*domain.Match, error) {
	return &domain.Match{Status: domain.MatchStatusCancelled}, nil
}
func (s *stubSyncMatchSvc) UpdateSlots(_ context.Context, _ int, _, _ *int) (*domain.Match, error) {
	return nil, nil
}

type stubProvider struct {
	fixture          *footballprovider.Fixture
	fetchErr         error
	liveCalled       bool
	byDateFixtures   []*footballprovider.Fixture
	byDateErr        error
	byDateCallCount  int
	byDateDatesAsked []string
}

func (p *stubProvider) GetFixture(_ context.Context, _ int64) (*footballprovider.Fixture, error) {
	return p.fixture, p.fetchErr
}
func (p *stubProvider) GetLiveFixtures(_ context.Context, _, _ int) ([]*footballprovider.Fixture, error) {
	p.liveCalled = true
	return nil, nil
}
func (p *stubProvider) GetFixturesByDate(_ context.Context, _, _ int, date string) ([]*footballprovider.Fixture, error) {
	p.byDateCallCount++
	p.byDateDatesAsked = append(p.byDateDatesAsked, date)
	return p.byDateFixtures, p.byDateErr
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
	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err == nil {
		t.Fatal("expected error when provider is nil")
	}
}

func TestMatchSync_DailyFixtureSync_NoCandidates_ReturnsEmptyResult(t *testing.T) {
	repo := &stubSyncMatchRepo{candidates: nil}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, &stubProvider{})

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
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

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
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
	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, &start, nil)
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

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
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

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
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
	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, &end)
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

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
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

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
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

// ── Additional DailyFixtureSync coverage ─────────────────────────────────────

func TestMatchSync_DailyFixtureSync_FinishedMatch_IsNoOp(t *testing.T) {
	// applyFinishedTransition: match already Finished → UpdateResult must not be called.
	id := int64(201)
	candidate := &domain.Match{
		ID: 10, Status: domain.MatchStatusFinished,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-3 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFullTime,
		HomeScore:  1, AwayScore: 0,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.finished != 0 {
		t.Errorf("UpdateResult should not be called when match is already Finished, got %d calls", matchSvc.finished)
	}
}

func TestMatchSync_DailyFixtureSync_CancelledMatch_IsNoOp(t *testing.T) {
	// applyFinishedTransition: match already Cancelled → UpdateResult must not be called.
	id := int64(202)
	candidate := &domain.Match{
		ID: 11, Status: domain.MatchStatusCancelled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-3 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFullTime,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.finished != 0 {
		t.Errorf("UpdateResult should not be called when match is already Cancelled, got %d calls", matchSvc.finished)
	}
}

func TestMatchSync_DailyFixtureSync_UpdateKickoffFails_ContinuesGracefully(t *testing.T) {
	// maybeCorrectKickoff: UpdateKickoff returns error → log and skip, no panic.
	id := int64(203)
	kickoff := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	candidate := &domain.Match{
		ID: 12, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        kickoff,
	}
	repo := &stubSyncMatchRepo{
		candidates: []*domain.Match{candidate},
		kickoffErr: errors.New("db write error"),
	}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		KickoffUTC: kickoff.Add(10 * time.Minute), // triggers UpdateKickoff
		Status:     footballprovider.StatusNotStarted,
	}}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Updated must remain 0 because UpdateKickoff failed.
	if result.Updated != 0 {
		t.Errorf("Updated: want 0 on UpdateKickoff failure, got %d", result.Updated)
	}
}

func TestMatchSync_DailyFixtureSync_KickoffDiffUnder60s_SkipsUpdate(t *testing.T) {
	// maybeCorrectKickoff: diff ≤ 60 s → no UpdateKickoff call.
	id := int64(204)
	kickoff := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	candidate := &domain.Match{
		ID: 13, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        kickoff,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		KickoffUTC: kickoff.Add(30 * time.Second), // diff = 30 s ≤ 60 s
		Status:     footballprovider.StatusNotStarted,
	}}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Updated != 0 {
		t.Errorf("Updated: want 0 when diff ≤ 60 s, got %d", result.Updated)
	}
}

func TestMatchSync_DailyFixtureSync_LiveMatch_KickoffNotUpdated(t *testing.T) {
	// maybeCorrectKickoff: only runs for Scheduled matches; Live match is skipped.
	id := int64(205)
	kickoff := time.Now().Add(-30 * time.Minute).Truncate(time.Second)
	candidate := &domain.Match{
		ID: 14, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        kickoff,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		KickoffUTC: kickoff.Add(30 * time.Minute), // large diff but match is Live
		Status:     footballprovider.StatusFirstHalf,
	}}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Updated != 0 {
		t.Errorf("Updated: want 0 for Live match, got %d", result.Updated)
	}
}

func TestMatchSync_DailyFixtureSync_BothDates_FiltersCorrectly(t *testing.T) {
	// filterByDateRange with both start and end set.
	eid1, eid2, eid3 := int64(301), int64(302), int64(303)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	before := &domain.Match{
		ID: 20, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &eid1,
		KickoffAt:        today.Add(-48 * time.Hour),
	}
	within := &domain.Match{
		ID: 21, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &eid2,
		KickoffAt:        today.Add(2 * time.Hour),
	}
	after := &domain.Match{
		ID: 22, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &eid3,
		KickoffAt:        today.Add(72 * time.Hour),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{before, within, after}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{Status: footballprovider.StatusNotStarted}}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	start := today
	end := today.Add(24 * time.Hour)
	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, &start, &end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total: want 1 (only 'within' match), got %d", result.Total)
	}
}

func TestMatchSync_DailyFixtureSync_UpdateResultNonValidationError_LogsAndContinues(t *testing.T) {
	id := int64(401)
	candidate := &domain.Match{
		ID: 40, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-2 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{finishErr: errors.New("database unavailable")}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFullTime,
		HomeScore:  1, AwayScore: 0,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: want 1, got %d", matchSvc.finished)
	}
}

func TestMatchSync_DailyFixtureSync_StartMatchNonValidationError_LogsAndContinues(t *testing.T) {
	id := int64(402)
	candidate := &domain.Match{
		ID: 41, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-5 * time.Minute),
	}
	matchSvc := &stubSyncMatchSvc{startErr: errors.New("database unavailable")}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFirstHalf,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.started != 1 {
		t.Errorf("StartMatch calls: want 1, got %d", matchSvc.started)
	}
}

func TestMatchSync_DailyFixtureSync_AfterExtraTime_UsesAETWinMethod(t *testing.T) {
	id := int64(403)
	candidate := &domain.Match{
		ID: 42, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-2 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusAfterET,
		HomeScore:  2, AwayScore: 1,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: want 1, got %d", matchSvc.finished)
	}
}

func TestMatchSync_DailyFixtureSync_StartMatchValidationError_IsIdempotentViaDaily(t *testing.T) {
	id := int64(404)
	candidate := &domain.Match{
		ID: 43, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-5 * time.Minute),
	}
	matchSvc := &stubSyncMatchSvc{startErr: apperrors.Validation("already live")}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFirstHalf,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.started != 1 {
		t.Errorf("StartMatch calls: want 1, got %d", matchSvc.started)
	}
}

// ── Missed live-phase (Scheduled → Finished) ──────────────────────────────────

func TestMatchSync_PollAndApply_ScheduledGoesDirectlyFinished_StartsAndRecordsResult(t *testing.T) {
	// processOne: fix.IsFinished() && m.Status == Scheduled
	// Must call StartMatch first (to lock predictions), then UpdateResult.
	candidate := &domain.Match{
		ID: 99, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  extID(500),
		KickoffAt:        time.Now().Add(-2 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: 500,
		Status:     footballprovider.StatusFullTime,
		HomeScore:  2, AwayScore: 1,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	liveCount, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if liveCount != 0 {
		t.Errorf("liveCount: want 0, got %d", liveCount)
	}
	if matchSvc.started != 1 {
		t.Errorf("StartMatch calls: want 1, got %d", matchSvc.started)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: want 1, got %d", matchSvc.finished)
	}
}

func TestMatchSync_PollAndApply_ScheduledDirectlyFinished_StartMatchFails_ReturnsError(t *testing.T) {
	// processOne: StartMatch returns non-validation error → processOne errors;
	// PollAndApply absorbs it, UpdateResult must not be called.
	candidate := &domain.Match{
		ID: 100, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  extID(501),
		KickoffAt:        time.Now().Add(-2 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{startErr: errors.New("db unavailable")}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: 501,
		Status:     footballprovider.StatusFullTime,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	liveCount, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollAndApply should absorb processOne errors: %v", err)
	}
	if liveCount != 0 {
		t.Errorf("liveCount: want 0, got %d", liveCount)
	}
	if matchSvc.started != 1 {
		t.Errorf("StartMatch calls: want 1, got %d", matchSvc.started)
	}
	if matchSvc.finished != 0 {
		t.Errorf("UpdateResult must not be called when StartMatch fails, got %d", matchSvc.finished)
	}
}

func TestMatchSync_DailyFixtureSync_ScheduledGoesDirectlyFinished_StartsAndRecordsResult(t *testing.T) {
	// applyFinishedTransition: m.Status == Scheduled, fix.IsFinished()
	// Must call StartMatch to lock predictions, then UpdateResult.
	id := int64(600)
	candidate := &domain.Match{
		ID: 60, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-3 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFullTime,
		HomeScore:  1, AwayScore: 0,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.started != 1 {
		t.Errorf("StartMatch calls: want 1, got %d", matchSvc.started)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: want 1, got %d", matchSvc.finished)
	}
}

func TestMatchSync_DailyFixtureSync_ScheduledGoesFinished_StartMatchFails_SkipsResult(t *testing.T) {
	// applyFinishedTransition: StartMatch returns non-validation error →
	// Warn is logged, UpdateResult must not be called.
	id := int64(601)
	candidate := &domain.Match{
		ID: 61, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-3 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{startErr: errors.New("db unavailable")}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFullTime,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.started != 1 {
		t.Errorf("StartMatch calls: want 1, got %d", matchSvc.started)
	}
	if matchSvc.finished != 0 {
		t.Errorf("UpdateResult must not be called when StartMatch fails, got %d", matchSvc.finished)
	}
}

func TestMatchSync_PollAndApply_LiveMatchWritesScoreToDB(t *testing.T) {
	// When the provider returns a live fixture and the local match is already
	// live, processOne must persist the current score to the repository so
	// the API can surface the live marcador to the frontend.
	id := int64(700)
	candidate := &domain.Match{
		ID: 70, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-1 * time.Hour),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFirstHalf,
		HomeScore:  2,
		AwayScore:  1,
	}}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	liveCount, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if liveCount != 1 {
		t.Errorf("liveCount: want 1, got %d", liveCount)
	}
	if repo.updateCount != 1 {
		t.Errorf("repo.Update calls: want 1, got %d", repo.updateCount)
	}
	if repo.updatedMatch == nil {
		t.Fatal("updatedMatch is nil")
	}
	if repo.updatedMatch.HomeScore == nil || *repo.updatedMatch.HomeScore != 2 {
		t.Errorf("HomeScore: want 2, got %v", repo.updatedMatch.HomeScore)
	}
	if repo.updatedMatch.AwayScore == nil || *repo.updatedMatch.AwayScore != 1 {
		t.Errorf("AwayScore: want 1, got %v", repo.updatedMatch.AwayScore)
	}
}

func TestMatchSync_PollAndApply_LiveMatch_UpdateScoreFails_StillReturnsLive(t *testing.T) {
	// A transient repository error when persisting the live score must not
	// affect the liveCount return — the match is still live and the poll
	// interval must remain in fast mode.
	id := int64(701)
	candidate := &domain.Match{
		ID: 71, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-1 * time.Hour),
	}
	repo := &stubSyncMatchRepo{
		candidates: []*domain.Match{candidate},
		updateErr:  errors.New("db timeout"),
	}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFirstHalf,
		HomeScore:  1,
		AwayScore:  0,
	}}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	liveCount, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if liveCount != 1 {
		t.Errorf("liveCount: want 1, got %d", liveCount)
	}
}

// ── autoLinkByDateRange ───────────────────────────────────────────────────────

func TestMatchSync_DailyFixtureSync_AutoLinks_UnlinkedMatch(t *testing.T) {
	// The provider returns a fixture for today. An internal match exists with
	// the same team names but no external link. DailyFixtureSync must call
	// LinkExternal and increment result.Linked.
	internalMatch := &domain.Match{ID: 99, Status: domain.MatchStatusScheduled}
	repo := &stubSyncMatchRepo{
		candidates:       []*domain.Match{}, // no linked candidates yet
		findByTeamsMatch: internalMatch,
	}
	provider := &stubProvider{
		byDateFixtures: []*footballprovider.Fixture{{
			ExternalID: 42, HomeTeam: "Germany", AwayTeam: "Curacao",
			Status: footballprovider.StatusNotStarted,
		}},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Linked != 1 {
		t.Errorf("Linked: want 1, got %d", result.Linked)
	}
	if !repo.linkCalled {
		t.Error("expected LinkExternal to be called")
	}
}

func TestMatchSync_DailyFixtureSync_AutoLink_AlreadyLinked_Skips(t *testing.T) {
	// A match that already has an external ID must not be re-linked.
	id := int64(42)
	internalMatch := &domain.Match{
		ID: 99, Status: domain.MatchStatusScheduled,
		ExternalMatchID: &id, // already linked
	}
	repo := &stubSyncMatchRepo{
		candidates:       []*domain.Match{internalMatch},
		findByTeamsMatch: internalMatch,
	}
	provider := &stubProvider{
		fixture:        &footballprovider.Fixture{ExternalID: id, Status: footballprovider.StatusNotStarted},
		byDateFixtures: []*footballprovider.Fixture{{ExternalID: id, HomeTeam: "Germany", AwayTeam: "Curacao"}},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Linked != 0 {
		t.Errorf("Linked: want 0 (already linked), got %d", result.Linked)
	}
	// linkCalledCount must be 0 from the auto-link phase (the initial candidates
	// stub returns the match already linked, so FindByTeams returns it with
	// ExternalMatchID set).
	if repo.linkCalledCount != 0 {
		t.Errorf("LinkExternal called unexpectedly: %d times", repo.linkCalledCount)
	}
}

func TestMatchSync_DailyFixtureSync_AutoLink_FindByTeamsReturnsNil_Skips(t *testing.T) {
	// Provider returns a fixture but no internal match has those team names —
	// FindByTeams returns nil. Must not call LinkExternal.
	repo := &stubSyncMatchRepo{
		findByTeamsMatch: nil, // no match
	}
	provider := &stubProvider{
		byDateFixtures: []*footballprovider.Fixture{{ExternalID: 99, HomeTeam: "Unknown", AwayTeam: "Also Unknown"}},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Linked != 0 {
		t.Errorf("Linked: want 0, got %d", result.Linked)
	}
	if repo.linkCalled {
		t.Error("LinkExternal must not be called when FindByTeams returns nil")
	}
}

func TestMatchSync_DailyFixtureSync_AutoLink_FindByTeamsReturnsError_Skips(t *testing.T) {
	// FindByTeams returns a hard error on the first call. tryAutoLink must
	// return early without calling LinkExternal.
	repo := &stubSyncMatchRepo{
		findByTeamsErr: errors.New("connection refused"),
	}
	provider := &stubProvider{
		byDateFixtures: []*footballprovider.Fixture{{ExternalID: 88, HomeTeam: "Spain", AwayTeam: "Cape Verde"}},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Linked != 0 {
		t.Errorf("Linked: want 0, got %d", result.Linked)
	}
	if repo.linkCalled {
		t.Error("LinkExternal must not be called when FindByTeams returns an error")
	}
}

func TestMatchSync_DailyFixtureSync_AutoLink_SwappedOrder_LinksMatch(t *testing.T) {
	// The provider designates home/away in the opposite order to the local DB
	// (common for neutral-venue World Cup fixtures). The first FindByTeams call
	// returns nil; the retry with home and away swapped finds the match and
	// LinkExternal must be called exactly once.
	internalMatch := &domain.Match{ID: 77, Status: domain.MatchStatusScheduled}
	type resp = struct {
		match *domain.Match
		err   error
	}
	repo := &stubSyncMatchRepo{
		findByTeamsResponses: []resp{
			{nil, nil},           // first call (original order) → not found
			{internalMatch, nil}, // second call (swapped order) → found
		},
	}
	provider := &stubProvider{
		byDateFixtures: []*footballprovider.Fixture{{
			ExternalID: 55, HomeTeam: "Cape Verde", AwayTeam: "Spain",
			Status: footballprovider.StatusNotStarted,
		}},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Linked != 1 {
		t.Errorf("Linked: want 1 (via swapped order), got %d", result.Linked)
	}
	// autoLinkByDateRange scans 32 days (30-day look-back + today + tomorrow),
	// so FindByTeams is called at least twice (original + swap on the first date
	// that has a fixture); the alreadyLinked guard prevents double-linking.
	if repo.findByTeamsCallCount < 2 {
		t.Errorf("FindByTeams call count: want >= 2 (original + swap), got %d", repo.findByTeamsCallCount)
	}
	if !repo.linkCalled {
		t.Error("expected LinkExternal to be called after swapped-order match")
	}
}

func TestMatchSync_DailyFixtureSync_AutoLink_SwappedOrder_BothNil_Skips(t *testing.T) {
	// Both the original and the swapped FindByTeams lookups return nil.
	// LinkExternal must not be called.
	type resp = struct {
		match *domain.Match
		err   error
	}
	repo := &stubSyncMatchRepo{
		findByTeamsResponses: []resp{
			{nil, nil}, // original order → not found
			{nil, nil}, // swapped order → still not found
		},
	}
	provider := &stubProvider{
		byDateFixtures: []*footballprovider.Fixture{{
			ExternalID: 56, HomeTeam: "Unknown A", AwayTeam: "Unknown B",
			Status: footballprovider.StatusNotStarted,
		}},
	}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Linked != 0 {
		t.Errorf("Linked: want 0, got %d", result.Linked)
	}
	if repo.linkCalled {
		t.Error("LinkExternal must not be called when both lookups return nil")
	}
}

func TestMatchSync_DailyFixtureSync_AutoLink_GetFixturesByDateError_ContinuesToPhase2(t *testing.T) {
	// GetFixturesByDate fails but phase 2 (linked candidates) must still run.
	id := int64(10)
	candidate := &domain.Match{
		ID: 10, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-time.Hour),
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{
		fixture:   &footballprovider.Fixture{ExternalID: id, Status: footballprovider.StatusFullTime},
		byDateErr: errors.New("provider down"),
	}
	matchSvc := &stubSyncMatchSvc{}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Phase 2 must still finish the live match.
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: want 1, got %d", matchSvc.finished)
	}
}

// ── Two-day UTC auto-link scan ────────────────────────────────────────────────

func TestMatchSync_DailyFixtureSync_NilDateRange_ScansThirtyDaysBackThroughTomorrowUTC(t *testing.T) {
	// When DailyFixtureSync is called with nil dates, autoLinkByDateRange must
	// query the provider for every day in [today-30d, tomorrow] UTC (32 calls).
	// The 30-day look-back ensures any matches missed during deployment gaps
	// (e.g. when team_name_aliases migration was not yet applied) are retroactively
	// linked without the operator supplying explicit dates. Tomorrow is included
	// so that evening fixtures for UTC-6 users (listed under the next UTC calendar
	// day in api-football) are captured in the same daily run.
	provider := &stubProvider{byDateFixtures: nil}
	svc := buildSyncSvc(&stubSyncMatchRepo{}, &stubSyncMatchSvc{}, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 30 days ago + 30 in-between days + today + tomorrow = 32
	const wantCalls = 32
	if provider.byDateCallCount != wantCalls {
		t.Errorf("GetFixturesByDate calls: want %d (30-day look-back + today + tomorrow), got %d",
			wantCalls, provider.byDateCallCount)
	}
	now := time.Now().UTC().Truncate(24 * time.Hour)
	wantFirst := now.AddDate(0, 0, -30).Format("2006-01-02")
	wantLast := now.AddDate(0, 0, 1).Format("2006-01-02")
	if len(provider.byDateDatesAsked) < 2 ||
		provider.byDateDatesAsked[0] != wantFirst ||
		provider.byDateDatesAsked[wantCalls-1] != wantLast {
		first, last := "", ""
		if len(provider.byDateDatesAsked) > 0 {
			first = provider.byDateDatesAsked[0]
			last = provider.byDateDatesAsked[len(provider.byDateDatesAsked)-1]
		}
		t.Errorf("date range: want [%s … %s], got [%s … %s]", wantFirst, wantLast, first, last)
	}
}

func TestMatchSync_DailyFixtureSync_NilDateRange_DeduplicatesAutoLink(t *testing.T) {
	// The same fixture appearing on both the today and tomorrow scans must
	// only be linked once (in-run alreadyLinked dedup set).
	teamMatch := &domain.Match{ID: 99, Status: domain.MatchStatusScheduled}
	repo := &stubSyncMatchRepo{findByTeamsMatch: teamMatch}
	fixture := &footballprovider.Fixture{
		ExternalID: 9999,
		HomeTeam:   "Sweden",
		AwayTeam:   "Tunisia",
	}
	provider := &stubProvider{byDateFixtures: []*footballprovider.Fixture{fixture}}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.linkCalledCount != 1 {
		t.Errorf("LinkExternal calls: want 1 (dedup), got %d", repo.linkCalledCount)
	}
}

// ── UpdateLiveProgress error paths ────────────────────────────────────────────

func TestMatchSync_PollAndApply_LiveMatchWritesPeriodAndPenaltyScore(t *testing.T) {
	// applyLiveScore stores the raw period code and penalty shootout tally in
	// the repository so the API can surface them to the frontend.
	id := int64(800)
	candidate := &domain.Match{
		ID: 80, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-1 * time.Hour),
	}
	penHome, penAway := 3, 2
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID:       id,
		Status:           footballprovider.StatusPenLive,
		HomeScore:        1,
		AwayScore:        1,
		PenaltyHomeScore: &penHome,
		PenaltyAwayScore: &penAway,
	}}
	svc := buildSyncSvc(repo, &stubSyncMatchSvc{}, provider)

	_, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedMatch == nil {
		t.Fatal("updatedMatch is nil")
	}
	if repo.updatedMatch.Period == nil || *repo.updatedMatch.Period != string(footballprovider.StatusPenLive) {
		t.Errorf("Period: want %q, got %v", string(footballprovider.StatusPenLive), repo.updatedMatch.Period)
	}
	if repo.updatedMatch.PenaltyHomeScore == nil || *repo.updatedMatch.PenaltyHomeScore != penHome {
		t.Errorf("PenaltyHomeScore: want %d, got %v", penHome, repo.updatedMatch.PenaltyHomeScore)
	}
	if repo.updatedMatch.PenaltyAwayScore == nil || *repo.updatedMatch.PenaltyAwayScore != penAway {
		t.Errorf("PenaltyAwayScore: want %d, got %v", penAway, repo.updatedMatch.PenaltyAwayScore)
	}
}

func TestMatchSync_PollAndApply_FinishedMatch_RecordsResult(t *testing.T) {
	// applyResult calls UpdateLiveProgress to clear the period after a result is
	// recorded. An error from that call must be logged and swallowed — the
	// overall PollAndApply must still succeed.
	id := int64(810)
	candidate := &domain.Match{
		ID: 81, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-2 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{
		candidates: []*domain.Match{candidate},
	}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFullTime,
		HomeScore:  2, AwayScore: 1,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.PollAndApply(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollAndApply: %v", err)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: want 1, got %d", matchSvc.finished)
	}
}

func TestMatchSync_DailyFixtureSync_FinishedMatch_RecordsResult(t *testing.T) {
	id := int64(820)
	candidate := &domain.Match{
		ID: 82, Status: domain.MatchStatusScheduled,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-3 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{
		candidates: []*domain.Match{candidate},
	}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusFullTime,
		HomeScore:  1, AwayScore: 0,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("DailyFixtureSync: %v", err)
	}
	if matchSvc.finished != 1 {
		t.Errorf("UpdateResult calls: want 1, got %d", matchSvc.finished)
	}
}

// ── penalty repair tests ──────────────────────────────────────────────────────

func TestMatchSync_DailyFixtureSync_FinishedPenaltyMatch_MissingPenaltyWinner_RepairsViaCorrectResult(t *testing.T) {
	// A match that was finalised manually without penalty_winner must be repaired
	// automatically on the next DailyFixtureSync when the provider reports
	// StatusAfterPEN with penalty scores.
	id := int64(830)
	phome, paway := 4, 5
	candidate := &domain.Match{
		ID: 83, Status: domain.MatchStatusFinished,
		HomeTeam: "Germany", AwayTeam: "Paraguay",
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-3 * time.Hour),
		// PenaltyWinner intentionally nil — simulates the broken state
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusAfterPEN,
		HomeScore:  1, AwayScore: 1,
		PenaltyHomeScore: &phome,
		PenaltyAwayScore: &paway,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("DailyFixtureSync: %v", err)
	}
	if matchSvc.corrected != 1 {
		t.Errorf("CorrectResult calls: want 1 (repair), got %d", matchSvc.corrected)
	}
	if matchSvc.lastCorrectPWinner == nil || *matchSvc.lastCorrectPWinner != "away" {
		t.Errorf("penalty_winner: want %q, got %v", "away", matchSvc.lastCorrectPWinner)
	}
}

func TestMatchSync_DailyFixtureSync_FinishedPenaltyMatch_AlreadyHasPenaltyWinner_Skips(t *testing.T) {
	// A match that already has penalty_winner set must not be touched.
	id := int64(831)
	pw := "home"
	phome, paway := 5, 3
	candidate := &domain.Match{
		ID: 84, Status: domain.MatchStatusFinished,
		HomeTeam: "Morocco", AwayTeam: "Spain",
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-3 * time.Hour),
		PenaltyWinner:    &pw,
		PenaltyHomeScore: &phome,
		PenaltyAwayScore: &paway,
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusAfterPEN,
		HomeScore:  1, AwayScore: 1,
		PenaltyHomeScore: &phome,
		PenaltyAwayScore: &paway,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("DailyFixtureSync: %v", err)
	}
	if matchSvc.corrected != 0 {
		t.Errorf("CorrectResult calls: want 0 (already repaired), got %d", matchSvc.corrected)
	}
}

// ── Phase-3 penalty repair (ListFinishedPenaltyMatchesMissingWinner) ─────────
//
// These tests exercise the dedicated sweep that runs AFTER ListSyncCandidates
// so that already-finished matches (excluded from the normal poll path) are
// still corrected on the next daily run.

func TestMatchSync_DailyFixtureSync_Phase3_RepairsFinishedPenaltyMatch(t *testing.T) {
	// Arrange: a finished match with penalty_winner NULL is returned by
	// ListFinishedPenaltyMatchesMissingWinner (not by ListSyncCandidates).
	id := int64(900)
	phome, paway := 3, 5
	broken := &domain.Match{
		ID: 90, Status: domain.MatchStatusFinished,
		HomeTeam: "Netherlands", AwayTeam: "Morocco",
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-24 * time.Hour),
		// PenaltyWinner intentionally nil — the broken state this sweep fixes
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{
		finishedPenaltyMissingWinner: []*domain.Match{broken},
	}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID:       id,
		Status:           footballprovider.StatusAfterPEN,
		HomeScore:        1,
		AwayScore:        1,
		PenaltyHomeScore: &phome,
		PenaltyAwayScore: &paway,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("DailyFixtureSync: %v", err)
	}
	if matchSvc.corrected != 1 {
		t.Errorf("CorrectResult calls: want 1 (phase-3 repair), got %d", matchSvc.corrected)
	}
	if matchSvc.lastCorrectPWinner == nil || *matchSvc.lastCorrectPWinner != "away" {
		t.Errorf("penalty_winner: want %q, got %v", "away", matchSvc.lastCorrectPWinner)
	}
	if result.Corrected != 1 {
		t.Errorf("result.Corrected: want 1, got %d", result.Corrected)
	}
}

func TestMatchSync_DailyFixtureSync_Phase3_ProviderFetchError_Skips(t *testing.T) {
	// If the provider returns an error for a match in the repair list the
	// match is skipped without propagating the error to the caller.
	id := int64(901)
	broken := &domain.Match{
		ID: 91, Status: domain.MatchStatusFinished,
		HomeTeam: "Brazil", AwayTeam: "Argentina",
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-48 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{
		finishedPenaltyMissingWinner: []*domain.Match{broken},
	}
	provider := &stubProvider{fetchErr: errors.New("provider down")}
	svc := buildSyncSvc(repo, matchSvc, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("DailyFixtureSync must not fail on provider error: %v", err)
	}
	if matchSvc.corrected != 0 {
		t.Errorf("CorrectResult calls: want 0 (skipped), got %d", matchSvc.corrected)
	}
	if result.Corrected != 0 {
		t.Errorf("result.Corrected: want 0, got %d", result.Corrected)
	}
}

func TestMatchSync_DailyFixtureSync_Phase3_ListError_DoesNotFail(t *testing.T) {
	// A DB error from ListFinishedPenaltyMatchesMissingWinner must not abort
	// DailyFixtureSync — the error is logged and the phase is skipped.
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{
		finishedPenaltyMissingWinnerErr: errors.New("db timeout"),
	}
	svc := buildSyncSvc(repo, matchSvc, &stubProvider{})

	_, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("DailyFixtureSync must not propagate list error: %v", err)
	}
	if matchSvc.corrected != 0 {
		t.Errorf("CorrectResult calls: want 0, got %d", matchSvc.corrected)
	}
}

// ── derivePenaltyWinner unit tests ────────────────────────────────────────────

func ptrInt(v int) *int { return &v }

func TestDerivePenaltyWinner_HomeWins_ReturnsHome(t *testing.T) {
	fix := &footballprovider.Fixture{
		Status:           footballprovider.StatusAfterPEN,
		PenaltyHomeScore: ptrInt(4),
		PenaltyAwayScore: ptrInt(2),
	}
	got := service.DerivePenaltyWinner(fix)
	if got == nil || *got != "home" {
		t.Errorf("got %v; want \"home\"", got)
	}
}

func TestDerivePenaltyWinner_AwayWins_ReturnsAway(t *testing.T) {
	fix := &footballprovider.Fixture{
		Status:           footballprovider.StatusAfterPEN,
		PenaltyHomeScore: ptrInt(2),
		PenaltyAwayScore: ptrInt(3),
	}
	got := service.DerivePenaltyWinner(fix)
	if got == nil || *got != "away" {
		t.Errorf("got %v; want \"away\"", got)
	}
}

func TestDerivePenaltyWinner_NonPENStatus_ReturnsNil(t *testing.T) {
	fix := &footballprovider.Fixture{
		Status:           footballprovider.StatusFullTime,
		PenaltyHomeScore: ptrInt(3),
		PenaltyAwayScore: ptrInt(1),
	}
	got := service.DerivePenaltyWinner(fix)
	if got != nil {
		t.Errorf("got %v; want nil for non-PEN status", got)
	}
}

func TestDerivePenaltyWinner_NilScores_ReturnsNil(t *testing.T) {
	fix := &footballprovider.Fixture{Status: footballprovider.StatusAfterPEN}
	got := service.DerivePenaltyWinner(fix)
	if got != nil {
		t.Errorf("got %v; want nil when penalty scores are absent", got)
	}
}

// TestMatchSync_PollAndApply_AfterPenalties_SetsPenaltyWinner verifies that
// when the provider reports PEN with a clear shootout tally, UpdateResult
// receives the derived penalty winner instead of nil.
func TestMatchSync_PollAndApply_AfterPenalties_SetsPenaltyWinner(t *testing.T) {
	extMatchID := int64(991)
	candidate := &domain.Match{
		ID: 91, Status: domain.MatchStatusLive,
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &extMatchID,
	}
	repo := &stubSyncMatchRepo{candidates: []*domain.Match{candidate}}
	matchSvc := &stubSyncMatchSvc{}
	penHome, penAway := 4, 2
	provider := &stubProvider{
		fixture: &footballprovider.Fixture{
			ExternalID: extMatchID,
			Status:     footballprovider.StatusAfterPEN,
			HomeScore:  1, AwayScore: 1,
			PenaltyHomeScore: &penHome,
			PenaltyAwayScore: &penAway,
		},
	}
	svc := buildSyncSvc(repo, matchSvc, provider)

	if _, err := svc.PollAndApply(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchSvc.lastPenaltyWinner == nil || *matchSvc.lastPenaltyWinner != "home" {
		t.Errorf("penalty_winner: got %v; want \"home\" (home won 4-2 on pens)", matchSvc.lastPenaltyWinner)
	}
}

func TestMatchSync_DailyFixtureSync_Phase3_MissingPenaltyScores_Skips(t *testing.T) {
	// Provider reports StatusAfterPEN but penalty scores are nil — derivePenaltyWinner
	// returns nil and the match must be skipped without calling CorrectResult.
	id := int64(902)
	broken := &domain.Match{
		ID: 92, Status: domain.MatchStatusFinished,
		HomeTeam: "England", AwayTeam: "France",
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-72 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{}
	repo := &stubSyncMatchRepo{
		finishedPenaltyMissingWinner: []*domain.Match{broken},
	}
	// Fixture has StatusAfterPEN but no penalty score data.
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID: id,
		Status:     footballprovider.StatusAfterPEN,
		HomeScore:  1,
		AwayScore:  1,
		// PenaltyHomeScore and PenaltyAwayScore intentionally nil
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("DailyFixtureSync must not fail: %v", err)
	}
	if matchSvc.corrected != 0 {
		t.Errorf("CorrectResult calls: want 0 (skipped — no penalty scores), got %d", matchSvc.corrected)
	}
	if result.Corrected != 0 {
		t.Errorf("result.Corrected: want 0, got %d", result.Corrected)
	}
}

func TestMatchSync_DailyFixtureSync_Phase3_CorrectResultError_Skips(t *testing.T) {
	// Provider returns valid penalty data but CorrectResult fails — the match
	// must be skipped and result.Corrected must not increment.
	id := int64(903)
	phome, paway := 4, 2
	broken := &domain.Match{
		ID: 93, Status: domain.MatchStatusFinished,
		HomeTeam: "Spain", AwayTeam: "Portugal",
		ExternalProvider: strPtr("api-football"),
		ExternalMatchID:  &id,
		KickoffAt:        time.Now().Add(-96 * time.Hour),
	}
	matchSvc := &stubSyncMatchSvc{correctErr: errors.New("db error")}
	repo := &stubSyncMatchRepo{
		finishedPenaltyMissingWinner: []*domain.Match{broken},
	}
	provider := &stubProvider{fixture: &footballprovider.Fixture{
		ExternalID:       id,
		Status:           footballprovider.StatusAfterPEN,
		HomeScore:        1,
		AwayScore:        1,
		PenaltyHomeScore: &phome,
		PenaltyAwayScore: &paway,
	}}
	svc := buildSyncSvc(repo, matchSvc, provider)

	result, err := svc.DailyFixtureSync(context.Background(), 1, 2026, nil, nil)
	if err != nil {
		t.Fatalf("DailyFixtureSync must not propagate CorrectResult error: %v", err)
	}
	if matchSvc.corrected != 1 {
		t.Errorf("CorrectResult calls: want 1 (attempted), got %d", matchSvc.corrected)
	}
	if result.Corrected != 0 {
		t.Errorf("result.Corrected: want 0 (failed repair not counted), got %d", result.Corrected)
	}
}
