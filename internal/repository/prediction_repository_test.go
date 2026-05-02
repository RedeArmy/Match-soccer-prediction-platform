package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── PredictionRepository ──────────────────────────────────────────────────────

func TestPredictionRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestPredictionRepository_Create_Duplicate_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p1); err != nil {
		t.Fatalf("first create: %v", err)
	}

	p2 := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	if err := repo.Create(context.Background(), p2); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for duplicate prediction, got %v", err)
	}
}

func TestPredictionRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	got, err := repo.GetByID(context.Background(), p.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected prediction, got nil")
	}
	if got.HomeScore != 1 {
		t.Errorf("home score: got %d, want 1", got.HomeScore)
	}
}

func TestPredictionRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for missing prediction, got %+v", got)
	}
}

func TestPredictionRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	pts := 5
	p.Points = &pts
	if err := repo.Update(context.Background(), p); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p.Points == nil || *p.Points != 5 {
		t.Errorf("points not updated: got %v", p.Points)
	}
}

func TestPredictionRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	ghost := &domain.Prediction{ID: 99999, HomeScore: 1, AwayScore: 0}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestPredictionRepository_GetByUserAndMatch_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 3, AwayScore: 2}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	got, err := repo.GetByUserAndMatch(context.Background(), u.ID, m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected prediction, got nil")
	}
	if got.ID != p.ID {
		t.Errorf("ID: got %d, want %d", got.ID, p.ID)
	}
}

func TestPredictionRepository_GetByUserAndMatch_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	got, err := repo.GetByUserAndMatch(context.Background(), 99999, 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestPredictionRepository_ListByUser_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 1}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 1 {
		t.Errorf(repoMsgExpect1Pred, len(preds))
	}
}

func TestPredictionRepository_ListByMatch_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 0, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := repo.ListByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 1 {
		t.Errorf(repoMsgExpect1Pred, len(preds))
	}
}

func TestPredictionRepository_UpdateManyPoints_PersistsPoints(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	p2 := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 0, AwayScore: 0}
	if err := repo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	u2 := seedUser(t)
	p2.UserID = u2.ID
	if err := repo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	points := map[int]int{p1.ID: 5, p2.ID: 2}
	if err := repo.UpdateManyPoints(context.Background(), points); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got1, _ := repo.GetByID(context.Background(), p1.ID)
	got2, _ := repo.GetByID(context.Background(), p2.ID)
	if got1.Points == nil || *got1.Points != 5 {
		t.Errorf("p1 points: got %v, want 5", got1.Points)
	}
	if got2.Points == nil || *got2.Points != 2 {
		t.Errorf("p2 points: got %v, want 2", got2.Points)
	}
}

func TestPredictionRepository_UpdateManyPoints_EmptyMap_IsNoop(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	if err := repo.UpdateManyPoints(context.Background(), map[int]int{}); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestPredictionRepository_UpdateManyPoints_LargeBatch(t *testing.T) {
	const batchSize = 10
	cleanTables(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	m := seedMatch(t)
	preds := make([]*domain.Prediction, batchSize)
	for i := range batchSize {
		u := seedUser(t)
		p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: i, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf("create prediction %d: %v", i, err)
		}
		preds[i] = p
	}

	wantPoints := make(map[int]int, batchSize)
	for i, p := range preds {
		wantPoints[p.ID] = i + 1
	}

	if err := predRepo.UpdateManyPoints(context.Background(), wantPoints); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	for _, p := range preds {
		got, err := predRepo.GetByID(context.Background(), p.ID)
		if err != nil {
			t.Fatalf("get prediction %d: %v", p.ID, err)
		}
		want := wantPoints[p.ID]
		if got.Points == nil || *got.Points != want {
			t.Errorf("prediction %d: got points=%v, want %d", p.ID, got.Points, want)
		}
	}
}

// ── PredictionRepository — TotalPointsByQuiniela ──────────────────────────────

func TestPredictionRepository_TotalPointsByQuiniela_ReturnsSumPerUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts1 := 5
	p1.Points = &pts1
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf("update prediction u1: %v", err)
	}

	p2 := &domain.Prediction{UserID: u2.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts2 := 3
	p2.Points = &pts2
	if err := predRepo.Update(context.Background(), p2); err != nil {
		t.Fatalf("update prediction u2: %v", err)
	}

	totals, err := predRepo.TotalPointsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(totals))
	}
	if totals[u1.ID] != 5 {
		t.Errorf("user1 total: got %d, want 5", totals[u1.ID])
	}
	if totals[u2.ID] != 3 {
		t.Errorf("user2 total: got %d, want 3", totals[u2.ID])
	}
}

func TestPredictionRepository_TotalPointsByQuiniela_ExcludesUnpaidMembers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, false)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := 3
	p1.Points = &pts
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	p2 := &domain.Prediction{UserID: u2.ID, MatchID: m.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts2 := 2
	p2.Points = &pts2
	if err := predRepo.Update(context.Background(), p2); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	totals, err := predRepo.TotalPointsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 1 {
		t.Fatalf("expected 1 entry (paid only), got %d", len(totals))
	}
	if _, ok := totals[u2.ID]; ok {
		t.Error("unpaid user must not appear in leaderboard totals")
	}
}

func TestPredictionRepository_TotalPointsByQuiniela_EmptyQuiniela_ReturnsEmptyMap(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	totals, err := predRepo.TotalPointsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 0 {
		t.Errorf("expected empty map for quiniela with no active paid members, got %v", totals)
	}
}

// ── PredictionRepository — TotalPointsByQuinielaAndPhase ─────────────────────

func TestPredictionRepository_TotalPointsByQuinielaAndPhase_MatchingPhase_ReturnsSumPerUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts1 := 5
	p1.Points = &pts1
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf("update prediction u1: %v", err)
	}

	p2 := &domain.Prediction{UserID: u2.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 1}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts2 := 2
	p2.Points = &pts2
	if err := predRepo.Update(context.Background(), p2); err != nil {
		t.Fatalf("update prediction u2: %v", err)
	}

	totals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(totals))
	}
	if totals[u1.ID] != 5 {
		t.Errorf("user1 phase total: got %d, want 5", totals[u1.ID])
	}
	if totals[u2.ID] != 2 {
		t.Errorf("user2 phase total: got %d, want 2", totals[u2.ID])
	}
}

func TestPredictionRepository_TotalPointsByQuinielaAndPhase_NonMatchingPhase_ReturnsZeroForAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := 5
	p.Points = &pts
	if err := predRepo.Update(context.Background(), p); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	totals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseFinal)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 1 {
		t.Fatalf("expected 1 member with 0 points, got %d entries", len(totals))
	}
	if totals[u.ID] != 0 {
		t.Errorf("expected 0 points for non-matching phase, got %d", totals[u.ID])
	}
}

func TestPredictionRepository_TotalPointsByQuinielaAndPhase_ExcludesUnpaidMembers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, false)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	for _, u := range []*domain.User{u1, u2} {
		p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		pts := 3
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}

	totals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if _, ok := totals[u2.ID]; ok {
		t.Error("unpaid member must not appear in phase totals")
	}
	if totals[u1.ID] != 3 {
		t.Errorf("paid member total: got %d, want 3", totals[u1.ID])
	}
}

func TestPredictionRepository_TotalPointsByQuinielaAndPhase_CrossPhaseIsolation(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	mGroup := seedMatchWithPhase(t, domain.PhaseGroupStage)
	mFinal := seedMatchWithPhase(t, domain.PhaseFinal)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	pGroup := &domain.Prediction{UserID: u.ID, MatchID: mGroup.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), pGroup); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := 5
	pGroup.Points = &pts
	if err := predRepo.Update(context.Background(), pGroup); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	pFinal := &domain.Prediction{UserID: u.ID, MatchID: mFinal.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), pFinal); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	finalPts := 2
	pFinal.Points = &finalPts
	if err := predRepo.Update(context.Background(), pFinal); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	groupTotals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if groupTotals[u.ID] != 5 {
		t.Errorf("group_stage total: got %d, want 5 (final points must not bleed across phases)", groupTotals[u.ID])
	}

	finalTotals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseFinal)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if finalTotals[u.ID] != 2 {
		t.Errorf("final total: got %d, want 2 (group_stage points must not bleed across phases)", finalTotals[u.ID])
	}
}

func TestPredictionRepository_ListByUserAndQuiniela_ActiveMember_ReturnsPredictions(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := predRepo.ListByUserAndQuiniela(context.Background(), u.ID, q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 1 {
		t.Fatalf(repoMsgExpect1Pred, len(preds))
	}
	if preds[0].UserID != u.ID {
		t.Errorf("expected user %d, got %d", u.ID, preds[0].UserID)
	}
	if preds[0].HomeScore != 2 || preds[0].AwayScore != 1 {
		t.Errorf("expected scores 2-1, got %d-%d", preds[0].HomeScore, preds[0].AwayScore)
	}
}

func TestPredictionRepository_ListByUserAndQuiniela_NonMember_ReturnsEmpty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := predRepo.ListByUserAndQuiniela(context.Background(), u.ID, q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 0 {
		t.Errorf("expected empty slice for non-member, got %d predictions", len(preds))
	}
}

func TestPredictionRepository_ListByUserAndQuiniela_InactiveMember_ReturnsEmpty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipLeft, false)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := predRepo.ListByUserAndQuiniela(context.Background(), u.ID, q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 0 {
		t.Errorf("expected empty slice for inactive member, got %d predictions", len(preds))
	}
}

// ── PredictionRepository — PredictionStatsByQuiniela ─────────────────────────

func TestPredictionRepository_PredictionStatsByQuiniela_ReturnsCountsPerUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)

	m1 := seedMatch(t)
	m2 := seedMatch(t)
	m3 := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	setPredPoints := func(userID, matchID, pts int) {
		p := &domain.Prediction{UserID: userID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}
	setPredPoints(u1.ID, m1.ID, domain.PointsExactScore)
	setPredPoints(u1.ID, m2.ID, domain.PointsCorrectOutcome)
	setPredPoints(u1.ID, m3.ID, domain.PointsIncorrectResult)
	setPredPoints(u2.ID, m1.ID, domain.PointsExactScore)
	setPredPoints(u2.ID, m2.ID, domain.PointsExactScore)

	stats, err := predRepo.PredictionStatsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected stats for 2 users, got %d", len(stats))
	}

	s1 := stats[u1.ID]
	if s1 == nil {
		t.Fatal("expected stats for u1, got nil")
	}
	if s1.CorrectCount != 2 {
		t.Errorf("u1 CorrectCount: got %d, want 2", s1.CorrectCount)
	}
	if s1.TotalCount != 3 {
		t.Errorf("u1 TotalCount: got %d, want 3", s1.TotalCount)
	}
	if s1.ExactCount != 1 {
		t.Errorf("u1 ExactCount: got %d, want 1", s1.ExactCount)
	}

	s2 := stats[u2.ID]
	if s2 == nil {
		t.Fatal("expected stats for u2, got nil")
	}
	if s2.CorrectCount != 2 {
		t.Errorf("u2 CorrectCount: got %d, want 2", s2.CorrectCount)
	}
	if s2.TotalCount != 2 {
		t.Errorf("u2 TotalCount: got %d, want 2", s2.TotalCount)
	}
	if s2.ExactCount != 2 {
		t.Errorf("u2 ExactCount: got %d, want 2", s2.ExactCount)
	}
}

func TestPredictionRepository_PredictionStatsByQuiniela_ExcludesUnpaidMembers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, false)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	for _, uid := range []int{u1.ID, u2.ID} {
		p := &domain.Prediction{UserID: uid, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		pts := domain.PointsExactScore
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}

	stats, err := predRepo.PredictionStatsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected stats for 1 user (paid only), got %d", len(stats))
	}
	if _, ok := stats[u2.ID]; ok {
		t.Error("unpaid member must not appear in stats")
	}
}

func TestPredictionRepository_PredictionStatsByQuiniela_UnscoredPredictions_ExcludedFromCounts(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	m1 := seedMatch(t)
	m2 := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u.ID, MatchID: m1.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := domain.PointsExactScore
	p1.Points = &pts
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	p2 := &domain.Prediction{UserID: u.ID, MatchID: m2.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	stats, err := predRepo.PredictionStatsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	s := stats[u.ID]
	if s == nil {
		t.Fatal("expected stats for user, got nil")
	}
	if s.TotalCount != 1 {
		t.Errorf("TotalCount: got %d, want 1 (unscored must be excluded)", s.TotalCount)
	}
	if s.ExactCount != 1 {
		t.Errorf("ExactCount: got %d, want 1", s.ExactCount)
	}
}

func TestPredictionRepository_PredictionStatsByQuiniela_NoMembers_ReturnsEmptyMap(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	stats, err := predRepo.PredictionStatsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stats) != 0 {
		t.Errorf("expected empty map for quiniela with no active paid members, got %d entries", len(stats))
	}
}

func TestPredictionRepository_PredictionStatsByQuiniela_CancelledContext_ReturnsError(t *testing.T) {
	predRepo := repository.NewPostgresPredictionRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := predRepo.PredictionStatsByQuiniela(ctx, 1)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── PredictionRepository — GetUserPredictionCounts ───────────────────────────

func TestPredictionRepository_GetUserPredictionCounts_ReturnsAggregates(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	m3 := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	createAndScore := func(matchID, pts int) {
		p := &domain.Prediction{UserID: u.ID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}
	createAndScore(m1.ID, domain.PointsExactScore)
	createAndScore(m2.ID, domain.PointsCorrectOutcome)
	createAndScore(m3.ID, domain.PointsIncorrectResult)

	p4 := &domain.Prediction{UserID: u.ID, MatchID: seedMatch(t).ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p4); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	counts, err := predRepo.GetUserPredictionCounts(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.TotalPredictions != 4 {
		t.Errorf("TotalPredictions: want 4, got %d", counts.TotalPredictions)
	}
	if counts.ScoredPredictions != 3 {
		t.Errorf("ScoredPredictions: want 3, got %d", counts.ScoredPredictions)
	}
	if counts.CorrectPredictions != 2 {
		t.Errorf("CorrectPredictions: want 2, got %d", counts.CorrectPredictions)
	}
	if counts.ExactPredictions != 1 {
		t.Errorf("ExactPredictions: want 1, got %d", counts.ExactPredictions)
	}
	if counts.TotalPoints != 7 {
		t.Errorf("TotalPoints: want 7, got %d", counts.TotalPoints)
	}
	if counts.LastPredictionAt == nil {
		t.Error("LastPredictionAt: want non-nil")
	}
}

func TestPredictionRepository_GetUserPredictionCounts_NoPredictions_ReturnsZeroes(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	counts, err := predRepo.GetUserPredictionCounts(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.TotalPredictions != 0 || counts.ScoredPredictions != 0 ||
		counts.CorrectPredictions != 0 || counts.ExactPredictions != 0 || counts.TotalPoints != 0 {
		t.Errorf("want all zeros for user with no predictions, got %+v", counts)
	}
	if counts.LastPredictionAt != nil {
		t.Error("LastPredictionAt: want nil for user with no predictions")
	}
}

func TestPredictionRepository_GetUserPredictionCounts_IsolatedPerUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := domain.PointsExactScore
	p1.Points = &pts
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	counts, err := predRepo.GetUserPredictionCounts(context.Background(), u2.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.TotalPredictions != 0 {
		t.Errorf("u2 should have 0 predictions, got %d", counts.TotalPredictions)
	}
}

func TestPredictionRepository_GetUserPredictionCounts_CancelledContext_ReturnsError(t *testing.T) {
	predRepo := repository.NewPostgresPredictionRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := predRepo.GetUserPredictionCounts(ctx, 1)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── PredictionRepository — GetUserPointsByPhase ───────────────────────────────

func TestPredictionRepository_GetUserPointsByPhase_ReturnsPerPhasePoints(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	mGroup := seedMatchWithPhase(t, domain.PhaseGroupStage)
	mFinal := seedMatchWithPhase(t, domain.PhaseFinal)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	createAndScore := func(matchID, pts int) {
		p := &domain.Prediction{UserID: u.ID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}
	createAndScore(mGroup.ID, domain.PointsExactScore)
	createAndScore(mFinal.ID, domain.PointsCorrectOutcome)

	byPhase, err := predRepo.GetUserPointsByPhase(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if byPhase[domain.PhaseGroupStage] != domain.PointsExactScore {
		t.Errorf("group stage: want %d, got %d", domain.PointsExactScore, byPhase[domain.PhaseGroupStage])
	}
	if byPhase[domain.PhaseFinal] != domain.PointsCorrectOutcome {
		t.Errorf("final: want %d, got %d", domain.PointsCorrectOutcome, byPhase[domain.PhaseFinal])
	}
}

func TestPredictionRepository_GetUserPointsByPhase_UnscoredExcluded(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	byPhase, err := predRepo.GetUserPointsByPhase(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(byPhase) != 0 {
		t.Errorf("want empty map for unscored predictions, got %v", byPhase)
	}
}

func TestPredictionRepository_GetUserPointsByPhase_NoPredictions_ReturnsEmptyMap(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	byPhase, err := predRepo.GetUserPointsByPhase(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(byPhase) != 0 {
		t.Errorf("want empty map, got %v", byPhase)
	}
}

func TestPredictionRepository_GetUserPointsByPhase_CancelledContext_ReturnsError(t *testing.T) {
	predRepo := repository.NewPostgresPredictionRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := predRepo.GetUserPointsByPhase(ctx, 1)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── PredictionRepository — ListUserScoredPointsChronological ─────────────────

func TestPredictionRepository_ListUserScoredPointsChronological_ReturnsAllScoredPoints(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	m1 := seedMatch(t)
	m2 := seedMatch(t)
	m3 := seedMatch(t)

	createAndScore := func(matchID, pts int) {
		p := &domain.Prediction{UserID: u.ID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}
	createAndScore(m1.ID, domain.PointsExactScore)
	createAndScore(m2.ID, domain.PointsCorrectOutcome)
	createAndScore(m3.ID, domain.PointsIncorrectResult)

	pts, err := predRepo.ListUserScoredPointsChronological(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pts) != 3 {
		t.Fatalf("want 3 scored points, got %d", len(pts))
	}
	sum := 0
	for _, p := range pts {
		sum += p
	}
	wantSum := domain.PointsExactScore + domain.PointsCorrectOutcome + domain.PointsIncorrectResult
	if sum != wantSum {
		t.Errorf("total points sum: want %d, got %d", wantSum, sum)
	}
}

func TestPredictionRepository_ListUserScoredPointsChronological_ExcludesUnscored(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u.ID, MatchID: m1.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	scored := domain.PointsExactScore
	p1.Points = &scored
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	p2 := &domain.Prediction{UserID: u.ID, MatchID: m2.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	pts, err := predRepo.ListUserScoredPointsChronological(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pts) != 1 {
		t.Fatalf("want 1 scored point entry, got %d", len(pts))
	}
	if pts[0] != domain.PointsExactScore {
		t.Errorf("pts[0]: want %d, got %d", domain.PointsExactScore, pts[0])
	}
}

func TestPredictionRepository_ListUserScoredPointsChronological_NoPredictions_ReturnsNil(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	pts, err := predRepo.ListUserScoredPointsChronological(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pts) != 0 {
		t.Errorf("want empty slice, got %v", pts)
	}
}

func TestPredictionRepository_ListUserScoredPointsChronological_CancelledContext_ReturnsError(t *testing.T) {
	predRepo := repository.NewPostgresPredictionRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := predRepo.ListUserScoredPointsChronological(ctx, 1)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── PredictionRepository — admin extensions ───────────────────────────────────

func TestPredictionRepository_ListAdmin_NoFilter_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAdmin(context.Background(), repository.PredictionAdminFilters{}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf(repoMsgExpect1Pred, len(results))
	}
}

func TestPredictionRepository_ListAdmin_FilterByUserID(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u1.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u2.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAdmin(context.Background(), repository.PredictionAdminFilters{UserID: &u1.ID}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].UserID != u1.ID {
		t.Errorf("expected 1 prediction for user %d, got %d", u1.ID, len(results))
	}
}

func TestPredictionRepository_ListAdmin_PaginationLimit(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u.ID, MatchID: m1.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u.ID, MatchID: m2.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAdmin(context.Background(), repository.PredictionAdminFilters{}, repository.Pagination{Limit: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 prediction with limit=1, got %d", len(results))
	}
}

func TestPredictionRepository_GlobalLeaderboard_RanksUsers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 1}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := predRepo.UpdateManyPoints(context.Background(), map[int]int{p1.ID: 10}); err != nil {
		t.Fatalf("update points: %v", err)
	}

	p2 := &domain.Prediction{UserID: u2.ID, MatchID: m.ID}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	entries, err := predRepo.GlobalLeaderboard(context.Background(), 10)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	if entries[0].UserID != u1.ID {
		t.Errorf("expected u1 first (most points), got userID=%d", entries[0].UserID)
	}
}

func TestPredictionRepository_GlobalLeaderboard_LimitRespected(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u1.ID, MatchID: m1.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u2.ID, MatchID: m2.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	entries, err := repo.GlobalLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry with limit=1, got %d", len(entries))
	}
}

// ── PredictionRepository — ListQuinielaIDsByMatch ────────────────────────────

func TestPredictionRepository_ListQuinielaIDsByMatch_ReturnsAffectedQuinielas(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q1 := seedQuiniela(t, u1.ID)
	q2 := seedQuiniela(t, u2.ID)

	seedMembership(t, q1.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q2.ID, u2.ID, domain.MembershipActive, true)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	if err := predRepo.Create(context.Background(), &domain.Prediction{UserID: u1.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := predRepo.Create(context.Background(), &domain.Prediction{UserID: u2.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	ids, err := predRepo.ListQuinielaIDsByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 quiniela IDs, got %d: %v", len(ids), ids)
	}
}

func TestPredictionRepository_ListQuinielaIDsByMatch_ExcludesUnpaidMembers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, false)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	if err := predRepo.Create(context.Background(), &domain.Prediction{UserID: u1.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := predRepo.Create(context.Background(), &domain.Prediction{UserID: u2.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	ids, err := predRepo.ListQuinielaIDsByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 quiniela ID (paid member only), got %d: %v", len(ids), ids)
	}
}

func TestPredictionRepository_ListQuinielaIDsByMatch_NoPredictions_ReturnsEmpty(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	ids, err := predRepo.ListQuinielaIDsByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty slice, got %v", ids)
	}
}
