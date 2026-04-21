package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// testTournamentRouter mounts all tournament routes mirroring the production layout.
func testTournamentRouter(h *handler.TournamentHandler, user *domain.User) http.Handler {
	r := chi.NewRouter()
	if user != nil {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(middleware.ContextWithUser(req.Context(), user)))
			})
		})
	}
	r.Get("/tournament/standings", h.GetAllStandings)
	r.Get("/tournament/standings/{group}", h.GetGroupStanding)
	r.Get("/tournament/slots", h.ListSlots)
	r.Post("/tournament/slots", h.CreateSlot)
	r.Patch("/tournament/slots/{id}", h.ConfirmSlot)
	return r
}

func tournamentHandler(svc *stubTournamentSvc, t *testing.T) *handler.TournamentHandler {
	return handler.NewTournamentHandler(svc, zaptest.NewLogger(t))
}

func sampleSlot() *domain.TournamentSlot {
	return &domain.TournamentSlot{
		ID:        1,
		Label:     "winner_group_a",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func sampleStandings() map[string][]*domain.GroupStanding {
	return map[string][]*domain.GroupStanding{
		"A": {
			{Group: "A", Team: "Mexico", Points: 6, Won: 2, GF: 4, GC: 1, GD: 3},
			{Group: "A", Team: "USA", Points: 3, Won: 1, GF: 2, GC: 2, GD: 0},
		},
	}
}

// ── GetAllStandings ───────────────────────────────────────────────────────────

func TestTournamentHandler_GetAllStandings_200_ReturnsGroups(t *testing.T) {
	svc := &stubTournamentSvc{standings: sampleStandings()}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/standings", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
	var resp handler.TournamentStandingsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if len(resp.Groups) != 1 {
		t.Errorf("groups: want 1, got %d", len(resp.Groups))
	}
	if len(resp.Groups["A"]) != 2 {
		t.Errorf("group A entries: want 2, got %d", len(resp.Groups["A"]))
	}
	if resp.Groups["A"][0].Team != "Mexico" {
		t.Errorf("first team: want Mexico, got %s", resp.Groups["A"][0].Team)
	}
}

func TestTournamentHandler_GetAllStandings_200_EmptyWhenNoMatches(t *testing.T) {
	svc := &stubTournamentSvc{standings: map[string][]*domain.GroupStanding{}}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/standings", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
}

func TestTournamentHandler_GetAllStandings_500_WhenServiceFails(t *testing.T) {
	svc := &stubTournamentSvc{err: errors.New("db error")}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/standings", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

// ── GetGroupStanding ──────────────────────────────────────────────────────────

func TestTournamentHandler_GetGroupStanding_200_ReturnsEntries(t *testing.T) {
	entries := []*domain.GroupStanding{
		{Group: "B", Team: "Brazil", Points: 9},
		{Group: "B", Team: "Germany", Points: 6},
	}
	svc := &stubTournamentSvc{entries: entries}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/standings/B", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
	var resp []handler.GroupStandingResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if len(resp) != 2 {
		t.Errorf("entries: want 2, got %d", len(resp))
	}
	if resp[0].Team != "Brazil" {
		t.Errorf("first team: want Brazil, got %s", resp[0].Team)
	}
}

func TestTournamentHandler_GetGroupStanding_404_WhenUnknownGroup(t *testing.T) {
	svc := &stubTournamentSvc{err: apperrors.NotFound("group not found")}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/standings/Z", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestTournamentHandler_GetGroupStanding_500_WhenServiceFails(t *testing.T) {
	svc := &stubTournamentSvc{err: errors.New("db error")}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/standings/A", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

// ── ListSlots ─────────────────────────────────────────────────────────────────

func TestTournamentHandler_ListSlots_200_ReturnsList(t *testing.T) {
	team := "Mexico"
	slots := []*domain.TournamentSlot{
		{ID: 1, Label: "winner_group_a", Team: &team, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: 2, Label: "runner_up_group_a", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	svc := &stubTournamentSvc{slots: slots}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/slots", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
	var resp []handler.TournamentSlotResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if len(resp) != 2 {
		t.Errorf("slots: want 2, got %d", len(resp))
	}
	if resp[0].Team == nil || *resp[0].Team != "Mexico" {
		t.Errorf("first slot team: want Mexico, got %v", resp[0].Team)
	}
}

func TestTournamentHandler_ListSlots_200_EmptyList(t *testing.T) {
	svc := &stubTournamentSvc{slots: []*domain.TournamentSlot{}}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/slots", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
}

func TestTournamentHandler_ListSlots_500_WhenServiceFails(t *testing.T) {
	svc := &stubTournamentSvc{err: errors.New("db error")}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, "/tournament/slots", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

// ── CreateSlot ────────────────────────────────────────────────────────────────

func TestTournamentHandler_CreateSlot_201_ReturnsSlot(t *testing.T) {
	svc := &stubTournamentSvc{slot: sampleSlot()}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"label":"winner_group_a"}`)
	req := httptest.NewRequest(http.MethodPost, "/tournament/slots", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	var resp handler.TournamentSlotResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Label != "winner_group_a" {
		t.Errorf("label: want winner_group_a, got %s", resp.Label)
	}
}

func TestTournamentHandler_CreateSlot_401_WhenNoUser(t *testing.T) {
	svc := &stubTournamentSvc{slot: sampleSlot()}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, nil)

	body := strings.NewReader(`{"label":"winner_group_a"}`)
	req := httptest.NewRequest(http.MethodPost, "/tournament/slots", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

func TestTournamentHandler_CreateSlot_422_WhenBadBody(t *testing.T) {
	svc := &stubTournamentSvc{slot: sampleSlot()}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	req := httptest.NewRequest(http.MethodPost, "/tournament/slots", strings.NewReader(`{bad json}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Errorf(fmtExpect422, rr.Code)
	}
}

func TestTournamentHandler_CreateSlot_422_WhenValidationFails(t *testing.T) {
	svc := &stubTournamentSvc{err: apperrors.Validation("label required")}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"label":""}`)
	req := httptest.NewRequest(http.MethodPost, "/tournament/slots", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rr.Code)
	}
}

func TestTournamentHandler_CreateSlot_500_WhenServiceFails(t *testing.T) {
	svc := &stubTournamentSvc{err: errors.New("db error")}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"label":"winner_group_a"}`)
	req := httptest.NewRequest(http.MethodPost, "/tournament/slots", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

// ── ConfirmSlot ───────────────────────────────────────────────────────────────

func TestTournamentHandler_ConfirmSlot_200_ReturnsSlot(t *testing.T) {
	team := "Mexico"
	slot := sampleSlot()
	slot.Team = &team
	svc := &stubTournamentSvc{slot: slot}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"team":"Mexico"}`)
	req := httptest.NewRequest(http.MethodPatch, "/tournament/slots/1", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
	var resp handler.TournamentSlotResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Team == nil || *resp.Team != "Mexico" {
		t.Errorf("team: want Mexico, got %v", resp.Team)
	}
}

func TestTournamentHandler_ConfirmSlot_401_WhenNoUser(t *testing.T) {
	svc := &stubTournamentSvc{slot: sampleSlot()}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, nil)

	body := strings.NewReader(`{"team":"Mexico"}`)
	req := httptest.NewRequest(http.MethodPatch, "/tournament/slots/1", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

func TestTournamentHandler_ConfirmSlot_422_WhenInvalidID(t *testing.T) {
	svc := &stubTournamentSvc{slot: sampleSlot()}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"team":"Mexico"}`)
	req := httptest.NewRequest(http.MethodPatch, "/tournament/slots/abc", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Errorf(fmtExpect422, rr.Code)
	}
}

func TestTournamentHandler_ConfirmSlot_422_WhenBadBody(t *testing.T) {
	svc := &stubTournamentSvc{slot: sampleSlot()}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	req := httptest.NewRequest(http.MethodPatch, "/tournament/slots/1", strings.NewReader(`{bad json}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Errorf(fmtExpect422, rr.Code)
	}
}

func TestTournamentHandler_ConfirmSlot_404_WhenNotFound(t *testing.T) {
	svc := &stubTournamentSvc{err: apperrors.NotFound("slot not found")}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"team":"Mexico"}`)
	req := httptest.NewRequest(http.MethodPatch, "/tournament/slots/99", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestTournamentHandler_ConfirmSlot_500_WhenServiceFails(t *testing.T) {
	svc := &stubTournamentSvc{err: errors.New("db error")}
	h := tournamentHandler(svc, t)
	router := testTournamentRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"team":"Mexico"}`)
	req := httptest.NewRequest(http.MethodPatch, "/tournament/slots/1", body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}
