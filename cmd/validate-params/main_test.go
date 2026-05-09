package main

import (
	"os"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── unit tests for validation logic ──────────────────────────────────────────

func TestBuildParamMap_IndexesByKey(t *testing.T) {
	input := []dbParam{
		{key: "a.b", value: "1", paramType: "int", category: "a"},
		{key: "c.d", value: "2", paramType: "int", category: "c"},
	}
	m := buildParamMap(input)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if m["a.b"].value != "1" {
		t.Errorf("a.b: got %q, want %q", m["a.b"].value, "1")
	}
}

func TestValidateType_Match_ReturnsNoErrors(t *testing.T) {
	spec := paramSpec{key: "a.b", paramType: "int"}
	db := dbParam{key: "a.b", paramType: "int"}
	errs := validateType(spec, db)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateType_Mismatch_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", paramType: "int"}
	db := dbParam{key: "a.b", paramType: "string"}
	errs := validateType(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestValidateCategory_Match_ReturnsNoErrors(t *testing.T) {
	spec := paramSpec{key: "a.b", category: "group"}
	db := dbParam{key: "a.b", category: "group"}
	errs := validateCategory(spec, db)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateCategory_Mismatch_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", category: "group"}
	db := dbParam{key: "a.b", category: "system"}
	errs := validateCategory(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestValidateDescription_Present_ReturnsNoErrors(t *testing.T) {
	spec := paramSpec{key: "a.b"}
	db := dbParam{key: "a.b", description: "some description"}
	errs := validateDescription(spec, db)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateDescription_Empty_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b"}
	db := dbParam{key: "a.b", description: ""}
	errs := validateDescription(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestValidateSingleParam_Missing_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	errs := validateSingleParam(spec, map[string]dbParam{})
	if len(errs) != 1 {
		t.Errorf("expected 1 MISSING error, got %d: %v", len(errs), errs)
	}
}

func TestValidateSingleParam_Valid_ReturnsNoErrors(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	db := map[string]dbParam{
		"a.b": {key: "a.b", value: "5", paramType: "int", category: "group", description: "ok"},
	}
	errs := validateSingleParam(spec, db)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateSingleParam_TypeMismatch_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	db := map[string]dbParam{
		"a.b": {key: "a.b", value: "5", paramType: "string", category: "group", description: "ok"},
	}
	errs := validateSingleParam(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error (type mismatch), got %d: %v", len(errs), errs)
	}
}

func TestValidateSingleParam_CategoryMismatch_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	db := map[string]dbParam{
		"a.b": {key: "a.b", value: "5", paramType: "int", category: "system", description: "ok"},
	}
	errs := validateSingleParam(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error (category mismatch), got %d: %v", len(errs), errs)
	}
}

func TestValidateSingleParam_MultipleErrors_CollectedTogether(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	db := map[string]dbParam{
		// wrong type, wrong category, empty description — three errors
		"a.b": {key: "a.b", value: "5", paramType: "bool", category: "system", description: ""},
	}
	errs := validateSingleParam(spec, db)
	if len(errs) != 3 {
		t.Errorf("expected 3 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateAllParams_AllValid_ReturnsNoErrors(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	saved := allParams
	allParams = []paramSpec{spec}
	defer func() { allParams = saved }()

	db := map[string]dbParam{
		"a.b": {key: "a.b", value: "5", paramType: "int", category: "group", description: "ok"},
	}
	errs := validateAllParams(db)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateAllParams_OneMissing_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	saved := allParams
	allParams = []paramSpec{spec}
	defer func() { allParams = saved }()

	errs := validateAllParams(map[string]dbParam{})
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestBuildExpectedKeysSet_ContainsAllParamKeys(t *testing.T) {
	keys := buildExpectedKeysSet()
	for _, spec := range allParams {
		if !keys[spec.key] {
			t.Errorf("expected key %q in set", spec.key)
		}
	}
	if len(keys) != len(allParams) {
		t.Errorf("set size %d != allParams size %d", len(keys), len(allParams))
	}
}

func TestCheckUnexpectedParams_NoPanic(t *testing.T) {
	known := []dbParam{{key: allParams[0].key}}
	unknown := []dbParam{{key: "unknown.key.xyz"}}
	// These just print to stdout; we verify they don't panic.
	checkUnexpectedParams(known)
	checkUnexpectedParams(unknown)
}

func TestReportResults_NoErrors_ReturnsNil(t *testing.T) {
	if err := reportResults(nil); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if err := reportResults([]string{}); err != nil {
		t.Errorf("expected nil for empty slice, got %v", err)
	}
}

func TestReportResults_WithErrors_ReturnsError(t *testing.T) {
	err := reportResults([]string{"❌ MISSING: a.b"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCheckValueOverride_NoPanic(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5"}
	checkValueOverride(spec, dbParam{key: "a.b", value: "5"})   // no override
	checkValueOverride(spec, dbParam{key: "a.b", value: "999"}) // override — prints warning
}

func TestValidateFromParams_AllValid_ReturnsNil(t *testing.T) {
	saved := allParams
	allParams = []paramSpec{{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}}
	defer func() { allParams = saved }()

	params := []dbParam{{key: "a.b", value: "5", paramType: "int", category: "group", description: "ok"}}
	if err := validateFromParams(params); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidateFromParams_MissingParam_ReturnsError(t *testing.T) {
	saved := allParams
	allParams = []paramSpec{{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}}
	defer func() { allParams = saved }()

	if err := validateFromParams(nil); err == nil {
		t.Fatal("expected error for missing param, got nil")
	}
}

func TestValidateFromParams_UnexpectedParamInDB_NoPanic(t *testing.T) {
	saved := allParams
	allParams = []paramSpec{{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}}
	defer func() { allParams = saved }()

	// DB contains a.b (valid) plus an unknown key — checkUnexpectedParams prints warning, no panic.
	params := []dbParam{
		{key: "a.b", value: "5", paramType: "int", category: "group", description: "ok"},
		{key: "unknown.key", value: "x", paramType: "string", category: "other", description: "?"},
	}
	if err := validateFromParams(params); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConnectDatabase_MissingEnvVar_ReturnsError(t *testing.T) {
	prev := os.Getenv("DATABASE_URL")
	os.Unsetenv("DATABASE_URL")
	defer os.Setenv("DATABASE_URL", prev)

	_, err := connectDatabase()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is not set")
	}
}

func TestRun_MissingEnvVar_PropagatesError(t *testing.T) {
	prev := os.Getenv("DATABASE_URL")
	os.Unsetenv("DATABASE_URL")
	defer os.Setenv("DATABASE_URL", prev)

	if err := run(); err == nil {
		t.Fatal("expected error from run() when DATABASE_URL is not set")
	}
}

// TestAllParamsHaveConstant verifies that every paramSpec in allParams references
// a valid ParamKey constant from domain/constants.go. This catches typos and
// ensures the validator stays synchronized with the domain package.
func TestAllParamsHaveConstant(t *testing.T) {
	// Map of all valid ParamKey constants (keep in sync with domain/constants.go)
	validKeys := map[string]bool{
		domain.ParamKeyScoringExactScore:        true,
		domain.ParamKeyScoringCorrectOutcome:    true,
		domain.ParamKeyScoringGoalDiff:          true,
		domain.ParamKeyPredictionDeadlineMin:    true,
		domain.ParamKeyGroupMinMembers:          true,
		domain.ParamKeyGroupMaxSize:             true,
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
		// Added by migration 000055
		domain.ParamKeyMessagingStreamWorkerCount:  true,
		domain.ParamKeyMessagingStreamReadBlockSec: true,
		domain.ParamKeyAuditMaxRetries:             true,
		domain.ParamKeyAuditRetryDelayMs:           true,
		domain.ParamKeyWorkerSnapshotConcurrency:   true,
		domain.ParamKeyWorkerSnapshotRetryBaseMs:   true,
		domain.ParamKeyWorkerSnapshotMaxAttempts:   true,
		domain.ParamKeyWorkerDLQMonitorIntervalSec: true,
		domain.ParamKeyWorkerPurgeIntervalHours:    true,
		domain.ParamKeyAPIBodySizeLimitBytes:       true,
		domain.ParamKeySnapshotKeepLatestCount:     true,
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
		// Added by migration 000055
		"worker": true,
		"api":    true,
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
	const expectedCount = 34 // Update when adding new system parameters (was 33; +1 snapshot.keep_latest_count from migration 000056)
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
