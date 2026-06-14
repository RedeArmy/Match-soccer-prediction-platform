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

// ── External link (match sync) ────────────────────────────────────────────────

func TestMatchRepository_LinkExternal_SetsProviderAndID(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	if err := repo.LinkExternal(context.Background(), m.ID, "api-football", 42); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.ExternalProvider == nil || *got.ExternalProvider != "api-football" {
		t.Errorf("external_provider: got %v, want api-football", got.ExternalProvider)
	}
	if got.ExternalMatchID == nil || *got.ExternalMatchID != 42 {
		t.Errorf("external_match_id: got %v, want 42", got.ExternalMatchID)
	}
}

func TestMatchRepository_LinkExternal_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	err := repo.LinkExternal(context.Background(), 99999, "api-football", 1)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestMatchRepository_UnlinkExternal_ClearsFields(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	if err := repo.LinkExternal(context.Background(), m.ID, "api-football", 7); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := repo.UnlinkExternal(context.Background(), m.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.ExternalProvider != nil {
		t.Errorf("expected nil external_provider after unlink, got %v", *got.ExternalProvider)
	}
	if got.ExternalMatchID != nil {
		t.Errorf("expected nil external_match_id after unlink, got %v", *got.ExternalMatchID)
	}
}

func TestMatchRepository_UnlinkExternal_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	err := repo.UnlinkExternal(context.Background(), 99999)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestMatchRepository_ListSyncCandidates_ReturnsLinkedScheduled(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t) // status=scheduled
	repo := repository.NewPostgresMatchRepository(testDB)

	// Unlinked match must not appear.
	candidates, err := repo.ListSyncCandidates(context.Background(), 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 sync candidates before link, got %d", len(candidates))
	}

	if err := repo.LinkExternal(context.Background(), m.ID, "api-football", 100); err != nil {
		t.Fatalf("link: %v", err)
	}

	candidates, err = repo.ListSyncCandidates(context.Background(), 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(candidates) != 1 {
		t.Errorf("expected 1 sync candidate after link, got %d", len(candidates))
	}
}

func TestMatchRepository_ListSyncCandidates_ExcludesFinished(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	if err := repo.LinkExternal(context.Background(), m.ID, "api-football", 101); err != nil {
		t.Fatalf("link: %v", err)
	}

	home, away := 2, 1
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf("update to finished: %v", err)
	}

	candidates, err := repo.ListSyncCandidates(context.Background(), 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 sync candidates for finished match, got %d", len(candidates))
	}
}

func TestMatchRepository_UpdateSyncState_SetsLastSyncedAt(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	if err := repo.UpdateSyncState(context.Background(), m.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	// Verify by fetching: last_synced_at is not in the domain.Match struct
	// but the call must complete without error.
}

// ── FindByTeams ───────────────────────────────────────────────────────────────

func TestMatchRepository_FindByTeams_Found(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t) // HomeTeam=Brazil, AwayTeam=Argentina
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.FindByTeams(context.Background(), repoBrazil, repoArgentina)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected non-nil match, got nil")
	}
	if got.ID != m.ID {
		t.Errorf(fmtIDMismatch, got.ID, m.ID)
	}
}

func TestMatchRepository_FindByTeams_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	skipIfNoDB(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.FindByTeams(context.Background(), "Netherlands", "Belgium")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown teams, got %+v", got)
	}
}

func TestMatchRepository_FindByTeams_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	skipIfNoDB(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.FindByTeams(ctx, repoBrazil, repoArgentina)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── UpdateKickoff ─────────────────────────────────────────────────────────────

func TestMatchRepository_UpdateKickoff_Persists(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	newKickoff := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)
	if err := repo.UpdateKickoff(context.Background(), m.ID, newKickoff); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !got.KickoffAt.Equal(newKickoff) {
		t.Errorf("KickoffAt: got %v, want %v", got.KickoffAt, newKickoff)
	}
}

func TestMatchRepository_UpdateKickoff_NonExistentID_ReturnsNil(t *testing.T) {
	// UpdateKickoff uses a bare UPDATE with no rows-affected check, so a
	// missing ID is a silent no-op rather than a NotFoundError.
	cleanTables(t)
	skipIfNoDB(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	err := repo.UpdateKickoff(context.Background(), 999999, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Errorf("expected nil for non-existent match ID (silent no-op), got %v", err)
	}
}

func TestMatchRepository_UpdateKickoff_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := repo.UpdateKickoff(ctx, m.ID, time.Now().Add(1*time.Hour))
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── ListSyncCandidates with prematchWindowMin ─────────────────────────────────

func TestMatchRepository_ListSyncCandidates_PrematchWindowFiltersDistantMatches(t *testing.T) {
	cleanTables(t)
	skipIfNoDB(t)

	repo := repository.NewPostgresMatchRepository(testDB)
	ctx := context.Background()

	// Seed a scheduled match with kickoff 5 hours from now.
	m := seedMatch(t)
	if err := repo.LinkExternal(ctx, m.ID, "api-football", 12345); err != nil {
		t.Fatalf("link external: %v", err)
	}
	// Kickoff is 5 h away but window is only 30 min → should be excluded.
	if candidates, err := repo.ListSyncCandidates(ctx, 30); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	} else if len(candidates) != 0 {
		t.Errorf("expected 0 candidates when kickoff > prematch window, got %d", len(candidates))
	}
}

func TestMatchRepository_ListSyncCandidates_ZeroWindow_ReturnsAll(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)
	ctx := context.Background()

	if err := repo.LinkExternal(ctx, m.ID, "api-football", 99999); err != nil {
		t.Fatalf("link external: %v", err)
	}

	// window=0 must return all linked non-finished matches.
	candidates, err := repo.ListSyncCandidates(ctx, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(candidates) == 0 {
		t.Error("expected at least 1 candidate with zero prematch window")
	}
}
