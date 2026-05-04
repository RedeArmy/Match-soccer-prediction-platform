package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// paramSpec defines the expected metadata for a system parameter.
type paramSpec struct {
	key          string
	defaultValue string
	paramType    string
	category     string
}

// allParams is the authoritative list of every system parameter that must exist
// in the database. This list is derived from domain/constants.go and must stay
// synchronized with migration 000049_complete_system_params_with_descriptions.up.sql.
var allParams = []paramSpec{
	// Scoring
	{key: domain.ParamKeyScoringExactScore, defaultValue: strconv.Itoa(domain.PointsExactScore), paramType: "int", category: "scoring"},
	{key: domain.ParamKeyScoringCorrectOutcome, defaultValue: strconv.Itoa(domain.PointsCorrectOutcome), paramType: "int", category: "scoring"},
	{key: domain.ParamKeyScoringGoalDiff, defaultValue: strconv.Itoa(domain.PointsGoalDifference), paramType: "int", category: "scoring"},

	// Prediction
	{key: domain.ParamKeyPredictionDeadlineMin, defaultValue: strconv.Itoa(int(domain.PredictionDeadlineOffset / time.Minute)), paramType: "int", category: "prediction"},

	// Group
	{key: domain.ParamKeyGroupMinMembers, defaultValue: strconv.Itoa(domain.MinMembersForActive), paramType: "int", category: "group"},
	{key: domain.ParamKeyGroupDefaultPrize, defaultValue: strconv.Itoa(domain.DefaultPrizeThreshold), paramType: "int", category: "group"},
	{key: domain.ParamKeyGroupInviteCodeLength, defaultValue: strconv.Itoa(domain.DefaultGroupInviteCodeLength), paramType: "int", category: "group"},

	// Conflict
	{key: domain.ParamKeyConflictStaleDays, defaultValue: strconv.Itoa(domain.DefaultConflictStaleDays), paramType: "int", category: "conflict"},
	{key: domain.ParamKeyConflictMaxScan, defaultValue: strconv.Itoa(domain.DefaultConflictMaxScan), paramType: "int", category: "conflict"},

	// Pagination
	{key: domain.ParamKeyPaginationDefaultLimit, defaultValue: strconv.Itoa(domain.DefaultPaginationDefaultLimit), paramType: "int", category: "pagination"},
	{key: domain.ParamKeyPaginationMaxLimit, defaultValue: strconv.Itoa(domain.DefaultPaginationMaxLimit), paramType: "int", category: "pagination"},

	// Tournament
	{key: domain.ParamKeyTournamentWinPoints, defaultValue: strconv.Itoa(domain.StandingsWinPoints), paramType: "int", category: "tournament"},

	// Admin
	{key: domain.ParamKeyAdminBulkMaxItems, defaultValue: strconv.Itoa(domain.DefaultAdminBulkMaxItems), paramType: "int", category: "admin"},

	// Cache
	{key: domain.ParamKeyCacheMatchTTL, defaultValue: strconv.Itoa(domain.DefaultCacheMatchTTLSeconds), paramType: "int", category: "cache"},
	{key: domain.ParamKeyCacheLeaderboardTTL, defaultValue: strconv.Itoa(domain.DefaultCacheLeaderboardTTLSeconds), paramType: "int", category: "cache"},
	{key: domain.ParamKeyCacheDashboardTTLSeconds, defaultValue: strconv.Itoa(domain.DefaultCacheDashboardTTLSeconds), paramType: "int", category: "cache"},

	// System
	{key: domain.ParamKeyAuditWriteTimeout, defaultValue: strconv.Itoa(domain.DefaultAuditWriteTimeoutSeconds), paramType: "int", category: "system"},
	{key: domain.ParamKeyAuthValidationTimeout, defaultValue: strconv.Itoa(domain.DefaultAuthValidationTimeoutSeconds), paramType: "int", category: "system"},
	{key: domain.ParamKeyPurgeRetentionDays, defaultValue: strconv.Itoa(domain.DefaultPurgeRetentionDays), paramType: "int", category: "system"},

	// DLQ
	{key: domain.ParamKeyDLQSampleSize, defaultValue: strconv.Itoa(domain.DefaultDLQSampleSize), paramType: "int", category: "dlq"},
	{key: domain.ParamKeyDLQReplayDefaultLimit, defaultValue: strconv.Itoa(domain.DefaultDLQReplayDefaultLimit), paramType: "int", category: "dlq"},

	// Messaging
	{key: domain.ParamKeyMessagingMaxRetries, defaultValue: strconv.Itoa(domain.DefaultMessagingMaxRetries), paramType: "int", category: "messaging"},
	{key: domain.ParamKeyMessagingStreamMaxLen, defaultValue: strconv.Itoa(domain.DefaultMessagingStreamMaxLen), paramType: "int", category: "messaging"},
}

type dbParam struct {
	key         string
	value       string
	paramType   string
	category    string
	isRuntime   bool
	description string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pool.Close()

	// Fetch all params from DB
	dbParams, err := fetchAllParams(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to fetch system_params: %w", err)
	}

	// Build lookup map
	dbMap := make(map[string]dbParam)
	for _, p := range dbParams {
		dbMap[p.key] = p
	}

	// Validate each expected param
	var errors []string
	for _, spec := range allParams {
		db, exists := dbMap[spec.key]
		if !exists {
			errors = append(errors, fmt.Sprintf("❌ MISSING: %s (expected default: %s)", spec.key, spec.defaultValue))
			continue
		}

		// Validate type
		if db.paramType != spec.paramType {
			errors = append(errors, fmt.Sprintf("❌ TYPE MISMATCH: %s (expected: %s, got: %s)", spec.key, spec.paramType, db.paramType))
		}

		// Validate category
		if db.category != spec.category {
			errors = append(errors, fmt.Sprintf("❌ CATEGORY MISMATCH: %s (expected: %s, got: %s)", spec.key, spec.category, db.category))
		}

		// Validate default value (only if DB value has not been overridden)
		if db.value != spec.defaultValue {
			fmt.Printf("⚠️  VALUE OVERRIDE: %s (code default: %s, DB value: %s) — operator override detected\n", spec.key, spec.defaultValue, db.value)
		}

		// Validate description exists
		if db.description == "" {
			errors = append(errors, fmt.Sprintf("❌ MISSING DESCRIPTION: %s", spec.key))
		}

		fmt.Printf("✅ %s = %s (%s, %s)\n", spec.key, db.value, db.paramType, db.category)
	}

	// Check for unexpected params in DB
	for _, db := range dbParams {
		found := false
		for _, spec := range allParams {
			if spec.key == db.key {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("⚠️  UNEXPECTED PARAM IN DB: %s (not defined in constants.go) — consider removing or documenting\n", db.key)
		}
	}

	if len(errors) > 0 {
		fmt.Println("\n❌ VALIDATION FAILED:")
		for _, err := range errors {
			fmt.Println(err)
		}
		return fmt.Errorf("system_params validation failed with %d error(s)", len(errors))
	}

	fmt.Printf("\n✅ VALIDATION PASSED: All %d system parameters are correctly configured\n", len(allParams))
	return nil
}

func fetchAllParams(ctx context.Context, pool *pgxpool.Pool) ([]dbParam, error) {
	rows, err := pool.Query(ctx, `
		SELECT key, value, type, category, is_runtime, COALESCE(description, '') as description
		FROM system_params
		ORDER BY category, key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var params []dbParam
	for rows.Next() {
		var p dbParam
		if err := rows.Scan(&p.key, &p.value, &p.paramType, &p.category, &p.isRuntime, &p.description); err != nil {
			return nil, err
		}
		params = append(params, p)
	}

	return params, rows.Err()
}
