package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	fmtExpectNil    = "expected nil, got %v"
	fmtExpectNilErr = "expected nil error, got %v"
	fmtExpect1Match = "expected 1 match, got %d"
	fmtNotFoundErr  = "expected not-found error, got %v"
	matchBrazil     = "Brazil"
	matchArgentina  = "Argentina"
	matchGermany    = "Germany"
	matchFrance     = "France"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

// stubMatchRepo implements repository.MatchRepository with configurable returns.
type stubMatchRepo struct {
	match   *domain.Match
	matches []*domain.Match
	err     error
}

func (r *stubMatchRepo) Create(_ context.Context, _ *domain.Match) error { return r.err }
func (r *stubMatchRepo) GetByID(_ context.Context, _ int) (*domain.Match, error) {
	return r.match, r.err
}
func (r *stubMatchRepo) Update(_ context.Context, m *domain.Match) error {
	if r.err != nil {
		return r.err
	}
	r.match = m
	return nil
}
func (r *stubMatchRepo) List(_ context.Context) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepo) ListByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepo) ListByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepo) LinkExternal(_ context.Context, _ int, _ string, _ int64) error {
	return r.err
}
func (r *stubMatchRepo) UnlinkExternal(_ context.Context, _ int) error { return r.err }
func (r *stubMatchRepo) ListSyncCandidates(_ context.Context, _ int) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepo) UpdateSyncState(_ context.Context, _ int) error { return r.err }
func (r *stubMatchRepo) FindByTeams(_ context.Context, _, _ string) (*domain.Match, error) {
	return nil, nil
}
func (r *stubMatchRepo) UpdateKickoff(_ context.Context, _ int, _ time.Time) error { return nil }
func (r *stubMatchRepo) ListByGroupLabel(_ context.Context, _ string) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubMatchRepo) UpdateSlots(_ context.Context, _ int, _, _ *int) (*domain.Match, error) {
	return r.match, r.err
}
func (r *stubMatchRepo) UpdateLiveProgress(_ context.Context, _ int, _ *string, _, _ *int) error {
	return nil
}
func (r *stubMatchRepo) ListFinishedPenaltyMatchesMissingWinner(_ context.Context) ([]*domain.Match, error) {
	return nil, nil
}

// stubPublisher records published envelopes without delivering them.
type stubPublisher struct {
	published []events.Envelope
	err       error
}

func (p *stubPublisher) Publish(_ context.Context, env events.Envelope) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, env)
	return nil
}

// stubScorer records ScoreMatch calls.
type stubScorer struct {
	called []int
	err    error
}

func (s *stubScorer) ScoreMatch(_ context.Context, matchID int) error {
	s.called = append(s.called, matchID)
	return s.err
}

// stubExtraScorer records ScoreExtras calls.
type stubExtraScorer struct {
	called []int
	err    error
}

func (s *stubExtraScorer) ScoreExtras(_ context.Context, matchID int) error {
	s.called = append(s.called, matchID)
	return s.err
}

// noopAuditLogger satisfies AuditLogger without doing anything.
type noopAuditLogger struct{}

func (*noopAuditLogger) Log(_ context.Context, _ *int, _ *domain.UserRole, _ string, _ *string, _ *int, _ map[string]any) {
}

// noopSystemParamService satisfies SystemParamService with domain-constant defaults.
type noopSystemParamService struct{}

func (*noopSystemParamService) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return nil, nil
}
func (*noopSystemParamService) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (*noopSystemParamService) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (*noopSystemParamService) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (*noopSystemParamService) GetString(_ context.Context, _ string, d string) string { return d }
func (*noopSystemParamService) GetInt(_ context.Context, _ string, d int) int          { return d }
func (*noopSystemParamService) GetDuration(_ context.Context, _ string, d time.Duration) time.Duration {
	return d
}
func (*noopSystemParamService) GetBool(_ context.Context, _ string, d bool) bool { return d }
func (*noopSystemParamService) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return nil
}
func (*noopSystemParamService) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (*noopSystemParamService) BulkPreview(_ context.Context, _ map[string]string) ([]domain.ParamDiff, error) {
	return nil, nil
}
func (*noopSystemParamService) GetHistory(_ context.Context, _ string, _ repository.CursorPage) ([]*domain.SystemParamHistory, string, error) {
	return nil, "", nil
}

func newMatchSvc(match *domain.Match) (MatchService, *stubPublisher) {
	pub := &stubPublisher{}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, &stubScorer{}, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())
	return svc, pub
}

// ── UpdateResult - status guard ───────────────────────────────────────────────

// TestUpdateResult_LiveMatch_ConfirmsResultAndEmitsEvent is the happy path:
// a live match can receive a final score and emits MatchFinished.
func TestUpdateResult_LiveMatch_ConfirmsResultAndEmitsEvent(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}
	svc, pub := newMatchSvc(match)

	result, err := svc.UpdateResult(context.Background(), 1, ScoreUpdate{HomeScore: 2, AwayScore: 1, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.Status != domain.MatchStatusFinished {
		t.Errorf("expected status finished, got %s", result.Status)
	}
	if len(pub.published) != 1 || pub.published[0].Type != events.EventMatchFinished {
		t.Errorf("expected one MatchFinished event, got %v", pub.published)
	}
}

// TestUpdateResult_LiveMatch_WithWinMethod_PopulatesEventPayload verifies that
// WinMethod is propagated into the MatchFinished event payload (covers winMethodString
// non-nil branch).
func TestUpdateResult_LiveMatch_WithWinMethod_PopulatesEventPayload(t *testing.T) {
	match := &domain.Match{ID: 5, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}
	svc, pub := newMatchSvc(match)

	wm := domain.WinMethodPenalties
	pw := "home"
	_, err := svc.UpdateResult(context.Background(), 5, ScoreUpdate{HomeScore: 3, AwayScore: 2, WinMethod: &wm, PenaltyWinner: &pw, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected one MatchFinished event, got %d", len(pub.published))
	}
	payload, ok := pub.published[0].Payload.(events.MatchFinished)
	if !ok {
		t.Fatalf("expected MatchFinished payload, got %T", pub.published[0].Payload)
	}
	if payload.WinMethod != string(domain.WinMethodPenalties) {
		t.Errorf("expected WinMethod %q in event, got %q", domain.WinMethodPenalties, payload.WinMethod)
	}
}

func TestUpdateResult_PenaltiesWinMethod_MissingPenaltyWinner_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 6, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}
	svc, _ := newMatchSvc(match)

	wm := domain.WinMethodPenalties
	_, err := svc.UpdateResult(context.Background(), 6, ScoreUpdate{HomeScore: 1, AwayScore: 1, WinMethod: &wm, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error when penalty_winner absent, got %v", err)
	}
}

func TestUpdateResult_PenaltiesWinMethod_InvalidPenaltyWinner_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 7, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}
	svc, _ := newMatchSvc(match)

	wm := domain.WinMethodPenalties
	bad := "draw"
	_, err := svc.UpdateResult(context.Background(), 7, ScoreUpdate{HomeScore: 1, AwayScore: 1, WinMethod: &wm, PenaltyWinner: &bad, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for penalty_winner=%q, got %v", bad, err)
	}
}

func TestUpdateResult_PenaltiesWinMethod_WithPenaltyScores_PersistsScores(t *testing.T) {
	match := &domain.Match{ID: 8, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}
	repo := &stubMatchRepo{match: match}
	pub := &stubPublisher{}
	svc := NewMatchService(repo, pub, &stubScorer{}, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	wm := domain.WinMethodPenalties
	pw := "away"
	phome, paway := 4, 5
	result, err := svc.UpdateResult(context.Background(), 8, ScoreUpdate{HomeScore: 1, AwayScore: 1, WinMethod: &wm, PenaltyWinner: &pw, PenaltyHomeScore: &phome, PenaltyAwayScore: &paway})
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.PenaltyHomeScore == nil || *result.PenaltyHomeScore != phome {
		t.Errorf("PenaltyHomeScore: want %d, got %v", phome, result.PenaltyHomeScore)
	}
	if result.PenaltyAwayScore == nil || *result.PenaltyAwayScore != paway {
		t.Errorf("PenaltyAwayScore: want %d, got %v", paway, result.PenaltyAwayScore)
	}
	if result.PenaltyWinner == nil || *result.PenaltyWinner != pw {
		t.Errorf("PenaltyWinner: want %q, got %v", pw, result.PenaltyWinner)
	}
}

// TestUpdateResult_ScheduledMatch_ReturnsValidationError enforces that a result
// cannot be set before the match is started. The admin must call StartMatch first,
// which closes the prediction window.
func TestUpdateResult_ScheduledMatch_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: matchFrance, AwayTeam: matchGermany,
		Status: domain.MatchStatusScheduled, KickoffAt: time.Now().Add(time.Hour)}
	svc, _ := newMatchSvc(match)

	_, err := svc.UpdateResult(context.Background(), 1, ScoreUpdate{HomeScore: 1, AwayScore: 0, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for scheduled match, got %v", err)
	}
}

// TestUpdateResult_FinishedMatch_ReturnsValidationError enforces that a confirmed
// result is permanent and cannot be overwritten.
func TestUpdateResult_FinishedMatch_ReturnsValidationError(t *testing.T) {
	home, away := 2, 1
	match := &domain.Match{ID: 1, HomeTeam: "Spain", AwayTeam: "England",
		Status:    domain.MatchStatusFinished,
		HomeScore: &home, AwayScore: &away,
		KickoffAt: time.Now().Add(-2 * time.Hour)}
	svc, _ := newMatchSvc(match)

	_, err := svc.UpdateResult(context.Background(), 1, ScoreUpdate{HomeScore: 3, AwayScore: 0, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

// TestUpdateResult_PublishFails_FallsBackToSynchronousScoring verifies that when
// the event bus is unavailable, predictions are still scored synchronously so
// no match is ever left unscored due to a transient Redis outage.
func TestUpdateResult_PublishFails_FallsBackToSynchronousScoring(t *testing.T) {
	match := &domain.Match{ID: 42, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}

	pub := &stubPublisher{err: errors.New("redis unavailable")}
	scorer := &stubScorer{}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, scorer, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	result, err := svc.UpdateResult(context.Background(), 42, ScoreUpdate{HomeScore: 2, AwayScore: 1, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if err != nil {
		t.Fatalf("UpdateResult must succeed even when publish fails: %v", err)
	}
	if result.Status != domain.MatchStatusFinished {
		t.Errorf("status: want finished, got %s", result.Status)
	}
	if len(pub.published) != 0 {
		t.Errorf("publish must not record events when it returns an error")
	}
	if len(scorer.called) != 1 || scorer.called[0] != 42 {
		t.Errorf("fallback scorer must be called with match ID 42, got %v", scorer.called)
	}
}

// TestUpdateResult_PublishFails_ScorerAlsoFails_StillReturnsResult verifies that
// a double failure (bus down + DB error in scorer) returns the confirmed match
// result to the caller. Both failures are logged; the HTTP response remains 200.
func TestUpdateResult_PublishFails_ScorerAlsoFails_StillReturnsResult(t *testing.T) {
	match := &domain.Match{ID: 7, HomeTeam: matchFrance, AwayTeam: matchGermany,
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}

	pub := &stubPublisher{err: errors.New("redis unavailable")}
	scorer := &stubScorer{err: errors.New("db timeout")}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, scorer, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	result, err := svc.UpdateResult(context.Background(), 7, ScoreUpdate{HomeScore: 1, AwayScore: 0, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if err != nil {
		t.Fatalf("UpdateResult must succeed regardless of scorer failure: %v", err)
	}
	if result.Status != domain.MatchStatusFinished {
		t.Errorf("status: want finished, got %s", result.Status)
	}
}

// ── StartMatch - status guard ─────────────────────────────────────────────────

func TestStartMatch_ScheduledMatch_TransitionsToLiveAndEmitsEvent(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusScheduled, KickoffAt: time.Now()}
	svc, pub := newMatchSvc(match)

	result, err := svc.StartMatch(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.Status != domain.MatchStatusLive {
		t.Errorf("expected status live, got %s", result.Status)
	}
	if len(pub.published) != 1 || pub.published[0].Type != events.EventMatchStarted {
		t.Errorf("expected one MatchStarted event, got %v", pub.published)
	}
}

func TestStartMatch_LiveMatch_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusLive}
	svc, _ := newMatchSvc(match)

	_, err := svc.StartMatch(context.Background(), 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for already-live match, got %v", err)
	}
}

func TestStartMatch_FinishedMatch_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusFinished}
	svc, _ := newMatchSvc(match)

	_, err := svc.StartMatch(context.Background(), 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

func TestStartMatch_PublishFails_StillReturnsMatch(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusScheduled, KickoffAt: time.Now()}
	pub := &stubPublisher{err: errors.New("redis unavailable")}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, &stubScorer{}, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	result, err := svc.StartMatch(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected nil error even when publish fails, got %v", err)
	}
	if result.Status != domain.MatchStatusLive {
		t.Errorf("expected live status, got %s", result.Status)
	}
}

// ── CreateMatch ───────────────────────────────────────────────────────────────

func TestCreateMatch_ValidMatch_ReturnsNil(t *testing.T) {
	svc, _ := newMatchSvc(nil)
	m := &domain.Match{HomeTeam: matchBrazil, AwayTeam: matchArgentina, KickoffAt: time.Now().Add(24 * time.Hour)}
	if err := svc.CreateMatch(context.Background(), m); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCreateMatch_InvalidMatch_ReturnsValidation(t *testing.T) {
	svc, _ := newMatchSvc(nil)
	m := &domain.Match{HomeTeam: "", AwayTeam: matchArgentina, KickoffAt: time.Now().Add(time.Hour)}
	if err := svc.CreateMatch(context.Background(), m); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for empty home team, got %v", err)
	}
}

// ── GetMatch ──────────────────────────────────────────────────────────────────

func TestGetMatch_Found_ReturnsMatch(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: matchBrazil, AwayTeam: matchArgentina, Status: domain.MatchStatusScheduled}
	svc, _ := newMatchSvc(match)

	got, err := svc.GetMatch(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if got.ID != 1 {
		t.Errorf("expected match ID 1, got %d", got.ID)
	}
}

func TestGetMatch_NotFound_ReturnsNotFound(t *testing.T) {
	svc, _ := newMatchSvc(nil) // repo returns nil, nil
	_, err := svc.GetMatch(context.Background(), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── ListMatches / ListMatchesByStatus ─────────────────────────────────────────

func TestListMatches_ReturnsSlice(t *testing.T) {
	pub := &stubPublisher{}
	matches := []*domain.Match{
		{ID: 1, HomeTeam: matchBrazil, AwayTeam: matchArgentina, Status: domain.MatchStatusScheduled},
	}
	svc := NewMatchService(&stubMatchRepo{matches: matches}, pub, &stubScorer{}, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.ListMatches(context.Background())
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtExpect1Match, len(got))
	}
}

func TestListMatchesByPhase_ReturnsFilteredSlice(t *testing.T) {
	pub := &stubPublisher{}
	matches := []*domain.Match{
		{ID: 1, HomeTeam: matchBrazil, AwayTeam: matchArgentina, Phase: domain.PhaseGroupStage},
	}
	svc := NewMatchService(&stubMatchRepo{matches: matches}, pub, &stubScorer{}, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.ListMatchesByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtExpect1Match, len(got))
	}
}

func TestListMatchesByStatus_ReturnsFilteredSlice(t *testing.T) {
	pub := &stubPublisher{}
	matches := []*domain.Match{
		{ID: 1, HomeTeam: matchFrance, AwayTeam: matchGermany, Status: domain.MatchStatusLive},
	}
	svc := NewMatchService(&stubMatchRepo{matches: matches}, pub, &stubScorer{}, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.ListMatchesByStatus(context.Background(), domain.MatchStatusLive)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtExpect1Match, len(got))
	}
}

// ── CorrectResult ─────────────────────────────────────────────────────────────

func TestCorrectResult_FinishedMatch_UpdatesScoreAndEmitsEvent(t *testing.T) {
	home, away := 1, 0
	match := &domain.Match{ID: 7, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusFinished, HomeScore: &home, AwayScore: &away}
	pub := &stubPublisher{}
	scorer := &stubScorer{}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, scorer, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	result, err := svc.CorrectResult(context.Background(), 7, ScoreUpdate{HomeScore: 2, AwayScore: 1, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.Status != domain.MatchStatusFinished {
		t.Errorf("status should remain finished after correction, got %s", result.Status)
	}
	if len(pub.published) != 1 || pub.published[0].Type != events.EventMatchFinished {
		t.Errorf("expected one MatchFinished re-emit, got %v", pub.published)
	}
}

func TestCorrectResult_LiveMatch_AllowedAndSetsFinished(t *testing.T) {
	match := &domain.Match{ID: 8, HomeTeam: matchFrance, AwayTeam: matchGermany,
		Status: domain.MatchStatusLive}
	svc, _ := newMatchSvc(match)

	result, err := svc.CorrectResult(context.Background(), 8, ScoreUpdate{HomeScore: 1, AwayScore: 0, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.Status != domain.MatchStatusFinished {
		t.Errorf("expected finished after correct on live, got %s", result.Status)
	}
}

func TestCorrectResult_ScheduledMatch_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 9, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusScheduled}
	svc, _ := newMatchSvc(match)

	_, err := svc.CorrectResult(context.Background(), 9, ScoreUpdate{HomeScore: 2, AwayScore: 0, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for scheduled match, got %v", err)
	}
}

func TestCorrectResult_CancelledMatch_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 10, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusCancelled}
	svc, _ := newMatchSvc(match)

	_, err := svc.CorrectResult(context.Background(), 10, ScoreUpdate{HomeScore: 2, AwayScore: 0, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for cancelled match, got %v", err)
	}
}

func TestCorrectResult_PublishFails_FallsBackToSynchronousScoring(t *testing.T) {
	match := &domain.Match{ID: 11, HomeTeam: matchFrance, AwayTeam: matchGermany,
		Status: domain.MatchStatusFinished}
	pub := &stubPublisher{err: errors.New("redis unavailable")}
	scorer := &stubScorer{}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, scorer, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.CorrectResult(context.Background(), 11, ScoreUpdate{HomeScore: 2, AwayScore: 1, WinMethod: nil, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if err != nil {
		t.Fatalf("CorrectResult must succeed even when publish fails: %v", err)
	}
	if len(scorer.called) != 1 || scorer.called[0] != 11 {
		t.Errorf("expected synchronous fallback scoring for match 11, got %v", scorer.called)
	}
}

func TestCorrectResult_WithWinMethod_IncludedInEvent(t *testing.T) {
	match := &domain.Match{ID: 12, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusFinished}
	pub := &stubPublisher{}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, &stubScorer{}, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop())

	wm := domain.WinMethodExtraTime
	_, err := svc.CorrectResult(context.Background(), 12, ScoreUpdate{HomeScore: 1, AwayScore: 0, WinMethod: &wm, PenaltyWinner: nil, PenaltyHomeScore: nil, PenaltyAwayScore: nil})
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	payload, ok := pub.published[0].Payload.(events.MatchFinished)
	if !ok {
		t.Fatalf("expected MatchFinished payload")
	}
	if payload.WinMethod != string(domain.WinMethodExtraTime) {
		t.Errorf("expected WinMethod extra_time, got %q", payload.WinMethod)
	}
}

// ── CancelMatch ───────────────────────────────────────────────────────────────

func TestCancelMatch_ScheduledMatch_SetsCancelled(t *testing.T) {
	match := &domain.Match{ID: 20, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusScheduled}
	svc, _ := newMatchSvc(match)

	result, err := svc.CancelMatch(context.Background(), 20)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.Status != domain.MatchStatusCancelled {
		t.Errorf("expected cancelled, got %s", result.Status)
	}
}

func TestCancelMatch_LiveMatch_SetsCancelled(t *testing.T) {
	match := &domain.Match{ID: 21, HomeTeam: matchFrance, AwayTeam: matchGermany,
		Status: domain.MatchStatusLive}
	svc, _ := newMatchSvc(match)

	result, err := svc.CancelMatch(context.Background(), 21)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.Status != domain.MatchStatusCancelled {
		t.Errorf("expected cancelled, got %s", result.Status)
	}
}

func TestCancelMatch_FinishedMatch_ReturnsValidationError(t *testing.T) {
	home, away := 2, 0
	match := &domain.Match{ID: 22, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusFinished, HomeScore: &home, AwayScore: &away}
	svc, _ := newMatchSvc(match)

	_, err := svc.CancelMatch(context.Background(), 22)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

func TestCancelMatch_AlreadyCancelled_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 23, HomeTeam: matchBrazil, AwayTeam: matchArgentina,
		Status: domain.MatchStatusCancelled}
	svc, _ := newMatchSvc(match)

	_, err := svc.CancelMatch(context.Background(), 23)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for already-cancelled match, got %v", err)
	}
}

func TestCancelMatch_NotFound_ReturnsNotFound(t *testing.T) {
	svc, _ := newMatchSvc(nil)

	_, err := svc.CancelMatch(context.Background(), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── UpdateSlots ───────────────────────────────────────────────────────────────

func TestMatchService_UpdateSlots_DelegatesToRepo(t *testing.T) {
	home := 1
	away := 2
	match := &domain.Match{ID: 5, HomeSlotID: &home, AwaySlotID: &away}
	svc, _ := newMatchSvc(match)

	got, err := svc.UpdateSlots(context.Background(), 5, &home, &away)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if got.ID != 5 {
		t.Errorf("expected match ID 5, got %d", got.ID)
	}
}

func TestMatchService_UpdateSlots_RepoError_Propagated(t *testing.T) {
	svc := NewMatchService(
		&stubMatchRepo{err: apperrors.NotFound("match not found")},
		&stubPublisher{}, &stubScorer{}, &stubExtraScorer{}, &noopAuditLogger{}, zap.NewNop(),
	)

	_, err := svc.UpdateSlots(context.Background(), 99, nil, nil)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}
