package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// stubSystemParamRepo implements repository.SystemParamRepository for unit tests.
type stubSystemParamRepo struct {
	param  *domain.SystemParam
	params []*domain.SystemParam
	err    error
	setErr error
}

func (r *stubSystemParamRepo) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return r.param, r.err
}
func (r *stubSystemParamRepo) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return r.params, r.err
}
func (r *stubSystemParamRepo) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return r.params, r.err
}
func (r *stubSystemParamRepo) Set(_ context.Context, key, value string, _ int) (*domain.SystemParam, error) {
	if r.setErr != nil {
		return nil, r.setErr
	}
	return &domain.SystemParam{Key: key, Value: value}, nil
}
func (r *stubSystemParamRepo) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return r.err
}

func param(key, value string) *domain.SystemParam {
	return &domain.SystemParam{Key: key, Value: value}
}

func newParamSvc(repo repository.SystemParamRepository) SystemParamService {
	return NewSystemParamService(repo, zap.NewNop())
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestSystemParamService_Get_CacheMiss_FetchesFromRepo(t *testing.T) {
	p := param("k", "v")
	svc := newParamSvc(&stubSystemParamRepo{param: p})

	got, err := svc.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Value != "v" {
		t.Errorf("expected param with value 'v', got %v", got)
	}
}

func TestSystemParamService_Get_CacheHit_DoesNotCallRepo(t *testing.T) {
	p := param("k", "cached")
	repo := &stubSystemParamRepo{param: p}
	svc := newParamSvc(repo)

	// First call populates cache.
	_, _ = svc.Get(context.Background(), "k")
	// Zero out param so second call would fail if it hit the repo.
	repo.param = nil

	got, err := svc.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Value != "cached" {
		t.Errorf("expected cached value, got %v", got)
	}
}

func TestSystemParamService_Get_RepoError_Propagates(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{err: errors.New("db down")})

	_, err := svc.Get(context.Background(), "k")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSystemParamService_Get_KeyAbsent_ReturnsNil(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: nil})

	got, err := svc.Get(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for absent key, got %v", got)
	}
}

// ── Set / eviction ────────────────────────────────────────────────────────────

func TestSystemParamService_Set_EvictsCache(t *testing.T) {
	p := param("k", "old")
	repo := &stubSystemParamRepo{param: p}
	svc := newParamSvc(repo)

	// Populate cache.
	_, _ = svc.Get(context.Background(), "k")

	// Overwrite in repo before Set so the next Get after eviction returns "new".
	repo.param = param("k", "new")

	_, err := svc.Set(context.Background(), "k", "new", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := svc.Get(context.Background(), "k")
	if got == nil || got.Value != "new" {
		t.Errorf("expected 'new' after eviction, got %v", got)
	}
}

func TestSystemParamService_Set_RepoError_Propagates(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{setErr: errors.New("conflict")})

	_, err := svc.Set(context.Background(), "k", "v", 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── GetAll / GetByCategory ────────────────────────────────────────────────────

func TestSystemParamService_GetAll_DelegatesToRepo(t *testing.T) {
	ps := []*domain.SystemParam{param("a", "1"), param("b", "2")}
	svc := newParamSvc(&stubSystemParamRepo{params: ps})

	got, err := svc.GetAll(context.Background())
	if err != nil || len(got) != 2 {
		t.Errorf("expected 2 params, got %v err=%v", got, err)
	}
}

func TestSystemParamService_GetByCategory_DelegatesToRepo(t *testing.T) {
	ps := []*domain.SystemParam{param("scoring.x", "3")}
	svc := newParamSvc(&stubSystemParamRepo{params: ps})

	got, err := svc.GetByCategory(context.Background(), "scoring")
	if err != nil || len(got) != 1 {
		t.Errorf("expected 1 param, got %v err=%v", got, err)
	}
}

// ── GetString ─────────────────────────────────────────────────────────────────

func TestSystemParamService_GetString_KeyPresent_ReturnsValue(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: param("k", "hello")})
	got := svc.GetString(context.Background(), "k", "default")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestSystemParamService_GetString_KeyAbsent_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: nil})
	got := svc.GetString(context.Background(), "k", "default")
	if got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
}

func TestSystemParamService_GetString_RepoError_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{err: errors.New("db error")})
	got := svc.GetString(context.Background(), "k", "fallback")
	if got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
}

// ── GetInt ────────────────────────────────────────────────────────────────────

func TestSystemParamService_GetInt_ValidInt_ReturnsValue(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: param("k", "42")})
	got := svc.GetInt(context.Background(), "k", 0)
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestSystemParamService_GetInt_InvalidInt_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: param("k", "not-a-number")})
	got := svc.GetInt(context.Background(), "k", 99)
	if got != 99 {
		t.Errorf("expected 99, got %d", got)
	}
}

func TestSystemParamService_GetInt_KeyAbsent_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: nil})
	got := svc.GetInt(context.Background(), "k", 7)
	if got != 7 {
		t.Errorf("expected 7, got %d", got)
	}
}

// ── GetDuration ───────────────────────────────────────────────────────────────

func TestSystemParamService_GetDuration_ValidDuration_ReturnsValue(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: param("k", "5m")})
	got := svc.GetDuration(context.Background(), "k", time.Second)
	if got != 5*time.Minute {
		t.Errorf("expected 5m, got %v", got)
	}
}

func TestSystemParamService_GetDuration_InvalidDuration_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: param("k", "bad")})
	got := svc.GetDuration(context.Background(), "k", 10*time.Second)
	if got != 10*time.Second {
		t.Errorf("expected 10s, got %v", got)
	}
}

func TestSystemParamService_GetDuration_KeyAbsent_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: nil})
	got := svc.GetDuration(context.Background(), "k", time.Minute)
	if got != time.Minute {
		t.Errorf("expected 1m, got %v", got)
	}
}

// ── GetBool ───────────────────────────────────────────────────────────────────

func TestSystemParamService_GetBool_True_ReturnsTrue(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: param("k", "true")})
	if !svc.GetBool(context.Background(), "k", false) {
		t.Error("expected true")
	}
}

func TestSystemParamService_GetBool_InvalidBool_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: param("k", "yes")})
	if svc.GetBool(context.Background(), "k", false) {
		t.Error("expected false (default)")
	}
}

func TestSystemParamService_GetBool_KeyAbsent_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: nil})
	if !svc.GetBool(context.Background(), "k", true) {
		t.Error("expected true (default)")
	}
}
