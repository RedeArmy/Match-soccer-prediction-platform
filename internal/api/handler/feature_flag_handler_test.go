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
	"github.com/rede/world-cup-quiniela/internal/domain"
)

// flagSvc overrides GetAll on the shared stubSystemParamSvc so individual tests
// can control which params (and errors) the feature-flag handler sees.
type flagSvc struct {
	stubSystemParamSvc
	params []*domain.SystemParam
	err    error
}

func (s *flagSvc) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return s.params, s.err
}

func featureFlagRouter(t *testing.T, svc *flagSvc) http.Handler {
	t.Helper()
	h := handler.NewFeatureFlagHandler(svc, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /feature-flags", h.GetAll)
	return mux
}

func TestFeatureFlagHandler_GetAll_ReturnsFilteredFlags(t *testing.T) {
	svc := &flagSvc{
		params: []*domain.SystemParam{
			{Key: "featflag.paypal", Value: "1"},
			{Key: "scoring.exact_score", Value: "5"},
		},
	}
	router := featureFlagRouter(t, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/feature-flags", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["paypal"] != "1" {
		t.Errorf(`flags["paypal"]: got %q, want "1"`, resp["paypal"])
	}
	if _, ok := resp["scoring.exact_score"]; ok {
		t.Error("non-featflag param should have been filtered out of response")
	}
}

func TestFeatureFlagHandler_GetAll_EmptyResult_ReturnsEmptyObject(t *testing.T) {
	svc := &flagSvc{params: []*domain.SystemParam{}}
	router := featureFlagRouter(t, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/feature-flags", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected empty map, got %v", resp)
	}
}

func TestFeatureFlagHandler_GetAll_ServiceError_Returns500(t *testing.T) {
	svc := &flagSvc{err: errors.New("db down")}
	router := featureFlagRouter(t, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/feature-flags", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}
