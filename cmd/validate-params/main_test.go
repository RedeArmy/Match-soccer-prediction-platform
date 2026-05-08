package main

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// TestAllParamsHaveConstant verifies that every paramSpec in allParams references
// a valid ParamKey constant from domain/constants.go. This catches typos and
// ensures the validator stays synchronized with the domain package.
func TestAllParamsHaveConstant(t *testing.T) {
	// Map of all valid ParamKey constants
	validKeys := map[string]bool{
		domain.ParamKeyScoringExactScore:        true,
		domain.ParamKeyScoringCorrectOutcome:    true,
		domain.ParamKeyScoringGoalDiff:          true,
		domain.ParamKeyPredictionDeadlineMin:    true,
		domain.ParamKeyGroupMinMembers:          true,
		domain.ParamKeyGroupInviteCodeLength:    true,
		domain.ParamKeyConflictStaleDays:        true,
		domain.ParamKeyConflictMaxScan:          true,
		domain.ParamKeyPaginationDefaultLimit:   true,
		domain.ParamKeyPaginationMaxLimit:       true,
		domain.ParamKeyTournamentWinPoints:      true,
		domain.ParamKeyAdminBulkMaxItems:        true,
		domain.ParamKeyCacheMatchTTL:            true,
		domain.ParamKeyCacheLeaderboardTTL:      true,
		domain.ParamKeyCacheDashboardTTLSeconds: true,
		domain.ParamKeyAuditWriteTimeout:        true,
		domain.ParamKeyAuthValidationTimeout:    true,
		domain.ParamKeyPurgeRetentionDays:       true,
		domain.ParamKeyDLQSampleSize:            true,
		domain.ParamKeyDLQReplayDefaultLimit:    true,
		domain.ParamKeyMessagingMaxRetries:      true,
		domain.ParamKeyMessagingStreamMaxLen:    true,
	}

	for _, spec := range allParams {
		if !validKeys[spec.key] {
			t.Errorf("paramSpec references unknown key %q - not a valid ParamKey* constant", spec.key)
		}
	}
}

// TestAllParamsHaveValidType verifies that every paramSpec uses a valid type
// matching the system_params table schema CHECK constraint.
func TestAllParamsHaveValidType(t *testing.T) {
	validTypes := map[string]bool{
		"string":   true,
		"int":      true,
		"bool":     true,
		"duration": true,
	}

	for _, spec := range allParams {
		if !validTypes[spec.paramType] {
			t.Errorf("%s: invalid type %q (must be: string, int, bool, duration)", spec.key, spec.paramType)
		}
	}
}

// TestAllParamsHaveValidCategory verifies that every paramSpec uses a recognized
// category name for organizational consistency.
func TestAllParamsHaveValidCategory(t *testing.T) {
	validCategories := map[string]bool{
		"scoring":    true,
		"prediction": true,
		"group":      true,
		"conflict":   true,
		"pagination": true,
		"tournament": true,
		"admin":      true,
		"cache":      true,
		"system":     true,
		"dlq":        true,
		"messaging":  true,
		"auth":       true,
	}

	for _, spec := range allParams {
		if !validCategories[spec.category] {
			t.Errorf("%s: unrecognized category %q - consider adding to validCategories or fixing typo", spec.key, spec.category)
		}
	}
}

// TestAllParamsCount verifies that we haven't accidentally removed params from
// the allParams slice. The count should match the number of ParamKey constants
// in domain/constants.go (excluding validation limits like MaxEmailLength).
func TestAllParamsCount(t *testing.T) {
	const expectedCount = 22 // Update when adding new system parameters
	if len(allParams) != expectedCount {
		t.Errorf("expected %d params in allParams, got %d - update expectedCount or fix allParams", expectedCount, len(allParams))
	}
}

// TestDefaultValuesAreNonEmpty verifies that every paramSpec has a non-empty
// default value. Empty defaults indicate a configuration bug.
func TestDefaultValuesAreNonEmpty(t *testing.T) {
	for _, spec := range allParams {
		if spec.defaultValue == "" {
			t.Errorf("%s: defaultValue is empty - every param must have a fallback", spec.key)
		}
	}
}

// TestNoDuplicateKeys verifies that each key appears exactly once in allParams.
func TestNoDuplicateKeys(t *testing.T) {
	seen := make(map[string]bool)
	for _, spec := range allParams {
		if seen[spec.key] {
			t.Errorf("duplicate key in allParams: %s", spec.key)
		}
		seen[spec.key] = true
	}
}
