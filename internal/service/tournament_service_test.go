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

type stubTournamentRepo struct {
	slot  *domain.TournamentSlot
	slots []*domain.TournamentSlot
	err   error
}

func (r *stubTournamentRepo) CreateSlot(_ context.Context, label string) (*domain.TournamentSlot, error) {
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

	slot, err := svc.CreateSlot(context.Background(), tournamentWinnerGroupA)
	if err != nil {
		t.Fatalf(tournamentUnexpectedErr, err)
	}
	if slot.Label != tournamentWinnerGroupA {
		t.Errorf("label: want winner_group_a, got %s", slot.Label)
	}
}

func TestTournamentService_CreateSlot_EmptyLabel_ReturnsValidation(t *testing.T) {
	svc := newTournamentSvc(nil, &stubTournamentRepo{})
	_, err := svc.CreateSlot(context.Background(), "")
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
