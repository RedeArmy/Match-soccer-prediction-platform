package service

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// TestSystemParamConstraintCoverage verifies that every domain.ParamKey* constant
// is registered in either paramIntConstraints or paramStringConstraints.
//
// Failing here means a new constant was added to the domain package without a
// corresponding entry in the constraint maps. Without a registration the service
// will silently accept any value for that key, bypassing business-rule enforcement.
func TestSystemParamConstraintCoverage(t *testing.T) {
	for _, key := range domain.AllParamKeys() {
		_, isInt := paramIntConstraints[key]
		_, isStr := paramStringConstraints[key]
		if !isInt && !isStr {
			t.Errorf("param key %q is not registered in paramIntConstraints or paramStringConstraints — add a range/format constraint in system_param_service.go", key)
		}
	}
}
