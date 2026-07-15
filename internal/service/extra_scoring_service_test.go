package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// stubExtraPredRepo implements repository.ExtraPredictionRepository with
// configurable returns. ScoreMatchBatch mirrors stubPredRepo's behaviour:
// it invokes the scorer with .list and records every prediction ID the
// scorer chose to update in .updated.
type stubExtraPredRepo struct {
	list    []*domain.ExtraPrediction
	err     error
	updated []*domain.ExtraPrediction
}

func (r *stubExtraPredRepo) Upsert(_ context.Context, p *domain.ExtraPrediction) (bool, error) {
	return true, r.err
}
func (r *stubExtraPredRepo) GetByUserAndMatch(_ context.Context, _, _ int) ([]*domain.ExtraPrediction, error) {
	return r.list, r.err
}
func (r *stubExtraPredRepo) ListByUserAndMatches(_ context.Context, _ int, _ []int) ([]*domain.ExtraPrediction, error) {
	return r.list, r.err
}
func (r *stubExtraPredRepo) ScoreMatchBatch(_ context.Context, _ int, scorer func([]*domain.ExtraPrediction) (map[int]int, error), _ int) error {
	points, err := scorer(r.list)
	if err != nil {
		return err
	}
	for _, p := range r.list {
		if pts, ok := points[p.ID]; ok {
			pts := pts
			p.Points = &pts
			r.updated = append(r.updated, p)
		}
	}
	return r.err
}

// stubExtraRuleRepo implements repository.ExtraRuleRepository with a single
// configurable rule (or none, to exercise the domain-default fallback).
type stubExtraRuleRepo struct {
	rule *domain.ExtraRule
	err  error
}

func (r *stubExtraRuleRepo) List(_ context.Context) ([]*domain.ExtraRule, error) {
	if r.rule != nil {
		return []*domain.ExtraRule{r.rule}, r.err
	}
	return nil, r.err
}
func (r *stubExtraRuleRepo) GetByType(_ context.Context, _ domain.ExtraType) (*domain.ExtraRule, error) {
	return r.rule, r.err
}
func (r *stubExtraRuleRepo) Update(_ context.Context, rule *domain.ExtraRule) (*domain.ExtraRule, error) {
	return rule, r.err
}

func intp(n int) *int       { return &n }
func strp(s string) *string { return &s }

// ── ScoreExtras ───────────────────────────────────────────────────────────────

func TestScoreExtras_FirstScorer_CorrectAndIncorrectGuesses(t *testing.T) {
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: intp(2), AwayScore: intp(1),
		FirstScoringTeam: strp("home"),
	}
	preds := []*domain.ExtraPrediction{
		{ID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"}, // correct
		{ID: 2, ExtraType: domain.ExtraTypeFirstScorer, Answer: "away"}, // wrong
	}
	extraRepo := &stubExtraPredRepo{list: preds}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, extraRepo, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(extraRepo.updated) != 2 {
		t.Fatalf("expected 2 updated extra predictions, got %d", len(extraRepo.updated))
	}
	if *preds[0].Points != domain.DefaultExtraFirstScorerPoints {
		t.Errorf("correct guess: got %d points, want %d", *preds[0].Points, domain.DefaultExtraFirstScorerPoints)
	}
	if *preds[1].Points != 0 {
		t.Errorf("wrong guess: got %d points, want 0", *preds[1].Points)
	}
}

func TestScoreExtras_HalftimeResult_DerivedFromScores(t *testing.T) {
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: intp(2), AwayScore: intp(2),
		HalftimeHomeScore: intp(1), HalftimeAwayScore: intp(0),
	}
	preds := []*domain.ExtraPrediction{
		{ID: 1, ExtraType: domain.ExtraTypeHalftimeResult, Answer: "home"}, // correct: 1-0 at HT
		{ID: 2, ExtraType: domain.ExtraTypeHalftimeResult, Answer: "draw"}, // wrong
	}
	extraRepo := &stubExtraPredRepo{list: preds}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, extraRepo, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *preds[0].Points != domain.DefaultExtraHalftimeResultPoints {
		t.Errorf("correct guess: got %d points, want %d", *preds[0].Points, domain.DefaultExtraHalftimeResultPoints)
	}
	if *preds[1].Points != 0 {
		t.Errorf("wrong guess: got %d points, want 0", *preds[1].Points)
	}
}

func TestScoreExtras_HalftimeDraw_ResolvesToDrawAnswer(t *testing.T) {
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: intp(1), AwayScore: intp(1),
		HalftimeHomeScore: intp(0), HalftimeAwayScore: intp(0),
	}
	preds := []*domain.ExtraPrediction{
		{ID: 1, ExtraType: domain.ExtraTypeHalftimeResult, Answer: "draw"},
	}
	extraRepo := &stubExtraPredRepo{list: preds}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, extraRepo, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *preds[0].Points != domain.DefaultExtraHalftimeResultPoints {
		t.Errorf("draw guess: got %d points, want %d", *preds[0].Points, domain.DefaultExtraHalftimeResultPoints)
	}
}

func TestScoreExtras_UnresolvedFirstScorer_LeavesPredictionUnscored(t *testing.T) {
	// Match finished but FirstScoringTeam was never resolved (legacy match, or
	// GetFixtureEvents failed and was never repaired).
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: intp(1), AwayScore: intp(0),
		FirstScoringTeam: nil,
	}
	preds := []*domain.ExtraPrediction{
		{ID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"},
	}
	extraRepo := &stubExtraPredRepo{list: preds}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, extraRepo, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(extraRepo.updated) != 0 {
		t.Errorf("expected 0 updated predictions when the answer is unresolved, got %d", len(extraRepo.updated))
	}
	if preds[0].Points != nil {
		t.Error("expected Points to remain nil for an unresolved extra")
	}
}

func TestScoreExtras_UnresolvedHalftimeScores_LeavesPredictionUnscored(t *testing.T) {
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: intp(1), AwayScore: intp(0),
		HalftimeHomeScore: nil, HalftimeAwayScore: nil,
	}
	preds := []*domain.ExtraPrediction{
		{ID: 1, ExtraType: domain.ExtraTypeHalftimeResult, Answer: "home"},
	}
	extraRepo := &stubExtraPredRepo{list: preds}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, extraRepo, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(extraRepo.updated) != 0 {
		t.Errorf("expected 0 updated predictions when halftime scores are unresolved, got %d", len(extraRepo.updated))
	}
}

func TestScoreExtras_ActiveRuleOverridesDefaultPoints(t *testing.T) {
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: intp(1), AwayScore: intp(0),
		FirstScoringTeam: strp("home"),
	}
	preds := []*domain.ExtraPrediction{
		{ID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"},
	}
	extraRepo := &stubExtraPredRepo{list: preds}
	ruleRepo := &stubExtraRuleRepo{rule: &domain.ExtraRule{ExtraType: domain.ExtraTypeFirstScorer, Points: 10, IsActive: true}}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, extraRepo, ruleRepo, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *preds[0].Points != 10 {
		t.Errorf("got %d points, want 10 (from active rule)", *preds[0].Points)
	}
}

func TestScoreExtras_InactiveRule_FallsBackToDefault(t *testing.T) {
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: intp(1), AwayScore: intp(0),
		FirstScoringTeam: strp("home"),
	}
	preds := []*domain.ExtraPrediction{
		{ID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"},
	}
	extraRepo := &stubExtraPredRepo{list: preds}
	ruleRepo := &stubExtraRuleRepo{rule: &domain.ExtraRule{ExtraType: domain.ExtraTypeFirstScorer, Points: 99, IsActive: false}}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, extraRepo, ruleRepo, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *preds[0].Points != domain.DefaultExtraFirstScorerPoints {
		t.Errorf("got %d points, want default %d (inactive rule falls back)", *preds[0].Points, domain.DefaultExtraFirstScorerPoints)
	}
}

func TestScoreExtras_RuleLookupError_FallsBackToDefault(t *testing.T) {
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: intp(1), AwayScore: intp(0),
		FirstScoringTeam: strp("home"),
	}
	preds := []*domain.ExtraPrediction{
		{ID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"},
	}
	extraRepo := &stubExtraPredRepo{list: preds}
	ruleRepo := &stubExtraRuleRepo{err: errors.New("db timeout")}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, extraRepo, ruleRepo, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *preds[0].Points != domain.DefaultExtraFirstScorerPoints {
		t.Errorf("got %d points, want default %d (rule lookup error falls back)", *preds[0].Points, domain.DefaultExtraFirstScorerPoints)
	}
}

func TestScoreExtras_MatchNotFound_ReturnsNotFound(t *testing.T) {
	svc := NewExtraScoringService(&stubMatchRepo{match: nil}, &stubExtraPredRepo{}, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 99); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestScoreExtras_MatchNotFinished_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, Status: domain.MatchStatusLive}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, &stubExtraPredRepo{}, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for non-finished match, got %v", err)
	}
}

func TestScoreExtras_MatchRepoError_Propagates(t *testing.T) {
	repoErr := errors.New("db timeout")
	svc := NewExtraScoringService(&stubMatchRepo{err: repoErr}, &stubExtraPredRepo{}, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); !errors.Is(err, repoErr) {
		t.Errorf("expected repo error to propagate, got %v", err)
	}
}

func TestScoreExtras_NoPredictions_NoOp(t *testing.T) {
	match := &domain.Match{ID: 1, Status: domain.MatchStatusFinished, HomeScore: intp(1), AwayScore: intp(0)}
	svc := NewExtraScoringService(&stubMatchRepo{match: match}, &stubExtraPredRepo{}, &stubExtraRuleRepo{}, zap.NewNop())

	if err := svc.ScoreExtras(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
