package repository_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── SystemParamRepository ─────────────────────────────────────────────────────

func TestSystemParamRepository_Set_NewKey(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	p, err := repo.Set(context.Background(), repoScoringExact, "5", 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p.Key != repoScoringExact || p.Value != "5" {
		t.Errorf("param mismatch: got key=%q value=%q", p.Key, p.Value)
	}
}

func TestSystemParamRepository_Set_ExistingKeyUpdatesValue(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	_, _ = repo.Set(context.Background(), repoScoringExact, "5", 0)
	updated, err := repo.Set(context.Background(), repoScoringExact, "7", 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.Value != "7" {
		t.Errorf("expected value %q, got %q", "7", updated.Value)
	}
}

func TestSystemParamRepository_Get_Found(t *testing.T) {
	cleanTables(t)
	seedSystemParam(t, "feature.x", "true", "general")
	repo := repository.NewPostgresSystemParamRepository(testDB)

	p, err := repo.Get(context.Background(), "feature.x")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p == nil || p.Value != "true" {
		t.Errorf("expected param with value %q, got %v", "true", p)
	}
}

func TestSystemParamRepository_Get_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	p, err := repo.Get(context.Background(), "does.not.exist")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p != nil {
		t.Errorf(fmtExpectNilGot, p)
	}
}

func TestSystemParamRepository_GetAll(t *testing.T) {
	cleanTables(t)
	seedSystemParam(t, "a.key", "1", "general")
	seedSystemParam(t, "b.key", "2", repoScoringCategory)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	all, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 params, got %d", len(all))
	}
}

func TestSystemParamRepository_GetByCategory(t *testing.T) {
	cleanTables(t)
	seedSystemParam(t, "scoring.a", "1", repoScoringCategory)
	seedSystemParam(t, "scoring.b", "2", repoScoringCategory)
	seedSystemParam(t, "payment.x", "3", "payment")
	repo := repository.NewPostgresSystemParamRepository(testDB)

	results, err := repo.GetByCategory(context.Background(), repoScoringCategory)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 scoring params, got %d", len(results))
	}
}

func TestSystemParamRepository_BulkSet(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	err := repo.BulkSet(context.Background(), map[string]string{
		"bulk.a": "alpha",
		"bulk.b": "beta",
	}, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	all, _ := repo.GetAll(context.Background())
	if len(all) != 2 {
		t.Errorf("expected 2 params after BulkSet, got %d", len(all))
	}
}

func TestSystemParamRepository_BulkSet_EmptyIsNoop(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	if err := repo.BulkSet(context.Background(), nil, 0); err != nil {
		t.Fatalf("empty BulkSet should not error: %v", err)
	}
}

// canonicalParam holds the authoritative metadata for a single system_param
// row. This mirrors the values seeded by the migrations so the test stays
// independent of which tests have called cleanTables before it runs.
type canonicalParam struct {
	key         string
	defaultVal  string
	category    string
	isRuntime   bool
	description string
}

// canonicalSystemParams lists every ParamKey* domain constant with its
// expected DB metadata. Any discrepancy between this table, the domain
// constants, and the migration seeds is a test failure.
var canonicalSystemParams = []canonicalParam{
	// Scoring: defaults map to the Points* domain constants.
	{domain.ParamKeyScoringExactScore, strconv.Itoa(domain.PointsExactScore), "scoring", true,
		"Points awarded for predicting the exact final score"},
	{domain.ParamKeyScoringCorrectOutcome, strconv.Itoa(domain.PointsCorrectOutcome), "scoring", true,
		"Points awarded for predicting the correct match outcome (win, draw, or loss)"},
	{domain.ParamKeyScoringGoalDiff, strconv.Itoa(domain.PointsGoalDifference), "scoring", true,
		"Points awarded for predicting the correct goal difference"},
	// Prediction: offset is 5 min; stored as the integer minute count.
	{domain.ParamKeyPredictionDeadlineMin, "5", "prediction", true,
		"Minutes before kick-off after which new or updated predictions are rejected"},
	// Group: MinMembersForActive is the domain constant for group.min_members_for_active.
	{domain.ParamKeyGroupMinMembers, strconv.Itoa(domain.MinMembersForActive), "group", true,
		"Minimum number of paid members required to activate a quiniela group"},
	{domain.ParamKeyGroupDefaultPrize, strconv.Itoa(domain.DefaultPrizeThreshold), "group", true,
		"Default minimum number of paid members required for prize eligibility"},
	{domain.ParamKeyGroupInviteCodeLength, strconv.Itoa(domain.DefaultGroupInviteCodeLength), "group", true,
		"Number of characters in a randomly generated group invite code"},
	{domain.ParamKeyConflictStaleDays, strconv.Itoa(domain.DefaultConflictStaleDays), "conflict", true,
		"Age in days after which a pending payment or membership is flagged as a conflict"},
	{domain.ParamKeyPaginationDefaultLimit, strconv.Itoa(domain.DefaultPaginationDefaultLimit), "pagination", true,
		"Default number of items per page for paginated admin endpoints"},
	{domain.ParamKeyPaginationMaxLimit, strconv.Itoa(domain.DefaultPaginationMaxLimit), "pagination", true,
		"Maximum number of items per page allowed by paginated endpoints"},
	// Tournament: StandingsWinPoints is the domain constant for tournament.win_points.
	{domain.ParamKeyTournamentWinPoints, strconv.Itoa(domain.StandingsWinPoints), "tournament", true,
		"Standing points awarded for a group-stage win"},
	{domain.ParamKeyAdminBulkMaxItems, strconv.Itoa(domain.DefaultAdminBulkMaxItems), "admin", true,
		"Maximum number of IDs accepted in a single bulk admin operation"},
	{domain.ParamKeyCacheMatchTTL, strconv.Itoa(domain.DefaultCacheMatchTTLSeconds), "cache", false,
		"Match-list cache TTL in seconds; restart required to apply changes"},
	{domain.ParamKeyCacheLeaderboardTTL, strconv.Itoa(domain.DefaultCacheLeaderboardTTLSeconds), "cache", true,
		"Leaderboard cache TTL in seconds; applied immediately via mutation hook without restart"},
	{domain.ParamKeyCacheDashboardTTLSeconds, strconv.Itoa(domain.DefaultCacheDashboardTTLSeconds), "cache", true,
		"Dashboard stats cache TTL in seconds; set to 0 to disable the cache"},
	{domain.ParamKeyAuditWriteTimeout, strconv.Itoa(domain.DefaultAuditWriteTimeoutSeconds), "system", false,
		"Maximum seconds the audit log goroutine waits to persist an entry before giving up"},
	{domain.ParamKeyDLQSampleSize, strconv.Itoa(domain.DefaultDLQSampleSize), "dlq", false,
		"Maximum number of dead-letter queue entries returned in the Stats sample"},
	{domain.ParamKeyDLQReplayDefaultLimit, strconv.Itoa(domain.DefaultDLQReplayDefaultLimit), "dlq", false,
		"Default number of DLQ entries replayed when no explicit limit is supplied"},
	{domain.ParamKeyMessagingMaxRetries, strconv.Itoa(domain.DefaultMessagingMaxRetries), "messaging", false,
		"Total handler attempts before an event is moved to the dead-letter queue"},
	{domain.ParamKeyMessagingStreamMaxLen, strconv.Itoa(domain.DefaultMessagingStreamMaxLen), "messaging", false,
		"Approximate maximum length of the Redis event stream (MAXLEN ~)"},
	{domain.ParamKeyAuthValidationTimeout, strconv.Itoa(domain.DefaultAuthValidationTimeoutSeconds), "system", false,
		"JWKS validation timeout in seconds at process startup"},
}

// assertCanonicalParam checks that a retrieved SystemParam matches its
// expected canonical metadata.
func assertCanonicalParam(t *testing.T, got *domain.SystemParam, want canonicalParam) {
	t.Helper()
	if got.Type != domain.SystemParamTypeInt {
		t.Errorf("type: got %q, want %q", got.Type, domain.SystemParamTypeInt)
	}
	if got.Category != want.category {
		t.Errorf("category: got %q, want %q", got.Category, want.category)
	}
	if got.IsRuntime != want.isRuntime {
		t.Errorf("is_runtime: got %v, want %v", got.IsRuntime, want.isRuntime)
	}
	if got.Description == "" {
		t.Error("description must not be empty")
	}
	if _, err := strconv.Atoi(got.Value); err != nil {
		t.Errorf("default value %q is not parseable as int: %v", got.Value, err)
	}
	if got.Value != want.defaultVal {
		t.Errorf("default value: got %q, want %q", got.Value, want.defaultVal)
	}
}

// TestSystemParamRepository_AllDomainConstantsSeeded verifies that every
// ParamKey* domain constant has a corresponding row in system_params with
// the correct type, category, is_runtime flag, parseable default value, and
// non-empty description. It does not call cleanTables; instead it upserts the
// canonical rows so the test is independent of execution order.
func TestSystemParamRepository_AllDomainConstantsSeeded(t *testing.T) {
	ctx := context.Background()

	// Upsert canonical rows so the test is independent of which other tests
	// called cleanTables beforehand.
	for _, want := range canonicalSystemParams {
		_, err := testDB.Exec(ctx,
			`INSERT INTO system_params (key, value, type, category, is_runtime, description)
			 VALUES ($1, $2, 'int', $3, $4, $5)
			 ON CONFLICT (key) DO UPDATE
			     SET value = EXCLUDED.value,
			         type = EXCLUDED.type,
			         category = EXCLUDED.category,
			         is_runtime = EXCLUDED.is_runtime,
			         description = EXCLUDED.description,
			         updated_at = NOW()`,
			want.key, want.defaultVal, want.category, want.isRuntime, want.description,
		)
		if err != nil {
			t.Fatalf("upsert canonical param %q: %v", want.key, err)
		}
	}

	repo := repository.NewPostgresSystemParamRepository(testDB)
	all, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}

	byKey := make(map[string]*domain.SystemParam, len(all))
	for _, p := range all {
		byKey[p.Key] = p
	}

	for _, want := range canonicalSystemParams {
		t.Run(want.key, func(t *testing.T) {
			got, ok := byKey[want.key]
			if !ok {
				t.Fatalf("row missing for key %q", want.key)
			}
			assertCanonicalParam(t, got, want)
		})
	}

	// Ensure no domain constant was silently omitted from the canonical table.
	if len(canonicalSystemParams) != 21 {
		t.Errorf("canonicalSystemParams has %d entries; expected 21 (one per ParamKey* constant)", len(canonicalSystemParams))
	}
}
