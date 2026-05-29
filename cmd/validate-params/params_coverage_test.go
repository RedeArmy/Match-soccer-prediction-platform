package main

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// TestAllParamKeysCoveredInAllParams verifies that every domain.AllParamKeys() entry
// has a corresponding paramSpec in allParams.
//
// Failing here means a new constant was added to the domain package (and to
// AllParamKeys) without being registered in the validate-params tool, so it
// would not be validated or seeded when the tool runs against the production DB.
func TestAllParamKeysCoveredInAllParams(t *testing.T) {
	indexed := make(map[string]struct{}, len(allParams))
	for _, p := range allParams {
		indexed[p.key] = struct{}{}
	}

	for _, key := range domain.AllParamKeys() {
		if _, ok := indexed[key]; !ok {
			t.Errorf("param key %q is in AllParamKeys() but missing from allParams in cmd/validate-params/main.go — add a paramSpec entry", key)
		}
	}
}

// TestAllParamsRegisteredInAllParamKeys is the inverse of TestAllParamKeysCoveredInAllParams.
// It verifies that every paramSpec in allParams has a corresponding entry in
// domain.AllParamKeys(). This closes the bidirectional coverage gap: together
// the two tests ensure AllParamKeys() and allParams are identical sets.
//
// Failing here means a paramSpec was added to allParams (or to the domain
// constants) without updating domain.AllParamKeys() in constants.go.
func TestAllParamsRegisteredInAllParamKeys(t *testing.T) {
	listed := make(map[string]struct{}, len(domain.AllParamKeys()))
	for _, key := range domain.AllParamKeys() {
		listed[key] = struct{}{}
	}

	for _, spec := range allParams {
		if _, ok := listed[spec.key]; !ok {
			t.Errorf("paramSpec key %q is in allParams but missing from domain.AllParamKeys() in constants.go — add it to AllParamKeys()", spec.key)
		}
	}
}
