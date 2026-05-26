//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"go.uber.org/zap/zaptest"
)

// seedE2EBalance directly sets a user's balance_cents. Use in flows that
// require a pre-existing balance (e.g. withdrawal tests) without going through
// the full payment ingestion path.
func seedE2EBalance(t *testing.T, userID, cents int) {
	t.Helper()
	_, err := e2eDB.Exec(context.Background(),
		`UPDATE users SET balance_cents = $1 WHERE id = $2`, cents, userID,
	)
	if err != nil {
		t.Fatalf("seedE2EBalance user %d: %v", userID, err)
	}
}

// setE2EKYCTier sets a user's kyc_tier directly in e2eDB.
// Call this in any test that exercises a money-movement endpoint after the
// KYC gate was introduced: withdrawals require tier ≥ 2, and high-value
// deposits are capped per tier. Without it the gate returns 403 FORBIDDEN.
func setE2EKYCTier(t *testing.T, userID int, tier domain.KYCTier) {
	t.Helper()
	_, err := e2eDB.Exec(context.Background(),
		`UPDATE users SET kyc_tier = $1 WHERE id = $2`, int(tier), userID,
	)
	if err != nil {
		t.Fatalf("setE2EKYCTier user %d tier %d: %v", userID, tier, err)
	}
}

// newE2EServerUnlimited is like newE2EServer but injects an unlimited rate
// limiter so flows that issue many sequential requests for the same user never
// hit 429 responses during testing.
func newE2EServerUnlimited(t *testing.T, jwksURL string) *api.Server {
	t.Helper()
	cfg := &config.Config{}
	cfg.Clerk.JWKSURL = jwksURL
	srv := api.New(e2eDB, cfg, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
	srv.SetLimiterStore(middleware.NewUnlimitedLimiterStore())
	return srv
}

// ── TestE2E_WithdrawalLifecycle_ApproveAndProcess ─────────────────────────────
//
// Verifies the full happy-path for a withdrawal request:
//
//  1. User submits a withdrawal request (balance pre-seeded directly).
//  2. GET /withdrawals confirms status=pending.
//  3. Admin lists pending withdrawals and sees the request.
//  4. Admin approves → status=approved.
//  5. Admin processes → status=processed; balance deduction committed.
func TestE2E_WithdrawalLifecycle_ApproveAndProcess(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "admin@e2e.test", "e2e-admin", domain.RoleAdmin)
	userID := seedE2EUser(t, "user@e2e.test", "e2e-user", domain.RoleUser)
	seedE2EBalance(t, userID, 50_000) // 500 GTQ available
	setE2EKYCTier(t, userID, domain.KYCTierTwo)

	adminToken := signJWT("e2e-admin")
	userToken := signJWT("e2e-user")

	// Step 1 — user creates withdrawal for 100 GTQ (10 000 cents).
	rec := doRequest(t, h, http.MethodPost, "/api/v1/withdrawals", userToken,
		jsonBody(t, map[string]any{
			"amount_cents":   10_000,
			"currency":       "GTQ",
			"method":         "paypal",
			"payout_details": map[string]string{"email": "user@paypal.test"},
		}))
	assertStatus(t, rec, http.StatusCreated, "create withdrawal")

	var created struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create-withdrawal: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("create withdrawal: expected non-zero ID")
	}
	if created.Status != "pending" {
		t.Errorf("create withdrawal: want status %q, got %q", "pending", created.Status)
	}

	// Step 2 — user lists their own withdrawals; exactly one pending entry.
	rec = doRequest(t, h, http.MethodGet, "/api/v1/withdrawals", userToken, nil)
	assertStatus(t, rec, http.StatusOK, "list mine")
	var listed []struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list-withdrawals: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("list withdrawals: expected 1, got %d", len(listed))
	}

	// Step 3 — admin sees it in the pending queue.
	rec = doRequest(t, h, http.MethodGet, "/api/v1/admin/withdrawals/pending", adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "admin list pending")
	var pending []struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&pending); err != nil {
		t.Fatalf("decode admin pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("admin pending: expected 1, got %d", len(pending))
	}

	// Step 4 — admin approves.
	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/withdrawals/%d/approve", created.ID),
		adminToken, jsonBody(t, map[string]any{"notes": "looks good"}))
	assertStatus(t, rec, http.StatusOK, "admin approve")
	var approved struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&approved); err != nil {
		t.Fatalf("decode approve response: %v", err)
	}
	if approved.Status != "approved" {
		t.Errorf("approve: want status %q, got %q", "approved", approved.Status)
	}

	// Step 5 — admin processes; funds are committed.
	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/withdrawals/%d/process", created.ID),
		adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "admin process")
	var processed struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&processed); err != nil {
		t.Fatalf("decode process response: %v", err)
	}
	if processed.Status != "processed" {
		t.Errorf("process: want status %q, got %q", "processed", processed.Status)
	}
}

// ── TestE2E_WithdrawalLifecycle_RejectReleasesBalance ─────────────────────────
//
// Verifies that rejecting a withdrawal releases the reserved balance so the
// user's available balance is fully restored:
//
//  1. User submits withdrawal (20 000 cents reserved).
//  2. GET /users/me/balance confirms available = total - reserved.
//  3. Admin rejects with notes.
//  4. GET /users/me/balance confirms available == total (reservation released).
func TestE2E_WithdrawalLifecycle_RejectReleasesBalance(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "admin@e2e.test", "e2e-admin", domain.RoleAdmin)
	userID := seedE2EUser(t, "user@e2e.test", "e2e-user", domain.RoleUser)
	const totalBalance = 50_000
	const withdrawalAmount = 20_000
	seedE2EBalance(t, userID, totalBalance)
	setE2EKYCTier(t, userID, domain.KYCTierTwo)

	adminToken := signJWT("e2e-admin")
	userToken := signJWT("e2e-user")

	// Step 1 — submit withdrawal; amount is reserved.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/withdrawals", userToken,
		jsonBody(t, map[string]any{
			"amount_cents": withdrawalAmount,
			"currency":     "GTQ",
			"method":       "paypal",
		}))
	assertStatus(t, rec, http.StatusCreated, "create withdrawal")
	var created struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create-withdrawal: %v", err)
	}

	// Step 2 — reservation reduces available balance.
	rec = doRequest(t, h, http.MethodGet, "/api/v1/users/me/balance", userToken, nil)
	assertStatus(t, rec, http.StatusOK, "get balance before reject")
	var beforeReject struct {
		AvailableCents int `json:"available_cents"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&beforeReject); err != nil {
		t.Fatalf("decode balance before reject: %v", err)
	}
	wantBefore := totalBalance - withdrawalAmount
	if beforeReject.AvailableCents != wantBefore {
		t.Errorf("available before reject: want %d, got %d", wantBefore, beforeReject.AvailableCents)
	}

	// Step 3 — admin rejects; notes are required.
	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/withdrawals/%d/reject", created.ID),
		adminToken, jsonBody(t, map[string]any{"notes": "bank account invalid"}))
	assertStatus(t, rec, http.StatusOK, "admin reject")
	var rejected struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&rejected); err != nil {
		t.Fatalf("decode reject response: %v", err)
	}
	if rejected.Status != "rejected" {
		t.Errorf("reject: want status %q, got %q", "rejected", rejected.Status)
	}

	// Step 4 — reservation is released; full balance restored.
	rec = doRequest(t, h, http.MethodGet, "/api/v1/users/me/balance", userToken, nil)
	assertStatus(t, rec, http.StatusOK, "get balance after reject")
	var afterReject struct {
		AvailableCents int `json:"available_cents"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&afterReject); err != nil {
		t.Fatalf("decode balance after reject: %v", err)
	}
	if afterReject.AvailableCents != totalBalance {
		t.Errorf("available after reject: want %d (fully restored), got %d",
			totalBalance, afterReject.AvailableCents)
	}
}

// ── TestE2E_GroupManagement_JoinApproveLeave ──────────────────────────────────
//
// Verifies the group membership lifecycle end-to-end:
//
//  1. Owner creates a free group (entry_fee=0).
//  2. Joiner requests membership via invite code → status=pending.
//  3. Owner approves the pending request → status=active.
//  4. GET /groups/{id}/members shows 2 members.
//  5. Joiner leaves the group.
//  6. GET /groups/{id}/members shows 1 member (owner only).
func TestE2E_GroupManagement_JoinApproveLeave(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "owner@e2e.test", "e2e-owner", domain.RoleUser)
	_ = seedE2EUser(t, "joiner@e2e.test", "e2e-joiner", domain.RoleUser)
	ownerToken := signJWT("e2e-owner")
	joinerToken := signJWT("e2e-joiner")

	// Step 1 — owner creates a free group and receives an invite code.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/groups", ownerToken,
		jsonBody(t, map[string]any{"name": "E2E Group", "entry_fee": 0, "currency": "GTQ"}))
	assertStatus(t, rec, http.StatusCreated, "create group")
	var group struct {
		ID         int    `json:"id"`
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&group); err != nil {
		t.Fatalf("decode create-group: %v", err)
	}
	if group.InviteCode == "" {
		t.Fatal("create group: expected non-empty invite_code")
	}

	// Step 2 — joiner requests membership; state is pending until approved.
	rec = doRequest(t, h, http.MethodPost, "/api/v1/groups/join", joinerToken,
		jsonBody(t, map[string]any{"invite_code": group.InviteCode}))
	assertStatus(t, rec, http.StatusOK, "join group")
	var membership struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&membership); err != nil {
		t.Fatalf("decode join-group: %v", err)
	}
	if membership.Status != "pending" {
		t.Errorf("join: want status %q, got %q", "pending", membership.Status)
	}

	// Step 3 — any active member (owner) approves the pending request.
	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/groups/%d/members/%d/approve", group.ID, membership.ID),
		ownerToken, nil)
	assertStatus(t, rec, http.StatusOK, "approve join")
	var approvedMember struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&approvedMember); err != nil {
		t.Fatalf("decode approve-join: %v", err)
	}
	if approvedMember.Status != "active" {
		t.Errorf("approve: want status %q, got %q", "active", approvedMember.Status)
	}

	// Step 4 — owner + joiner are both active members.
	rec = doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/api/v1/groups/%d/members", group.ID), ownerToken, nil)
	assertStatus(t, rec, http.StatusOK, "list members after join")
	var afterJoin struct {
		Data []struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&afterJoin); err != nil {
		t.Fatalf("decode list-members: %v", err)
	}
	if len(afterJoin.Data) != 2 {
		t.Errorf("list members: expected 2, got %d", len(afterJoin.Data))
	}

	// Step 5 — joiner self-removes via DELETE /groups/{id}/members/me.
	rec = doRequest(t, h, http.MethodDelete,
		fmt.Sprintf("/api/v1/groups/%d/members/me", group.ID), joinerToken, nil)
	assertStatus(t, rec, http.StatusNoContent, "leave group")

	// Step 6 — only the owner remains active; joiner's row has status="left".
	rec = doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/api/v1/groups/%d/members", group.ID), ownerToken, nil)
	assertStatus(t, rec, http.StatusOK, "list members after leave")
	var afterLeave struct {
		Data []struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&afterLeave); err != nil {
		t.Fatalf("decode list-members after leave: %v", err)
	}
	var activeAfterLeave int
	for _, m := range afterLeave.Data {
		if m.Status != "left" {
			activeAfterLeave++
		}
	}
	if activeAfterLeave != 1 {
		t.Errorf("list members after leave: expected 1 active member, got %d", activeAfterLeave)
	}
}

// ── TestE2E_LeaderboardRanking_ExactScoreBeatsOutcomeOnly ────────────────────
//
// Verifies that the group leaderboard ranks members correctly after scoring.
// Owner predicts the exact score (5 pts); user B predicts the correct outcome
// with a wrong margin (2 pts). Leaderboard must rank owner above user B.
//
//  1. Owner creates free group; user B joins and is approved.
//  2. Admin creates a group-stage match.
//  3. Owner predicts 2-1 (exact); user B predicts 3-1 (correct outcome).
//  4. Admin starts and finalises the match (result: 2-1).
//  5. GET /groups/{id}/leaderboard → owner rank 1 (5 pts), user B rank 2 (2 pts).
func TestE2E_LeaderboardRanking_ExactScoreBeatsOutcomeOnly(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "admin@e2e.test", "e2e-admin", domain.RoleAdmin)
	_ = seedE2EUser(t, "owner@e2e.test", "e2e-owner", domain.RoleUser)
	_ = seedE2EUser(t, "userb@e2e.test", "e2e-userb", domain.RoleUser)

	adminToken := signJWT("e2e-admin")
	ownerToken := signJWT("e2e-owner")
	userBToken := signJWT("e2e-userb")

	// Step 1 — owner creates free group; user B joins and is approved.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/groups", ownerToken,
		jsonBody(t, map[string]any{"name": "Leaderboard Group", "entry_fee": 0, "currency": "GTQ"}))
	assertStatus(t, rec, http.StatusCreated, "create group")
	var group struct {
		ID         int    `json:"id"`
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&group); err != nil {
		t.Fatalf("decode create-group: %v", err)
	}

	rec = doRequest(t, h, http.MethodPost, "/api/v1/groups/join", userBToken,
		jsonBody(t, map[string]any{"invite_code": group.InviteCode}))
	assertStatus(t, rec, http.StatusOK, "user B join")
	var memberB struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&memberB); err != nil {
		t.Fatalf("decode join: %v", err)
	}

	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/groups/%d/members/%d/approve", group.ID, memberB.ID),
		ownerToken, nil)
	assertStatus(t, rec, http.StatusOK, "approve user B")

	// Step 2 — admin creates a group-stage match.
	kickoff := time.Now().Add(2 * time.Hour).UTC()
	rec = doRequest(t, h, http.MethodPost, "/api/v1/matches", adminToken,
		jsonBody(t, map[string]any{
			"home_team":   "Mexico",
			"away_team":   "Canada",
			"phase":       "group_stage",
			"group_label": "A",
			"kickoff_at":  kickoff.Format(time.RFC3339Nano),
		}))
	assertStatus(t, rec, http.StatusCreated, "create match")
	var matchResp struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&matchResp); err != nil {
		t.Fatalf("decode create-match: %v", err)
	}

	// Step 3 — owner predicts exact score; user B predicts correct outcome only.
	rec = doRequest(t, h, http.MethodPost, "/api/v1/predictions", ownerToken,
		jsonBody(t, map[string]any{"match_id": matchResp.ID, "home_score": 2, "away_score": 1}))
	assertStatus(t, rec, http.StatusCreated, "owner predicts 2-1 (exact)")

	rec = doRequest(t, h, http.MethodPost, "/api/v1/predictions", userBToken,
		jsonBody(t, map[string]any{"match_id": matchResp.ID, "home_score": 3, "away_score": 1}))
	assertStatus(t, rec, http.StatusCreated, "user B predicts 3-1 (correct outcome)")

	// Step 4 — admin starts and finalises the match. The InMemoryBus delivers
	// MatchFinished synchronously so scoring is complete before UpdateResult returns.
	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/matches/%d/start", matchResp.ID), adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "start match")

	rec = doRequest(t, h, http.MethodPatch,
		fmt.Sprintf("/api/v1/matches/%d", matchResp.ID), adminToken,
		jsonBody(t, map[string]any{"home_score": 2, "away_score": 1}))
	assertStatus(t, rec, http.StatusOK, "set result 2-1")

	// Step 5 — verify leaderboard ordering.
	rec = doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/api/v1/groups/%d/leaderboard", group.ID), ownerToken, nil)
	assertStatus(t, rec, http.StatusOK, "get leaderboard")

	var lb struct {
		Entries []struct {
			Rank        int `json:"rank"`
			TotalPoints int `json:"total_points"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&lb); err != nil {
		t.Fatalf("decode leaderboard: %v — body: %s", err, rec.Body.String())
	}
	if len(lb.Entries) < 2 {
		t.Fatalf("leaderboard: expected ≥2 entries, got %d", len(lb.Entries))
	}
	if lb.Entries[0].Rank != 1 {
		t.Errorf("entry[0].rank: want 1, got %d", lb.Entries[0].Rank)
	}
	if lb.Entries[0].TotalPoints != domain.PointsExactScore {
		t.Errorf("rank-1 points: want %d (exact score), got %d",
			domain.PointsExactScore, lb.Entries[0].TotalPoints)
	}
	if lb.Entries[1].Rank != 2 {
		t.Errorf("entry[1].rank: want 2, got %d", lb.Entries[1].Rank)
	}
	if lb.Entries[1].TotalPoints != domain.PointsCorrectOutcome {
		t.Errorf("rank-2 points: want %d (correct outcome), got %d",
			domain.PointsCorrectOutcome, lb.Entries[1].TotalPoints)
	}
}

// ── TestE2E_Versioning_APIVersionHeaderPresentOnV1 ────────────────────────────
//
// Contract test: every response from a /api/v1 endpoint must carry the
// API-Version: v1 header. This guards against the header being accidentally
// dropped when middleware order changes or the subrouter is restructured.
//
// The test also asserts that no Deprecation header is present on active routes,
// confirming that Deprecated() is not applied globally.
func TestE2E_Versioning_APIVersionHeaderPresentOnV1(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "user@e2e.test", "e2e-user", domain.RoleUser)
	userToken := signJWT("e2e-user")

	// Probe with GET /api/v1/matches — a stable, open-read endpoint.
	rec := doRequest(t, h, http.MethodGet, "/api/v1/matches", userToken, nil)
	assertStatus(t, rec, http.StatusOK, "probe v1 endpoint")

	if got := rec.Header().Get("API-Version"); got != "v1" {
		t.Errorf("API-Version header: want %q, got %q — header must be present on all /api/v1 responses", "v1", got)
	}
	if got := rec.Header().Get("Deprecation"); got != "" {
		t.Errorf("Deprecation header must be absent on active routes; got %q", got)
	}
}
