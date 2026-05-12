package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

func balanceRouter(svc *stubBalanceSvc) http.Handler {
	h := handler.NewBalanceHandler(svc, zaptest.NewLogger(t_balance))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/me/balance", h.GetBalance)
	mux.HandleFunc("GET /users/me/balance/ledger", h.GetLedger)
	return mux
}

// t_balance is a package-level *testing.T used only to build the logger in
// balanceRouter. Handler unit tests pass their own *testing.T for assertions.
var t_balance *testing.T

func withBalanceUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.ContextWithUser(r.Context(), &domain.User{ID: 5})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestBalanceHandler_GetBalance_OK(t *testing.T) {
	t_balance = t
	svc := &stubBalanceSvc{balanceCents: 10000, reservedCents: 2000}
	router := withBalanceUser(balanceRouter(svc))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/me/balance", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp handler.BalanceResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.BalanceCents != 10000 {
		t.Errorf("balance_cents: got %d, want 10000", resp.BalanceCents)
	}
	if resp.ReservedCents != 2000 {
		t.Errorf("reserved_cents: got %d, want 2000", resp.ReservedCents)
	}
	if resp.AvailableCents != 8000 {
		t.Errorf("available_cents: got %d, want 8000", resp.AvailableCents)
	}
}

func TestBalanceHandler_GetBalance_Unauthenticated(t *testing.T) {
	t_balance = t
	router := balanceRouter(&stubBalanceSvc{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/me/balance", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestBalanceHandler_GetBalance_ServiceError(t *testing.T) {
	t_balance = t
	svc := &stubBalanceSvc{err: errors.New("db down")}
	router := withBalanceUser(balanceRouter(svc))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/me/balance", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestBalanceHandler_GetLedger_OK(t *testing.T) {
	t_balance = t
	now := time.Now().UTC()
	entries := []*domain.BalanceLedger{
		{ID: 1, UserID: 5, DeltaCents: 5000, Kind: domain.LedgerKindBankTransfer, CreatedAt: now},
	}
	svc := &stubBalanceSvc{entries: entries}
	router := withBalanceUser(balanceRouter(svc))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/me/balance/ledger", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp []handler.LedgerEntryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("expected 1 entry, got %d", len(resp))
	}
}

func TestBalanceHandler_GetLedger_Unauthenticated(t *testing.T) {
	t_balance = t
	router := balanceRouter(&stubBalanceSvc{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/me/balance/ledger", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestBalanceHandler_GetLedger_Empty(t *testing.T) {
	t_balance = t
	svc := &stubBalanceSvc{entries: nil}
	router := withBalanceUser(balanceRouter(svc))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/me/balance/ledger", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
