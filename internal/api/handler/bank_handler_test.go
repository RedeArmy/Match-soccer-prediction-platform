package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubBankLister struct {
	banks []repository.Bank
	err   error
}

func (s *stubBankLister) ListActive(_ context.Context) ([]repository.Bank, error) {
	return s.banks, s.err
}

type stubBankAccountTypeLister struct {
	types []repository.BankAccountType
	err   error
}

func (s *stubBankAccountTypeLister) ListActive(_ context.Context) ([]repository.BankAccountType, error) {
	return s.types, s.err
}

// ── BankHandler.List ──────────────────────────────────────────────────────────

func TestBankHandler_List_OK(t *testing.T) {
	banks := []repository.Bank{{ID: 1, Name: "BAC Guatemala", Active: true}}
	h := handler.NewBankHandler(&stubBankLister{banks: banks}, &stubBankAccountTypeLister{}, zaptest.NewLogger(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/banks", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("len: got %d, want 1", len(resp))
	}
	if resp[0]["name"] != "BAC Guatemala" {
		t.Errorf("name: got %v, want BAC Guatemala", resp[0]["name"])
	}
}

func TestBankHandler_List_RepoError(t *testing.T) {
	h := handler.NewBankHandler(
		&stubBankLister{err: errors.New("db error")},
		&stubBankAccountTypeLister{},
		zaptest.NewLogger(t),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/banks", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 on repo error")
	}
}

// ── BankHandler.ListAccountTypes ──────────────────────────────────────────────

func TestBankHandler_ListAccountTypes_OK(t *testing.T) {
	types := []repository.BankAccountType{{ID: 2, Name: "Monetaria", Active: true}}
	h := handler.NewBankHandler(&stubBankLister{}, &stubBankAccountTypeLister{types: types}, zaptest.NewLogger(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bank-account-types", nil)
	w := httptest.NewRecorder()
	h.ListAccountTypes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("len: got %d, want 1", len(resp))
	}
	if resp[0]["name"] != "Monetaria" {
		t.Errorf("name: got %v, want Monetaria", resp[0]["name"])
	}
}

func TestBankHandler_ListAccountTypes_RepoError(t *testing.T) {
	h := handler.NewBankHandler(
		&stubBankLister{},
		&stubBankAccountTypeLister{err: errors.New("db error")},
		zaptest.NewLogger(t),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bank-account-types", nil)
	w := httptest.NewRecorder()
	h.ListAccountTypes(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 on repo error")
	}
}
