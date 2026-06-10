package footballprovider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/footballprovider"
)

func fixturePayload(id int64, status string, home, away int) string {
	return `{"results":1,"errors":[],"response":[{"fixture":{"id":` +
		string(rune('0'+id%10)) + // simplification for static test IDs
		`,"date":"2026-06-14T20:00:00+00:00","status":{"short":"` + status + `","elapsed":null}},` +
		`"goals":{"home":` + string(rune('0'+home)) + `,"away":` + string(rune('0'+away)) + `}}]}`
}

// buildServer returns a test HTTP server that answers GET /fixtures.
func buildServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-apisports-key") == "" {
			http.Error(w, "missing key", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func newClient(t *testing.T, srv *httptest.Server) *footballprovider.APIFootballClient {
	t.Helper()
	return footballprovider.NewAPIFootballClient("test-key", srv.URL, srv.Client())
}

// buildJSON builds a fixture response JSON programmatically to avoid integer
// encoding bugs in the hand-crafted string helper above.
func buildFixtureJSON(id int64, status string, home, away *int) string {
	type fixtureResp struct {
		Results  int              `json:"results"`
		Errors   []any            `json:"errors"`
		Response []map[string]any `json:"response"`
	}
	goals := map[string]any{"home": home, "away": away}
	fixture := map[string]any{
		"fixture": map[string]any{
			"id":   id,
			"date": "2026-06-14T20:00:00+00:00",
			"status": map[string]any{
				"short":   status,
				"elapsed": nil,
			},
		},
		"goals": goals,
	}
	r := fixtureResp{Results: 1, Errors: []any{}, Response: []map[string]any{fixture}}
	b, _ := json.Marshal(r)
	return string(b)
}

func intPtr(n int) *int { return &n }

func TestGetFixture_FullTime_DecodesCorrectly(t *testing.T) {
	body := buildFixtureJSON(867234, "FT", intPtr(2), intPtr(1))
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fix, err := newClient(t, srv).GetFixture(context.Background(), 867234)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix.ExternalID != 867234 {
		t.Errorf("ExternalID: got %d, want 867234", fix.ExternalID)
	}
	if fix.Status != footballprovider.StatusFullTime {
		t.Errorf("Status: got %q, want FT", fix.Status)
	}
	if fix.HomeScore != 2 || fix.AwayScore != 1 {
		t.Errorf("Score: got %d-%d, want 2-1", fix.HomeScore, fix.AwayScore)
	}
}

func TestGetFixture_AfterPenalties_DecodesAETPEN(t *testing.T) {
	body := buildFixtureJSON(100, "PEN", intPtr(1), intPtr(1))
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fix, err := newClient(t, srv).GetFixture(context.Background(), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix.Status != footballprovider.StatusAfterPEN {
		t.Errorf("Status: got %q, want PEN", fix.Status)
	}
}

func TestGetFixture_NotFound_ReturnsError(t *testing.T) {
	body := `{"results":0,"errors":[],"response":[]}`
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	_, err := newClient(t, srv).GetFixture(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for empty response, got nil")
	}
}

func TestGetFixture_RateLimit_Returns429Error(t *testing.T) {
	srv := buildServer(t, http.StatusTooManyRequests, `{}`)
	defer srv.Close()

	_, err := newClient(t, srv).GetFixture(context.Background(), 1)
	if err == nil {
		t.Fatal("expected rate-limit error, got nil")
	}
}

func TestGetFixture_APIErrorField_ReturnsError(t *testing.T) {
	body := `{"results":0,"errors":{"requests":"You have reached the request limit for the current Subscription Plan"},"response":[]}`
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	_, err := newClient(t, srv).GetFixture(context.Background(), 1)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

func TestGetLiveFixtures_ReturnsMultiple(t *testing.T) {
	type apiResp struct {
		Results  int              `json:"results"`
		Errors   []any            `json:"errors"`
		Response []map[string]any `json:"response"`
	}
	makeItem := func(id int64, status string) map[string]any {
		h, a := 1, 0
		return map[string]any{
			"fixture": map[string]any{
				"id": id, "date": "2026-06-14T20:00:00+00:00",
				"status": map[string]any{"short": status, "elapsed": 45},
			},
			"goals": map[string]any{"home": &h, "away": &a},
		}
	}
	r := apiResp{
		Results: 2, Errors: []any{},
		Response: []map[string]any{makeItem(10, "1H"), makeItem(11, "2H")},
	}
	b, _ := json.Marshal(r)
	srv := buildServer(t, http.StatusOK, string(b))
	defer srv.Close()

	fixtures, err := newClient(t, srv).GetLiveFixtures(context.Background(), 1, 2026)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixtures) != 2 {
		t.Fatalf("got %d fixtures, want 2", len(fixtures))
	}
	if !fixtures[0].Status.IsLive() {
		t.Errorf("fixture[0] Status %q should be live", fixtures[0].Status)
	}
}

func TestFixtureStatus_IsLive(t *testing.T) {
	live := []footballprovider.FixtureStatus{
		footballprovider.StatusFirstHalf,
		footballprovider.StatusHalfTime,
		footballprovider.StatusSecondHalf,
		footballprovider.StatusExtraTime,
		footballprovider.StatusPenLive,
	}
	for _, s := range live {
		if !s.IsLive() {
			t.Errorf("expected %q to be live", s)
		}
	}
	notLive := []footballprovider.FixtureStatus{
		footballprovider.StatusNotStarted,
		footballprovider.StatusFullTime,
		footballprovider.StatusAfterET,
		footballprovider.StatusAfterPEN,
		footballprovider.StatusCancelled,
	}
	for _, s := range notLive {
		if s.IsLive() {
			t.Errorf("expected %q to NOT be live", s)
		}
	}
}

func TestGetFixture_ContextCancellation_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := footballprovider.NewAPIFootballClient("k", srv.URL, srv.Client()).GetFixture(ctx, 1)
	if err == nil {
		t.Fatal("expected context-cancelled error, got nil")
	}
}
