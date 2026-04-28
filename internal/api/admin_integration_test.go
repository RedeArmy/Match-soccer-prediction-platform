package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// adminRoute is a single (method, path) admin endpoint entry.
// Path parameters are replaced with concrete placeholder values so chi can
// match the route pattern and dispatch to the correct handler.
type adminRoute struct {
	method string
	path   string
}

// allAdminRoutes enumerates every admin endpoint registered in server.go.
// Keep this list in sync with the r.Route("/admin", ...) block.
var allAdminRoutes = []adminRoute{
	// Users
	{http.MethodGet, "/api/v1/admin/users"},
	{http.MethodGet, "/api/v1/admin/users/1"},
	{http.MethodPost, "/api/v1/admin/users/1/ban"},
	{http.MethodDelete, "/api/v1/admin/users/1/ban"},
	{http.MethodPost, "/api/v1/admin/users/bulk-ban"},
	// Groups
	{http.MethodDelete, "/api/v1/admin/groups/1"},
	{http.MethodDelete, "/api/v1/admin/groups/1/members/2"},
	{http.MethodPatch, "/api/v1/admin/groups/1/settings"},
	{http.MethodPost, "/api/v1/admin/groups/1/transfer-ownership"},
	{http.MethodGet, "/api/v1/admin/groups/1/payments"},
	{http.MethodGet, "/api/v1/admin/groups/1/leaderboard/history"},
	// Payments
	{http.MethodGet, "/api/v1/admin/payments/pending"},
	{http.MethodGet, "/api/v1/admin/payments"},
	{http.MethodPost, "/api/v1/admin/payments/1/validate"},
	{http.MethodPost, "/api/v1/admin/payments/1/reject"},
	// Leaderboard & predictions
	{http.MethodGet, "/api/v1/admin/leaderboard"},
	{http.MethodGet, "/api/v1/admin/predictions"},
	{http.MethodGet, "/api/v1/admin/predictions/match/1"},
	// DLQ
	{http.MethodGet, "/api/v1/admin/dlq"},
	{http.MethodPost, "/api/v1/admin/dlq/replay"},
	{http.MethodDelete, "/api/v1/admin/dlq"},
	// Audit log
	{http.MethodGet, "/api/v1/admin/audit-log"},
	{http.MethodGet, "/api/v1/admin/audit-log/entity/user/1"},
	// System params
	{http.MethodGet, "/api/v1/admin/system-params"},
	{http.MethodGet, "/api/v1/admin/system-params/scoring.exact"},
	{http.MethodPatch, "/api/v1/admin/system-params/scoring.exact"},
	{http.MethodPost, "/api/v1/admin/system-params/bulk"},
	// Tiebreaker
	{http.MethodGet, "/api/v1/admin/tiebreaker/submissions"},
	// Conflicts
	{http.MethodGet, "/api/v1/admin/conflicts"},
	{http.MethodPost, "/api/v1/admin/conflicts/group_no_owner/1/resolve"},
	// Stats / observability
	{http.MethodGet, "/api/v1/admin/stats"},
	{http.MethodGet, "/api/v1/admin/stats/conflicts/summary"},
	// Bulk group operations
	{http.MethodPost, "/api/v1/admin/groups/bulk-delete"},
	{http.MethodPost, "/api/v1/admin/groups/1/members/bulk-remove"},
	{http.MethodPost, "/api/v1/admin/groups/1/leaderboard/recalculate"},
}

// newAdminTestServer builds a Server with a fake (unreachable) database pool
// so the full route table — including all /admin/* paths — is registered.
// The empty Config leaves JWKS URL blank, which disables RequireAuth and
// makes the auth middleware a passthrough for integration test purposes.
func newAdminTestServer(t *testing.T) *api.Server {
	t.Helper()
	return api.New(fakePool(t), &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
}

// TestAdminRoutes_NilDB_Returns404 verifies that admin routes are absent from
// the chi route table when the database pool is nil. The server registers only
// a minimal stub table for the four known non-admin prefixes (/matches,
// /predictions, /groups, /users); every /admin/* path falls through to chi's
// built-in 404 handler.
//
// This guards against accidental changes that would make admin endpoints
// reachable without a real database connection.
func TestAdminRoutes_NilDB_Returns404(t *testing.T) {
	h := newTestServer(t).Routes() // nil DB

	for _, tc := range allAdminRoutes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("expected 404 (route absent without DB), got %d", rec.Code)
			}
		})
	}
}

// TestAdminRoutes_Unauthenticated_Return401 verifies two things for every admin
// endpoint:
//
//  1. The route is registered — a 404 response indicates it was never wired.
//  2. The RequireRole middleware rejects requests with no user ID in context.
//
// The server is built with a non-nil fake pool so the full route table is
// wired. RequireAuth is disabled (empty JWKS URL), so the missing userID in
// context is caught by RequireRole — the first middleware on the admin
// sub-router — which returns 401. This mirrors the production behaviour: a
// caller without a valid JWT never reaches any admin handler.
func TestAdminRoutes_Unauthenticated_Return401(t *testing.T) {
	h := newAdminTestServer(t).Routes()

	for _, tc := range allAdminRoutes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Fatalf("route not registered: %s %s returned 404", tc.method, tc.path)
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for unauthenticated request, got %d", rec.Code)
			}
		})
	}
}

// TestAdminRoutes_AuthenticatedUser_DBUnavailable_Returns500 verifies that once
// a request carries a user ID in context (simulating a passed RequireAuth
// check), RequireRole advances to the database-lookup phase. The fake pool is
// unreachable, so GetByClerkSubject fails with a connection error, and the
// middleware responds with 500.
//
// This confirms the middleware chain is wired correctly past the initial
// userID-presence check: after authentication, RequireRole always performs a
// live database lookup to verify the caller's role before forwarding to any
// admin handler.
func TestAdminRoutes_AuthenticatedUser_DBUnavailable_Returns500(t *testing.T) {
	h := newAdminTestServer(t).Routes()

	for _, tc := range allAdminRoutes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req = req.WithContext(middleware.ContextWithUserID(req.Context(), "clerk_fake_subject"))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("expected 500 (DB unreachable during role lookup), got %d", rec.Code)
			}
		})
	}
}
