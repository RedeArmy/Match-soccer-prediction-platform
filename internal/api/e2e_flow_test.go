package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/migrations"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// e2eDB is shared across E2E tests. It is nil when Docker is unavailable,
// in which case every E2E test skips itself via skipIfNoE2EDB.
var e2eDB *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := trySetupE2EDB()
	e2eDB = pool
	code := m.Run()
	if cleanup != nil {
		cleanup()
	}
	os.Exit(code)
}

// trySetupE2EDB starts a PostgreSQL testcontainer and runs migrations. It
// returns (nil, nil) when Docker is unavailable so that callers can skip
// gracefully instead of failing. A non-nil cleanup must be called on process exit.
func trySetupE2EDB() (*pgxpool.Pool, func()) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("quiniela_e2e"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		log.Printf("e2e DB unavailable (Docker not running?): %v — E2E tests will be skipped", err)
		return nil, nil
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("e2e container connection string: %v", err)
	}
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		log.Fatalf("e2e DB migrate: %v", err)
	}

	pool, err := database.NewPool(ctx, database.Config{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		log.Fatalf("e2e DB pool: %v", err)
	}

	cleanup := func() {
		pool.Close()
		if err := container.Terminate(ctx); err != nil {
			log.Printf("e2e container terminate: %v", err)
		}
	}
	return pool, cleanup
}

// skipIfNoE2EDB skips the test when no containerised database is available.
func skipIfNoE2EDB(t *testing.T) {
	t.Helper()
	if e2eDB == nil {
		t.Skip("e2e DB unavailable (Docker not running) — skipping E2E test")
	}
}

// cleanE2ETables truncates every application table so each E2E test begins
// with an empty database. RESTART IDENTITY resets serial sequences.
func cleanE2ETables(t *testing.T) {
	t.Helper()
	_, err := e2eDB.Exec(context.Background(),
		`TRUNCATE leaderboard_snapshots, payment_records, audit_log, system_params,
		         tournament_slots, tiebreaker_config, group_memberships, tiebreakers,
		         predictions, quinielas, matches, stadiums, users RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("clean e2e tables: %v", err)
	}
}

// newE2EServer builds a full-featured Server wired to e2eDB. The JWKS URL
// enables RequireAuth so tokens produced by testJWKSServer are accepted.
func newE2EServer(t *testing.T, jwksURL string) *api.Server {
	t.Helper()
	cfg := &config.Config{}
	cfg.Clerk.JWKSURL = jwksURL
	return api.New(e2eDB, cfg, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
}

// seedE2EUser inserts a user with the given clerk subject and role directly
// into e2eDB and returns the new user ID.
func seedE2EUser(t *testing.T, email, clerkSubject string, role domain.UserRole) int {
	t.Helper()
	var id int
	err := e2eDB.QueryRow(context.Background(),
		`INSERT INTO users (name, email, role, clerk_subject) VALUES ($1, $2, $3, $4) RETURNING id`,
		email, email, string(role), clerkSubject,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed e2e user %q: %v", clerkSubject, err)
	}
	return id
}

// doRequest is a thin helper that fires one HTTP request through the router
// and returns the response recorder. Content-Type is set to JSON when body is non-nil.
func doRequest(t *testing.T, h http.Handler, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// jsonBody marshals v to JSON or fatals the test.
func jsonBody(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return b
}

// assertStatus fatals when the recorder does not carry the expected HTTP status.
func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int, label string) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("%s: expected HTTP %d, got %d — body: %s", label, want, rec.Code, rec.Body.String())
	}
}

// ── E2E Tests ─────────────────────────────────────────────────────────────────

// TestE2E_PredictionFlow_FullCycle verifies the canonical happy path end-to-end:
//
//  1. Admin creates a group-stage match with kickoff in the future.
//  2. Regular user submits an exact-score prediction (2-1).
//  3. Admin transitions the match to Live (StartMatch).
//  4. Admin sets the final result (2-1), publishing MatchFinished.
//  5. The InMemoryBus delivers MatchFinished synchronously; ScoreMatch runs
//     before UpdateResult returns, so points are in the DB immediately.
//  6. Admin fetches predictions for the match — asserts points == 5 (exact score).
func TestE2E_PredictionFlow_FullCycle(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServer(t, jwksURL).Routes()

	_ = seedE2EUser(t, "admin@e2e.test", "e2e-admin", domain.RoleAdmin)
	_ = seedE2EUser(t, "user@e2e.test", "e2e-user", domain.RoleUser)
	adminToken := signJWT("e2e-admin")
	userToken := signJWT("e2e-user")

	// Step 1 — admin creates a group-stage match.
	groupLabel := "A"
	kickoff := time.Now().Add(2 * time.Hour).UTC()
	rec := doRequest(t, h, http.MethodPost, "/api/v1/matches", adminToken,
		jsonBody(t, map[string]any{
			"home_team":   "Brazil",
			"away_team":   "Argentina",
			"phase":       "group_stage",
			"group_label": groupLabel,
			"kickoff_at":  kickoff.Format(time.RFC3339Nano),
		}))
	assertStatus(t, rec, http.StatusCreated, "create match")

	var matchResp struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&matchResp); err != nil {
		t.Fatalf("decode create-match response: %v", err)
	}
	matchID := matchResp.ID
	if matchID == 0 {
		t.Fatal("create match: expected non-zero ID")
	}

	// Step 2 — user submits an exact prediction (2-1).
	rec = doRequest(t, h, http.MethodPost, "/api/v1/predictions", userToken,
		jsonBody(t, map[string]any{
			"match_id":   matchID,
			"home_score": 2,
			"away_score": 1,
		}))
	assertStatus(t, rec, http.StatusCreated, "submit prediction")

	// Step 3 — admin starts the match (Scheduled → Live).
	rec = doRequest(t, h, http.MethodPost, fmt.Sprintf("/api/v1/matches/%d/start", matchID), adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "start match")

	// Step 4 — admin updates the result (2-1); MatchFinished event triggers scoring.
	home, away := 2, 1
	rec = doRequest(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/matches/%d", matchID), adminToken,
		jsonBody(t, map[string]any{
			"home_score": home,
			"away_score": away,
		}))
	assertStatus(t, rec, http.StatusOK, "update result")

	// Step 5 — verify points via admin predictions list.
	// InMemoryBus is synchronous: scoring completed before UpdateResult returned.
	rec = doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/predictions?match_id=%d", matchID),
		adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "list predictions")

	var predsResp struct {
		Data []struct {
			Points *int `json:"points"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&predsResp); err != nil {
		t.Fatalf("decode predictions response: %v", err)
	}
	if len(predsResp.Data) != 1 {
		t.Fatalf("expected 1 prediction, got %d", len(predsResp.Data))
	}
	if predsResp.Data[0].Points == nil {
		t.Fatal("expected points to be set after scoring")
	}
	// Exact score for group stage → domain.PointsExactScore (5).
	if got := *predsResp.Data[0].Points; got != domain.PointsExactScore {
		t.Errorf("points: got %d, want %d (exact score)", got, domain.PointsExactScore)
	}
}

// TestE2E_ListAdmin_Filters exercises the whereBuilder-refactored ListAdmin
// with user_id and match_id filters, verifying both filtering correctness and
// the absence of the WHERE 1=1 / manual-counter pattern regressions.
func TestE2E_ListAdmin_Filters(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, signJWT := testJWKSServer(t)
	h := newE2EServer(t, jwksURL).Routes()

	_ = seedE2EUser(t, "admin@e2e.test", "e2e-admin", domain.RoleAdmin)
	userAID := seedE2EUser(t, "user-a@e2e.test", "e2e-user-a", domain.RoleUser)
	_ = seedE2EUser(t, "user-b@e2e.test", "e2e-user-b", domain.RoleUser)

	adminToken := signJWT("e2e-admin")
	userAToken := signJWT("e2e-user-a")
	userBToken := signJWT("e2e-user-b")

	kickoff := time.Now().Add(2 * time.Hour).UTC()

	// Create two matches (different teams) so we can verify match_id isolation.
	type matchFixture struct {
		home, away, label string
		offset            time.Duration
	}
	fixtures := []matchFixture{
		{"France", "Spain", "B", 0},
		{"Germany", "Italy", "C", time.Hour},
	}
	matchIDs := make([]int, len(fixtures))
	for i, f := range fixtures {
		rec := doRequest(t, h, http.MethodPost, "/api/v1/matches", adminToken,
			jsonBody(t, map[string]any{
				"home_team": f.home, "away_team": f.away,
				"phase": "group_stage", "group_label": f.label,
				"kickoff_at": kickoff.Add(f.offset).Format(time.RFC3339Nano),
			}))
		assertStatus(t, rec, http.StatusCreated, "create match")
		var m struct {
			ID int `json:"id"`
		}
		json.NewDecoder(rec.Body).Decode(&m)
		matchIDs[i] = m.ID
	}

	// Both users predict on match 0; only user A predicts on match 1.
	for _, tc := range []struct {
		token      string
		matchIdx   int
		home, away int
	}{
		{userAToken, 0, 1, 0},
		{userBToken, 0, 2, 2},
		{userAToken, 1, 0, 1},
	} {
		rec := doRequest(t, h, http.MethodPost, "/api/v1/predictions", tc.token,
			jsonBody(t, map[string]any{
				"match_id":   matchIDs[tc.matchIdx],
				"home_score": tc.home,
				"away_score": tc.away,
			}))
		assertStatus(t, rec, http.StatusCreated, "submit prediction")
	}

	type pagedPreds struct {
		Data []struct {
			ID int `json:"id"`
		} `json:"data"`
	}

	// Filter by user A → 2 predictions (one per match).
	rec := doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/predictions?user_id=%d", userAID),
		adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "list by user_id")
	var byUser pagedPreds
	json.NewDecoder(rec.Body).Decode(&byUser)
	if len(byUser.Data) != 2 {
		t.Errorf("user_id filter: expected 2 predictions, got %d", len(byUser.Data))
	}

	// Filter by match 0 → 2 predictions (one per user).
	rec = doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/predictions?match_id=%d", matchIDs[0]),
		adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "list by match_id")
	var byMatch pagedPreds
	json.NewDecoder(rec.Body).Decode(&byMatch)
	if len(byMatch.Data) != 2 {
		t.Errorf("match_id filter: expected 2 predictions, got %d", len(byMatch.Data))
	}

	// Combined filter: user A + match 1 → exactly 1 prediction.
	rec = doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/api/v1/admin/predictions?user_id=%d&match_id=%d", userAID, matchIDs[1]),
		adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "list by user_id+match_id")
	var byCombined pagedPreds
	json.NewDecoder(rec.Body).Decode(&byCombined)
	if len(byCombined.Data) != 1 {
		t.Errorf("combined filter: expected 1 prediction, got %d", len(byCombined.Data))
	}

	// No filters → 3 predictions total.
	rec = doRequest(t, h, http.MethodGet, "/api/v1/admin/predictions", adminToken, nil)
	assertStatus(t, rec, http.StatusOK, "list all")
	var all pagedPreds
	json.NewDecoder(rec.Body).Decode(&all)
	if len(all.Data) != 3 {
		t.Errorf("no filter: expected 3 predictions, got %d", len(all.Data))
	}
}
