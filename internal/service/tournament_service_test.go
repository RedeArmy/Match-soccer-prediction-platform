package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	tournamentUnexpectedErr = "unexpected error: %v"
	tournamentValidationFmt = "expected validation error, got %v"
	tournamentMexico        = "Mexico"
	tournamentWinnerGroupA  = "winner_group_a"
	tournamentFrance        = "France"
	tournamentSpain         = "Spain"
	tournamentItaly         = "Italy"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubMatchRepoTournament struct {
	matches []*domain.Match
	err     error
}

func (r *stubMatchRepoTournament) Create(_ context.Context, _ *domain.Match) error { return r.err }
func (r *stubMatchRepoTournament) GetByID(_ context.Context, _ int) (*domain.Match, error) {
	return nil, r.err
}
func (r *stubMatchRepoTournament) Update(_ context.Context, _ *domain.Match) error { return r.err }
func (r *stubMatchRepoTournament) List(_ context.Context) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepoTournament) ListByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepoTournament) ListByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepoTournament) LinkExternal(_ context.Context, _ int, _ string, _ int64) error {
	return r.err
}
func (r *stubMatchRepoTournament) UnlinkExternal(_ context.Context, _ int) error { return r.err }
func (r *stubMatchRepoTournament) ListSyncCandidates(_ context.Context, _ int) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepoTournament) UpdateSyncState(_ context.Context, _ int) error { return r.err }
func (r *stubMatchRepoTournament) FindByTeams(_ context.Context, _, _ string) (*domain.Match, error) {
	return nil, nil
}
func (r *stubMatchRepoTournament) UpdateKickoff(_ context.Context, _ int, _ time.Time) error {
	return nil
}
func (r *stubMatchRepoTournament) UpdateSlots(_ context.Context, _ int, _, _ *int) (*domain.Match, error) {
	return nil, r.err
}
func (r *stubMatchRepoTournament) UpdateLiveProgress(_ context.Context, _ int, _ *string, _, _ *int) error {
	return nil
}
func (r *stubMatchRepoTournament) ListFinishedPenaltyMatchesMissingWinner(_ context.Context) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubMatchRepoTournament) ListByGroupLabel(_ context.Context, _ string) ([]*domain.Match, error) {
	return r.matches, r.err
}

type stubTournamentRepo struct {
	slot  *domain.TournamentSlot
	slots []*domain.TournamentSlot
	err   error
}

func (r *stubTournamentRepo) CreateSlot(_ context.Context, label, _ string) (*domain.TournamentSlot, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &domain.TournamentSlot{ID: 1, Label: label}, nil
}
func (r *stubTournamentRepo) GetSlot(_ context.Context, _ int) (*domain.TournamentSlot, error) {
	return r.slot, r.err
}
func (r *stubTournamentRepo) ListSlots(_ context.Context) ([]*domain.TournamentSlot, error) {
	return r.slots, r.err
}
func (r *stubTournamentRepo) ConfirmSlot(_ context.Context, _, _ int, team string) (*domain.TournamentSlot, error) {
	if r.err != nil {
		return nil, r.err
	}
	s := &domain.TournamentSlot{ID: 1, Label: tournamentWinnerGroupA, Team: &team}
	return s, nil
}
func (r *stubTournamentRepo) FindSlotByAutoSource(_ context.Context, _ string) (*domain.TournamentSlot, error) {
	return r.slot, r.err
}
func (r *stubTournamentRepo) AutoConfirmSlot(_ context.Context, _ int, team string) (*domain.TournamentSlot, error) {
	if r.err != nil {
		return nil, r.err
	}
	s := &domain.TournamentSlot{ID: 1, Label: tournamentWinnerGroupA, Team: &team}
	return s, nil
}

type stubTeamRepo struct {
	names []string
	err   error
}

func (r *stubTeamRepo) ListTeamNames(_ context.Context) ([]string, error) {
	return r.names, r.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func groupLabel(s string) *string { return &s }

func finishedMatch(group, home, away string, hs, as int) *domain.Match {
	return &domain.Match{
		HomeTeam:   home,
		AwayTeam:   away,
		HomeScore:  &hs,
		AwayScore:  &as,
		Phase:      domain.PhaseGroupStage,
		GroupLabel: groupLabel(group),
		Status:     domain.MatchStatusFinished,
		KickoffAt:  time.Now(),
	}
}

func scheduledMatch(group, home, away string) *domain.Match {
	return &domain.Match{
		HomeTeam:   home,
		AwayTeam:   away,
		Phase:      domain.PhaseGroupStage,
		GroupLabel: groupLabel(group),
		Status:     domain.MatchStatusScheduled,
		KickoffAt:  time.Now().Add(24 * time.Hour),
	}
}

func newTournamentSvc(matches []*domain.Match, tbRepo *stubTournamentRepo) TournamentService {
	return NewTournamentService(
		&stubMatchRepoTournament{matches: matches},
		tbRepo,
		&stubTeamRepo{},
		&noopSystemParamService{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
}

// ── GetAllStandings ───────────────────────────────────────────────────────────

func TestTournamentService_GetAllStandings_EmptyWhenNoMatches(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})

	standings, err := svc.GetAllStandings(context.Background())
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	if len(standings) != 0 {
		t.Errorf("expected empty map, got %d groups", len(standings))
	}
}

func TestTournamentService_GetAllStandings_AccumulatesPoints(t *testing.T) {
	// Mexico beats USA 2-1; Canada draws USA 1-1 (not yet played).
	matches := []*domain.Match{
		finishedMatch("A", tournamentMexico, "USA", 2, 1),
		scheduledMatch("A", "Canada", "USA"),
	}
	svc := newTournamentSvc(matches, &stubTournamentRepo{})

	standings, err := svc.GetAllStandings(context.Background())
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	entries := standings["A"]
	if len(entries) != 3 {
		t.Fatalf("expected 3 teams in group A, got %d", len(entries))
	}
	// Mexico should be first: 3 pts, +1 GD.
	if entries[0].Team != tournamentMexico {
		t.Errorf("expected Mexico first, got %s", entries[0].Team)
	}
	if entries[0].Points != 3 {
		t.Errorf("Mexico points: want 3, got %d", entries[0].Points)
	}
	if entries[0].Won != 1 || entries[0].Lost != 0 {
		t.Errorf("Mexico W/L: want 1/0, got %d/%d", entries[0].Won, entries[0].Lost)
	}
}

func TestTournamentService_GetAllStandings_DrawDistributesPoints(t *testing.T) {
	matches := []*domain.Match{finishedMatch("B", "Brazil", "Argentina", 1, 1)}
	svc := newTournamentSvc(matches, &stubTournamentRepo{})

	standings, err := svc.GetAllStandings(context.Background())
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	for _, e := range standings["B"] {
		if e.Points != 1 {
			t.Errorf("%s: want 1 point from draw, got %d", e.Team, e.Points)
		}
		if e.Drawn != 1 {
			t.Errorf("%s: want drawn=1, got %d", e.Team, e.Drawn)
		}
	}
}

func TestTournamentService_GetAllStandings_SkipsMatchesWithoutGroupLabel(t *testing.T) {
	ko := &domain.Match{
		HomeTeam:  tournamentFrance,
		AwayTeam:  "Germany",
		HomeScore: intPtr(1),
		AwayScore: intPtr(0),
		Phase:     domain.PhaseRoundOf16,
		Status:    domain.MatchStatusFinished,
	}
	svc := newTournamentSvc([]*domain.Match{ko}, &stubTournamentRepo{})

	standings, err := svc.GetAllStandings(context.Background())
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	if len(standings) != 0 {
		t.Errorf("expected no groups for knockout-only matches, got %d", len(standings))
	}
}

func TestTournamentService_GetAllStandings_RepoError_Propagates(t *testing.T) {
	svc := NewTournamentService(
		&stubMatchRepoTournament{err: errors.New("db error")},
		&stubTournamentRepo{},
		&stubTeamRepo{},
		&noopSystemParamService{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	_, err := svc.GetAllStandings(context.Background())
	if err == nil {
		t.Fatal("expected error from match repo, got nil")
	}
}

// ── GetGroupStanding ──────────────────────────────────────────────────────────

func TestTournamentService_GetGroupStanding_ReturnsSpecificGroup(t *testing.T) {
	matches := []*domain.Match{
		finishedMatch("A", tournamentMexico, "USA", 1, 0),
		finishedMatch("B", "Brazil", "Germany", 2, 0),
	}
	svc := newTournamentSvc(matches, &stubTournamentRepo{})

	entries, err := svc.GetGroupStanding(context.Background(), "B")
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(entries))
	}
	if entries[0].Group != "B" {
		t.Errorf("expected group B, got %s", entries[0].Group)
	}
}

func TestTournamentService_GetGroupStanding_EmptyGroup_ReturnsValidation(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})
	_, err := svc.GetGroupStanding(context.Background(), "")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tournamentValidationFmt, err)
	}
}

func TestTournamentService_GetGroupStanding_UnknownGroup_ReturnsNotFound(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})
	_, err := svc.GetGroupStanding(context.Background(), "Z")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ── CreateSlot ────────────────────────────────────────────────────────────────

func TestTournamentService_CreateSlot_ReturnsSlot(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})

	slot, err := svc.CreateSlot(context.Background(), tournamentWinnerGroupA, "")
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	if slot.Label != tournamentWinnerGroupA {
		t.Errorf("label: want winner_group_a, got %s", slot.Label)
	}
}

func TestTournamentService_CreateSlot_EmptyLabel_ReturnsValidation(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})
	_, err := svc.CreateSlot(context.Background(), "", "")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tournamentValidationFmt, err)
	}
}

// ── ConfirmSlot ───────────────────────────────────────────────────────────────

func TestTournamentService_ConfirmSlot_SetsTeam(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})

	slot, err := svc.ConfirmSlot(context.Background(), 1, 7, tournamentMexico)
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	if slot.Team == nil || *slot.Team != tournamentMexico {
		t.Errorf("team: want Mexico, got %v", slot.Team)
	}
}

func TestTournamentService_ConfirmSlot_EmptyTeam_ReturnsValidation(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})
	_, err := svc.ConfirmSlot(context.Background(), 1, 7, "")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tournamentValidationFmt, err)
	}
}

func TestTournamentService_ConfirmSlot_RepoError_Propagates(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{err: apperrors.NotFound("slot not found")})
	_, err := svc.ConfirmSlot(context.Background(), 99, 7, tournamentMexico)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ── ListSlots ─────────────────────────────────────────────────────────────────

func TestTournamentService_ListSlots_ReturnsList(t *testing.T) {
	team := tournamentMexico
	slots := []*domain.TournamentSlot{
		{ID: 1, Label: tournamentWinnerGroupA, Team: &team},
		{ID: 2, Label: "runner_up_group_a"},
	}
	svc := newTournamentSvc(nil, &stubTournamentRepo{slots: slots})

	got, err := svc.ListSlots(context.Background())
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 slots, got %d", len(got))
	}
}

// ── buildStandings unit tests ─────────────────────────────────────────────────

func TestBuildStandings_SortOrder_PointsThenGDThenGF(t *testing.T) {
	// Group C: France 6pts (+3 GD), Spain 3pts (+1 GD), Italy 0pts
	matches := []*domain.Match{
		finishedMatch("C", tournamentFrance, tournamentItaly, 2, 0),
		finishedMatch("C", tournamentFrance, tournamentSpain, 1, 0),
		finishedMatch("C", tournamentSpain, tournamentItaly, 1, 0),
	}
	result := buildStandings(matches, domain.StandingsWinPoints)
	entries := result["C"]
	if len(entries) != 3 {
		t.Fatalf("expected 3 teams, got %d", len(entries))
	}
	if entries[0].Team != tournamentFrance || entries[1].Team != tournamentSpain || entries[2].Team != tournamentItaly {
		t.Errorf("order: want France/Spain/Italy, got %s/%s/%s",
			entries[0].Team, entries[1].Team, entries[2].Team)
	}
}

func TestBuildStandings_GoalDifferenceCalculated(t *testing.T) {
	matches := []*domain.Match{finishedMatch("D", "Portugal", "Morocco", 3, 1)}
	result := buildStandings(matches, domain.StandingsWinPoints)

	for _, e := range result["D"] {
		if e.Team == "Portugal" && e.GD != 2 {
			t.Errorf("Portugal GD: want 2, got %d", e.GD)
		}
		if e.Team == "Morocco" && e.GD != -2 {
			t.Errorf("Morocco GD: want -2, got %d", e.GD)
		}
	}
}

func intPtr(v int) *int { return &v }

// ── slotWinnerLoser ───────────────────────────────────────────────────────────

func TestSlotWinnerLoser_HomeWins_ReturnsHomeAsWinner(t *testing.T) {
	hs, as := 3, 1
	m := &domain.Match{HomeTeam: tournamentMexico, AwayTeam: "USA", HomeScore: &hs, AwayScore: &as}
	w, l := slotWinnerLoser(m)
	if w != tournamentMexico || l != "USA" {
		t.Errorf("got winner=%q loser=%q; want Mexico/USA", w, l)
	}
}

func TestSlotWinnerLoser_AwayWins_ReturnsAwayAsWinner(t *testing.T) {
	hs, as := 0, 2
	m := &domain.Match{HomeTeam: tournamentMexico, AwayTeam: "USA", HomeScore: &hs, AwayScore: &as}
	w, l := slotWinnerLoser(m)
	if w != "USA" || l != tournamentMexico {
		t.Errorf("got winner=%q loser=%q; want USA/Mexico", w, l)
	}
}

func TestSlotWinnerLoser_Draw_ReturnsBothEmpty(t *testing.T) {
	hs, as := 1, 1
	m := &domain.Match{HomeTeam: tournamentMexico, AwayTeam: "USA", HomeScore: &hs, AwayScore: &as}
	w, l := slotWinnerLoser(m)
	if w != "" || l != "" {
		t.Errorf("got winner=%q loser=%q; want empty/empty for draw", w, l)
	}
}

func TestSlotWinnerLoser_Penalties_HomeWinner(t *testing.T) {
	hs, as := 0, 0
	wm := domain.WinMethodPenalties
	pw := "home"
	m := &domain.Match{
		HomeTeam: tournamentMexico, AwayTeam: "USA",
		HomeScore: &hs, AwayScore: &as,
		WinMethod: &wm, PenaltyWinner: &pw,
	}
	w, l := slotWinnerLoser(m)
	if w != tournamentMexico || l != "USA" {
		t.Errorf("got winner=%q loser=%q; want Mexico/USA for home penalty winner", w, l)
	}
}

func TestSlotWinnerLoser_Penalties_AwayWinner(t *testing.T) {
	hs, as := 1, 1
	wm := domain.WinMethodPenalties
	pw := "away"
	m := &domain.Match{
		HomeTeam: tournamentMexico, AwayTeam: "USA",
		HomeScore: &hs, AwayScore: &as,
		WinMethod: &wm, PenaltyWinner: &pw,
	}
	w, l := slotWinnerLoser(m)
	if w != "USA" || l != tournamentMexico {
		t.Errorf("got winner=%q loser=%q; want USA/Mexico for away penalty winner", w, l)
	}
}

func TestSlotWinnerLoser_Penalties_NilPenaltyWinner_ReturnsBothEmpty(t *testing.T) {
	hs, as := 0, 0
	wm := domain.WinMethodPenalties
	m := &domain.Match{
		HomeTeam: tournamentMexico, AwayTeam: "USA",
		HomeScore: &hs, AwayScore: &as,
		WinMethod: &wm, PenaltyWinner: nil,
	}
	w, l := slotWinnerLoser(m)
	if w != "" || l != "" {
		t.Errorf("got winner=%q loser=%q; want empty/empty when PenaltyWinner is nil", w, l)
	}
}

func TestSlotWinnerLoser_NilScores_ReturnsBothEmpty(t *testing.T) {
	m := &domain.Match{HomeTeam: tournamentMexico, AwayTeam: "USA"}
	w, l := slotWinnerLoser(m)
	if w != "" || l != "" {
		t.Errorf("got winner=%q loser=%q; want empty/empty for nil scores", w, l)
	}
}

// ── AutoConfirmGroupSlots ─────────────────────────────────────────────────────

func TestAutoConfirmGroupSlots_AllMatchesFinished_ConfirmsSlots(t *testing.T) {
	matches := []*domain.Match{
		finishedMatch("A", tournamentMexico, "USA", 2, 1),
		finishedMatch("A", tournamentFrance, "Germany", 1, 0),
		finishedMatch("A", tournamentMexico, tournamentFrance, 0, 0),
		finishedMatch("A", "USA", "Germany", 1, 1),
	}
	repo := &stubTournamentRepo{slot: &domain.TournamentSlot{ID: 1, Team: nil}}
	svc := newTournamentSvc(matches, repo)
	if err := svc.AutoConfirmGroupSlots(context.Background(), "A"); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestAutoConfirmGroupSlots_GroupNotComplete_Noop(t *testing.T) {
	matches := []*domain.Match{
		finishedMatch("A", tournamentMexico, "USA", 2, 1),
		scheduledMatch("A", tournamentFrance, "Germany"),
	}
	repo := &stubTournamentRepo{slot: &domain.TournamentSlot{ID: 1, Team: nil}}
	svc := newTournamentSvc(matches, repo)
	if err := svc.AutoConfirmGroupSlots(context.Background(), "A"); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestAutoConfirmGroupSlots_SlotAlreadyConfirmed_Noop(t *testing.T) {
	team := tournamentMexico
	matches := []*domain.Match{
		finishedMatch("A", tournamentMexico, "USA", 2, 1),
		finishedMatch("A", tournamentFrance, "Germany", 1, 0),
	}
	repo := &stubTournamentRepo{slot: &domain.TournamentSlot{ID: 1, Team: &team}}
	svc := newTournamentSvc(matches, repo)
	if err := svc.AutoConfirmGroupSlots(context.Background(), "A"); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestAutoConfirmGroupSlots_NoSlotRegistered_Noop(t *testing.T) {
	matches := []*domain.Match{
		finishedMatch("A", tournamentMexico, "USA", 2, 1),
		finishedMatch("A", tournamentFrance, "Germany", 1, 0),
	}
	repo := &stubTournamentRepo{slot: nil} // no slot for this bracket code
	svc := newTournamentSvc(matches, repo)
	if err := svc.AutoConfirmGroupSlots(context.Background(), "A"); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestAutoConfirmGroupSlots_RepoError_Propagates(t *testing.T) {
	dbErr := errors.New("db error")
	svc := NewTournamentService(
		&stubMatchRepoTournament{err: dbErr},
		&stubTournamentRepo{},
		&stubTeamRepo{},
		&noopSystemParamService{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	if err := svc.AutoConfirmGroupSlots(context.Background(), "A"); err == nil {
		t.Fatal("expected error from ListByGroupLabel, got nil")
	}
}

// ── AutoConfirmMatchResultSlots ───────────────────────────────────────────────

func TestAutoConfirmMatchResultSlots_EmptyMatchCode_Noop(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})
	if err := svc.AutoConfirmMatchResultSlots(context.Background(), "", tournamentMexico, "USA"); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestAutoConfirmMatchResultSlots_WithMatchCode_ConfirmsSlots(t *testing.T) {
	repo := &stubTournamentRepo{slot: &domain.TournamentSlot{ID: 1, Team: nil}}
	svc := newTournamentSvc(nil, repo)
	if err := svc.AutoConfirmMatchResultSlots(context.Background(), "M73", tournamentMexico, "USA"); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestAutoConfirmMatchResultSlots_EmptyWinnerAndLoser_Noop(t *testing.T) {
	repo := &stubTournamentRepo{slot: &domain.TournamentSlot{ID: 1, Team: nil}}
	svc := newTournamentSvc(nil, repo)
	if err := svc.AutoConfirmMatchResultSlots(context.Background(), "M73", "", ""); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestAutoConfirmMatchResultSlots_NoSlotForCode_Noop(t *testing.T) {
	repo := &stubTournamentRepo{slot: nil}
	svc := newTournamentSvc(nil, repo)
	if err := svc.AutoConfirmMatchResultSlots(context.Background(), "M73", tournamentMexico, "USA"); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestAutoConfirmMatchResultSlots_SlotAlreadyConfirmed_Noop(t *testing.T) {
	team := tournamentMexico
	repo := &stubTournamentRepo{slot: &domain.TournamentSlot{ID: 1, Team: &team}}
	svc := newTournamentSvc(nil, repo)
	if err := svc.AutoConfirmMatchResultSlots(context.Background(), "M73", tournamentMexico, "USA"); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

// ── BackfillSlots ─────────────────────────────────────────────────────────────

func TestBackfillSlots_FinishedKnockoutMatch_ProcessesIt(t *testing.T) {
	mc := "M73"
	hs, as := 2, 1
	ko := &domain.Match{
		HomeTeam:  tournamentMexico,
		AwayTeam:  "USA",
		HomeScore: &hs,
		AwayScore: &as,
		Status:    domain.MatchStatusFinished,
		MatchCode: &mc,
		Phase:     domain.PhaseRoundOf32,
	}
	repo := &stubTournamentRepo{slot: &domain.TournamentSlot{ID: 1, Team: nil}}
	svc := newTournamentSvc([]*domain.Match{ko}, repo)
	if err := svc.BackfillSlots(context.Background()); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

func TestBackfillSlots_DBError_ReturnsNil(t *testing.T) {
	svc := NewTournamentService(
		&stubMatchRepoTournament{err: errors.New("db down")},
		&stubTournamentRepo{slot: nil},
		&stubTeamRepo{},
		&noopSystemParamService{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	if err := svc.BackfillSlots(context.Background()); err != nil {
		t.Fatalf("BackfillSlots must return nil even on DB errors, got: %v", err)
	}
}

func TestBackfillSlots_SkipsUnfinishedKnockoutMatches(t *testing.T) {
	mc := "M74"
	scheduled := &domain.Match{
		HomeTeam:  tournamentMexico,
		AwayTeam:  "USA",
		Status:    domain.MatchStatusScheduled,
		MatchCode: &mc,
		Phase:     domain.PhaseRoundOf32,
	}
	repo := &stubTournamentRepo{slot: &domain.TournamentSlot{ID: 1, Team: nil}}
	svc := newTournamentSvc([]*domain.Match{scheduled}, repo)
	if err := svc.BackfillSlots(context.Background()); err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
}

// ── ListTeamNames ─────────────────────────────────────────────────────────────

func TestTournamentService_ListTeamNames_ReturnsSortedNames(t *testing.T) {
	expected := []string{"Argentina", "Brazil", "Mexico"}
	svc := NewTournamentService(
		&stubMatchRepoTournament{},
		&stubTournamentRepo{},
		&stubTeamRepo{names: expected},
		&noopSystemParamService{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	names, err := svc.ListTeamNames(context.Background())
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("name[%d]: got %q, want %q", i, n, expected[i])
		}
	}
}

func TestTournamentService_ListTeamNames_PropagatesRepoError(t *testing.T) {
	svc := NewTournamentService(
		&stubMatchRepoTournament{},
		&stubTournamentRepo{},
		&stubTeamRepo{err: errors.New("db down")},
		&noopSystemParamService{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	if _, err := svc.ListTeamNames(context.Background()); err == nil {
		t.Fatal("expected error from team repo, got nil")
	}
}

// ── parseBestThirdEligibleGroups ──────────────────────────────────────────────

func TestParseBestThirdEligibleGroups_ValidDescription_ReturnsGroups(t *testing.T) {
	got := parseBestThirdEligibleGroups("Mejor 3.° (A/B/C/D/F)")
	want := map[string]bool{"A": true, "B": true, "C": true, "D": true, "F": true}
	if len(got) != len(want) {
		t.Fatalf("expected %d groups, got %d: %v", len(want), len(got), got)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("expected group %s to be eligible", k)
		}
	}
}

func TestParseBestThirdEligibleGroups_MissingParentheses_ReturnsNil(t *testing.T) {
	if got := parseBestThirdEligibleGroups("Mejor 3.° without parens"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestParseBestThirdEligibleGroups_EmptyString_ReturnsNil(t *testing.T) {
	if got := parseBestThirdEligibleGroups(""); got != nil {
		t.Errorf("expected nil for empty string, got %v", got)
	}
}

// ── matchBestThirdsToSlots ────────────────────────────────────────────────────

func TestMatchBestThirdsToSlots_PerfectBipartiteMatch(t *testing.T) {
	// Slot 1 accepts A or C; slot 2 accepts A or B.
	// Teams: A, B. Augmenting path: slot 2 initially wants A (taken by slot 1),
	// then slot 1 re-routes to C — but C is not in teams, so slot 1 keeps A
	// and slot 2 instead takes B.
	slots := []thirdSlotEntry{
		{id: 1, eligibleGroups: map[string]bool{"A": true, "C": true}},
		{id: 2, eligibleGroups: map[string]bool{"A": true, "B": true}},
	}
	teams := []*domain.GroupStanding{
		{Group: "A", Team: "TeamA"},
		{Group: "B", Team: "TeamB"},
	}
	got, ok := matchBestThirdsToSlots(slots, teams)
	if !ok {
		t.Fatal("expected matching to succeed")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(got))
	}
	if got[1] != "TeamA" {
		t.Errorf("slot 1: want TeamA, got %s", got[1])
	}
	if got[2] != "TeamB" {
		t.Errorf("slot 2: want TeamB, got %s", got[2])
	}
}

func TestMatchBestThirdsToSlots_NoEligibleTeam_ReturnsFalse(t *testing.T) {
	slots := []thirdSlotEntry{
		{id: 1, eligibleGroups: map[string]bool{"X": true}},
	}
	teams := []*domain.GroupStanding{{Group: "A", Team: "TeamA"}}
	_, ok := matchBestThirdsToSlots(slots, teams)
	if ok {
		t.Fatal("expected matching to fail when no eligible team exists")
	}
}

func TestMatchBestThirdsToSlots_EmptyInputs_ReturnsEmptyMapOK(t *testing.T) {
	got, ok := matchBestThirdsToSlots(nil, nil)
	if !ok {
		t.Fatal("expected ok=true for empty inputs")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// ── AutoConfirmBestThirdSlots helpers ─────────────────────────────────────────

// buildCompleteGroupStage creates finished group-stage matches for all 12 groups.
// Groups A–H: 4-team round robin where T3 beats T4 → T3 gets 3 pts as 3rd place.
// Groups I–L: 3 teams where T3 loses all matches → T3 gets 0 pts as 3rd place.
// This guarantees the top-8 thirds are always from groups A–H.
func buildCompleteGroupStage() []*domain.Match {
	var matches []*domain.Match
	for _, grp := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		t1, t2, t3, t4 := grp+"1", grp+"2", grp+"3", grp+"4"
		matches = append(matches,
			finishedMatch(grp, t1, t2, 1, 0),
			finishedMatch(grp, t1, t3, 1, 0),
			finishedMatch(grp, t1, t4, 1, 0),
			finishedMatch(grp, t2, t3, 1, 0),
			finishedMatch(grp, t2, t4, 1, 0),
			finishedMatch(grp, t3, t4, 1, 0), // T3 beats T4: 3 pts
		)
	}
	for _, grp := range []string{"I", "J", "K", "L"} {
		t1, t2, t3 := grp+"1", grp+"2", grp+"3"
		matches = append(matches,
			finishedMatch(grp, t1, t2, 1, 0),
			finishedMatch(grp, t1, t3, 1, 0),
			finishedMatch(grp, t2, t3, 1, 0), // T3 loses all: 0 pts
		)
	}
	return matches
}

// bestThirdSlots returns the 8 r32 "Mejor 3.°" slots for the WC2026 bracket.
func bestThirdSlots() []*domain.TournamentSlot {
	return []*domain.TournamentSlot{
		{ID: 1, Label: "r32_02_b", Description: "Mejor 3.° (A/B/C/D/F)"},
		{ID: 2, Label: "r32_05_b", Description: "Mejor 3.° (C/D/F/G/H)"},
		{ID: 3, Label: "r32_07_b", Description: "Mejor 3.° (C/E/F/H/I)"},
		{ID: 4, Label: "r32_08_b", Description: "Mejor 3.° (E/H/I/J/K)"},
		{ID: 5, Label: "r32_09_b", Description: "Mejor 3.° (B/E/F/I/J)"},
		{ID: 6, Label: "r32_10_b", Description: "Mejor 3.° (A/E/H/I/J)"},
		{ID: 7, Label: "r32_13_b", Description: "Mejor 3.° (E/F/G/I/J)"},
		{ID: 8, Label: "r32_15_b", Description: "Mejor 3.° (D/E/I/J/L)"},
	}
}

// ── AutoConfirmBestThirdSlots ─────────────────────────────────────────────────

func TestAutoConfirmBestThirdSlots_GroupStageNotComplete_ReturnsValidation(t *testing.T) {
	matches := []*domain.Match{
		finishedMatch("A", "T1", "T2", 1, 0),
		scheduledMatch("A", "T3", "T4"),
	}
	svc := newTournamentSvc(matches, &stubTournamentRepo{})
	_, err := svc.AutoConfirmBestThirdSlots(context.Background(), 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestAutoConfirmBestThirdSlots_MatchRepoError_Propagates(t *testing.T) {
	svc := NewTournamentService(
		&stubMatchRepoTournament{err: errors.New("db down")},
		&stubTournamentRepo{},
		&stubTeamRepo{},
		&noopSystemParamService{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	_, err := svc.AutoConfirmBestThirdSlots(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from match repo, got nil")
	}
}

func TestAutoConfirmBestThirdSlots_SlotRepoError_Propagates(t *testing.T) {
	matches := buildCompleteGroupStage()
	repo := &stubTournamentRepo{err: errors.New("slot db down")}
	svc := newTournamentSvc(matches, repo)
	_, err := svc.AutoConfirmBestThirdSlots(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from slot repo, got nil")
	}
}

func TestAutoConfirmBestThirdSlots_HappyPath_Returns8Assignments(t *testing.T) {
	matches := buildCompleteGroupStage()
	repo := &stubTournamentRepo{slots: bestThirdSlots()}
	svc := newTournamentSvc(matches, repo)

	results, err := svc.AutoConfirmBestThirdSlots(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 8 {
		t.Fatalf("expected 8 assignments, got %d", len(results))
	}
	advancingGroups := map[string]bool{
		"A": true, "B": true, "C": true, "D": true,
		"E": true, "F": true, "G": true, "H": true,
	}
	for _, a := range results {
		if !advancingGroups[a.Group] {
			t.Errorf("slot %s: group %q should not advance", a.SlotLabel, a.Group)
		}
	}
}

func TestAutoConfirmBestThirdSlots_AlreadyConfirmedSlot_StillInResults(t *testing.T) {
	matches := buildCompleteGroupStage()
	slots := bestThirdSlots()
	confirmed := "AlreadyConfirmed"
	slots[0].Team = &confirmed // first slot already done — skips ConfirmSlot call
	repo := &stubTournamentRepo{slots: slots}
	svc := newTournamentSvc(matches, repo)

	results, err := svc.AutoConfirmBestThirdSlots(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 8 {
		t.Fatalf("expected 8 results (already-done slot still included), got %d", len(results))
	}
}

func TestAutoConfirmBestThirdSlots_NoMatchingPossible_ReturnsInternal(t *testing.T) {
	matches := buildCompleteGroupStage()
	// Only slot eligible for groups I/J/K/L/M — top-8 thirds are from A–H, so matching fails.
	slots := []*domain.TournamentSlot{
		{ID: 1, Label: "r32_xx_b", Description: "Mejor 3.° (I/J/K/L/M)"},
	}
	repo := &stubTournamentRepo{slots: slots}
	svc := newTournamentSvc(matches, repo)

	_, err := svc.AutoConfirmBestThirdSlots(context.Background(), 1)
	if !errors.Is(err, apperrors.ErrInternal) {
		t.Errorf("expected ErrInternal, got %v", err)
	}
}
