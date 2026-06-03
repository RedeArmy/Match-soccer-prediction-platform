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
		"a.b": {key: "a.b", value: "5", defaultValue: "5", paramType: "int", category: "group", description: "ok"},
	}
	errs := validateSingleParam(spec, db)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateSingleParam_TypeMismatch_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	db := map[string]dbParam{
		"a.b": {key: "a.b", value: "5", defaultValue: "5", paramType: "string", category: "group", description: "ok"},
	}
	errs := validateSingleParam(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error (type mismatch), got %d: %v", len(errs), errs)
	}
}

func TestValidateSingleParam_CategoryMismatch_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	db := map[string]dbParam{
		"a.b": {key: "a.b", value: "5", defaultValue: "5", paramType: "int", category: "system", description: "ok"},
	}
	errs := validateSingleParam(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error (category mismatch), got %d: %v", len(errs), errs)
	}
}

func TestValidateSingleParam_MultipleErrors_CollectedTogether(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	db := map[string]dbParam{
		// wrong type, wrong category, empty description — three errors; defaultValue matches to isolate those three
		"a.b": {key: "a.b", value: "5", defaultValue: "5", paramType: "bool", category: "system", description: ""},
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
		"a.b": {key: "a.b", value: "5", defaultValue: "5", paramType: "int", category: "group", description: "ok"},
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

func TestCheckUnexpectedParams_KnownKey_ReturnsNoErrors(t *testing.T) {
	errs := checkUnexpectedParams([]dbParam{{key: allParams[0].key}})
	if len(errs) != 0 {
		t.Errorf("expected no errors for a known key, got %v", errs)
	}
}

func TestCheckUnexpectedParams_OrphanKey_ReturnsError(t *testing.T) {
	errs := checkUnexpectedParams([]dbParam{{key: "orphan.key.xyz"}})
	if len(errs) != 1 {
		t.Errorf("expected 1 error for orphan key, got %d: %v", len(errs), errs)
	}
}

func TestCheckUnexpectedParams_MixedKeys_OnlyOrphansError(t *testing.T) {
	params := []dbParam{
		{key: allParams[0].key},    // known
		{key: "orphan.legacy.key"}, // orphan
	}
	errs := checkUnexpectedParams(params)
	if len(errs) != 1 {
		t.Errorf("expected 1 error (orphan only), got %d: %v", len(errs), errs)
	}
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

	params := []dbParam{{key: "a.b", value: "5", defaultValue: "5", paramType: "int", category: "group", description: "ok"}}
	if err := validateFromParams(params); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidateIsRuntime_Match_ReturnsNoErrors(t *testing.T) {
	spec := paramSpec{key: "a.b", isRuntime: true}
	db := dbParam{key: "a.b", isRuntime: true}
	errs := validateIsRuntime(spec, db)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateIsRuntime_FalseFalse_ReturnsNoErrors(t *testing.T) {
	spec := paramSpec{key: "a.b", isRuntime: false}
	db := dbParam{key: "a.b", isRuntime: false}
	errs := validateIsRuntime(spec, db)
	if len(errs) != 0 {
		t.Errorf("expected no errors for matching false/false, got %v", errs)
	}
}

func TestValidateIsRuntime_Mismatch_TrueExpectedFalseGot_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", isRuntime: true}
	db := dbParam{key: "a.b", isRuntime: false}
	errs := validateIsRuntime(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for true/false mismatch, got %d: %v", len(errs), errs)
	}
}

func TestValidateIsRuntime_Mismatch_FalseExpectedTrueGot_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", isRuntime: false}
	db := dbParam{key: "a.b", isRuntime: true}
	errs := validateIsRuntime(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for false/true mismatch, got %d: %v", len(errs), errs)
	}
}

// TestAllParamsIsRuntimeCoverage verifies that the allParams catalog contains
// at least one runtime and one non-runtime param. A catalog where all params
// share the same flag is a sign that isRuntime was forgotten during a bulk-add.
func TestAllParamsIsRuntimeCoverage(t *testing.T) {
	var hasRuntime, hasNonRuntime bool
	for _, spec := range allParams {
		if spec.isRuntime {
			hasRuntime = true
		} else {
			hasNonRuntime = true
		}
	}
	if !hasRuntime {
		t.Error("allParams has no runtime params (isRuntime=true) — check that isRuntime is populated")
	}
	if !hasNonRuntime {
		t.Error("allParams has no non-runtime params (isRuntime=false) — check that isRuntime is populated")
	}
}

func TestValidateSingleParam_DefaultValueMismatch_ReturnsError(t *testing.T) {
	spec := paramSpec{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}
	db := map[string]dbParam{
		// default_value in DB differs from code constant — migration seed out of sync
		"a.b": {key: "a.b", value: "5", defaultValue: "99", paramType: "int", category: "group", description: "ok"},
	}
	errs := validateSingleParam(spec, db)
	if len(errs) != 1 {
		t.Errorf("expected 1 error (default value mismatch), got %d: %v", len(errs), errs)
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

func TestValidateFromParams_OrphanParamInDB_ReturnsError(t *testing.T) {
	saved := allParams
	allParams = []paramSpec{{key: "a.b", defaultValue: "5", paramType: "int", category: "group"}}
	defer func() { allParams = saved }()

	// DB contains a.b (valid) plus an orphan key with no Go constant — must fail.
	params := []dbParam{
		{key: "a.b", value: "5", defaultValue: "5", paramType: "int", category: "group", description: "ok"},
		{key: "orphan.legacy.key", value: "x", defaultValue: "x", paramType: "string", category: "other", description: "?"},
	}
	if err := validateFromParams(params); err == nil {
		t.Fatal("expected error for orphan DB param, got nil")
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

// TestAllParamsHaveConstant verifies bidirectional coverage between allParams
// and domain.AllParamKeys():
//
//   - Every paramSpec key must appear in domain.AllParamKeys() — catches typos
//     and keys added to allParams without a corresponding Go constant.
//   - Every domain.AllParamKeys() entry must have a paramSpec — catches constants
//     added to constants.go without being wired into the validator.
//
// Previously this test maintained a ~140-entry hardcoded map that had to be
// kept in sync manually. Using domain.AllParamKeys() removes that maintenance
// burden: the test is automatically correct whenever AllParamKeys() is updated.
func TestAllParamsHaveConstant(t *testing.T) {
	domainKeys := make(map[string]bool, len(domain.AllParamKeys()))
	for _, k := range domain.AllParamKeys() {
		domainKeys[k] = true
	}

	// allParams → domain: every paramSpec must reference a known constant.
	for _, spec := range allParams {
		if !domainKeys[spec.key] {
			t.Errorf("paramSpec references unknown key %q — not in domain.AllParamKeys()", spec.key)
		}
	}

	// domain → allParams: every constant must have a paramSpec entry.
	allParamsKeys := buildExpectedKeysSet()
	for _, k := range domain.AllParamKeys() {
		if !allParamsKeys[k] {
			t.Errorf("domain.AllParamKeys() contains %q but it has no paramSpec in allParams — add it or remove the constant", k)
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
		// Added by migration 000073
		"payment": true,
		// Added by migration 000080
		"breaker":    true,
		"repository": true,
		// Added by migration 000087
		"notify": true,
		// Added by migrations 000121 + 000125
		"kyc": true,
		// Added by migration 000150
		"fx": true,
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
	const expectedCount = 138 // Update when adding new system parameters (+10 kyc gate from 000121, +1 kyc cache ttl from 000125, +2 ip velocity from 000129, +1 sse max conns from 000136, +1 scoring chunk size from 000138, +4 ip rate limit from 000142, +1 kyc doc retention from 000144, +1 usd_gtq_rate from 000147, +1 exchange_rate_margin from 000148, +4 fx margin from 000150, +1 intent_max_cents from 000151, +2 admin rate limit from 000153, +1 audit max_in_flight from 000154, +1 fx history retention from 000155, +1 outbox retention from 000158, +3 sse evict+lb publish from 000160, +3 fx timeouts from 000161, +1 payment intent retention from 000162)
	if len(allParams) != expectedCount {
		t.Errorf("expected %d params in allParams, got %d - update expectedCount or fix allParams", expectedCount, len(allParams))
	}
}

// TestDefaultValuesAreNonEmpty verifies that every non-string paramSpec has a
// non-empty default value. String params whose defaults are intentionally empty
// (e.g. VAPID keys, admin email list) must be set by the operator at deployment
// time and are therefore exempt from this check.
func TestDefaultValuesAreNonEmpty(t *testing.T) {
	for _, spec := range allParams {
		if spec.paramType == "string" {
			continue // operator-supplied credentials: empty default is valid
		}
		if spec.defaultValue == "" {
			t.Errorf("%s: defaultValue is empty - every non-string param must have a fallback", spec.key)
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

// TestValidateFromParams_FullSnapshot_ReturnsNil is the end-to-end correctness
// test for the entire allParams catalog. It builds a synthetic database snapshot
// where every param exists with its canonical default value, matching type,
// matching category, and a placeholder description, then asserts that
// validateFromParams returns nil.
//
// This test fails when any entry in allParams has inconsistent fields (e.g. a
// defaultValue that does not match what would be seeded by the migration), or
// when a new param is added without keeping its spec self-consistent.
func TestValidateFromParams_FullSnapshot_ReturnsNil(t *testing.T) {
	params := make([]dbParam, len(allParams))
	for i, spec := range allParams {
		params[i] = dbParam{
			key:          spec.key,
			value:        spec.defaultValue,
			defaultValue: spec.defaultValue,
			paramType:    spec.paramType,
			category:     spec.category,
			isRuntime:    spec.isRuntime, // must mirror the spec to pass validateIsRuntime
			description:  "canonical default — synthetic test snapshot",
		}
	}
	if err := validateFromParams(params); err != nil {
		t.Errorf("full-snapshot validation failed: %v", err)
	}
}
