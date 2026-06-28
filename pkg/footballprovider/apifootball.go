package footballprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL      = "https://v3.football.api-sports.io"
	defaultTimeout      = 10 * time.Second
	maxResponseBodySize = 512 * 1024 // 512 KiB — generous for a single fixture
)

// APIFootballClient calls the API-Football v3 REST API.
// Authentication uses the x-apisports-key header; the key is obtained from
// the API-Football / API-Sports dashboard.
//
// All requests are protected by the injected *http.Client timeout so a
// single slow response cannot stall the polling goroutine indefinitely.
type APIFootballClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewAPIFootballClient constructs a client authenticated with apiKey.
// baseURL defaults to the production endpoint when empty.
// A custom *http.Client may be supplied for testing (e.g. httptest.Server).
func NewAPIFootballClient(apiKey, baseURL string, httpClient *http.Client) *APIFootballClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &APIFootballClient{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    httpClient,
	}
}

// ── API-Football response schema ─────────────────────────────────────────────

// apiResponse is the top-level envelope for all API-Football v3 responses.
type apiResponse struct {
	Results  int              `json:"results"`
	Response []apiFixtureItem `json:"response"`
	Errors   json.RawMessage  `json:"errors"`
}

type apiFixtureItem struct {
	Fixture apiFixtureDetail `json:"fixture"`
	Teams   apiTeams         `json:"teams"`
	Goals   apiGoals         `json:"goals"`
	Score   apiScore         `json:"score"`
}

type apiScore struct {
	Penalty apiGoals `json:"penalty"`
}

type apiTeams struct {
	Home apiTeamDetail `json:"home"`
	Away apiTeamDetail `json:"away"`
}

type apiTeamDetail struct {
	Name string `json:"name"`
}

type apiFixtureDetail struct {
	ID     int64     `json:"id"`
	Date   time.Time `json:"date"` // RFC 3339 — API-Football sends ISO 8601
	Status apiStatus `json:"status"`
}

type apiStatus struct {
	Short   string `json:"short"`
	Elapsed *int   `json:"elapsed"`
}

type apiGoals struct {
	Home *int `json:"home"`
	Away *int `json:"away"`
}

// pathFixtures is the API-Football endpoint shared by GetFixture,
// GetLiveFixtures, and GetFixturesByDate.
const pathFixtures = "/fixtures"

// ── Public methods ────────────────────────────────────────────────────────────

// GetFixture fetches the current state of a single fixture by its
// provider-assigned ID. Returns an error when the HTTP request fails or
// the fixture ID is not found in the API-Football response.
func (c *APIFootballClient) GetFixture(ctx context.Context, externalID int64) (*Fixture, error) {
	items, err := c.fetch(ctx, pathFixtures, map[string]string{
		"id": strconv.FormatInt(externalID, 10),
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("footballprovider: fixture %d not found", externalID)
	}
	return itemToFixture(items[0]), nil
}

// GetLiveFixtures returns all fixtures currently in play for the given league
// and season. Used by the scheduler to determine whether to apply the fast or
// slow polling interval on the next tick.
func (c *APIFootballClient) GetLiveFixtures(ctx context.Context, leagueID, season int) ([]*Fixture, error) {
	items, err := c.fetch(ctx, pathFixtures, map[string]string{
		"live":   "all",
		"league": strconv.Itoa(leagueID),
		"season": strconv.Itoa(season),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*Fixture, 0, len(items))
	for _, item := range items {
		out = append(out, itemToFixture(item))
	}
	return out, nil
}

// GetFixturesByDate returns all fixtures for the given league, season, and UTC
// date (formatted as YYYY-MM-DD). Used by the daily fixture sync job to link
// matches and update kickoff times for every game scheduled on a given day.
func (c *APIFootballClient) GetFixturesByDate(ctx context.Context, leagueID, season int, date string) ([]*Fixture, error) {
	items, err := c.fetch(ctx, pathFixtures, map[string]string{
		"league": strconv.Itoa(leagueID),
		"season": strconv.Itoa(season),
		"date":   date,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*Fixture, 0, len(items))
	for _, item := range items {
		out = append(out, itemToFixture(item))
	}
	return out, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (c *APIFootballClient) fetch(ctx context.Context, path string, params map[string]string) ([]apiFixtureItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("footballprovider: build request: %w", err)
	}
	req.Header.Set("x-apisports-key", c.apiKey)

	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("footballprovider: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("footballprovider: read body: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("footballprovider: rate limit exceeded (429)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("footballprovider: unexpected status %d", resp.StatusCode)
	}

	var envelope apiResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("footballprovider: decode response: %w", err)
	}

	// API-Football signals some errors as a JSON object in the "errors" field
	// rather than via HTTP status codes. A non-null, non-empty-array errors
	// value indicates a request-level problem (invalid key, unknown league, …).
	if len(envelope.Errors) > 0 && string(envelope.Errors) != "[]" && string(envelope.Errors) != "null" {
		return nil, fmt.Errorf("footballprovider: API error: %s", envelope.Errors)
	}

	return envelope.Response, nil
}

func itemToFixture(item apiFixtureItem) *Fixture {
	status := parseStatus(item.Fixture.Status.Short)
	home, away := 0, 0
	if item.Goals.Home != nil {
		home = *item.Goals.Home
	}
	if item.Goals.Away != nil {
		away = *item.Goals.Away
	}
	return &Fixture{
		ExternalID:       item.Fixture.ID,
		HomeTeam:         item.Teams.Home.Name,
		AwayTeam:         item.Teams.Away.Name,
		Status:           status,
		HomeScore:        home,
		AwayScore:        away,
		KickoffUTC:       item.Fixture.Date.UTC(),
		PenaltyHomeScore: item.Score.Penalty.Home,
		PenaltyAwayScore: item.Score.Penalty.Away,
	}
}

func parseStatus(short string) FixtureStatus {
	switch FixtureStatus(short) {
	case StatusNotStarted, StatusFirstHalf, StatusHalfTime, StatusSecondHalf,
		StatusExtraTime, StatusPenLive, StatusFullTime, StatusAfterET, StatusAfterPEN,
		StatusPostponed, StatusCancelled, StatusAbandoned:
		return FixtureStatus(short)
	default:
		return StatusUnknown
	}
}

var _ Client = (*APIFootballClient)(nil)
