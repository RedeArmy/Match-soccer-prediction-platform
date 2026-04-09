package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// newMatchRouter wires MatchHandler into a chi router so that {id} URL params
// are resolved correctly by chi.URLParam.
func newMatchRouter(svc *stubMatchSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewMatchHandler(svc, zap.NewNop())
	r.Get("/", h.ListMatches)
	r.Post("/", h.CreateMatch)
	r.Get("/{id}", h.GetMatch)
	r.Patch("/{id}", h.UpdateResult)
	r.Post("/{id}/start", h.StartMatch)
	return r
}

func do(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── ListMatches ───────────────────────────────────────────────────────────────

func TestListMatches_Success_Returns200(t *testing.T) {
	svc := &stubMatchSvc{matches: []*domain.Match{{ID: 1, HomeTeam: homeTeam, AwayTeam: awayTeam}}}
	w := do(newMatchRouter(svc), http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestListMatches_ServiceError_Returns500(t *testing.T) {
	svc := &stubMatchSvc{err: apperrors.Internal(nil)}
	w := do(newMatchRouter(svc), http.MethodGet, "/", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListMatches_WithPhaseFilter_Returns200(t *testing.T) {
	svc := &stubMatchSvc{matches: []*domain.Match{{ID: 1, HomeTeam: homeTeam, AwayTeam: awayTeam, Phase: domain.PhaseGroupStage}}}
	w := do(newMatchRouter(svc), http.MethodGet, "/?phase=group_stage", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

// ── GetMatch ──────────────────────────────────────────────────────────────────

func TestGetMatch_Success_Returns200(t *testing.T) {
	svc := &stubMatchSvc{match: &domain.Match{ID: 1, HomeTeam: homeTeam, AwayTeam: awayTeam}}
	w := do(newMatchRouter(svc), http.MethodGet, "/1", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestGetMatch_WithStadium_IncludesStadiumInResponse(t *testing.T) {
	stadiumID := 1
	svc := &stubMatchSvc{match: &domain.Match{
		ID:        1,
		HomeTeam:  homeTeam,
		AwayTeam:  awayTeam,
		StadiumID: &stadiumID,
		Stadium: &domain.Stadium{
			ID:       1,
			Name:     "MetLife Stadium",
			Capacity: 82500,
			City: &domain.City{
				ID:   1,
				Name: "East Rutherford",
				State: &domain.State{
					ID:   1,
					Name: "New Jersey",
					Code: "NJ",
					Country: &domain.Country{
						ID:   1,
						Name: "United States",
						Code: "US",
					},
				},
			},
		},
	}}
	w := do(newMatchRouter(svc), http.MethodGet, "/1", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
	if !strings.Contains(w.Body.String(), "MetLife Stadium") {
		t.Errorf("expected stadium name in response, got: %s", w.Body.String())
	}
}

func TestGetMatch_InvalidID_Returns422(t *testing.T) {
	w := do(newMatchRouter(&stubMatchSvc{}), http.MethodGet, "/abc", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestGetMatch_NotFound_Returns404(t *testing.T) {
	svc := &stubMatchSvc{err: apperrors.NotFound("match not found")}
	w := do(newMatchRouter(svc), http.MethodGet, "/99", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── CreateMatch ───────────────────────────────────────────────────────────────

func TestCreateMatch_Success_Returns201(t *testing.T) {
	kickoff := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"home_team":"` + homeTeam + `","away_team":"` + awayTeam + `","kickoff_at":"` + kickoff + `"}`
	w := do(newMatchRouter(&stubMatchSvc{}), http.MethodPost, "/", body)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestCreateMatch_InvalidJSON_Returns422(t *testing.T) {
	w := do(newMatchRouter(&stubMatchSvc{}), http.MethodPost, "/", `not json`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestCreateMatch_ServiceError_Returns422(t *testing.T) {
	kickoff := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"home_team":"` + homeTeam + `","away_team":"` + awayTeam + `","kickoff_at":"` + kickoff + `"}`
	svc := &stubMatchSvc{err: apperrors.Validation("teams must differ")}
	w := do(newMatchRouter(svc), http.MethodPost, "/", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── UpdateResult ──────────────────────────────────────────────────────────────

func TestUpdateResult_Success_Returns200(t *testing.T) {
	svc := &stubMatchSvc{match: &domain.Match{ID: 1}}
	w := do(newMatchRouter(svc), http.MethodPatch, "/1", `{"home_score":2,"away_score":1}`)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestUpdateResult_InvalidID_Returns422(t *testing.T) {
	w := do(newMatchRouter(&stubMatchSvc{}), http.MethodPatch, "/abc", `{"home_score":2,"away_score":1}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestUpdateResult_InvalidJSON_Returns422(t *testing.T) {
	w := do(newMatchRouter(&stubMatchSvc{}), http.MethodPatch, "/1", `not json`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestUpdateResult_MissingScores_Returns422(t *testing.T) {
	// Valid JSON but home_score/away_score are absent (nil pointers after decode).
	w := do(newMatchRouter(&stubMatchSvc{}), http.MethodPatch, "/1", `{}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestUpdateResult_ServiceError_Returns422(t *testing.T) {
	svc := &stubMatchSvc{err: apperrors.Validation("match is not live")}
	w := do(newMatchRouter(svc), http.MethodPatch, "/1", `{"home_score":2,"away_score":1}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── StartMatch ────────────────────────────────────────────────────────────────

func TestStartMatch_Success_Returns200(t *testing.T) {
	svc := &stubMatchSvc{match: &domain.Match{ID: 1, Status: domain.MatchStatusLive}}
	w := do(newMatchRouter(svc), http.MethodPost, "/1/start", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestStartMatch_InvalidID_Returns422(t *testing.T) {
	w := do(newMatchRouter(&stubMatchSvc{}), http.MethodPost, "/abc/start", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestStartMatch_ServiceError_Returns422(t *testing.T) {
	svc := &stubMatchSvc{err: apperrors.Validation("already live")}
	w := do(newMatchRouter(svc), http.MethodPost, "/1/start", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}
