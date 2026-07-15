package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

func TestExtraPredictionRepository_Upsert_CreatesNewRow(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	p := &domain.ExtraPrediction{UserID: u.ID, MatchID: m.ID, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"}
	created, err := repo.Upsert(context.Background(), p)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !created {
		t.Error("expected created=true for a brand new row")
	}
	if p.ID == 0 {
		t.Error("expected ID to be hydrated after insert")
	}
	if p.Points != nil {
		t.Error("expected Points to be nil before scoring")
	}
}

func TestExtraPredictionRepository_Upsert_ConflictUpdatesAnswer(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	first := &domain.ExtraPrediction{UserID: u.ID, MatchID: m.ID, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"}
	if _, err := repo.Upsert(context.Background(), first); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	second := &domain.ExtraPrediction{UserID: u.ID, MatchID: m.ID, ExtraType: domain.ExtraTypeFirstScorer, Answer: "away"}
	created, err := repo.Upsert(context.Background(), second)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if created {
		t.Error("expected created=false on conflict")
	}
	if second.Answer != "away" {
		t.Errorf("answer: got %q, want \"away\" (updated)", second.Answer)
	}
	if second.ID != first.ID {
		t.Error("expected same row ID on conflict-upsert")
	}
}

func TestExtraPredictionRepository_Upsert_DifferentExtraTypes_BothPersist(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	if _, err := repo.Upsert(context.Background(), &domain.ExtraPrediction{
		UserID: u.ID, MatchID: m.ID, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home",
	}); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if _, err := repo.Upsert(context.Background(), &domain.ExtraPrediction{
		UserID: u.ID, MatchID: m.ID, ExtraType: domain.ExtraTypeHalftimeResult, Answer: "1-1",
	}); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetByUserAndMatch(context.Background(), u.ID, m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 extra predictions, got %d", len(got))
	}
}

func TestExtraPredictionRepository_GetByUserAndMatch_NoRows_ReturnsEmpty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	got, err := repo.GetByUserAndMatch(context.Background(), u.ID, m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 rows, got %d", len(got))
	}
}

func TestExtraPredictionRepository_ListByUserAndMatches_BulkFetch(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	if _, err := repo.Upsert(context.Background(), &domain.ExtraPrediction{
		UserID: u.ID, MatchID: m1.ID, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home",
	}); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if _, err := repo.Upsert(context.Background(), &domain.ExtraPrediction{
		UserID: u.ID, MatchID: m2.ID, ExtraType: domain.ExtraTypeHalftimeResult, Answer: "0-1",
	}); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.ListByUserAndMatches(context.Background(), u.ID, []int{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 extra predictions across both matches, got %d", len(got))
	}
}

func TestExtraPredictionRepository_ListByUserAndMatches_EmptyMatchIDs_ReturnsNil(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	got, err := repo.ListByUserAndMatches(context.Background(), u.ID, nil)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for empty matchIDs, got %v", got)
	}
}

func TestExtraPredictionRepository_ScoreMatchBatch_UpdatesPoints(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	p := &domain.ExtraPrediction{UserID: u.ID, MatchID: m.ID, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"}
	if _, err := repo.Upsert(context.Background(), p); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	err := repo.ScoreMatchBatch(context.Background(), m.ID, func(preds []*domain.ExtraPrediction) (map[int]int, error) {
		if len(preds) != 1 {
			t.Fatalf("scorer: expected 1 prediction, got %d", len(preds))
		}
		return map[int]int{preds[0].ID: 3}, nil
	}, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetByUserAndMatch(context.Background(), u.ID, m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 || got[0].Points == nil || *got[0].Points != 3 {
		t.Fatalf("expected points=3 after scoring, got %+v", got)
	}
	if got[0].ScoredAt == nil {
		t.Error("expected ScoredAt to be set after scoring")
	}
}

func TestExtraPredictionRepository_ScoreMatchBatch_IdempotentOnRescore(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	p := &domain.ExtraPrediction{UserID: u.ID, MatchID: m.ID, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"}
	if _, err := repo.Upsert(context.Background(), p); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	scorer := func(preds []*domain.ExtraPrediction) (map[int]int, error) {
		out := make(map[int]int, len(preds))
		for _, pred := range preds {
			out[pred.ID] = 3
		}
		return out, nil
	}
	if err := repo.ScoreMatchBatch(context.Background(), m.ID, scorer, 0); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	// Second call must be a no-op: the WHERE scored_at IS NULL guard skips
	// already-scored rows, so a different point value in this second scorer
	// call must NOT overwrite the first result.
	if err := repo.ScoreMatchBatch(context.Background(), m.ID, func(preds []*domain.ExtraPrediction) (map[int]int, error) {
		out := make(map[int]int, len(preds))
		for _, pred := range preds {
			out[pred.ID] = 99
		}
		return out, nil
	}, 0); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetByUserAndMatch(context.Background(), u.ID, m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 || got[0].Points == nil || *got[0].Points != 3 {
		t.Fatalf("expected points to remain 3 after re-score, got %+v", got)
	}
}

func TestExtraPredictionRepository_ScoreMatchBatch_NoRows_ScorerCalledWithEmptySlice(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	called := false
	err := repo.ScoreMatchBatch(context.Background(), m.ID, func(preds []*domain.ExtraPrediction) (map[int]int, error) {
		called = true
		return nil, nil
	}, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !called {
		t.Error("expected scorer to be called even with zero predictions (matches PredictionRepository behaviour)")
	}
}

func TestExtraPredictionRepository_Upsert_CancelledContextReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresExtraPredictionRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.Upsert(ctx, &domain.ExtraPrediction{UserID: u.ID, MatchID: m.ID, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"})
	if err == nil {
		t.Error(repoMsgCancelledCtx)
	}
}
