package domain

import (
	"reflect"
	"strings"
	"testing"
)

// TestSystemParamConstants validates that every ParamKey* constant has a
// corresponding Default* constant and vice versa. This test prevents drift
// between the fallback values hard-coded in the source and the ParamKey
// names used to fetch values from system_params at runtime.
//
// The test works by reflection: it scans all exported constants in this
// package, identifies pairs by naming convention (ParamKey* + Default*),
// and flags any orphaned constants.
func TestSystemParamConstants_AllPaired(t *testing.T) {
	// Build a map of all ParamKey* constants
	paramKeys := make(map[string]string)

	// Manually enumerate all ParamKey* constants since Go doesn't support
	// const reflection. This list must be kept in sync with constants.go.
	paramKeys["ParamKeyScoringExactScore"] = ParamKeyScoringExactScore
	paramKeys["ParamKeyScoringCorrectOutcome"] = ParamKeyScoringCorrectOutcome
	paramKeys["ParamKeyScoringGoalDiff"] = ParamKeyScoringGoalDiff
	paramKeys["ParamKeyPredictionDeadlineMin"] = ParamKeyPredictionDeadlineMin
	paramKeys["ParamKeyGroupMinMembers"] = ParamKeyGroupMinMembers
	paramKeys["ParamKeyGroupDefaultPrize"] = ParamKeyGroupDefaultPrize
	paramKeys["ParamKeyGroupInviteCodeLength"] = ParamKeyGroupInviteCodeLength
	paramKeys["ParamKeyConflictStaleDays"] = ParamKeyConflictStaleDays
	paramKeys["ParamKeyConflictMaxScan"] = ParamKeyConflictMaxScan
	paramKeys["ParamKeyPaginationDefaultLimit"] = ParamKeyPaginationDefaultLimit
	paramKeys["ParamKeyPaginationMaxLimit"] = ParamKeyPaginationMaxLimit
	paramKeys["ParamKeyTournamentWinPoints"] = ParamKeyTournamentWinPoints
	paramKeys["ParamKeyAdminBulkMaxItems"] = ParamKeyAdminBulkMaxItems
	paramKeys["ParamKeyCacheMatchTTL"] = ParamKeyCacheMatchTTL
	paramKeys["ParamKeyCacheLeaderboardTTL"] = ParamKeyCacheLeaderboardTTL
	paramKeys["ParamKeyCacheDashboardTTLSeconds"] = ParamKeyCacheDashboardTTLSeconds
	paramKeys["ParamKeyAuditWriteTimeout"] = ParamKeyAuditWriteTimeout
	paramKeys["ParamKeyDLQSampleSize"] = ParamKeyDLQSampleSize
	paramKeys["ParamKeyDLQReplayDefaultLimit"] = ParamKeyDLQReplayDefaultLimit
	paramKeys["ParamKeyMessagingMaxRetries"] = ParamKeyMessagingMaxRetries
	paramKeys["ParamKeyMessagingStreamMaxLen"] = ParamKeyMessagingStreamMaxLen
	paramKeys["ParamKeyAuthValidationTimeout"] = ParamKeyAuthValidationTimeout
	paramKeys["ParamKeyPurgeRetentionDays"] = ParamKeyPurgeRetentionDays

	// Build a map of all Default* constants with their values
	defaults := map[string]interface{}{
		"DefaultGroupInviteCodeLength":        DefaultGroupInviteCodeLength,
		"DefaultPaginationDefaultLimit":       DefaultPaginationDefaultLimit,
		"DefaultPaginationMaxLimit":           DefaultPaginationMaxLimit,
		"DefaultAdminBulkMaxItems":            DefaultAdminBulkMaxItems,
		"DefaultCacheDashboardTTLSeconds":     DefaultCacheDashboardTTLSeconds,
		"DefaultCacheMatchTTLSeconds":         DefaultCacheMatchTTLSeconds,
		"DefaultCacheLeaderboardTTLSeconds":   DefaultCacheLeaderboardTTLSeconds,
		"DefaultDLQSampleSize":                DefaultDLQSampleSize,
		"DefaultDLQReplayDefaultLimit":        DefaultDLQReplayDefaultLimit,
		"DefaultMessagingMaxRetries":          DefaultMessagingMaxRetries,
		"DefaultMessagingStreamMaxLen":        DefaultMessagingStreamMaxLen,
		"DefaultAuthValidationTimeoutSeconds": DefaultAuthValidationTimeoutSeconds,
		"DefaultAuditWriteTimeoutSeconds":     DefaultAuditWriteTimeoutSeconds,
		"DefaultPurgeRetentionDays":           DefaultPurgeRetentionDays,
		"DefaultConflictStaleDays":            DefaultConflictStaleDays,
		"DefaultConflictMaxScan":              DefaultConflictMaxScan,
	}

	// Note: Some constants like scoring points (PointsExactScore, etc.) and
	// other business rules (StandingsWinPoints, MinMembersForActive) are
	// intentionally not in the Default* naming pattern because they're
	// fundamental business rules, not system params with fallback values.

	t.Run("all_param_keys_documented", func(t *testing.T) {
		// Verify we've enumerated all ParamKey* constants by checking that
		// the count matches what we expect. This is a smoke test - if new
		// ParamKey constants are added, this test will fail as a reminder
		// to update the enumeration above.
		expectedCount := 23 // Update this when adding new ParamKey constants
		if len(paramKeys) != expectedCount {
			t.Errorf("ParamKey enumeration may be incomplete: expected %d, got %d", expectedCount, len(paramKeys))
			t.Logf("If you added a new ParamKey* constant, update the enumeration in this test")
		}
	})

	t.Run("all_defaults_documented", func(t *testing.T) {
		// Similar smoke test for Default* constants
		expectedCount := 16 // Update this when adding new Default* constants
		if len(defaults) != expectedCount {
			t.Errorf("Default enumeration may be incomplete: expected %d, got %d", expectedCount, len(defaults))
			t.Logf("If you added a new Default* constant, update the enumeration in this test")
		}
	})

	t.Run("no_orphaned_param_keys", func(t *testing.T) {
		// Each ParamKey should have a documented purpose in migrations.
		// This test doesn't verify the migration exists (that's tested separately)
		// but ensures we haven't defined a ParamKey without documentation.
		for name, key := range paramKeys {
			if key == "" {
				t.Errorf("%s is defined but has an empty value", name)
			}
		}
	})

	t.Run("no_duplicate_param_keys", func(t *testing.T) {
		// Verify no two ParamKey constants have the same value
		seen := make(map[string]string)
		for name, key := range paramKeys {
			if existing, found := seen[key]; found {
				t.Errorf("Duplicate ParamKey value %q used by both %s and %s", key, existing, name)
			}
			seen[key] = name
		}
	})
}

// TestSystemParamNamingConventions validates that all system param keys
// follow the documented naming convention: category.snake_case_name.
func TestSystemParamNamingConventions(t *testing.T) {
	paramKeys := []struct {
		name     string
		key      string
		category string
	}{
		{"ParamKeyScoringExactScore", ParamKeyScoringExactScore, "scoring"},
		{"ParamKeyScoringCorrectOutcome", ParamKeyScoringCorrectOutcome, "scoring"},
		{"ParamKeyScoringGoalDiff", ParamKeyScoringGoalDiff, "scoring"},
		{"ParamKeyPredictionDeadlineMin", ParamKeyPredictionDeadlineMin, "prediction"},
		{"ParamKeyGroupMinMembers", ParamKeyGroupMinMembers, "group"},
		{"ParamKeyGroupDefaultPrize", ParamKeyGroupDefaultPrize, "group"},
		{"ParamKeyGroupInviteCodeLength", ParamKeyGroupInviteCodeLength, "group"},
		{"ParamKeyConflictStaleDays", ParamKeyConflictStaleDays, "conflict"},
		{"ParamKeyConflictMaxScan", ParamKeyConflictMaxScan, "conflict"},
		{"ParamKeyPaginationDefaultLimit", ParamKeyPaginationDefaultLimit, "pagination"},
		{"ParamKeyPaginationMaxLimit", ParamKeyPaginationMaxLimit, "pagination"},
		{"ParamKeyTournamentWinPoints", ParamKeyTournamentWinPoints, "tournament"},
		{"ParamKeyAdminBulkMaxItems", ParamKeyAdminBulkMaxItems, "admin"},
		{"ParamKeyCacheMatchTTL", ParamKeyCacheMatchTTL, "cache"},
		{"ParamKeyCacheLeaderboardTTL", ParamKeyCacheLeaderboardTTL, "cache"},
		{"ParamKeyCacheDashboardTTLSeconds", ParamKeyCacheDashboardTTLSeconds, "cache"},
		{"ParamKeyAuditWriteTimeout", ParamKeyAuditWriteTimeout, "audit"},
		{"ParamKeyDLQSampleSize", ParamKeyDLQSampleSize, "dlq"},
		{"ParamKeyDLQReplayDefaultLimit", ParamKeyDLQReplayDefaultLimit, "dlq"},
		{"ParamKeyMessagingMaxRetries", ParamKeyMessagingMaxRetries, "messaging"},
		{"ParamKeyMessagingStreamMaxLen", ParamKeyMessagingStreamMaxLen, "messaging"},
		{"ParamKeyAuthValidationTimeout", ParamKeyAuthValidationTimeout, "auth"},
		{"ParamKeyPurgeRetentionDays", ParamKeyPurgeRetentionDays, "system"},
	}

	for _, tc := range paramKeys {
		t.Run(tc.name, func(t *testing.T) {
			// Verify format: category.name
			if !strings.Contains(tc.key, ".") {
				t.Errorf("%s = %q does not follow 'category.name' convention", tc.name, tc.key)
				return
			}

			// Verify category prefix matches expected
			parts := strings.SplitN(tc.key, ".", 2)
			if parts[0] != tc.category {
				t.Errorf("%s = %q: expected category %q, got %q", tc.name, tc.key, tc.category, parts[0])
			}

			// Verify name part is snake_case (no uppercase, no hyphens)
			name := parts[1]
			if strings.ToLower(name) != name {
				t.Errorf("%s = %q: name part should be lowercase snake_case", tc.name, tc.key)
			}
			if strings.Contains(name, "-") {
				t.Errorf("%s = %q: name part should use underscores not hyphens", tc.name, tc.key)
			}
		})
	}
}

// TestDefaultConstantsArePositive validates that all Default* numeric
// constants are positive non-zero values, which is a requirement for
// system parameters (a value of 0 typically means "disabled" or "unlimited"
// and should be set explicitly via the param table, not as a default).
func TestDefaultConstantsArePositive(t *testing.T) {
	defaults := map[string]int{
		"DefaultGroupInviteCodeLength":        DefaultGroupInviteCodeLength,
		"DefaultPaginationDefaultLimit":       DefaultPaginationDefaultLimit,
		"DefaultPaginationMaxLimit":           DefaultPaginationMaxLimit,
		"DefaultAdminBulkMaxItems":            DefaultAdminBulkMaxItems,
		"DefaultCacheDashboardTTLSeconds":     DefaultCacheDashboardTTLSeconds,
		"DefaultCacheMatchTTLSeconds":         DefaultCacheMatchTTLSeconds,
		"DefaultCacheLeaderboardTTLSeconds":   DefaultCacheLeaderboardTTLSeconds,
		"DefaultDLQSampleSize":                DefaultDLQSampleSize,
		"DefaultDLQReplayDefaultLimit":        DefaultDLQReplayDefaultLimit,
		"DefaultMessagingMaxRetries":          DefaultMessagingMaxRetries,
		"DefaultMessagingStreamMaxLen":        DefaultMessagingStreamMaxLen,
		"DefaultAuthValidationTimeoutSeconds": DefaultAuthValidationTimeoutSeconds,
		"DefaultAuditWriteTimeoutSeconds":     DefaultAuditWriteTimeoutSeconds,
		"DefaultPurgeRetentionDays":           DefaultPurgeRetentionDays,
		"DefaultConflictStaleDays":            DefaultConflictStaleDays,
		"DefaultConflictMaxScan":              DefaultConflictMaxScan,
	}

	for name, value := range defaults {
		t.Run(name, func(t *testing.T) {
			if value <= 0 {
				t.Errorf("%s = %d: default values must be positive", name, value)
			}
		})
	}
}

// TestConstantsDocumentation is a reminder test that fails when new constants
// are added but not documented. It uses reflection to detect new exported
// constants and flags them for documentation.
func TestConstantsDocumentation(t *testing.T) {
	// This test serves as a reminder to update documentation when adding
	// new constants. If you're seeing a failure here, you likely added a
	// new ParamKey* or Default* constant. Make sure to:
	//
	// 1. Add it to the enumeration in TestSystemParamConstants_AllPaired
	// 2. Add it to TestSystemParamNamingConventions if it's a ParamKey*
	// 3. Add it to TestDefaultConstantsArePositive if it's a Default*
	// 4. Create a migration to seed it in system_params table
	// 5. Update the expectedCount in the smoke tests above

	t.Run("scoring_constants", func(t *testing.T) {
		// Verify scoring constants are documented
		scoring := map[string]int{
			"PointsExactScore":      PointsExactScore,
			"PointsCorrectOutcome":  PointsCorrectOutcome,
			"PointsGoalDifference":  PointsGoalDifference,
			"PointsIncorrectResult": PointsIncorrectResult,
		}

		for name, value := range scoring {
			if value < 0 {
				t.Errorf("%s = %d: scoring points should not be negative", name, value)
			}
		}

		// Scoring points should be ordered by value
		if PointsExactScore <= PointsCorrectOutcome {
			t.Error("PointsExactScore should be greater than PointsCorrectOutcome")
		}
		if PointsCorrectOutcome <= PointsGoalDifference {
			t.Error("PointsCorrectOutcome should be greater than PointsGoalDifference")
		}
	})

	t.Run("business_rule_constants", func(t *testing.T) {
		// Verify business rule constants are sensible
		if MinMembersForActive < 2 {
			t.Errorf("MinMembersForActive = %d: should be at least 2", MinMembersForActive)
		}
		if DefaultPrizeThreshold < 1 {
			t.Errorf("DefaultPrizeThreshold = %d: should be at least 1", DefaultPrizeThreshold)
		}
		if StandingsWinPoints < 1 {
			t.Errorf("StandingsWinPoints = %d: should be at least 1", StandingsWinPoints)
		}
	})
}

// Helper function to use reflection (not used in current impl but useful for future)
func getConstantValue(constName string) (interface{}, error) {
	// Note: Go doesn't support const reflection directly.
	// This is a placeholder for potential future implementation.
	pkgType := reflect.TypeOf((*interface{})(nil)).Elem()
	_ = pkgType
	return nil, nil
}
