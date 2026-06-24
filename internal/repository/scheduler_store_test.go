package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// seedPrediction inserts a prediction for the given user+match and returns it.
func seedPrediction(t *testing.T, userID, matchID int) *domain.Prediction {
	t.Helper()
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: userID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("seedPrediction: %v", err)
	}
	return p
}

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

// ── ListUnpredictedUpcomingMatchesByUsers ─────────────────────────────────────

func TestSchedulerStore_ListUnpredictedUpcomingMatchesByUsers_EmptyInput(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	result, err := store.ListUnpredictedUpcomingMatchesByUsers(context.Background(), nil, time.Now())
	if err != nil {
		t.Fatalf("ListUnpredictedUpcomingMatchesByUsers: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty userIDs; got %v", result)
	}
}

func TestSchedulerStore_ListUnpredictedUpcomingMatchesByUsers_ReturnsUnpredictedMatch(t *testing.T) {
	cleanTables(t)

	u := seedUser(t)
	m := seedMatch(t) // scheduled, kickoff_at = now+24h
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	// No prediction for this user+match — expect it to appear.
	store := repository.NewPostgresSchedulerStore(testDB)
	result, err := store.ListUnpredictedUpcomingMatchesByUsers(context.Background(), []int{u.ID}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("ListUnpredictedUpcomingMatchesByUsers: %v", err)
	}

	matches, ok := result[u.ID]
	if !ok || len(matches) == 0 {
		t.Fatalf("expected ≥1 unpredicted match for user %d; got %v", u.ID, result)
	}

	found := false
	for _, dm := range matches {
		if dm.MatchID == m.ID {
			found = true
			if dm.HomeTeam == "" || dm.AwayTeam == "" {
				t.Errorf("match %d: expected non-empty team names", dm.MatchID)
			}
			if dm.KickoffAt.IsZero() {
				t.Errorf("match %d: expected non-zero kickoff", dm.MatchID)
			}
		}
	}
	if !found {
		t.Errorf("match %d not returned in unpredicted list for user %d", m.ID, u.ID)
	}
}

func TestSchedulerStore_ListUnpredictedUpcomingMatchesByUsers_ExcludesPredictedMatch(t *testing.T) {
	cleanTables(t)

	u := seedUser(t)
	m := seedMatch(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	seedPrediction(t, u.ID, m.ID) // user already predicted this match

	store := repository.NewPostgresSchedulerStore(testDB)
	result, err := store.ListUnpredictedUpcomingMatchesByUsers(context.Background(), []int{u.ID}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("ListUnpredictedUpcomingMatchesByUsers: %v", err)
	}

	if matches, ok := result[u.ID]; ok && len(matches) > 0 {
		for _, dm := range matches {
			if dm.MatchID == m.ID {
				t.Errorf("match %d should be excluded (already predicted)", m.ID)
			}
		}
	}
}

func TestSchedulerStore_ListUnpredictedUpcomingMatchesByUsers_ExcludesMatchBeforeAfter(t *testing.T) {
	cleanTables(t)

	u := seedUser(t)
	m := seedMatch(t) // kickoff_at = now+24h
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	// after = now+48h → match at now+24h is before after, must be excluded
	store := repository.NewPostgresSchedulerStore(testDB)
	result, err := store.ListUnpredictedUpcomingMatchesByUsers(context.Background(), []int{u.ID}, time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatalf("ListUnpredictedUpcomingMatchesByUsers: %v", err)
	}

	if matches, ok := result[u.ID]; ok {
		for _, dm := range matches {
			if dm.MatchID == m.ID {
				t.Errorf("match %d (kickoff +24h) should be excluded when after=+48h", m.ID)
			}
		}
	}
}

func TestSchedulerStore_ListUnpredictedUpcomingMatchesByUsers_MultipleUsers(t *testing.T) {
	cleanTables(t)

	u1 := seedUser(t)
	u2 := seedUser(t)
	m := seedMatch(t)
	q := seedQuiniela(t, u1.ID)
	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)
	// u1 predicted; u2 did not
	seedPrediction(t, u1.ID, m.ID)

	store := repository.NewPostgresSchedulerStore(testDB)
	result, err := store.ListUnpredictedUpcomingMatchesByUsers(context.Background(), []int{u1.ID, u2.ID}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("ListUnpredictedUpcomingMatchesByUsers: %v", err)
	}

	// u1 should have no unpredicted matches (already predicted)
	for _, dm := range result[u1.ID] {
		if dm.MatchID == m.ID {
			t.Errorf("user %d already predicted match %d — should not appear", u1.ID, m.ID)
		}
	}

	// u2 should see the unpredicted match
	found := false
	for _, dm := range result[u2.ID] {
		if dm.MatchID == m.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("match %d should appear for user %d (no prediction)", m.ID, u2.ID)
	}
}

// ── GetUserTimezones ──────────────────────────────────────────────────────────

func TestSchedulerStore_GetUserTimezones_EmptyInput_ReturnsNil(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	result, err := store.GetUserTimezones(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty input; got %v", result)
	}
}

func TestSchedulerStore_GetUserTimezones_ReturnsStoredTimezone(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)
	if err := repo.UpdateTimezone(context.Background(), u.ID, "America/New_York"); err != nil {
		t.Fatalf("seed timezone: %v", err)
	}
	store := repository.NewPostgresSchedulerStore(testDB)

	result, err := store.GetUserTimezones(context.Background(), []int{u.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tz := result[u.ID]; tz != "America/New_York" {
		t.Errorf("timezone for user %d: got %q; want \"America/New_York\"", u.ID, tz)
	}
}

func TestSchedulerStore_GetUserTimezones_OmitsUnknownUser(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	const unknownID = 999999
	result, err := store.GetUserTimezones(context.Background(), []int{unknownID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result[unknownID]; ok {
		t.Errorf("unknown user %d should not appear in result", unknownID)
	}
}

// ── ListDailySummaryTargets ───────────────────────────────────────────────────

// seedMatchOnDate inserts a match with its kickoff set to noon on the given date.
func seedMatchOnDate(t *testing.T, date time.Time) *domain.Match {
	t.Helper()
	skipIfNoDB(t)
	mrepo := repository.NewPostgresMatchRepository(testDB)
	kickoff := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC)
	m := &domain.Match{
		HomeTeam:  "Home",
		AwayTeam:  "Away",
		Status:    domain.MatchStatusScheduled,
		Phase:     domain.PhaseGroupStage,
		KickoffAt: kickoff,
	}
	label := "A"
	m.GroupLabel = &label
	if err := mrepo.Create(context.Background(), m); err != nil {
		t.Fatalf("seedMatchOnDate: %v", err)
	}
	return m
}

func TestSchedulerStore_ListDailySummaryTargets_NoMatches_ReturnsNil(t *testing.T) {
	cleanTables(t)
	store := repository.NewPostgresSchedulerStore(testDB)

	result, err := store.ListDailySummaryTargets(context.Background(), time.Now().UTC(), "UTC", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no payloads when DB is empty; got %d", len(result))
	}
}

func TestSchedulerStore_ListDailySummaryTargets_MatchWithoutResult_ReturnsNil(t *testing.T) {
	cleanTables(t)
	today := time.Now().UTC()
	seedMatchOnDate(t, today) // no scores set → not all results in

	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	store := repository.NewPostgresSchedulerStore(testDB)
	result, err := store.ListDailySummaryTargets(context.Background(), today, "UTC", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no payloads when match has no result; got %d", len(result))
	}
}

func TestSchedulerStore_ListDailySummaryTargets_AllResultsIn_ReturnsPaidUsers(t *testing.T) {
	cleanTables(t)
	today := time.Now().UTC()
	m := seedMatchOnDate(t, today)

	// Set a final result with updated_at well in the past (> resultDelay).
	mrepo := repository.NewPostgresMatchRepository(testDB)
	home, away := 2, 1
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	if err := mrepo.Update(context.Background(), m); err != nil {
		t.Fatalf("set match result: %v", err)
	}
	// Back-date updated_at so it passes the resultDelay check (cutoff = now - delay).
	_, err := testDB.Exec(context.Background(),
		`UPDATE matches SET updated_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, m.ID)
	if err != nil {
		t.Fatalf("back-date match: %v", err)
	}

	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	seedPrediction(t, u.ID, m.ID)

	store := repository.NewPostgresSchedulerStore(testDB)
	// resultDelay = 1 hour; last_result_at is 2 hours ago → eligible.
	result, err := store.ListDailySummaryTargets(context.Background(), today, "UTC", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 payload; got %d", len(result))
	}
	if result[0].UserID != u.ID {
		t.Errorf("payload user_id: got %d; want %d", result[0].UserID, u.ID)
	}
	if len(result[0].Matches) != 1 {
		t.Errorf("payload matches: got %d; want 1", len(result[0].Matches))
	}
}

func TestSchedulerStore_ListDailySummaryTargets_ResultTooRecent_ReturnsNil(t *testing.T) {
	cleanTables(t)
	today := time.Now().UTC()
	m := seedMatchOnDate(t, today)

	mrepo := repository.NewPostgresMatchRepository(testDB)
	home, away := 1, 0
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	if err := mrepo.Update(context.Background(), m); err != nil {
		t.Fatalf("set match result: %v", err)
	}
	// updated_at is NOW() (just inserted) — within the 30-minute delay.

	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	seedPrediction(t, u.ID, m.ID)

	store := repository.NewPostgresSchedulerStore(testDB)
	// resultDelay = 30 minutes; result was entered moments ago → not eligible yet.
	result, err := store.ListDailySummaryTargets(context.Background(), today, "UTC", 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no payloads when result is too recent; got %d", len(result))
	}
}

func TestSchedulerStore_ListDailySummaryTargets_UnpaidMember_Excluded(t *testing.T) {
	cleanTables(t)
	today := time.Now().UTC()
	m := seedMatchOnDate(t, today)

	mrepo := repository.NewPostgresMatchRepository(testDB)
	home, away := 1, 0
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	if err := mrepo.Update(context.Background(), m); err != nil {
		t.Fatalf("set match result: %v", err)
	}
	_, err := testDB.Exec(context.Background(),
		`UPDATE matches SET updated_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, m.ID)
	if err != nil {
		t.Fatalf("back-date match: %v", err)
	}

	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, false) // paid=false
	seedPrediction(t, u.ID, m.ID)

	store := repository.NewPostgresSchedulerStore(testDB)
	result, err := store.ListDailySummaryTargets(context.Background(), today, "UTC", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no payloads for unpaid member; got %d", len(result))
	}
}

func TestSchedulerStore_ListDailySummaryTargets_UserWithoutPrediction_Excluded(t *testing.T) {
	cleanTables(t)
	today := time.Now().UTC()
	m := seedMatchOnDate(t, today)

	mrepo := repository.NewPostgresMatchRepository(testDB)
	home, away := 2, 1
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	if err := mrepo.Update(context.Background(), m); err != nil {
		t.Fatalf("set match result: %v", err)
	}
	_, err := testDB.Exec(context.Background(),
		`UPDATE matches SET updated_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, m.ID)
	if err != nil {
		t.Fatalf("back-date match: %v", err)
	}

	// Active paid member with NO predictions for the day.
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	store := repository.NewPostgresSchedulerStore(testDB)
	result, err := store.ListDailySummaryTargets(context.Background(), today, "UTC", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no payloads for user with no predictions; got %d", len(result))
	}
}

func TestSchedulerStore_ListDailySummaryTargets_OnlyUsersWithPredictionsIncluded(t *testing.T) {
	cleanTables(t)
	today := time.Now().UTC()
	m := seedMatchOnDate(t, today)

	mrepo := repository.NewPostgresMatchRepository(testDB)
	home, away := 1, 1
	m.Status = domain.MatchStatusFinished
	m.HomeScore = &home
	m.AwayScore = &away
	if err := mrepo.Update(context.Background(), m); err != nil {
		t.Fatalf("set match result: %v", err)
	}
	_, err := testDB.Exec(context.Background(),
		`UPDATE matches SET updated_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, m.ID)
	if err != nil {
		t.Fatalf("back-date match: %v", err)
	}

	// uWith: paid active member who submitted a prediction.
	uWith := seedUser(t)
	qWith := seedQuiniela(t, uWith.ID)
	seedMembership(t, qWith.ID, uWith.ID, domain.MembershipActive, true)
	seedPrediction(t, uWith.ID, m.ID)

	// uWithout: paid active member with no prediction for the same match.
	uWithout := seedUser(t)
	qWithout := seedQuiniela(t, uWithout.ID)
	seedMembership(t, qWithout.ID, uWithout.ID, domain.MembershipActive, true)
	_ = uWithout // not used further; included to confirm exclusion by absence

	store := repository.NewPostgresSchedulerStore(testDB)
	result, err := store.ListDailySummaryTargets(context.Background(), today, "UTC", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected exactly 1 payload (user with prediction); got %d", len(result))
	}
	if result[0].UserID != uWith.ID {
		t.Errorf("payload user_id: got %d; want %d", result[0].UserID, uWith.ID)
	}
}

// Ensure the notification package import compiles (type assertion).
var _ []notification.DailySummaryPayload
