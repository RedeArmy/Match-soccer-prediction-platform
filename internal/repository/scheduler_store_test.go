package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── PostgresSchedulerStore ────────────────────────────────────────────────────

func TestSchedulerStore_CountPendingTransfers_Empty(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	n, err := store.CountPendingTransfers(context.Background())
	if err != nil {
		t.Fatalf("CountPendingTransfers: %v", err)
	}
	if n != 0 {
		t.Errorf("got %d; want 0", n)
	}
}

func TestSchedulerStore_CountPendingTransfers_ReturnsCount(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedBankTransferProof(t, u.ID, 5000)
	seedBankTransferProof(t, u.ID, 3000)
	store := repository.NewPostgresSchedulerStore(testDB)

	n, err := store.CountPendingTransfers(context.Background())
	if err != nil {
		t.Fatalf("CountPendingTransfers: %v", err)
	}
	if n != 2 {
		t.Errorf("got %d; want 2", n)
	}
}

func TestSchedulerStore_CountPendingWithdrawals_Empty(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	n, err := store.CountPendingWithdrawals(context.Background())
	if err != nil {
		t.Fatalf("CountPendingWithdrawals: %v", err)
	}
	if n != 0 {
		t.Errorf("got %d; want 0", n)
	}
}

func TestSchedulerStore_CountPendingWithdrawals_ReturnsCount(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 50000)
	seedWithdrawalRequest(t, u.ID, 10000)
	store := repository.NewPostgresSchedulerStore(testDB)

	n, err := store.CountPendingWithdrawals(context.Background())
	if err != nil {
		t.Fatalf("CountPendingWithdrawals: %v", err)
	}
	if n != 1 {
		t.Errorf("got %d; want 1", n)
	}
}

func TestSchedulerStore_OldestPendingTransferSince_NoProofs(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	ts, err := store.OldestPendingTransferSince(context.Background())
	if err != nil {
		t.Fatalf("OldestPendingTransferSince: %v", err)
	}
	if !ts.IsZero() {
		t.Errorf("expected zero time; got %v", ts)
	}
}

func TestSchedulerStore_OldestPendingTransferSince_WithProof(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedBankTransferProof(t, u.ID, 5000)
	store := repository.NewPostgresSchedulerStore(testDB)

	ts, err := store.OldestPendingTransferSince(context.Background())
	if err != nil {
		t.Fatalf("OldestPendingTransferSince: %v", err)
	}
	if ts.IsZero() {
		t.Error("expected non-zero time for existing pending proof")
	}
}

func TestSchedulerStore_DailySummary_EmptyDB(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)
	since := time.Now().UTC().Add(-24 * time.Hour)

	row, err := store.DailySummary(context.Background(), since)
	if err != nil {
		t.Fatalf("DailySummary: %v", err)
	}
	if row.NewUsers != 0 || row.NewTransfers != 0 || row.TotalCreditedCents != 0 {
		t.Errorf("expected all-zero row on empty DB; got %+v", row)
	}
}

func TestSchedulerStore_DailySummary_CountsInWindow(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedBankTransferProof(t, u.ID, 10000)
	store := repository.NewPostgresSchedulerStore(testDB)
	since := time.Now().UTC().Add(-24 * time.Hour)

	row, err := store.DailySummary(context.Background(), since)
	if err != nil {
		t.Fatalf("DailySummary: %v", err)
	}
	if row.NewUsers < 1 {
		t.Errorf("NewUsers: got %d; want ≥ 1", row.NewUsers)
	}
	if row.NewTransfers < 1 {
		t.Errorf("NewTransfers: got %d; want ≥ 1", row.NewTransfers)
	}
}

func TestSchedulerStore_WeeklySummary_EmptyDB(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)
	since := time.Now().UTC().Add(-7 * 24 * time.Hour)

	row, err := store.WeeklySummary(context.Background(), since)
	if err != nil {
		t.Fatalf("WeeklySummary: %v", err)
	}
	if row.TotalRevenueCents != 0 || row.NewUsers != 0 {
		t.Errorf("expected all-zero row on empty DB; got %+v", row)
	}
}

func TestSchedulerStore_WeeklySummary_CountsInWindow(t *testing.T) {
	cleanTables(t)
	_ = seedUser(t)
	store := repository.NewPostgresSchedulerStore(testDB)
	since := time.Now().UTC().Add(-7 * 24 * time.Hour)

	row, err := store.WeeklySummary(context.Background(), since)
	if err != nil {
		t.Fatalf("WeeklySummary: %v", err)
	}
	if row.NewUsers < 1 {
		t.Errorf("NewUsers: got %d; want ≥ 1", row.NewUsers)
	}
}

func TestSchedulerStore_ListFinishedMatchesMissingResult_Empty(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	matches, err := store.ListFinishedMatchesMissingResult(context.Background())
	if err != nil {
		t.Fatalf("ListFinishedMatchesMissingResult: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("got %d matches; want 0", len(matches))
	}
}

func TestSchedulerStore_ListFinishedMatchesMissingResult_ReturnsFinishedWithoutScore(t *testing.T) {
	cleanTables(t)
	matchRepo := repository.NewPostgresMatchRepository(testDB)

	m := seedMatch(t)
	m.Status = domain.MatchStatusFinished
	if err := matchRepo.Update(context.Background(), m); err != nil {
		t.Fatalf("Update match status: %v", err)
	}

	store := repository.NewPostgresSchedulerStore(testDB)
	matches, err := store.ListFinishedMatchesMissingResult(context.Background())
	if err != nil {
		t.Fatalf("ListFinishedMatchesMissingResult: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("got %d matches; want 1", len(matches))
	}
	if matches[0].ID != m.ID {
		t.Errorf("match ID: got %d; want %d", matches[0].ID, m.ID)
	}
}

func TestSchedulerStore_ListFinishedMatchesMissingResult_ExcludesMatchesWithScore(t *testing.T) {
	cleanTables(t)
	matchRepo := repository.NewPostgresMatchRepository(testDB)

	m := seedMatch(t)
	home, away := 2, 1
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	if err := matchRepo.Update(context.Background(), m); err != nil {
		t.Fatalf("Update match with score: %v", err)
	}

	store := repository.NewPostgresSchedulerStore(testDB)
	matches, err := store.ListFinishedMatchesMissingResult(context.Background())
	if err != nil {
		t.Fatalf("ListFinishedMatchesMissingResult: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("got %d matches; want 0 (score already set)", len(matches))
	}
}

// ── ListStaleBankTransfers ────────────────────────────────────────────────────

func TestSchedulerStore_ListStaleBankTransfers_Empty(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	proofs, err := store.ListStaleBankTransfers(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ListStaleBankTransfers: %v", err)
	}
	if len(proofs) != 0 {
		t.Errorf("got %d proofs; want 0", len(proofs))
	}
}

func TestSchedulerStore_ListStaleBankTransfers_ReturnsPendingBeforeCutoff(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedBankTransferProof(t, u.ID, 5000)
	store := repository.NewPostgresSchedulerStore(testDB)

	// Cutoff in the future — the recently seeded proof is stale relative to it.
	proofs, err := store.ListStaleBankTransfers(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("ListStaleBankTransfers: %v", err)
	}
	if len(proofs) != 1 {
		t.Fatalf("got %d proofs; want 1", len(proofs))
	}
	if proofs[0].UserID != u.ID {
		t.Errorf("UserID: got %d; want %d", proofs[0].UserID, u.ID)
	}
}

func TestSchedulerStore_ListStaleBankTransfers_ExcludesAfterCutoff(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedBankTransferProof(t, u.ID, 5000)
	store := repository.NewPostgresSchedulerStore(testDB)

	// Cutoff in the past — the recently seeded proof is NOT stale relative to it.
	proofs, err := store.ListStaleBankTransfers(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("ListStaleBankTransfers: %v", err)
	}
	if len(proofs) != 0 {
		t.Errorf("got %d proofs; want 0 (proof created after cutoff)", len(proofs))
	}
}

// ── ListStaleWithdrawals ──────────────────────────────────────────────────────

func TestSchedulerStore_ListStaleWithdrawals_Empty(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	reqs, err := store.ListStaleWithdrawals(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ListStaleWithdrawals: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("got %d reqs; want 0", len(reqs))
	}
}

func TestSchedulerStore_ListStaleWithdrawals_ReturnsPendingBeforeCutoff(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 50000)
	seedWithdrawalRequest(t, u.ID, 10000)
	store := repository.NewPostgresSchedulerStore(testDB)

	reqs, err := store.ListStaleWithdrawals(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("ListStaleWithdrawals: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("got %d reqs; want 1", len(reqs))
	}
	if reqs[0].UserID != u.ID {
		t.Errorf("UserID: got %d; want %d", reqs[0].UserID, u.ID)
	}
}

func TestSchedulerStore_ListStaleWithdrawals_ExcludesAfterCutoff(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 50000)
	seedWithdrawalRequest(t, u.ID, 10000)
	store := repository.NewPostgresSchedulerStore(testDB)

	reqs, err := store.ListStaleWithdrawals(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("ListStaleWithdrawals: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("got %d reqs; want 0 (request created after cutoff)", len(reqs))
	}
}

func TestSchedulerStore_ListUpcomingMatchesWithDeadline_NoMatches(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	results, err := store.ListUpcomingMatchesWithDeadline(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("ListUpcomingMatchesWithDeadline: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results; want 0", len(results))
	}
}

func TestSchedulerStore_ListUpcomingMatchesWithDeadline_MatchBeyondWindow_Excluded(t *testing.T) {
	cleanTables(t)
	_ = seedMatch(t) // seedMatch creates match kicking off in 24 hours
	store := repository.NewPostgresSchedulerStore(testDB)

	// Only look 30 minutes ahead — 24-hour match is outside the window.
	results, err := store.ListUpcomingMatchesWithDeadline(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("ListUpcomingMatchesWithDeadline: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results; want 0 (match outside window)", len(results))
	}
}

func TestSchedulerStore_ListUpcomingMatchesWithDeadline_UserMissingPrediction(t *testing.T) {
	cleanTables(t)

	// Seed a match that kicks off within the next 48 hours (seedMatch = 24 h).
	m := seedMatch(t)

	// Seed a user, quiniela, and approved membership.
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	// No prediction inserted for this user+match — expect the user to appear.
	store := repository.NewPostgresSchedulerStore(testDB)
	results, err := store.ListUpcomingMatchesWithDeadline(context.Background(), 48*time.Hour)
	if err != nil {
		t.Fatalf("ListUpcomingMatchesWithDeadline: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected ≥ 1 deadline match; got 0")
	}

	found := false
	for _, dm := range results {
		if dm.Match.ID == m.ID {
			found = true
			if len(dm.MissingUserIDs) == 0 {
				t.Errorf("match %d: expected ≥ 1 missing user ID", m.ID)
			}
		}
	}
	if !found {
		t.Errorf("match %d not found in deadline results", m.ID)
	}
}
