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

const (
	svcUnexpectedErr = "unexpected error: %v"
	svcDefaultParam  = "default"
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
	return NewSystemParamService(repo, nil, zap.NewNop())
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestSystemParamService_Get_CacheMiss_FetchesFromRepo(t *testing.T) {
	p := param("k", "v")
	svc := newParamSvc(&stubSystemParamRepo{param: p})

	got, err := svc.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf(svcUnexpectedErr, err)
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
		t.Fatalf(svcUnexpectedErr, err)
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
		t.Fatalf(svcUnexpectedErr, err)
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
		t.Fatalf(svcUnexpectedErr, err)
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
	got := svc.GetString(context.Background(), "k", svcDefaultParam)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestSystemParamService_GetString_KeyAbsent_ReturnsDefault(t *testing.T) {
	svc := newParamSvc(&stubSystemParamRepo{param: nil})
	got := svc.GetString(context.Background(), "k", svcDefaultParam)
	if got != svcDefaultParam {
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

// ── validateParamValue ────────────────────────────────────────────────────────

func TestValidateParamValue_InvalidInt_ReturnsError(t *testing.T) {
	if err := validateParamValue("not-an-int", domain.SystemParamTypeInt); err == nil {
		t.Error("expected error for invalid int value")
	}
}

func TestValidateParamValue_ValidInt_ReturnsNil(t *testing.T) {
	if err := validateParamValue("42", domain.SystemParamTypeInt); err != nil {
		t.Errorf("unexpected error for valid int: %v", err)
	}
}

func TestValidateParamValue_InvalidBool_ReturnsError(t *testing.T) {
	if err := validateParamValue("maybe", domain.SystemParamTypeBool); err == nil {
		t.Error("expected error for invalid bool value")
	}
}

func TestValidateParamValue_ValidBool_ReturnsNil(t *testing.T) {
	if err := validateParamValue("true", domain.SystemParamTypeBool); err != nil {
		t.Errorf("unexpected error for valid bool: %v", err)
	}
}

func TestValidateParamValue_InvalidDuration_ReturnsError(t *testing.T) {
	if err := validateParamValue("nope", domain.SystemParamTypeDuration); err == nil {
		t.Error("expected error for invalid duration value")
	}
}

func TestValidateParamValue_ValidDuration_ReturnsNil(t *testing.T) {
	if err := validateParamValue("5m", domain.SystemParamTypeDuration); err != nil {
		t.Errorf("unexpected error for valid duration: %v", err)
	}
}

func TestValidateParamValue_StringType_AlwaysValid(t *testing.T) {
	if err := validateParamValue("anything goes", domain.SystemParamTypeString); err != nil {
		t.Errorf("unexpected error for string type: %v", err)
	}
}

// ── BulkSet ───────────────────────────────────────────────────────────────────

func typedParam(key, value string, typ domain.SystemParamType) *domain.SystemParam {
	return &domain.SystemParam{Key: key, Value: value, Type: typ}
}

func TestSystemParamService_BulkSet_Success_EvictsCache(t *testing.T) {
	p := typedParam("x", "1", domain.SystemParamTypeInt)
	repo := &stubSystemParamRepo{param: p}
	svc := newParamSvc(repo)

	// Populate cache so we can verify eviction.
	_, _ = svc.Get(context.Background(), "x")

	repo.param = typedParam("x", "99", domain.SystemParamTypeInt)
	if err := svc.BulkSet(context.Background(), map[string]string{"x": "99"}, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := svc.Get(context.Background(), "x")
	if got == nil || got.Value != "99" {
		t.Errorf("expected evicted cache to return '99', got %v", got)
	}
}

func TestSystemParamService_BulkSet_InvalidValue_ReturnsValidationError(t *testing.T) {
	repo := &stubSystemParamRepo{param: typedParam("n", "5", domain.SystemParamTypeInt)}
	svc := newParamSvc(repo)

	err := svc.BulkSet(context.Background(), map[string]string{"n": "not-a-number"}, 1)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestSystemParamService_BulkSet_RepoError_Propagates(t *testing.T) {
	repo := &stubSystemParamRepo{param: nil, err: errors.New("db down")}
	svc := newParamSvc(repo)

	err := svc.BulkSet(context.Background(), map[string]string{"k": "v"}, 1)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

func TestSystemParamService_Set_WithAudit_CallsAuditLogger(t *testing.T) {
	repo := &stubSystemParamRepo{param: typedParam("scoring.exact_score", "5", domain.SystemParamTypeInt)}
	audit := &spyAuditLogger{}
	svc := NewSystemParamService(repo, audit, zap.NewNop())

	if _, err := svc.Set(context.Background(), "scoring.exact_score", "5", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit logger to be called on Set")
	}
	if audit.action != domain.AuditActionParamUpdated {
		t.Errorf("expected action %q, got %q", domain.AuditActionParamUpdated, audit.action)
	}
}

func TestSystemParamService_BulkSet_WithAudit_CallsAuditLogger(t *testing.T) {
	repo := &stubSystemParamRepo{param: typedParam("scoring.exact_score", "5", domain.SystemParamTypeInt)}
	audit := &spyAuditLogger{}
	svc := NewSystemParamService(repo, audit, zap.NewNop())

	if err := svc.BulkSet(context.Background(), map[string]string{"scoring.exact_score": "5"}, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit logger to be called on BulkSet")
	}
	if audit.action != domain.AuditActionParamUpdated {
		t.Errorf("expected action %q, got %q", domain.AuditActionParamUpdated, audit.action)
	}
}

// ── Runtime TTL ───────────────────────────────────────────────────────────────

func TestSystemParamService_RuntimeParam_UsesShortTTL(t *testing.T) {
	runtimeParam := &domain.SystemParam{Key: "rt.key", Value: "v", IsRuntime: true}
	infraParam := &domain.SystemParam{Key: "infra.key", Value: "v", IsRuntime: false}

	svc := newParamSvc(&stubSystemParamRepo{param: runtimeParam}).(*systemParamService)
	svc.runtimeTTL = 0 // zero TTL → always expired, forces re-fetch every call

	// Populate cache for runtime param — entry should be written with runtimeTTL (0 here).
	_, _ = svc.Get(context.Background(), "rt.key")
	svc.mu.RLock()
	rtEntry := svc.cache["rt.key"]
	svc.mu.RUnlock()
	if rtEntry == nil {
		t.Fatal("expected cache entry for runtime param")
	}

	// Populate cache for infra param — entry should use the regular ttl (5 min).
	svc.repo = &stubSystemParamRepo{param: infraParam}
	_, _ = svc.Get(context.Background(), "infra.key")
	svc.mu.RLock()
	infraEntry := svc.cache["infra.key"]
	svc.mu.RUnlock()
	if infraEntry == nil {
		t.Fatal("expected cache entry for infra param")
	}

	// infra entry should expire later than the zero-TTL runtime entry.
	if !infraEntry.expiresAt.After(rtEntry.expiresAt) {
		t.Error("infra param should have a longer cache TTL than runtime param")
	}
}
