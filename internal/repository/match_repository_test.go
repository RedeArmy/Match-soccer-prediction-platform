package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── MatchRepository ───────────────────────────────────────────────────────────

func TestMatchRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)
	label := repoGroupLabel
	m := &domain.Match{
		HomeTeam:   "France",
		AwayTeam:   "Germany",
		Status:     domain.MatchStatusScheduled,
		Phase:      domain.PhaseGroupStage,
		GroupLabel: &label,
		KickoffAt:  time.Now().Add(48 * time.Hour).UTC(),
	}

	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestMatchRepository_Create_DuplicateTeamsKickoff_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)
	label := repoGroupLabel
	kickoff := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Microsecond)
	m1 := &domain.Match{
		HomeTeam: "Spain", AwayTeam: "Portugal",
		Status: domain.MatchStatusScheduled, Phase: domain.PhaseGroupStage,
		GroupLabel: &label, KickoffAt: kickoff,
	}
	if err := repo.Create(context.Background(), m1); err != nil {
		t.Fatalf("first create: %v", err)
	}

	m2 := &domain.Match{
		HomeTeam: "Spain", AwayTeam: "Portugal",
		Status: domain.MatchStatusScheduled, Phase: domain.PhaseGroupStage,
		GroupLabel: &label, KickoffAt: kickoff,
	}
	if err := repo.Create(context.Background(), m2); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for duplicate teams/kickoff, got %v", err)
	}
}

func TestMatchRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	created := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected match, got nil")
	}
	if got.HomeTeam != created.HomeTeam {
		t.Errorf("home team: got %q, want %q", got.HomeTeam, created.HomeTeam)
	}
}

func TestMatchRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for missing match, got %+v", got)
	}
}

func TestMatchRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	m.Status = domain.MatchStatusLive
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.Status != domain.MatchStatusLive {
		t.Errorf("status not updated: got %q", m.Status)
	}
}

func TestMatchRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)
	ghost := &domain.Match{ID: 99999, HomeTeam: "X", AwayTeam: "Y", Status: domain.MatchStatusScheduled, KickoffAt: time.Now().Add(time.Hour).UTC()}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestMatchRepository_List_ReturnsAll(t *testing.T) {
	cleanTables(t)
	seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	matches, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(matches) == 0 {
		t.Error("expected at least one match")
	}
}

func TestMatchRepository_ListByStatus_FiltersCorrectly(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t) // status = scheduled

	// Promote one to live.
	repo := repository.NewPostgresMatchRepository(testDB)
	m.Status = domain.MatchStatusLive
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf("update to live: %v", err)
	}

	live, err := repo.ListByStatus(context.Background(), domain.MatchStatusLive)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(live) != 1 {
		t.Errorf("expected 1 live match, got %d", len(live))
	}

	scheduled, err := repo.ListByStatus(context.Background(), domain.MatchStatusScheduled)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(scheduled) != 0 {
		t.Errorf("expected 0 scheduled matches, got %d", len(scheduled))
	}
}

func TestMatchRepository_ListByPhase_FiltersCorrectly(t *testing.T) {
	cleanTables(t)
	seedMatch(t) // phase = group_stage

	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.ListByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 group_stage match, got %d", len(got))
	}

	none, err := repo.ListByPhase(context.Background(), domain.PhaseFinal)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 final matches, got %d", len(none))
	}
}

func TestMatchRepository_GetByID_HydratesStadiumLocation(t *testing.T) {
	cleanTables(t)
	created := seedMatchWithStadium(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.Stadium == nil {
		t.Fatal("expected stadium to be hydrated")
	}
	if got.Stadium.City == nil {
		t.Fatal("expected city to be hydrated")
	}
	if got.Stadium.City.Name != "East Rutherford" {
		t.Errorf("city: got %q, want %q", got.Stadium.City.Name, "East Rutherford")
	}
	if got.Stadium.City.State == nil {
		t.Fatal("expected state to be hydrated")
	}
	if got.Stadium.City.State.Code != "NJ" {
		t.Errorf("state code: got %q, want %q", got.Stadium.City.State.Code, "NJ")
	}
	if got.Stadium.City.State.Country == nil {
		t.Fatal("expected country to be hydrated")
	}
	if got.Stadium.City.State.Country.Code != "US" {
		t.Errorf("country code: got %q, want %q", got.Stadium.City.State.Country.Code, "US")
	}
}

func TestMatchRepository_List_HydratesStadiumLocation(t *testing.T) {
	cleanTables(t)
	seedMatchWithStadium(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	matches, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	m := matches[0]
	if m.Stadium == nil {
		t.Fatal("expected stadium to be hydrated in list result")
	}
	if m.Stadium.City == nil {
		t.Fatal("expected city to be hydrated in list result")
	}
	if m.Stadium.City.State == nil {
		t.Fatal("expected state to be hydrated in list result")
	}
	if m.Stadium.City.State.Country == nil {
		t.Fatal("expected country to be hydrated in list result")
	}
}

// ── WinMethod persistence ─────────────────────────────────────────────────────

func TestMatchRepository_Update_PersistsWinMethod(t *testing.T) {
	cleanTables(t)
	m := seedMatchWithPhase(t, domain.PhaseRoundOf16)
	repo := repository.NewPostgresMatchRepository(testDB)

	home, away := 2, 1
	wm := domain.WinMethodPenalties
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	m.WinMethod = &wm
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.WinMethod == nil {
		t.Fatal("expected WinMethod to be non-nil after update")
	}
	if *got.WinMethod != domain.WinMethodPenalties {
		t.Errorf("WinMethod: got %q, want %q", *got.WinMethod, domain.WinMethodPenalties)
	}
}

func TestMatchRepository_Update_NilWinMethod_RemainsNil(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t) // group_stage — WinMethod must stay nil
	repo := repository.NewPostgresMatchRepository(testDB)

	home, away := 1, 0
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.WinMethod != nil {
		t.Errorf("expected nil WinMethod for group-stage match, got %q", *got.WinMethod)
	}
}
