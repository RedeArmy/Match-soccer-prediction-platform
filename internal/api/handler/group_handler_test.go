package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── helpers ───────────────────────────────────────────────────────────────────

const (
	groupsPath       = "/groups"
	groupByIDPath    = "/groups/1"
	groupsMePath     = "/groups/me"
	groupsJoinPath   = "/groups/join"
	groupMembersPath = "/groups/1/members"

	clerkSubject = "user_clerk_abc"
	errDBDown    = "db down"
)

// testGroupRouter wires GroupHandler on a chi router that already has the
// Clerk subject injected into the request context (simulating RequireAuth).
func testGroupRouter(h *handler.GroupHandler) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := middleware.ContextWithUserID(req.Context(), clerkSubject)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Post("/groups", h.Create)
	r.Post("/groups/join", h.Join)
	r.Get("/groups/me", h.ListMyGroups)
	r.Get("/groups/{id}", h.GetByID)
	r.Get("/groups/{id}/members", h.ListMembers)
	return r
}

func fixedQuiniela() *domain.Quiniela {
	return &domain.Quiniela{
		ID:         1,
		Name:       "Test Group",
		OwnerID:    10,
		InviteCode: "ABC123DEFG",
		EntryFee:   0,
		Currency:   "MXN",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

func fixedMembership() *domain.GroupMembership {
	now := time.Now()
	return &domain.GroupMembership{
		ID:         1,
		QuinielaID: 1,
		UserID:     10,
		Status:     domain.MembershipActive,
		JoinedAt:   &now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func newGroupHandler(t *testing.T, qs *stubQuinielaSvc, ms *stubMemberSvc, ur *stubUserRepo) *handler.GroupHandler {
	t.Helper()
	return handler.NewGroupHandler(qs, ms, ur, zaptest.NewLogger(t))
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestGroupCreate_Returns201(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{quiniela: fixedQuiniela()},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	body := `{"name":"Test Group"}`
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf(fmtExpect200+" (201)", rec.Code)
	}
}

func TestGroupCreate_Returns409_OnDuplicateName(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{err: apperrors.Conflict("a group with this name already exists")},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	body := `{"name":"Duplicate"}`
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestGroupCreate_Returns422_OnEmptyBody(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{err: apperrors.Validation("quiniela name must not be empty")},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestGroupCreate_ResponseBody_ContainsInviteCode(t *testing.T) {
	q := fixedQuiniela()
	h := newGroupHandler(t,
		&stubQuinielaSvc{quiniela: q},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	body := `{"name":"Test Group"}`
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	var resp handler.GroupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.InviteCode != q.InviteCode {
		t.Errorf("expected invite_code %q, got %q", q.InviteCode, resp.InviteCode)
	}
}

func TestGroupCreate_Returns401_WhenUserNotFound(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{},
		&stubUserRepo{user: nil},
	)
	body := `{"name":"Test Group"}`
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestGroupGetByID_Returns200(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{quiniela: fixedQuiniela()},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodGet, groupByIDPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestGroupGetByID_Returns404_WhenNotFound(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{err: apperrors.NotFound("quiniela 1 not found")},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodGet, groupByIDPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Join ──────────────────────────────────────────────────────────────────────

func TestGroupJoin_Returns200(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{membership: fixedMembership()},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	body := `{"invite_code":"ABC123DEFG"}`
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestGroupJoin_Returns422_WhenNoInviteCode(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestGroupJoin_Returns409_WhenAlreadyMember(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{err: apperrors.Conflict("you are already a member of this group")},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	body := `{"invite_code":"ABC123DEFG"}`
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestGroupJoin_Returns404_WhenCodeNotFound(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{err: apperrors.NotFound("group not found for the given invite code")},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	body := `{"invite_code":"BADCODE123"}`
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── ListMembers ───────────────────────────────────────────────────────────────

func TestGroupListMembers_Returns200(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{memberships: []*domain.GroupMembership{fixedMembership()}},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodGet, groupMembersPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestGroupListMembers_ReturnsJSONArray(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{memberships: []*domain.GroupMembership{fixedMembership(), fixedMembership()}},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodGet, groupMembersPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	var out []handler.MemberResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 members, got %d", len(out))
	}
}

// ── ListMyGroups ──────────────────────────────────────────────────────────────

func TestGroupListMyGroups_Returns200(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{memberships: []*domain.GroupMembership{fixedMembership()}},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodGet, groupsMePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestGroupListMyGroups_Returns401_WhenUserNotFound(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{},
		&stubUserRepo{user: nil},
	)
	req := httptest.NewRequest(http.MethodGet, groupsMePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGroupListMyGroups_Returns500_OnServiceError(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{err: apperrors.Internal(errors.New(errDBDown))},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodGet, groupsMePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

// ── error paths ───────────────────────────────────────────────────────────────

func TestGroupCreate_Returns400_OnMalformedJSON(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(`{bad json`))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestGroupCreate_Returns500_OnServiceError(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{err: apperrors.Internal(errors.New(errDBDown))},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(`{"name":"Pool"}`))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

func TestGroupGetByID_Returns500_OnServiceError(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{err: apperrors.Internal(errors.New(errDBDown))},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodGet, groupByIDPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

func TestGroupJoin_Returns400_OnMalformedJSON(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(`{bad`))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}

func TestGroupJoin_Returns500_OnServiceError(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{err: apperrors.Internal(errors.New(errDBDown))},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(`{"invite_code":"ABC123DEFG"}`))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

func TestGroupListMembers_Returns500_OnServiceError(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{err: apperrors.Internal(errors.New(errDBDown))},
		&stubUserRepo{user: &domain.User{ID: 10}},
	)
	req := httptest.NewRequest(http.MethodGet, groupMembersPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}
