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
// synchronised with the migrations that seed system_params:
//   - 000051_sync_system_params_canonical  (22 base params)
//   - 000055_add_worker_messaging_audit_params (+10)
//   - 000056_add_snapshot_keep_latest_param (+1)
//   - 000058_seed_group_max_size_param (+1)
//   - 000066_seed_scoring_bonus_params (+2)
//   - 000073_add_payment_webhook_secrets_params (+3)
var allParams = []paramSpec{
	// Scoring
	{key: domain.ParamKeyScoringExactScore, defaultValue: strconv.Itoa(domain.PointsExactScore), paramType: "int", category: "scoring"},
	{key: domain.ParamKeyScoringCorrectOutcome, defaultValue: strconv.Itoa(domain.PointsCorrectOutcome), paramType: "int", category: "scoring"},
	{key: domain.ParamKeyScoringGoalDiff, defaultValue: strconv.Itoa(domain.PointsGoalDifference), paramType: "int", category: "scoring"},
	{key: domain.ParamKeyScoringExtraTimeBonus, defaultValue: strconv.Itoa(domain.DefaultScoringExtraTimeBonus), paramType: "int", category: "scoring"},
	{key: domain.ParamKeyScoringPenaltiesBonus, defaultValue: strconv.Itoa(domain.DefaultScoringPenaltiesBonus), paramType: "int", category: "scoring"},

	// Prediction
	{key: domain.ParamKeyPredictionDeadlineMin, defaultValue: strconv.Itoa(int(domain.PredictionDeadlineOffset / time.Minute)), paramType: "int", category: "prediction"},

	// Group
	{key: domain.ParamKeyGroupMinMembers, defaultValue: strconv.Itoa(domain.MinMembersForActive), paramType: "int", category: "group"},
	{key: domain.ParamKeyGroupMaxSize, defaultValue: strconv.Itoa(domain.MaxMembersPerGroup), paramType: "int", category: "group"},
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
	{key: domain.ParamKeyMessagingStreamWorkerCount, defaultValue: strconv.Itoa(domain.DefaultMessagingStreamWorkerCount), paramType: "int", category: "messaging"},
	{key: domain.ParamKeyMessagingStreamReadBlockSec, defaultValue: strconv.Itoa(domain.DefaultMessagingStreamReadBlockSec), paramType: "int", category: "messaging"},

	// Audit retry policy
	{key: domain.ParamKeyAuditMaxRetries, defaultValue: strconv.Itoa(domain.DefaultAuditMaxRetries), paramType: "int", category: "system"},
	{key: domain.ParamKeyAuditRetryDelayMs, defaultValue: strconv.Itoa(domain.DefaultAuditRetryDelayMs), paramType: "int", category: "system"},

	// Worker: snapshot generation
	{key: domain.ParamKeyWorkerSnapshotConcurrency, defaultValue: strconv.Itoa(domain.DefaultWorkerSnapshotConcurrency), paramType: "int", category: "worker"},
	{key: domain.ParamKeyWorkerSnapshotRetryBaseMs, defaultValue: strconv.Itoa(domain.DefaultWorkerSnapshotRetryBaseMs), paramType: "int", category: "worker"},
	{key: domain.ParamKeyWorkerSnapshotMaxAttempts, defaultValue: strconv.Itoa(domain.DefaultWorkerSnapshotMaxAttempts), paramType: "int", category: "worker"},

	// Worker: background maintenance
	{key: domain.ParamKeyWorkerDLQMonitorIntervalSec, defaultValue: strconv.Itoa(domain.DefaultWorkerDLQMonitorIntervalSec), paramType: "int", category: "worker"},
	{key: domain.ParamKeyWorkerPurgeIntervalHours, defaultValue: strconv.Itoa(domain.DefaultWorkerPurgeIntervalHours), paramType: "int", category: "worker"},

	// API request limits
	{key: domain.ParamKeyAPIBodySizeLimitBytes, defaultValue: strconv.Itoa(domain.DefaultAPIBodySizeLimitBytes), paramType: "int", category: "api"},

	// Snapshot retention
	{key: domain.ParamKeySnapshotKeepLatestCount, defaultValue: strconv.Itoa(domain.DefaultSnapshotKeepLatestCount), paramType: "int", category: "worker"},

	// Payment / balance
	{key: domain.ParamKeyPaymentMaxUploadBytes, defaultValue: strconv.Itoa(domain.DefaultPaymentMaxUploadBytes), paramType: "int", category: "payment"},
	{key: domain.ParamKeyWithdrawalMinCents, defaultValue: strconv.Itoa(domain.DefaultWithdrawalMinCents), paramType: "int", category: "payment"},
	{key: domain.ParamKeyWithdrawalMaxCents, defaultValue: strconv.Itoa(domain.DefaultWithdrawalMaxCents), paramType: "int", category: "payment"},
	// Added by migration 000074
	{key: domain.ParamKeyBankTransferMinAmountCents, defaultValue: strconv.Itoa(domain.DefaultBankTransferMinAmountCents), paramType: "int", category: "payment"},
	{key: domain.ParamKeyBankTransferMaxAmountCents, defaultValue: strconv.Itoa(domain.DefaultBankTransferMaxAmountCents), paramType: "int", category: "payment"},
}

type dbParam struct {
	key          string
	value        string
	defaultValue string
	paramType    string
	category     string
	isRuntime    bool
	description  string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	pool, err := connectDatabase()
	if err != nil {
		return err
	}
	defer pool.Close()

	dbParams, err := fetchAllParams(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to fetch system_params: %w", err)
	}

	return validateFromParams(dbParams)
}

// validateFromParams runs the full in-memory validation pipeline against a
// pre-fetched slice of database params. Extracted from run so it can be unit
// tested without a live database connection.
func validateFromParams(dbParams []dbParam) error {
	dbMap := buildParamMap(dbParams)
	errors := validateAllParams(dbMap)
	checkUnexpectedParams(dbParams)
	return reportResults(errors)
}

func connectDatabase() (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable not set")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return pool, nil
}

func buildParamMap(dbParams []dbParam) map[string]dbParam {
	dbMap := make(map[string]dbParam, len(dbParams))
	for _, p := range dbParams {
		dbMap[p.key] = p
	}
	return dbMap
}

func validateAllParams(dbMap map[string]dbParam) []string {
	var errors []string
	for _, spec := range allParams {
		errs := validateSingleParam(spec, dbMap)
		errors = append(errors, errs...)
	}
	return errors
}

func validateSingleParam(spec paramSpec, dbMap map[string]dbParam) []string {
	db, exists := dbMap[spec.key]
	if !exists {
		return []string{fmt.Sprintf("❌ MISSING: %s (expected default: %s)", spec.key, spec.defaultValue)}
	}

	var errors []string
	errors = append(errors, validateType(spec, db)...)
	errors = append(errors, validateCategory(spec, db)...)
	errors = append(errors, validateDescription(spec, db)...)
	errors = append(errors, validateDefaultValue(spec, db)...)

	checkValueOverride(spec, db)
	printValidParam(spec, db)

	return errors
}

func validateDefaultValue(spec paramSpec, db dbParam) []string {
	if db.defaultValue != spec.defaultValue {
		return []string{fmt.Sprintf("❌ DEFAULT VALUE MISMATCH: %s (code: %s, db.default_value: %s) — migration seed may be out of sync with domain constants", spec.key, spec.defaultValue, db.defaultValue)}
	}
	return nil
}

func validateType(spec paramSpec, db dbParam) []string {
	if db.paramType != spec.paramType {
		return []string{fmt.Sprintf("❌ TYPE MISMATCH: %s (expected: %s, got: %s)", spec.key, spec.paramType, db.paramType)}
	}
	return nil
}

func validateCategory(spec paramSpec, db dbParam) []string {
	if db.category != spec.category {
		return []string{fmt.Sprintf("❌ CATEGORY MISMATCH: %s (expected: %s, got: %s)", spec.key, spec.category, db.category)}
	}
	return nil
}

func validateDescription(spec paramSpec, db dbParam) []string {
	if db.description == "" {
		return []string{fmt.Sprintf("❌ MISSING DESCRIPTION: %s", spec.key)}
	}
	return nil
}

func checkValueOverride(spec paramSpec, db dbParam) {
	if db.value != spec.defaultValue {
		fmt.Printf("⚠️  VALUE OVERRIDE: %s (code default: %s, DB value: %s) — operator override detected\n",
			spec.key, spec.defaultValue, db.value)
	}
}

func printValidParam(spec paramSpec, db dbParam) {
	fmt.Printf("✅ %s = %s (%s, %s)\n", spec.key, db.value, db.paramType, db.category)
}

func checkUnexpectedParams(dbParams []dbParam) {
	expectedKeys := buildExpectedKeysSet()
	for _, db := range dbParams {
		if !expectedKeys[db.key] {
			fmt.Printf("⚠️  UNEXPECTED PARAM IN DB: %s (not defined in constants.go) — consider removing or documenting\n", db.key)
		}
	}
}

func buildExpectedKeysSet() map[string]bool {
	expected := make(map[string]bool, len(allParams))
	for _, spec := range allParams {
		expected[spec.key] = true
	}
	return expected
}

func reportResults(errors []string) error {
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
		SELECT key, value, default_value, type, category, is_runtime, COALESCE(description, '') as description
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
		if err := rows.Scan(&p.key, &p.value, &p.defaultValue, &p.paramType, &p.category, &p.isRuntime, &p.description); err != nil {
			return nil, err
		}
		params = append(params, p)
	}

	return params, rows.Err()
}
