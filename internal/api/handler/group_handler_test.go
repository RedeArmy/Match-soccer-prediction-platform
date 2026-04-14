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
	groupRotatePath  = "/groups/1/invite-code/rotate"

	fmtExpect404  = "expected 404, got %d"
	fmtDecodeFail = "decode: %v"

	clerkSubject = "user_clerk_abc"
	errDBDown    = "db down"

	groupName  = "Test Group"
	inviteCode = "ABC123DEFG"

	bodyCreateGroup = `{"name":"` + groupName + `"}`
	bodyJoinGroup   = `{"invite_code":"` + inviteCode + `"}`
)

// testGroupRouter wires GroupHandler on a chi router that injects a resolved
// domain.User into the context (simulating RequireAuth + ResolveUser middleware).
func testGroupRouter(h *handler.GroupHandler) http.Handler {
	return buildGroupRouter(h, &domain.User{ID: 10})
}

// testGroupRouterNoUser wires GroupHandler without injecting any user into the
// context — simulates a request where ResolveUser did not run (unauthenticated).
func testGroupRouterNoUser(h *handler.GroupHandler) http.Handler {
	return buildGroupRouter(h, nil)
}

func buildGroupRouter(h *handler.GroupHandler, user *domain.User) http.Handler {
	r := chi.NewRouter()
	if user != nil {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := middleware.ContextWithUser(req.Context(), user)
				next.ServeHTTP(w, req.WithContext(ctx))
			})
		})
	}
	r.Post("/groups", h.Create)
	r.Post("/groups/join", h.Join)
	r.Get("/groups/me", h.ListMyGroups)
	r.Get("/groups/{id}", h.GetByID)
	r.Get("/groups/{id}/members", h.ListMembers)
	r.Post("/groups/{id}/invite-code/rotate", h.RotateInviteCode)
	return r
}

func fixedQuiniela() *domain.Quiniela {
	return &domain.Quiniela{
		ID:         1,
		Name:       groupName,
		OwnerID:    10,
		InviteCode: inviteCode,
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

func newGroupHandler(t *testing.T, qs *stubQuinielaSvc, ms *stubMemberSvc) *handler.GroupHandler {
	t.Helper()
	return handler.NewGroupHandler(qs, ms, zaptest.NewLogger(t))
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestGroupCreate_Returns201(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{quiniela: fixedQuiniela()}, &stubMemberSvc{})
	body := bodyCreateGroup
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
	h := newGroupHandler(t, &stubQuinielaSvc{quiniela: q}, &stubMemberSvc{})
	body := bodyCreateGroup
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	var resp handler.GroupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.InviteCode != q.InviteCode {
		t.Errorf("expected invite_code %q, got %q", q.InviteCode, resp.InviteCode)
	}
}

func TestGroupCreate_ResponseBody_ContainsInviteCodeExpiresAt_WhenSet(t *testing.T) {
	q := fixedQuiniela()
	exp := time.Now().Add(30 * 24 * time.Hour).UTC()
	q.InviteCodeExpiresAt = &exp

	h := newGroupHandler(t, &stubQuinielaSvc{quiniela: q}, &stubMemberSvc{})
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(bodyCreateGroup))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	var resp handler.GroupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.InviteCodeExpiresAt == nil {
		t.Fatal("expected invite_code_expires_at to be set in response")
	}
}

// TestGroupCreate_Returns401_WhenNoUser verifies that Create returns 401 when
// no resolved user is in the context (ResolveUser middleware did not run).
func TestGroupCreate_Returns401_WhenNoUser(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{}, &stubMemberSvc{})
	body := bodyCreateGroup
	req := httptest.NewRequest(http.MethodPost, groupsPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouterNoUser(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestGroupGetByID_Returns200(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{quiniela: fixedQuiniela()}, &stubMemberSvc{})
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
	)
	req := httptest.NewRequest(http.MethodGet, groupByIDPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf(fmtExpect404, rec.Code)
	}
}

// ── Join ──────────────────────────────────────────────────────────────────────

func TestGroupJoin_Returns200(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{}, &stubMemberSvc{membership: fixedMembership()})
	body := bodyJoinGroup
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestGroupJoin_Returns422_WhenNoInviteCode(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{}, &stubMemberSvc{})
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
	)
	body := bodyJoinGroup
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
	)
	body := `{"invite_code":"BADCODE123"}`
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf(fmtExpect404, rec.Code)
	}
}

// ── ListMembers ───────────────────────────────────────────────────────────────

func TestGroupListMembers_Returns200(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{memberships: []*domain.GroupMembership{fixedMembership()}},
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
	)
	req := httptest.NewRequest(http.MethodGet, groupMembersPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	var out []handler.MemberResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf(fmtDecodeFail, err)
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
	)
	req := httptest.NewRequest(http.MethodGet, groupsMePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

// TestGroupListMyGroups_Returns401_WhenNoUser verifies that ListMyGroups
// returns 401 when no resolved user is in the context.
func TestGroupListMyGroups_Returns401_WhenNoUser(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{}, &stubMemberSvc{})
	req := httptest.NewRequest(http.MethodGet, groupsMePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouterNoUser(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGroupListMyGroups_Returns500_OnServiceError(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{},
		&stubMemberSvc{err: apperrors.Internal(errors.New(errDBDown))},
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
	h := newGroupHandler(t, &stubQuinielaSvc{}, &stubMemberSvc{})
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
	)
	req := httptest.NewRequest(http.MethodGet, groupByIDPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

func TestGroupJoin_Returns400_OnMalformedJSON(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{}, &stubMemberSvc{})
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
	)
	req := httptest.NewRequest(http.MethodPost, groupsJoinPath, bytes.NewBufferString(bodyJoinGroup))
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
	)
	req := httptest.NewRequest(http.MethodGet, groupMembersPath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rec.Code)
	}
}

// ── RotateInviteCode ──────────────────────────────────────────────────────────

func TestGroupRotateInviteCode_Returns200(t *testing.T) {
	q := fixedQuiniela()
	h := newGroupHandler(t, &stubQuinielaSvc{quiniela: q}, &stubMemberSvc{})
	req := httptest.NewRequest(http.MethodPost, groupRotatePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rec.Code)
	}
}

func TestGroupRotateInviteCode_Returns401_WhenNoUser(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{}, &stubMemberSvc{})
	req := httptest.NewRequest(http.MethodPost, groupRotatePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouterNoUser(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rec.Code)
	}
}

func TestGroupRotateInviteCode_Returns403_WhenNotOwner(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{err: apperrors.Forbidden("only the group owner can rotate the invite code")},
		&stubMemberSvc{},
	)
	req := httptest.NewRequest(http.MethodPost, groupRotatePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGroupRotateInviteCode_Returns404_WhenNotFound(t *testing.T) {
	h := newGroupHandler(t,
		&stubQuinielaSvc{err: apperrors.NotFound("quiniela 1 not found")},
		&stubMemberSvc{},
	)
	req := httptest.NewRequest(http.MethodPost, groupRotatePath, nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf(fmtExpect404, rec.Code)
	}
}

func TestGroupRotateInviteCode_Returns422_InvalidID(t *testing.T) {
	h := newGroupHandler(t, &stubQuinielaSvc{}, &stubMemberSvc{})
	req := httptest.NewRequest(http.MethodPost, "/groups/abc/invite-code/rotate", nil)
	rec := httptest.NewRecorder()
	testGroupRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rec.Code)
	}
}
