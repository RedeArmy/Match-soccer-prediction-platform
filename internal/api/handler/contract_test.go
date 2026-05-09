package handler_test

// contract_test.go enforces JSON field-name stability for every public
// response type. A rename of a json tag (e.g. "home_score" → "homeScore")
// breaks all existing API clients silently — no compiler error, no other
// test failure. The table below is the authoritative record of the API
// contract; any intentional rename must update this table and triggers a
// corresponding version bump.
//
// How it works: each response struct is marshalled to JSON and the resulting
// object keys are compared against the golden set. Extra fields in the struct
// are not an error (additive changes are backwards-compatible); missing fields
// are fatal (removals and renames break clients).

import (
	"encoding/json"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
)

type contractCase struct {
	name     string
	value    any
	wantKeys []string // required JSON keys; subset check (additions are OK)
}

var contractCases = []contractCase{
	{
		name: "MatchResponse",
		value: handler.MatchResponse{
			ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina",
			Status: "scheduled", Phase: "group_stage",
			KickoffAt: "2026-06-01T20:00:00Z",
			CreatedAt: "2026-01-01T00:00:00Z",
			UpdatedAt: "2026-01-01T00:00:00Z",
		},
		wantKeys: []string{
			"id", "home_team", "away_team", "home_score", "away_score",
			"status", "phase", "stadium_id", "kickoff_at", "created_at", "updated_at",
		},
	},
	{
		name: "PredictionResponse",
		value: handler.PredictionResponse{
			ID: 1, UserID: 2, MatchID: 3,
			HomeScore: 2, AwayScore: 1,
			CreatedAt: "2026-01-01T00:00:00Z",
			UpdatedAt: "2026-01-01T00:00:00Z",
		},
		wantKeys: []string{
			"id", "user_id", "match_id", "home_score", "away_score",
			"points", "created_at", "updated_at",
		},
	},
	{
		name: "GroupResponse",
		value: handler.GroupResponse{
			ID: 1, Name: "Mi Quiniela", OwnerID: 5,
			InviteCode: "ABCDEFGHIJ", Status: "active",
			EntryFee: 100, Currency: "GTQ",
			CreatedAt: "2026-01-01T00:00:00Z",
			UpdatedAt: "2026-01-01T00:00:00Z",
		},
		wantKeys: []string{
			"id", "name", "owner_user_id", "invite_code", "invite_code_expires_at",
			"status", "entry_fee", "currency", "created_at", "updated_at",
		},
	},
	{
		name: "MemberResponse",
		value: handler.MemberResponse{
			ID: 1, QuinielaID: 2, UserID: 3,
			Role: "member", Status: "active", Paid: true,
			CreatedAt: "2026-01-01T00:00:00Z",
			UpdatedAt: "2026-01-01T00:00:00Z",
		},
		wantKeys: []string{
			"id", "quiniela_id", "user_id", "role", "status",
			"paid", "joined_at", "created_at", "updated_at",
		},
	},
	{
		name: "LeaderboardEntryResponse",
		value: handler.LeaderboardEntryResponse{
			Rank: 1, UserID: 2, UserName: "eder",
			TotalPoints: 42, PrizeWinner: true,
		},
		wantKeys: []string{"rank", "user_id", "user_name", "total_points", "prize_winner"},
	},
	{
		name: "LeaderboardResponse",
		value: handler.LeaderboardResponse{
			QuinielaID: 1, ActivePaidMembers: 5,
			WinnerCount: 2, EligibleForPrizes: true,
			Entries: []handler.LeaderboardEntryResponse{},
		},
		wantKeys: []string{
			"quiniela_id", "active_paid_members", "winner_count",
			"eligible_for_prizes", "entries",
		},
	},
	{
		name: "UserStatsResponse",
		value: handler.UserStatsResponse{
			TotalPredictions: 10, ScoredPredictions: 8,
			CorrectPredictions: 5, ExactPredictions: 2,
			TotalPoints: 18, AccuracyPct: 62.5,
			AvgPointsPerPred: 2.25,
			CurrentStreak:    3, LongestStreak: 5,
			PointsByPhase: map[string]int{"group_stage": 18},
		},
		wantKeys: []string{
			"total_predictions", "scored_predictions", "correct_predictions",
			"exact_predictions", "total_points", "points_by_phase",
			"accuracy_pct", "avg_points_per_prediction",
			"current_streak", "longest_streak",
		},
	},
	{
		name: "SystemParamResponse",
		value: handler.SystemParamResponse{
			Key: "scoring.exact_score", Value: "5", DefaultValue: "5",
			Type: "int", Category: "scoring", IsRuntime: true,
			Description: "Points for exact score",
			UpdatedAt:   "2026-01-01T00:00:00Z",
		},
		wantKeys: []string{
			"key", "value", "default_value", "type", "category",
			"is_runtime", "description", "updated_at",
		},
	},
	{
		name:     "ErrorResponse",
		value:    handler.ErrorResponse{Error: handler.ErrorDetail{Code: "validation_error", Message: "something went wrong"}},
		wantKeys: []string{"error"},
	},
}

// TestResponseContractFieldNames marshals each response type and asserts every
// documented JSON key is present. Fails immediately on a missing key so the
// error points to the exact field that was renamed or removed.
func TestResponseContractFieldNames(t *testing.T) {
	for _, tc := range contractCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(data, &obj); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			for _, key := range tc.wantKeys {
				if _, ok := obj[key]; !ok {
					t.Errorf("field %q missing from JSON output — rename detected; update the contract table and bump the API version", key)
				}
			}
		})
	}
}
