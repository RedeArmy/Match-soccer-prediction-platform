package footballprovider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestNewAPIFootballClient_DefaultsApplied(t *testing.T) {
	c := footballprovider.NewAPIFootballClient("key", "", nil)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestGetFixture_UnexpectedStatus_ReturnsError(t *testing.T) {
	srv := buildServer(t, http.StatusInternalServerError, `{}`)
	defer srv.Close()

	_, err := newClient(t, srv).GetFixture(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestGetFixture_MalformedJSON_ReturnsError(t *testing.T) {
	srv := buildServer(t, http.StatusOK, `not-json`)
	defer srv.Close()

	_, err := newClient(t, srv).GetFixture(context.Background(), 1)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

func TestGetLiveFixtures_APIError_ReturnsError(t *testing.T) {
	body := `{"results":0,"errors":{"requests":"rate limit"},"response":[]}`
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	_, err := newClient(t, srv).GetLiveFixtures(context.Background(), 1, 2026)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

func TestFixtureStatus_IsFinished(t *testing.T) {
	finished := []footballprovider.FixtureStatus{
		footballprovider.StatusFullTime,
		footballprovider.StatusAfterET,
		footballprovider.StatusAfterPEN,
	}
	for _, s := range finished {
		if !s.IsFinished() {
			t.Errorf("expected %q to be finished", s)
		}
	}
	notFinished := []footballprovider.FixtureStatus{
		footballprovider.StatusNotStarted,
		footballprovider.StatusFirstHalf,
		footballprovider.StatusHalfTime,
		footballprovider.StatusPostponed,
		footballprovider.StatusCancelled,
	}
	for _, s := range notFinished {
		if s.IsFinished() {
			t.Errorf("expected %q to NOT be finished", s)
		}
	}
}

func TestFixtureStatus_IsCancelled(t *testing.T) {
	cancelled := []footballprovider.FixtureStatus{
		footballprovider.StatusPostponed,
		footballprovider.StatusCancelled,
		footballprovider.StatusAbandoned,
	}
	for _, s := range cancelled {
		if !s.IsCancelled() {
			t.Errorf("expected %q to be cancelled", s)
		}
	}
	notCancelled := []footballprovider.FixtureStatus{
		footballprovider.StatusNotStarted,
		footballprovider.StatusFullTime,
		footballprovider.StatusFirstHalf,
	}
	for _, s := range notCancelled {
		if s.IsCancelled() {
			t.Errorf("expected %q to NOT be cancelled", s)
		}
	}
}

func TestGetFixture_AllKnownStatuses_ParseCorrectly(t *testing.T) {
	known := []string{"NS", "1H", "HT", "2H", "ET", "PEN_LIVE", "FT", "AET", "PEN", "PST", "CANC", "ABD"}
	for _, code := range known {
		body := buildFixtureJSON(1, code, intPtr(0), intPtr(0))
		srv := buildServer(t, http.StatusOK, body)
		fix, err := newClient(t, srv).GetFixture(context.Background(), 1)
		srv.Close()
		if err != nil {
			t.Errorf("status %q: unexpected error: %v", code, err)
			continue
		}
		if string(fix.Status) != code {
			t.Errorf("status %q: got %q", code, fix.Status)
		}
	}
}

func TestGetFixture_UnknownStatus_NormalisedToUnknown(t *testing.T) {
	body := buildFixtureJSON(1, "WEIRD", intPtr(0), intPtr(0))
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fix, err := newClient(t, srv).GetFixture(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix.Status != footballprovider.StatusUnknown {
		t.Errorf("expected StatusUnknown, got %q", fix.Status)
	}
}

// ── GetFixturesByDate ─────────────────────────────────────────────────────────

func TestGetFixturesByDate_ReturnsAllFixtures(t *testing.T) {
	// Two fixtures on the same date; both should be returned.
	type resp struct {
		Results  int              `json:"results"`
		Errors   []any            `json:"errors"`
		Response []map[string]any `json:"response"`
	}
	makeItem := func(id int64, status string) map[string]any {
		return map[string]any{
			"fixture": map[string]any{
				"id":     id,
				"date":   "2026-06-14T20:00:00+00:00",
				"status": map[string]any{"short": status, "elapsed": nil},
			},
			"teams": map[string]any{
				"home": map[string]any{"name": "Mexico"},
				"away": map[string]any{"name": "Canada"},
			},
			"goals": map[string]any{"home": intPtr(1), "away": intPtr(0)},
		}
	}
	body, _ := json.Marshal(resp{
		Results: 2,
		Errors:  []any{},
		Response: []map[string]any{
			makeItem(101, "FT"),
			makeItem(102, "NS"),
		},
	})
	srv := buildServer(t, http.StatusOK, string(body))
	defer srv.Close()

	fixtures, err := newClient(t, srv).GetFixturesByDate(context.Background(), 1, 2026, "2026-06-14")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixtures) != 2 {
		t.Errorf("expected 2 fixtures, got %d", len(fixtures))
	}
	if fixtures[0].ExternalID != 101 {
		t.Errorf("first fixture ExternalID: got %d, want 101", fixtures[0].ExternalID)
	}
	if fixtures[1].Status != footballprovider.StatusNotStarted {
		t.Errorf("second fixture status: got %q, want NS", fixtures[1].Status)
	}
}

func TestGetFixturesByDate_EmptyDate_ReturnsEmpty(t *testing.T) {
	body := `{"results":0,"errors":[],"response":[]}`
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fixtures, err := newClient(t, srv).GetFixturesByDate(context.Background(), 1, 2026, "2026-06-15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixtures) != 0 {
		t.Errorf("expected 0 fixtures, got %d", len(fixtures))
	}
}

func TestGetFixturesByDate_RateLimit_ReturnsError(t *testing.T) {
	srv := buildServer(t, http.StatusTooManyRequests, "")
	defer srv.Close()

	_, err := newClient(t, srv).GetFixturesByDate(context.Background(), 1, 2026, "2026-06-14")
	if err == nil {
		t.Fatal("expected error on 429, got nil")
	}
}

func TestGetFixturesByDate_APIError_ReturnsError(t *testing.T) {
	body := `{"results":0,"errors":{"key":"invalid"},"response":[]}`
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	_, err := newClient(t, srv).GetFixturesByDate(context.Background(), 1, 2026, "2026-06-14")
	if err == nil {
		t.Fatal("expected error on API error field, got nil")
	}
}

// ── Team name decoding ────────────────────────────────────────────────────────

func buildFixtureJSONWithTeams(id int64, homeTeam, awayTeam string) string {
	type fixtureResp struct {
		Results  int              `json:"results"`
		Errors   []any            `json:"errors"`
		Response []map[string]any `json:"response"`
	}
	item := map[string]any{
		"fixture": map[string]any{
			"id":     id,
			"date":   "2026-06-14T20:00:00+00:00",
			"status": map[string]any{"short": "NS", "elapsed": nil},
		},
		"teams": map[string]any{
			"home": map[string]any{"name": homeTeam},
			"away": map[string]any{"name": awayTeam},
		},
		"goals": map[string]any{"home": nil, "away": nil},
	}
	r := fixtureResp{Results: 1, Errors: []any{}, Response: []map[string]any{item}}
	b, _ := json.Marshal(r)
	return string(b)
}

func TestGetFixture_TeamNames_DecodedFromTeamsField(t *testing.T) {
	body := buildFixtureJSONWithTeams(999, "Argentina", "Brazil")
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fix, err := newClient(t, srv).GetFixture(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix.HomeTeam != "Argentina" {
		t.Errorf("HomeTeam: got %q, want Argentina", fix.HomeTeam)
	}
	if fix.AwayTeam != "Brazil" {
		t.Errorf("AwayTeam: got %q, want Brazil", fix.AwayTeam)
	}
}

func TestGetLiveFixtures_TeamNames_DecodedFromTeamsField(t *testing.T) {
	body := `{"results":1,"errors":[],"response":[` +
		`{"fixture":{"id":42,"date":"2026-06-14T20:00:00+00:00","status":{"short":"1H","elapsed":30}},` +
		`"teams":{"home":{"name":"Spain"},"away":{"name":"Germany"}},` +
		`"goals":{"home":1,"away":0}}]}`
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fixtures, err := newClient(t, srv).GetLiveFixtures(context.Background(), 1, 2026)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("expected 1 fixture, got %d", len(fixtures))
	}
	if fixtures[0].HomeTeam != "Spain" {
		t.Errorf("HomeTeam: got %q, want Spain", fixtures[0].HomeTeam)
	}
	if fixtures[0].AwayTeam != "Germany" {
		t.Errorf("AwayTeam: got %q, want Germany", fixtures[0].AwayTeam)
	}
}

func TestGetFixturesByDate_TeamNames_DecodedFromTeamsField(t *testing.T) {
	body := buildFixtureJSONWithTeams(55, "France", "Portugal")
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fixtures, err := newClient(t, srv).GetFixturesByDate(context.Background(), 1, 2026, "2026-06-14")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("expected 1 fixture, got %d", len(fixtures))
	}
	if fixtures[0].HomeTeam != "France" {
		t.Errorf("HomeTeam: got %q, want France", fixtures[0].HomeTeam)
	}
	if fixtures[0].AwayTeam != "Portugal" {
		t.Errorf("AwayTeam: got %q, want Portugal", fixtures[0].AwayTeam)
	}
}

// TestGetFixture_NilGoals_DefaultsToZero verifies that nil goal values in the
// API response are decoded as 0 rather than panicking.
func TestGetFixture_NilGoals_DefaultsToZero(t *testing.T) {
	body := buildFixtureJSONWithTeams(77, "USA", "Mexico")
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fix, err := newClient(t, srv).GetFixture(context.Background(), 77)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix.HomeScore != 0 || fix.AwayScore != 0 {
		t.Errorf("expected zero scores for nil goals, got home=%d away=%d", fix.HomeScore, fix.AwayScore)
	}
}

// ── Halftime score decoding ───────────────────────────────────────────────────

func TestGetFixture_HalftimeScore_DecodedFromScoreField(t *testing.T) {
	type fixtureResp struct {
		Results  int              `json:"results"`
		Errors   []any            `json:"errors"`
		Response []map[string]any `json:"response"`
	}
	item := map[string]any{
		"fixture": map[string]any{
			"id":     123,
			"date":   "2026-06-14T20:00:00+00:00",
			"status": map[string]any{"short": "FT", "elapsed": nil},
		},
		"goals": map[string]any{"home": intPtr(2), "away": intPtr(1)},
		"score": map[string]any{
			"halftime": map[string]any{"home": intPtr(1), "away": intPtr(0)},
		},
	}
	b, _ := json.Marshal(fixtureResp{Results: 1, Errors: []any{}, Response: []map[string]any{item}})
	srv := buildServer(t, http.StatusOK, string(b))
	defer srv.Close()

	fix, err := newClient(t, srv).GetFixture(context.Background(), 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix.HalftimeHomeScore == nil || *fix.HalftimeHomeScore != 1 {
		t.Errorf("HalftimeHomeScore: got %v, want 1", fix.HalftimeHomeScore)
	}
	if fix.HalftimeAwayScore == nil || *fix.HalftimeAwayScore != 0 {
		t.Errorf("HalftimeAwayScore: got %v, want 0", fix.HalftimeAwayScore)
	}
}

func TestGetFixture_MissingHalftimeScore_NilFields(t *testing.T) {
	body := buildFixtureJSON(1, "NS", intPtr(0), intPtr(0))
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	fix, err := newClient(t, srv).GetFixture(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix.HalftimeHomeScore != nil || fix.HalftimeAwayScore != nil {
		t.Errorf("expected nil halftime scores, got %v/%v", fix.HalftimeHomeScore, fix.HalftimeAwayScore)
	}
}

// ── GetFixtureEvents ──────────────────────────────────────────────────────────

func TestGetFixtureEvents_ReturnsGoalEvents(t *testing.T) {
	body := `{"results":2,"errors":[],"response":[` +
		`{"time":{"elapsed":23},"team":{"name":"Argentina"},"type":"Goal"},` +
		`{"time":{"elapsed":67},"team":{"name":"Brazil"},"type":"Goal"}]}`
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	events, err := newClient(t, srv).GetFixtureEvents(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].TeamName != "Argentina" || events[0].ElapsedMin != 23 {
		t.Errorf("events[0]: got %+v", events[0])
	}
	if events[1].TeamName != "Brazil" || events[1].ElapsedMin != 67 {
		t.Errorf("events[1]: got %+v", events[1])
	}
}

func TestGetFixtureEvents_NoGoals_ReturnsEmpty(t *testing.T) {
	srv := buildServer(t, http.StatusOK, `{"results":0,"errors":[],"response":[]}`)
	defer srv.Close()

	events, err := newClient(t, srv).GetFixtureEvents(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestGetFixtureEvents_RateLimit_ReturnsError(t *testing.T) {
	srv := buildServer(t, http.StatusTooManyRequests, `{}`)
	defer srv.Close()

	_, err := newClient(t, srv).GetFixtureEvents(context.Background(), 1)
	if err == nil {
		t.Fatal("expected rate-limit error, got nil")
	}
}

func TestGetFixtureEvents_APIError_ReturnsError(t *testing.T) {
	body := `{"results":0,"errors":{"requests":"limit reached"},"response":[]}`
	srv := buildServer(t, http.StatusOK, body)
	defer srv.Close()

	_, err := newClient(t, srv).GetFixtureEvents(context.Background(), 1)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

func TestGetFixtureEvents_PassesFixtureAndTypeParams(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":0,"errors":[],"response":[]}`))
	}))
	defer srv.Close()

	_, _ = newClient(t, srv).GetFixtureEvents(context.Background(), 999)

	for _, want := range []string{"fixture=999", "type=Goal"} {
		if !strings.Contains(capturedQuery, want) {
			t.Errorf("expected query param %q in %q", want, capturedQuery)
		}
	}
}

// ── FirstScoringTeam ──────────────────────────────────────────────────────────

func TestFirstScoringTeam_HomeScoresFirst(t *testing.T) {
	events := []footballprovider.MatchEvent{
		{TeamName: "Mexico", ElapsedMin: 10},
		{TeamName: "Canada", ElapsedMin: 55},
	}
	got := footballprovider.FirstScoringTeam(events, "Mexico", "Canada")
	if got == nil || *got != "home" {
		t.Errorf("got %v, want \"home\"", got)
	}
}

func TestFirstScoringTeam_AwayScoresFirst(t *testing.T) {
	events := []footballprovider.MatchEvent{
		{TeamName: "Canada", ElapsedMin: 30},
		{TeamName: "Mexico", ElapsedMin: 80},
	}
	got := footballprovider.FirstScoringTeam(events, "Mexico", "Canada")
	if got == nil || *got != "away" {
		t.Errorf("got %v, want \"away\"", got)
	}
}

func TestFirstScoringTeam_UnorderedEvents_PicksEarliestElapsed(t *testing.T) {
	events := []footballprovider.MatchEvent{
		{TeamName: "Mexico", ElapsedMin: 80},
		{TeamName: "Canada", ElapsedMin: 12},
	}
	got := footballprovider.FirstScoringTeam(events, "Mexico", "Canada")
	if got == nil || *got != "away" {
		t.Errorf("got %v, want \"away\" (earliest elapsed wins)", got)
	}
}

func TestFirstScoringTeam_NoEvents_ReturnsNil(t *testing.T) {
	got := footballprovider.FirstScoringTeam(nil, "Mexico", "Canada")
	if got != nil {
		t.Errorf("expected nil, got %v", *got)
	}
}

func TestFirstScoringTeam_UnrecognisedTeamName_ReturnsNil(t *testing.T) {
	events := []footballprovider.MatchEvent{{TeamName: "Unknown FC", ElapsedMin: 5}}
	got := footballprovider.FirstScoringTeam(events, "Mexico", "Canada")
	if got != nil {
		t.Errorf("expected nil for unrecognised team name, got %v", *got)
	}
}

// TestGetFixturesByDate_PassesQueryParams verifies that league, season, and
// date are forwarded as query parameters to the API.
func TestGetFixturesByDate_PassesQueryParams(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":0,"errors":[],"response":[]}`))
	}))
	defer srv.Close()

	client := footballprovider.NewAPIFootballClient("test-key", srv.URL, srv.Client())
	_, _ = client.GetFixturesByDate(context.Background(), 123, 2026, "2026-06-14")

	for _, want := range []string{"league=123", "season=2026", "date=2026-06-14"} {
		if !strings.Contains(capturedQuery, want) {
			t.Errorf("expected query param %q in %q", want, capturedQuery)
		}
	}
}
