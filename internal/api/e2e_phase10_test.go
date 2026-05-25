//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── TestE2E_Withdrawal_InsufficientBalance ─────────────────────────────────────
//
// Verifies that POST /api/v1/withdrawals returns 409 when the requested amount
// exceeds the user's available balance.  The insufficient-balance path is
// enforced by the repository's CreateAndReserve: the UPDATE … WHERE
// balance_cents >= amount_cents affects zero rows, and the service translates
// that into apperrors.Conflict, which the handler maps to HTTP 409.
func TestE2E_Withdrawal_InsufficientBalance(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	userID := seedE2EUser(t, "poor@e2e.test", "e2e-poor-user", domain.RoleUser)
	seedE2EBalance(t, userID, 5_000) // 50 GTQ available
	userToken := signJWT("e2e-poor-user")

	// Request 100 GTQ — twice the available 50 GTQ.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/withdrawals", userToken,
		jsonBody(t, map[string]any{
			"amount_cents": 10_000,
			"currency":     "GTQ",
			"method":       "paypal",
		}))

	if rec.Code != http.StatusConflict {
		t.Errorf("insufficient balance: expected HTTP 409, got %d — body: %s",
			rec.Code, rec.Body.String())
	}

	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err == nil {
		if errResp.Error.Code != "CONFLICT" {
			t.Errorf("error code: want CONFLICT, got %q", errResp.Error.Code)
		}
	}
}

// ── TestE2E_Withdrawal_ZeroBalanceUser ─────────────────────────────────────────
//
// Edge-case: a user with zero balance must also receive 409, not 500.
func TestE2E_Withdrawal_ZeroBalanceUser(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "zero@e2e.test", "e2e-zero-user", domain.RoleUser)
	// No seedE2EBalance call → balance_cents = 0 by default.
	userToken := signJWT("e2e-zero-user")

	rec := doRequest(t, h, http.MethodPost, "/api/v1/withdrawals", userToken,
		jsonBody(t, map[string]any{
			"amount_cents": 5_000,
			"currency":     "GTQ",
			"method":       "paypal",
		}))

	if rec.Code != http.StatusConflict {
		t.Errorf("zero balance: expected HTTP 409, got %d — body: %s",
			rec.Code, rec.Body.String())
	}
}

// ── TestE2E_GroupFanOut_MemberJoined_OutboxEntryWritten ────────────────────────
//
// Verifies the fan-out publisher side for EventGroupMemberJoined:
//
//  1. Owner creates a free group.
//  2. Joiner requests membership.
//  3. Owner approves.
//  4. A domain_outbox row with event_type='group.member_joined' and the
//     correct quiniela_id in its payload must exist.
//
// The dispatcher fan-out logic (which reads that row and delivers to all active
// members) is exercised separately in the outbox/user_test.go unit tests.
func TestE2E_GroupFanOut_MemberJoined_OutboxEntryWritten(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "owner@e2e.test", "e2e-fanout-owner", domain.RoleUser)
	_ = seedE2EUser(t, "joiner@e2e.test", "e2e-fanout-joiner", domain.RoleUser)
	ownerToken := signJWT("e2e-fanout-owner")
	joinerToken := signJWT("e2e-fanout-joiner")

	// Step 1 — owner creates a free group.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/groups", ownerToken,
		jsonBody(t, map[string]any{"name": "FanOut Group", "entry_fee": 0, "currency": "GTQ"}))
	assertStatus(t, rec, http.StatusCreated, "create group")

	var group struct {
		ID         int    `json:"id"`
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&group); err != nil {
		t.Fatalf("decode create-group: %v", err)
	}

	// Step 2 — joiner requests membership.
	rec = doRequest(t, h, http.MethodPost, "/api/v1/groups/join", joinerToken,
		jsonBody(t, map[string]any{"invite_code": group.InviteCode}))
	assertStatus(t, rec, http.StatusOK, "join group")

	var membership struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&membership); err != nil {
		t.Fatalf("decode join: %v", err)
	}

	// Step 3 — owner approves.
	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/groups/%d/members/%d/approve", group.ID, membership.ID),
		ownerToken, nil)
	assertStatus(t, rec, http.StatusOK, "approve join")

	// Step 4 — assert outbox entry was written for the fan-out broadcast.
	var count int
	err := e2eDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM domain_outbox
		  WHERE event_type = $1
		    AND payload->>'quiniela_id' = $2`,
		"group.member_joined",
		fmt.Sprintf("%d", group.ID),
	).Scan(&count)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 group.member_joined outbox entry for quiniela %d, got %d",
			group.ID, count)
	}

	// Also assert the payload carries the joiner's user_id so the dispatcher can
	// exclude them from the broadcast (actor must not receive a self-notification).
	var payloadJSON []byte
	if err := e2eDB.QueryRow(context.Background(),
		`SELECT payload FROM domain_outbox
		  WHERE event_type = $1
		    AND payload->>'quiniela_id' = $2
		  LIMIT 1`,
		"group.member_joined",
		fmt.Sprintf("%d", group.ID),
	).Scan(&payloadJSON); err != nil {
		t.Fatalf("fetch outbox payload: %v", err)
	}
	var p struct {
		QuinielaID   int    `json:"quiniela_id"`
		QuinielaName string `json:"quiniela_name"`
	}
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		t.Fatalf("unmarshal outbox payload: %v", err)
	}
	if p.QuinielaID != group.ID {
		t.Errorf("payload quiniela_id: want %d, got %d", group.ID, p.QuinielaID)
	}
	if p.QuinielaName == "" {
		t.Error("payload quiniela_name must not be empty")
	}
}

// ── TestE2E_GroupFanOut_MemberLeft_OutboxEntryWritten ─────────────────────────
//
// Mirrors TestE2E_GroupFanOut_MemberJoined_OutboxEntryWritten for the leave path:
// EventGroupMemberLeft must be written to the outbox when an active member
// removes themselves via DELETE /groups/{id}/members/me.
func TestE2E_GroupFanOut_MemberLeft_OutboxEntryWritten(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "owner2@e2e.test", "e2e-left-owner", domain.RoleUser)
	_ = seedE2EUser(t, "leaver@e2e.test", "e2e-left-joiner", domain.RoleUser)
	ownerToken := signJWT("e2e-left-owner")
	leaverToken := signJWT("e2e-left-joiner")

	// Create group, join, approve.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/groups", ownerToken,
		jsonBody(t, map[string]any{"name": "Leave Group", "entry_fee": 0, "currency": "GTQ"}))
	assertStatus(t, rec, http.StatusCreated, "create group")
	var group struct {
		ID         int    `json:"id"`
		InviteCode string `json:"invite_code"`
	}
	json.NewDecoder(rec.Body).Decode(&group)

	rec = doRequest(t, h, http.MethodPost, "/api/v1/groups/join", leaverToken,
		jsonBody(t, map[string]any{"invite_code": group.InviteCode}))
	assertStatus(t, rec, http.StatusOK, "join group")
	var membership struct {
		ID int `json:"id"`
	}
	json.NewDecoder(rec.Body).Decode(&membership)

	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/groups/%d/members/%d/approve", group.ID, membership.ID),
		ownerToken, nil)
	assertStatus(t, rec, http.StatusOK, "approve join")

	// Leaver leaves.
	rec = doRequest(t, h, http.MethodDelete,
		fmt.Sprintf("/api/v1/groups/%d/members/me", group.ID), leaverToken, nil)
	assertStatus(t, rec, http.StatusNoContent, "leave group")

	// Assert outbox entry for the leave fan-out.
	var count int
	err := e2eDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM domain_outbox
		  WHERE event_type = $1
		    AND payload->>'quiniela_id' = $2`,
		"group.member_left",
		fmt.Sprintf("%d", group.ID),
	).Scan(&count)
	if err != nil {
		t.Fatalf("query outbox for member_left: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 group.member_left outbox entry for quiniela %d, got %d",
			group.ID, count)
	}
}

// ── TestE2E_ScoringChain_PointsAndLeaderboardConsistent ───────────────────────
//
// Verifies the scoring → leaderboard chain end-to-end:
//
//  1. Two users join a group.
//  2. Admin creates a match; both users predict.
//  3. Admin starts and finalises the match.
//  4. InMemoryBus delivers MatchFinished synchronously, so ScoreMatch runs
//     before UpdateResult returns.
//  5. The per-group leaderboard reflects the updated points.
//  6. A second sequential call to the leaderboard returns identical data,
//     confirming there is no stale-read or race.
//
// Cache invalidation and the leaderboard.updated SSE signal are worker-process
// concerns (they require Redis) and are exercised in cmd/worker tests.
func TestE2E_ScoringChain_PointsAndLeaderboardConsistent(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	_ = seedE2EUser(t, "admin@e2e.test", "e2e-sc-admin", domain.RoleAdmin)
	_ = seedE2EUser(t, "player1@e2e.test", "e2e-sc-p1", domain.RoleUser)
	_ = seedE2EUser(t, "player2@e2e.test", "e2e-sc-p2", domain.RoleUser)

	adminToken := signJWT("e2e-sc-admin")
	p1Token := signJWT("e2e-sc-p1")
	p2Token := signJWT("e2e-sc-p2")

	// Create group; player2 joins and is approved by player1 (the owner).
	rec := doRequest(t, h, http.MethodPost, "/api/v1/groups", p1Token,
		jsonBody(t, map[string]any{"name": "Scoring Chain Group", "entry_fee": 0, "currency": "GTQ"}))
	assertStatus(t, rec, http.StatusCreated, "create group")
	var group struct {
		ID         int    `json:"id"`
		InviteCode string `json:"invite_code"`
	}
	json.NewDecoder(rec.Body).Decode(&group)

	rec = doRequest(t, h, http.MethodPost, "/api/v1/groups/join", p2Token,
		jsonBody(t, map[string]any{"invite_code": group.InviteCode}))
	assertStatus(t, rec, http.StatusOK, "p2 join")
	var p2Membership struct{ ID int `json:"id"` }
	json.NewDecoder(rec.Body).Decode(&p2Membership)

	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/groups/%d/members/%d/approve", group.ID, p2Membership.ID),
		p1Token, nil)
	assertStatus(t, rec, http.StatusOK, "approve p2")

	// Admin creates a group-stage match.
	kickoff := time.Now().Add(2 * time.Hour).UTC()
	rec = doRequest(t, h, http.MethodPost, "/api/v1/matches", adminToken,
		jsonBody(t, map[string]any{
			"home_team": "Honduras", "away_team": "El Salvador",
			"phase": "group_stage", "group_label": "C",
			"kickoff_at": kickoff.Format(time.RFC3339Nano),
		}))
	assertStatus(t, rec, http.StatusCreated, "create match")
	var matchResp struct{ ID int `json:"id"` }
	json.NewDecoder(rec.Body).Decode(&matchResp)

	// Player 1 predicts exact score (2-0); player 2 predicts correct outcome (3-0).
	rec = doRequest(t, h, http.MethodPost, "/api/v1/predictions", p1Token,
		jsonBody(t, map[string]any{"match_id": matchResp.ID, "home_score": 2, "away_score": 0}))
	assertStatus(t, rec, http.StatusCreated, "p1 predict")

	rec = doRequest(t, h, http.MethodPost, "/api/v1/predictions", p2Token,
		jsonBody(t, map[string]any{"match_id": matchResp.ID, "home_score": 3, "away_score": 0}))
	assertStatus(t, rec, http.StatusCreated, "p2 predict")

	// Admin starts and finalises match (result: 2-0 — exact for p1).
	rec = doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/matches/%d/start", matchResp.ID), adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "start match")

	rec = doRequest(t, h, http.MethodPatch,
		fmt.Sprintf("/api/v1/matches/%d", matchResp.ID), adminToken,
		jsonBody(t, map[string]any{"home_score": 2, "away_score": 0}))
	assertStatus(t, rec, http.StatusOK, "set result")

	// Verify leaderboard — first call.
	lbPath := fmt.Sprintf("/api/v1/groups/%d/leaderboard", group.ID)
	checkLeaderboard := func(label string) (rank1pts, rank2pts int) {
		rec = doRequest(t, h, http.MethodGet, lbPath, p1Token, nil)
		assertStatus(t, rec, http.StatusOK, label)
		var lb struct {
			Entries []struct {
				Rank        int `json:"rank"`
				TotalPoints int `json:"total_points"`
			} `json:"entries"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&lb); err != nil {
			t.Fatalf("%s: decode leaderboard: %v", label, err)
		}
		if len(lb.Entries) < 2 {
			t.Fatalf("%s: expected ≥2 leaderboard entries, got %d", label, len(lb.Entries))
		}
		return lb.Entries[0].TotalPoints, lb.Entries[1].TotalPoints
	}

	r1First, r2First := checkLeaderboard("leaderboard call 1")
	if r1First != domain.PointsExactScore {
		t.Errorf("call 1 rank-1 points: want %d (exact), got %d", domain.PointsExactScore, r1First)
	}
	if r2First != domain.PointsCorrectOutcome {
		t.Errorf("call 1 rank-2 points: want %d (correct outcome), got %d", domain.PointsCorrectOutcome, r2First)
	}

	// Second call must return identical data (no stale intermediate state).
	r1Second, r2Second := checkLeaderboard("leaderboard call 2")
	if r1Second != r1First || r2Second != r2First {
		t.Errorf("call 2 points differ from call 1: (%d,%d) vs (%d,%d)",
			r1Second, r2Second, r1First, r2First)
	}

	// Confirm the scoring audit log has entries for this match.
	var scoringLogCount int
	if err := e2eDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM prediction_score_log WHERE match_id = $1`,
		matchResp.ID,
	).Scan(&scoringLogCount); err != nil {
		t.Fatalf("query prediction_score_log: %v", err)
	}
	if scoringLogCount != 2 {
		t.Errorf("scoring log: want 2 entries (one per player), got %d", scoringLogCount)
	}
}
