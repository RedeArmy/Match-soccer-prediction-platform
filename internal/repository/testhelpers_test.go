package repository_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/migrations"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// codeSeq generates unique invite codes across the test run.
var codeSeq int32

func nextCode() string {
	return fmt.Sprintf("CODE%06d", atomic.AddInt32(&codeSeq, 1))
}

const (
	dbImage    = "postgres:17-alpine"
	dbName     = "quiniela_test"
	dbUser     = "test"
	dbPassword = "test"

	fmtUnexpectedErr = "unexpected error: %v"
	fmtCreateErr     = "create: %v"
	fmtUpdatePredErr = "update prediction: %v"
	msgNonZeroID     = "expected non-zero ID after create"
	fmtNotFoundErr   = "expected not-found error, got %v"
	fmtIDMismatch    = "id: got %d, want %d"
	fmtExpectNilGot  = "expected nil, got %+v"

	defaultCurrency = "MXN"

	repoBrazil    = "Brazil"
	repoArgentina = "Argentina"

	// repoGroupLabel is the default group label used by seed helpers that create
	// group_stage matches. The DB CHECK constraint requires a value in A–L.
	repoGroupLabel       = "A"
	repoMexico           = "Mexico"
	repoTotalGoals       = "Total goals"
	repoScoringExact     = "scoring.exact"
	repoScoringCategory  = "scoring"
	repoFakeReceipt      = "fake receipt"
	repoPolicyViolation  = "policy violation"
	repoResourceQuiniela = "quiniela"
	repoMsgCancelledCtx  = "expected error for cancelled context, got nil"
	repoMsgExpect1Pred   = "expected 1 prediction, got %d"
	repoMsgStatusActive  = "status: got %q, want active"
)

// testDB is shared across all tests in this package. It is initialised once in
// TestMain to avoid starting a container per test function.
var testDB *pgxpool.Pool

func TestMain(m *testing.M) {
	var cleanup func()
	testDB, cleanup = mustSetupDB()
	defer cleanup()
	os.Exit(m.Run())
}

// mustSetupDB starts a PostgreSQL container, runs migrations, and returns a
// ready connection pool together with a cleanup function. Extracted from
// TestMain to keep its cognitive complexity within the allowed limit.
func mustSetupDB() (*pgxpool.Pool, func()) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, dbImage,
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername(dbUser),
		tcpostgres.WithPassword(dbPassword),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	pool, err := database.NewPool(ctx, database.Config{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		log.Fatalf("new pool: %v", err)
	}

	cleanup := func() {
		pool.Close()
		if err := container.Terminate(ctx); err != nil {
			log.Printf("terminate postgres container: %v", err)
		}
	}
	return pool, cleanup
}

// cleanTables truncates every table in reverse foreign-key order so each test
// starts with an empty database. RESTART IDENTITY resets serial sequences.
func cleanTables(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(context.Background(),
		`TRUNCATE leaderboard_snapshots, payment_records, audit_log, system_params,
		         tournament_slots, tiebreaker_config, group_memberships, tiebreakers,
		         predictions, quinielas, matches, stadiums, users RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("clean tables: %v", err)
	}
}

func isNotFound(err error) bool {
	return errors.Is(err, apperrors.ErrNotFound)
}

// ── seed helpers ──────────────────────────────────────────────────────────────

func seedUser(t *testing.T) *domain.User {
	t.Helper()
	repo := repository.NewPostgresUserRepository(testDB)
	code := nextCode()
	u := &domain.User{Name: "User " + code, Email: code + "@example.com", Role: domain.RoleUser}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func seedMatch(t *testing.T) *domain.Match {
	t.Helper()
	return seedMatchWithPhase(t, domain.PhaseGroupStage)
}

func seedMatchWithPhase(t *testing.T, phase domain.MatchPhase) *domain.Match {
	t.Helper()
	repo := repository.NewPostgresMatchRepository(testDB)
	m := &domain.Match{
		HomeTeam:  repoBrazil,
		AwayTeam:  repoArgentina,
		Status:    domain.MatchStatusScheduled,
		Phase:     phase,
		KickoffAt: time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond),
	}
	if phase == domain.PhaseGroupStage {
		label := repoGroupLabel
		m.GroupLabel = &label
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed match (phase=%s): %v", phase, err)
	}
	return m
}

func seedQuiniela(t *testing.T, ownerID int) *domain.Quiniela {
	t.Helper()
	repo := repository.NewPostgresQuinielaRepository(testDB)
	q := &domain.Quiniela{Name: fmt.Sprintf("Oficina %s", nextCode()), OwnerID: ownerID, InviteCode: nextCode(), Currency: defaultCurrency}
	if err := repo.Create(context.Background(), q); err != nil {
		t.Fatalf("seed quiniela: %v", err)
	}
	return q
}

// seedQuinielaWithMaxMembers is preserved for tests that verify MaxMembersPerGroup
// enforcement. The cap is now the platform constant (20), not a per-group value.
func seedQuinielaWithMaxMembers(t *testing.T, ownerID int, _ *int) *domain.Quiniela {
	t.Helper()
	return seedQuiniela(t, ownerID)
}

// fillGroupToCapacity seeds domain.MaxMembersPerGroup active members into the
// given quiniela, starting from currentCount. It returns the next available
// user for the caller to use as the over-limit joiner.
func fillGroupToCapacity(t *testing.T, quinielaID, currentCount int) {
	t.Helper()
	now := time.Now().UTC()
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	for i := currentCount; i < domain.MaxMembersPerGroup; i++ {
		u := seedUser(t)
		m := &domain.GroupMembership{QuinielaID: quinielaID, UserID: u.ID, Status: domain.MembershipActive, Paid: true, JoinedAt: &now}
		if err := repo.Create(context.Background(), m); err != nil {
			t.Fatalf("fillGroupToCapacity member %d: %v", i+1, err)
		}
	}
}

func seedMembership(t *testing.T, quinielaID, userID int, status domain.MembershipStatus, paid bool) *domain.GroupMembership {
	t.Helper()
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()
	m := &domain.GroupMembership{
		QuinielaID: quinielaID,
		UserID:     userID,
		Status:     status,
		Paid:       paid,
		JoinedAt:   &now,
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return m
}

// seedMatchWithStadium inserts a stadium linked to an existing city (populated by
// migration 000015) and a match referencing that stadium. cleanTables() does not
// truncate cities/states/countries, so their migration data is always available.
func seedMatchWithStadium(t *testing.T) *domain.Match {
	t.Helper()
	var cityID int
	if err := testDB.QueryRow(context.Background(),
		`SELECT ci.id FROM cities ci
		 JOIN states  st ON ci.state_id   = st.id
		 JOIN countries co ON st.country_id = co.id
		 WHERE ci.name = 'East Rutherford' AND st.code = 'NJ' AND co.code = 'US'`,
	).Scan(&cityID); err != nil {
		t.Fatalf("get city id: %v", err)
	}

	var stadiumID int
	if err := testDB.QueryRow(context.Background(),
		`INSERT INTO stadiums (name, city_id, capacity) VALUES ('Test Arena', $1, 50000) RETURNING id`,
		cityID,
	).Scan(&stadiumID); err != nil {
		t.Fatalf("seed stadium: %v", err)
	}

	label := repoGroupLabel
	repo := repository.NewPostgresMatchRepository(testDB)
	m := &domain.Match{
		HomeTeam:   repoBrazil,
		AwayTeam:   repoArgentina,
		Status:     domain.MatchStatusScheduled,
		Phase:      domain.PhaseGroupStage,
		GroupLabel: &label,
		StadiumID:  &stadiumID,
		KickoffAt:  time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond),
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed match with stadium: %v", err)
	}
	return m
}

func seedActiveMembership(t *testing.T, quinielaID, userID int) *domain.GroupMembership {
	t.Helper()
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now()
	m := &domain.GroupMembership{
		QuinielaID: quinielaID,
		UserID:     userID,
		Status:     domain.MembershipActive,
		Role:       domain.MembershipRoleMember,
		JoinedAt:   &now,
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return m
}

func seedPaymentRecord(t *testing.T, quinielaID, userID int) *domain.PaymentRecord {
	t.Helper()
	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	pr := &domain.PaymentRecord{
		QuinielaID: quinielaID,
		UserID:     userID,
		Amount:     10000,
		Currency:   defaultCurrency,
	}
	if err := repo.Create(context.Background(), pr); err != nil {
		t.Fatalf("seed payment record: %v", err)
	}
	return pr
}

// seedTiebreakerConfig creates the global tiebreaker config (id=1) so that
// tests which create domain.Tiebreaker rows can satisfy the foreign key on
// tiebreaker_config_id. Must be called after cleanTables.
func seedTiebreakerConfig(t *testing.T) *domain.TiebreakerConfig {
	t.Helper()
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)
	cfg, err := repo.Upsert(context.Background(), "Total goals in the Final")
	if err != nil {
		t.Fatalf("seed tiebreaker config: %v", err)
	}
	return cfg
}

func seedSystemParam(t *testing.T, key, value, category string) *domain.SystemParam {
	t.Helper()
	var p domain.SystemParam
	err := testDB.QueryRow(context.Background(),
		`INSERT INTO system_params (key, value, default_value, category)
		 VALUES ($1, $2, $2, $3)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		 RETURNING key, value, default_value, type, category, is_runtime, description, created_at, updated_at`,
		key, value, category,
	).Scan(&p.Key, &p.Value, &p.DefaultValue, &p.Type, &p.Category, &p.IsRuntime, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		t.Fatalf("seed system param: %v", err)
	}
	return &p
}
