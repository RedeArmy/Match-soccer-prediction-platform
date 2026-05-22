package main

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// TestAllParamKeysCoveredInAllParams verifies that every domain.ParamKey* constant
// has a corresponding entry in allParams.
//
// Failing here means a new constant was added to the domain package without being
// included in the validate-params tool, so it would not be validated or seeded
// when the tool runs against the production database.
func TestAllParamKeysCoveredInAllParams(t *testing.T) {
	indexed := make(map[string]struct{}, len(allParams))
	for _, p := range allParams {
		indexed[p.key] = struct{}{}
	}

	for _, key := range domain.AllParamKeys() {
		if _, ok := indexed[key]; !ok {
			t.Errorf("param key %q is missing from allParams in cmd/validate-params/main.go — add it to keep the validator in sync", key)
		}
	}
}
