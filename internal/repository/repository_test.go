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

	repoBrazil           = "Brazil"
	repoArgentina        = "Argentina"
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

// ── helpers ───────────────────────────────────────────────────────────────────

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
	repo := repository.NewPostgresMatchRepository(testDB)
	m := &domain.Match{
		HomeTeam:  repoBrazil,
		AwayTeam:  repoArgentina,
		Status:    domain.MatchStatusScheduled,
		Phase:     domain.PhaseGroupStage,
		KickoffAt: time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond),
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed match: %v", err)
	}
	return m
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
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed match (phase=%s): %v", phase, err)
	}
	return m
}

func seedQuiniela(t *testing.T, ownerID int) *domain.Quiniela {
	t.Helper()
	repo := repository.NewPostgresQuinielaRepository(testDB)
	q := &domain.Quiniela{Name: fmt.Sprintf("Oficina %s", nextCode()), OwnerID: ownerID, InviteCode: nextCode(), Currency: defaultCurrency, PrizeThreshold: domain.DefaultPrizeThreshold}
	if err := repo.Create(context.Background(), q); err != nil {
		t.Fatalf("seed quiniela: %v", err)
	}
	return q
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

// ── UserRepository ────────────────────────────────────────────────────────────

func TestUserRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	u := &domain.User{Name: "Bob", Email: "bob@example.com", Role: domain.RoleUser}

	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if u.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if u.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt after create")
	}
}

func TestUserRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	created := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.Email != created.Email {
		t.Errorf("email: got %q, want %q", got.Email, created.Email)
	}
}

func TestUserRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for missing user, got %+v", got)
	}
}

func TestUserRepository_GetByClerkSubject_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	// Set a clerk_subject via Update so GetByClerkSubject can find it.
	u.ClerkSubject = "user_clerk_abc123"
	if err := repo.Update(context.Background(), u); err != nil {
		t.Fatalf("set clerk_subject: %v", err)
	}

	got, err := repo.GetByClerkSubject(context.Background(), "user_clerk_abc123")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.ID != u.ID {
		t.Errorf(fmtIDMismatch, got.ID, u.ID)
	}
}

func TestUserRepository_GetByClerkSubject_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.GetByClerkSubject(context.Background(), "user_nonexistent")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown clerk subject, got %+v", got)
	}
}

func TestUserRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	u.Name = "Alice Updated"
	if err := repo.Update(context.Background(), u); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if u.Name != "Alice Updated" {
		t.Errorf("name not updated: got %q", u.Name)
	}
}

func TestUserRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	ghost := &domain.User{ID: 99999, Name: "Ghost", Email: "g@g.com", Role: domain.RoleUser}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUserRepository_Delete_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	if err := repo.Delete(context.Background(), u.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), u.ID)
	if got != nil {
		t.Error("expected user to be deleted")
	}
}

func TestUserRepository_Delete_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	if err := repo.Delete(context.Background(), 99999); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUserRepository_List_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	users, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
	if users[0].ID != u.ID {
		t.Errorf("unexpected user ID: got %d, want %d", users[0].ID, u.ID)
	}
}

// ── MatchRepository ───────────────────────────────────────────────────────────

func TestMatchRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)
	m := &domain.Match{
		HomeTeam:  "France",
		AwayTeam:  "Germany",
		Status:    domain.MatchStatusScheduled,
		Phase:     domain.PhaseGroupStage,
		KickoffAt: time.Now().Add(48 * time.Hour).UTC(),
	}

	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestMatchRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	created := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected match, got nil")
	}
	if got.HomeTeam != created.HomeTeam {
		t.Errorf("home team: got %q, want %q", got.HomeTeam, created.HomeTeam)
	}
}

func TestMatchRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for missing match, got %+v", got)
	}
}

func TestMatchRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	m.Status = domain.MatchStatusLive
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.Status != domain.MatchStatusLive {
		t.Errorf("status not updated: got %q", m.Status)
	}
}

func TestMatchRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresMatchRepository(testDB)
	ghost := &domain.Match{ID: 99999, HomeTeam: "X", AwayTeam: "Y", Status: domain.MatchStatusScheduled, KickoffAt: time.Now().Add(time.Hour).UTC()}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestMatchRepository_List_ReturnsAll(t *testing.T) {
	cleanTables(t)
	seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	matches, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(matches) == 0 {
		t.Error("expected at least one match")
	}
}

func TestMatchRepository_ListByStatus_FiltersCorrectly(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t) // status = scheduled

	// Promote one to live.
	repo := repository.NewPostgresMatchRepository(testDB)
	m.Status = domain.MatchStatusLive
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf("update to live: %v", err)
	}

	live, err := repo.ListByStatus(context.Background(), domain.MatchStatusLive)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(live) != 1 {
		t.Errorf("expected 1 live match, got %d", len(live))
	}

	scheduled, err := repo.ListByStatus(context.Background(), domain.MatchStatusScheduled)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(scheduled) != 0 {
		t.Errorf("expected 0 scheduled matches, got %d", len(scheduled))
	}
}

func TestMatchRepository_ListByPhase_FiltersCorrectly(t *testing.T) {
	cleanTables(t)
	seedMatch(t) // phase = group_stage

	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.ListByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 group_stage match, got %d", len(got))
	}

	none, err := repo.ListByPhase(context.Background(), domain.PhaseFinal)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 final matches, got %d", len(none))
	}
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

	repo := repository.NewPostgresMatchRepository(testDB)
	m := &domain.Match{
		HomeTeam:  repoBrazil,
		AwayTeam:  repoArgentina,
		Status:    domain.MatchStatusScheduled,
		Phase:     domain.PhaseGroupStage,
		StadiumID: &stadiumID,
		KickoffAt: time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond),
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed match with stadium: %v", err)
	}
	return m
}

func TestMatchRepository_GetByID_HydratesStadiumLocation(t *testing.T) {
	cleanTables(t)
	created := seedMatchWithStadium(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.Stadium == nil {
		t.Fatal("expected stadium to be hydrated")
	}
	if got.Stadium.City == nil {
		t.Fatal("expected city to be hydrated")
	}
	if got.Stadium.City.Name != "East Rutherford" {
		t.Errorf("city: got %q, want %q", got.Stadium.City.Name, "East Rutherford")
	}
	if got.Stadium.City.State == nil {
		t.Fatal("expected state to be hydrated")
	}
	if got.Stadium.City.State.Code != "NJ" {
		t.Errorf("state code: got %q, want %q", got.Stadium.City.State.Code, "NJ")
	}
	if got.Stadium.City.State.Country == nil {
		t.Fatal("expected country to be hydrated")
	}
	if got.Stadium.City.State.Country.Code != "US" {
		t.Errorf("country code: got %q, want %q", got.Stadium.City.State.Country.Code, "US")
	}
}

func TestMatchRepository_List_HydratesStadiumLocation(t *testing.T) {
	cleanTables(t)
	seedMatchWithStadium(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	matches, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	m := matches[0]
	if m.Stadium == nil {
		t.Fatal("expected stadium to be hydrated in list result")
	}
	if m.Stadium.City == nil {
		t.Fatal("expected city to be hydrated in list result")
	}
	if m.Stadium.City.State == nil {
		t.Fatal("expected state to be hydrated in list result")
	}
	if m.Stadium.City.State.Country == nil {
		t.Fatal("expected country to be hydrated in list result")
	}
}

// ── PredictionRepository ──────────────────────────────────────────────────────

func TestPredictionRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestPredictionRepository_Create_Duplicate_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p1); err != nil {
		t.Fatalf("first create: %v", err)
	}

	p2 := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	if err := repo.Create(context.Background(), p2); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for duplicate prediction, got %v", err)
	}
}

func TestPredictionRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	got, err := repo.GetByID(context.Background(), p.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected prediction, got nil")
	}
	if got.HomeScore != 1 {
		t.Errorf("home score: got %d, want 1", got.HomeScore)
	}
}

func TestPredictionRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for missing prediction, got %+v", got)
	}
}

func TestPredictionRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	pts := 5
	p.Points = &pts
	if err := repo.Update(context.Background(), p); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p.Points == nil || *p.Points != 5 {
		t.Errorf("points not updated: got %v", p.Points)
	}
}

func TestPredictionRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	ghost := &domain.Prediction{ID: 99999, HomeScore: 1, AwayScore: 0}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestPredictionRepository_GetByUserAndMatch_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 3, AwayScore: 2}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	got, err := repo.GetByUserAndMatch(context.Background(), u.ID, m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected prediction, got nil")
	}
	if got.ID != p.ID {
		t.Errorf("ID: got %d, want %d", got.ID, p.ID)
	}
}

func TestPredictionRepository_GetByUserAndMatch_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	got, err := repo.GetByUserAndMatch(context.Background(), 99999, 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestPredictionRepository_ListByUser_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 1}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 1 {
		t.Errorf(repoMsgExpect1Pred, len(preds))
	}
}

func TestPredictionRepository_ListByMatch_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 0, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := repo.ListByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 1 {
		t.Errorf(repoMsgExpect1Pred, len(preds))
	}
}

func TestPredictionRepository_UpdateManyPoints_PersistsPoints(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	p2 := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 0, AwayScore: 0}
	if err := repo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	// Need a second user to create a second prediction on the same match.
	u2 := seedUser(t)
	p2.UserID = u2.ID
	if err := repo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	points := map[int]int{p1.ID: 5, p2.ID: 2}
	if err := repo.UpdateManyPoints(context.Background(), points); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got1, _ := repo.GetByID(context.Background(), p1.ID)
	got2, _ := repo.GetByID(context.Background(), p2.ID)
	if got1.Points == nil || *got1.Points != 5 {
		t.Errorf("p1 points: got %v, want 5", got1.Points)
	}
	if got2.Points == nil || *got2.Points != 2 {
		t.Errorf("p2 points: got %v, want 2", got2.Points)
	}
}

func TestPredictionRepository_UpdateManyPoints_EmptyMap_IsNoop(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPredictionRepository(testDB)

	if err := repo.UpdateManyPoints(context.Background(), map[int]int{}); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

// ── QuinielaRepository ────────────────────────────────────────────────────────

func TestQuinielaRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := &domain.Quiniela{Name: "Test Pool", OwnerID: u.ID, PrizeThreshold: domain.DefaultPrizeThreshold}
	if err := repo.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestQuinielaRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected quiniela, got nil")
	}
	if got.Name != q.Name {
		t.Errorf("name: got %q, want %q", got.Name, q.Name)
	}
}

func TestQuinielaRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for missing quiniela, got %+v", got)
	}
}

func TestQuinielaRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q.Name = "Renamed Pool"
	if err := repo.Update(context.Background(), q); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q.Name != "Renamed Pool" {
		t.Errorf("name not updated: got %q", q.Name)
	}
}

func TestQuinielaRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)
	ghost := &domain.Quiniela{ID: 99999, Name: "Ghost", OwnerID: 1}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestQuinielaRepository_Delete_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.Delete(context.Background(), q.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), q.ID)
	if got != nil {
		t.Error("expected quiniela to be deleted")
	}
}

func TestQuinielaRepository_Delete_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.Delete(context.Background(), 99999); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestQuinielaRepository_ListByOwner_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	quinielas, err := repo.ListByOwner(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(quinielas) != 1 {
		t.Errorf("expected 1 quiniela, got %d", len(quinielas))
	}
}

// ── TiebreakerRepository ──────────────────────────────────────────────────────

func TestTiebreakerRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	tb := &domain.Tiebreaker{UserID: u.ID, Prediction: 42}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if tb.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestTiebreakerRepository_GetByUser_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, Prediction: 10}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	got, err := repo.GetByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected tiebreaker, got nil")
	}
	if got.Prediction != 10 {
		t.Errorf("prediction: got %d, want 10", got.Prediction)
	}
}

func TestTiebreakerRepository_GetByUser_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	got, err := repo.GetByUser(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestTiebreakerRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, Prediction: 7}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	tb.Prediction = 9
	if err := repo.Update(context.Background(), tb); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if tb.Prediction != 9 {
		t.Errorf("prediction not updated: got %d", tb.Prediction)
	}
}

func TestTiebreakerRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	ghost := &domain.Tiebreaker{ID: 99999, Prediction: 5}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestTiebreakerRepository_ListByUserIDs_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, Prediction: 3}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	tbs, err := repo.ListByUserIDs(context.Background(), []int{u.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(tbs) != 1 {
		t.Errorf("expected 1 tiebreaker, got %d", len(tbs))
	}
}

// ── QuinielaRepository (new fields) ──────────────────────────────────────────

func TestQuinielaRepository_Create_HydratesInviteCode(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)
	code := nextCode()
	q := &domain.Quiniela{Name: "Pool A", OwnerID: u.ID, InviteCode: code, Currency: defaultCurrency, PrizeThreshold: domain.DefaultPrizeThreshold}

	if err := repo.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q.InviteCode != code {
		t.Errorf("invite_code: got %q, want %q", q.InviteCode, code)
	}
}

func TestQuinielaRepository_Create_DuplicateName_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q1 := &domain.Quiniela{Name: "Same Name", OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency, PrizeThreshold: domain.DefaultPrizeThreshold}
	if err := repo.Create(context.Background(), q1); err != nil {
		t.Fatalf("first create: %v", err)
	}
	q2 := &domain.Quiniela{Name: "Same Name", OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency, PrizeThreshold: domain.DefaultPrizeThreshold}
	err := repo.Create(context.Background(), q2)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error for duplicate name, got %v", err)
	}
}

func TestQuinielaRepository_GetByInviteCode_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByInviteCode(context.Background(), q.InviteCode)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected quiniela, got nil")
	}
	if got.ID != q.ID {
		t.Errorf(fmtIDMismatch, got.ID, q.ID)
	}
}

func TestQuinielaRepository_GetByInviteCode_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByInviteCode(context.Background(), "NOTEXISTS")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown code, got %+v", got)
	}
}

// ── GroupMembershipRepository ─────────────────────────────────────────────────

func TestGroupMembershipRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()
	m := &domain.GroupMembership{
		QuinielaID: q.ID,
		UserID:     u.ID,
		Status:     domain.MembershipActive,
		Paid:       true,
		JoinedAt:   &now,
	}

	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if !m.Paid {
		t.Error("expected Paid = true after hydration")
	}
}

func TestGroupMembershipRepository_Create_FreeMembership_PaidFalse(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	m := &domain.GroupMembership{
		QuinielaID: q.ID,
		UserID:     u.ID,
		Status:     domain.MembershipPending,
		Paid:       false,
	}

	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.Paid {
		t.Error("expected Paid = false")
	}
	if m.JoinedAt != nil {
		t.Error("expected JoinedAt = nil for pending membership")
	}
}

func TestGroupMembershipRepository_Create_ExceedsMaxMembers_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	maxMembers := 1
	quinielaRepo := repository.NewPostgresQuinielaRepository(testDB)
	q := &domain.Quiniela{
		Name:           "Capped " + nextCode(),
		OwnerID:        owner.ID,
		InviteCode:     nextCode(),
		Currency:       defaultCurrency,
		PrizeThreshold: domain.DefaultPrizeThreshold,
		MaxMembers:     &maxMembers,
	}
	if err := quinielaRepo.Create(context.Background(), q); err != nil {
		t.Fatalf("seed quiniela: %v", err)
	}

	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()

	// First active membership — must succeed.
	m1 := &domain.GroupMembership{QuinielaID: q.ID, UserID: owner.ID, Status: domain.MembershipActive, Paid: true, JoinedAt: &now}
	if err := repo.Create(context.Background(), m1); err != nil {
		t.Fatalf("first membership: %v", err)
	}

	// Second active membership — must be rejected by the DB trigger.
	u2 := seedUser(t)
	m2 := &domain.GroupMembership{QuinielaID: q.ID, UserID: u2.ID, Status: domain.MembershipActive, Paid: true, JoinedAt: &now}
	if err := repo.Create(context.Background(), m2); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict when exceeding max_members, got %v", err)
	}
}

func TestGroupMembershipRepository_Update_ExceedsMaxMembers_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	maxMembers := 1
	quinielaRepo := repository.NewPostgresQuinielaRepository(testDB)
	q := &domain.Quiniela{
		Name:           "Capped " + nextCode(),
		OwnerID:        owner.ID,
		InviteCode:     nextCode(),
		Currency:       defaultCurrency,
		PrizeThreshold: domain.DefaultPrizeThreshold,
		MaxMembers:     &maxMembers,
	}
	if err := quinielaRepo.Create(context.Background(), q); err != nil {
		t.Fatalf("seed quiniela: %v", err)
	}

	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()

	// Fill the one active slot.
	m1 := &domain.GroupMembership{QuinielaID: q.ID, UserID: owner.ID, Status: domain.MembershipActive, Paid: true, JoinedAt: &now}
	if err := repo.Create(context.Background(), m1); err != nil {
		t.Fatalf("first membership: %v", err)
	}

	// Create a second member as pending, then try to approve (pending → active).
	u2 := seedUser(t)
	m2 := &domain.GroupMembership{QuinielaID: q.ID, UserID: u2.ID, Status: domain.MembershipPending, Paid: false}
	if err := repo.Create(context.Background(), m2); err != nil {
		t.Fatalf("pending membership: %v", err)
	}
	m2.Status = domain.MembershipActive
	m2.JoinedAt = &now
	if err := repo.Update(context.Background(), m2); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict when approving past max_members, got %v", err)
	}
}

func TestGroupMembershipRepository_GetByQuinielaAndUser_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	created := seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.GetByQuinielaAndUser(context.Background(), q.ID, u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected membership, got nil")
	}
	if got.ID != created.ID {
		t.Errorf(fmtIDMismatch, got.ID, created.ID)
	}
	if got.Status != domain.MembershipActive {
		t.Errorf(repoMsgStatusActive, got.Status)
	}
}

func TestGroupMembershipRepository_GetByQuinielaAndUser_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.GetByQuinielaAndUser(context.Background(), 99999, 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestGroupMembershipRepository_Update_ChangesStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	m := seedMembership(t, q.ID, u.ID, domain.MembershipPending, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	now := time.Now().UTC()
	m.Status = domain.MembershipActive
	m.Paid = true
	m.JoinedAt = &now
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.Status != domain.MembershipActive {
		t.Errorf("status not updated: got %q", m.Status)
	}
	if !m.Paid {
		t.Error("paid not updated to true")
	}
}

func TestGroupMembershipRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	ghost := &domain.GroupMembership{ID: 99999, Status: domain.MembershipLeft}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestGroupMembershipRepository_MarkPaid_SetsPaidTrue(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.MarkPaid(context.Background(), q.ID, u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !got.Paid {
		t.Error("expected Paid = true after MarkPaid")
	}
}

func TestGroupMembershipRepository_MarkPaid_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	if _, err := repo.MarkPaid(context.Background(), 99999, 99999); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestGroupMembershipRepository_ListByQuiniela_ReturnsAllMembers(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedMembership(t, q.ID, owner.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, member.ID, domain.MembershipActive, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	members, err := repo.ListByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

func TestGroupMembershipRepository_ListByQuiniela_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	members, err := repo.ListByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

func TestGroupMembershipRepository_ListByUser_ReturnsGroups(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	seedMembership(t, q1.ID, u.ID, domain.MembershipActive, true)
	seedMembership(t, q2.ID, u.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	groups, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}

func TestGroupMembershipRepository_ListByUser_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	groups, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestGroupMembershipRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	created := seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected membership, got nil")
	}
	if got.ID != created.ID {
		t.Errorf(fmtIDMismatch, got.ID, created.ID)
	}
	if got.Status != domain.MembershipActive {
		t.Errorf(repoMsgStatusActive, got.Status)
	}
}

func TestGroupMembershipRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestGroupMembershipRepository_CountActive_ReturnsCount(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedMembership(t, q.ID, owner.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, member.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	count, err := repo.CountActive(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if count != 2 {
		t.Errorf("expected 2 active members, got %d", count)
	}
}

func TestGroupMembershipRepository_CountActive_IgnoresPendingAndLeft(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	u3 := seedUser(t)
	q := seedQuiniela(t, u1.ID)
	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipPending, false)
	seedMembership(t, q.ID, u3.ID, domain.MembershipLeft, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	count, err := repo.CountActive(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if count != 1 {
		t.Errorf("expected 1 active member, got %d", count)
	}
}

// ── GroupMembershipRepository — OldestActiveMember ────────────────────────────

func TestGroupMembershipRepository_OldestActiveMember_ReturnsMemberWithEarliestJoinedAt(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	m1 := seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	time.Sleep(2 * time.Millisecond)
	_ = seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.OldestActiveMember(context.Background(), q.ID, owner.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected a membership, got nil")
	}
	if got.ID != m1.ID {
		t.Errorf("expected oldest member ID %d, got %d", m1.ID, got.ID)
	}
}

func TestGroupMembershipRepository_OldestActiveMember_ExcludesSpecifiedUser(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	_ = seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	time.Sleep(2 * time.Millisecond)
	m2 := seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.OldestActiveMember(context.Background(), q.ID, u1.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected a membership, got nil")
	}
	if got.ID != m2.ID {
		t.Errorf("expected member ID %d (excluded u1), got %d", m2.ID, got.ID)
	}
}

func TestGroupMembershipRepository_OldestActiveMember_NoSuccessor_ReturnsNil(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedMembership(t, q.ID, owner.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.OldestActiveMember(context.Background(), q.ID, owner.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil when no successor, got membership ID %d", got.ID)
	}
}

// ── GroupMembershipRepository — SetRole ────────────────────────────────────────

func TestGroupMembershipRepository_SetRole_UpdatesRole(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	u := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	m := seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	if err := repo.SetRole(context.Background(), m.ID, domain.MembershipRoleCreateOwner); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	updated, err := repo.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.Role != domain.MembershipRoleCreateOwner {
		t.Errorf("expected role %q, got %q", domain.MembershipRoleCreateOwner, updated.Role)
	}
}

func TestGroupMembershipRepository_SetRole_NotFound_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	err := repo.SetRole(context.Background(), 999999, domain.MembershipRoleCreateOwner)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing membership, got %v", err)
	}
}

// ── UserRepository — ListByIDs ─────────────────────────────────────────────────

func TestUserRepository_ListByIDs_ReturnsMatchingUsers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	_ = seedUser(t) // u3 not requested
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.ListByIDs(context.Background(), []int{u1.ID, u2.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 users, got %d", len(got))
	}
}

func TestUserRepository_ListByIDs_EmptySlice_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.ListByIDs(context.Background(), []int{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for empty ids, got %v", got)
	}
}

func TestUserRepository_ListByIDs_IgnoresDeletedUsers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	userRepo := repository.NewPostgresUserRepository(testDB)

	// Soft-delete u2 — it must not appear in ListByIDs results.
	if err := userRepo.Delete(context.Background(), u2.ID); err != nil {
		t.Fatalf("delete u2: %v", err)
	}

	got, err := userRepo.ListByIDs(context.Background(), []int{u1.ID, u2.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 active user, got %d", len(got))
	}
	if got[0].ID != u1.ID {
		t.Errorf("expected user %d, got %d", u1.ID, got[0].ID)
	}
}

// ── QuinielaRepository — RotateInviteCode ─────────────────────────────────────

func TestQuinielaRepository_RotateInviteCode_UpdatesCode(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	newCode := "NEWCODE001"
	exp := time.Now().Add(30 * 24 * time.Hour).UTC()
	got, err := repo.RotateInviteCode(context.Background(), q.ID, newCode, &exp)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected non-nil quiniela after RotateInviteCode")
	}
	if got.InviteCode != newCode {
		t.Errorf("invite code: got %q, want %q", got.InviteCode, newCode)
	}
	if got.InviteCodeExpiresAt == nil {
		t.Fatal("expected InviteCodeExpiresAt to be set")
	}
}

func TestQuinielaRepository_RotateInviteCode_NotFound_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	exp := time.Now().Add(time.Hour)
	_, err := repo.RotateInviteCode(context.Background(), 99999, "NEWCODE002", &exp)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── QuinielaRepository — UpdateStatus ────────────────────────────────────────

func TestQuinielaRepository_UpdateStatus_SetsStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.UpdateStatus(context.Background(), q.ID, domain.QuinielaStatusActive); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, err := repo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.Status != domain.QuinielaStatusActive {
		t.Errorf(repoMsgStatusActive, got.Status)
	}
}

func TestQuinielaRepository_UpdateStatus_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.UpdateStatus(context.Background(), 99999, domain.QuinielaStatusActive); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestQuinielaRepository_CreateWithMembership_HydratesBothIDs(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := &domain.Quiniela{Name: "Atomic Pool", OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency, PrizeThreshold: domain.DefaultPrizeThreshold}
	now := time.Now().UTC()
	m := &domain.GroupMembership{UserID: u.ID, Status: domain.MembershipActive, Paid: false, JoinedAt: &now}

	if err := repo.CreateWithMembership(context.Background(), q, m); err != nil {
		t.Fatalf("CreateWithMembership: %v", err)
	}
	if q.ID == 0 {
		t.Error("expected quiniela ID to be hydrated")
	}
	if m.ID == 0 {
		t.Error("expected membership ID to be hydrated")
	}
	if m.QuinielaID != q.ID {
		t.Errorf("membership.QuinielaID: got %d, want %d", m.QuinielaID, q.ID)
	}
}

func TestQuinielaRepository_CreateWithMembership_QuinielaVisibleAfterCommit(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := &domain.Quiniela{Name: "Visible Pool", OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency, PrizeThreshold: domain.DefaultPrizeThreshold}
	now := time.Now().UTC()
	m := &domain.GroupMembership{UserID: u.ID, Status: domain.MembershipActive, Paid: false, JoinedAt: &now}

	if err := repo.CreateWithMembership(context.Background(), q, m); err != nil {
		t.Fatalf("CreateWithMembership: %v", err)
	}

	got, err := repo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf("GetByID after CreateWithMembership: %v", err)
	}
	if got == nil || got.Name != q.Name {
		t.Errorf("expected quiniela %q to be visible after commit, got %v", q.Name, got)
	}
}

func TestQuinielaRepository_CreateWithMembership_DuplicateName_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)
	code := nextCode()

	q1 := &domain.Quiniela{Name: "Same Name " + code, OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency, PrizeThreshold: domain.DefaultPrizeThreshold}
	now := time.Now().UTC()
	m1 := &domain.GroupMembership{UserID: u.ID, Status: domain.MembershipActive, Paid: false, JoinedAt: &now}
	if err := repo.CreateWithMembership(context.Background(), q1, m1); err != nil {
		t.Fatalf("first CreateWithMembership: %v", err)
	}

	q2 := &domain.Quiniela{Name: q1.Name, OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency, PrizeThreshold: domain.DefaultPrizeThreshold}
	now2 := time.Now().UTC()
	m2 := &domain.GroupMembership{UserID: u.ID, Status: domain.MembershipActive, Paid: false, JoinedAt: &now2}
	err := repo.CreateWithMembership(context.Background(), q2, m2)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict for duplicate name, got %v", err)
	}
}

// ── PredictionRepository — TotalPointsByQuiniela ──────────────────────────────

func TestPredictionRepository_TotalPointsByQuiniela_ReturnsSumPerUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	// Both users are active + paid members.
	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	// User 1: one prediction with 5 points.
	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts1 := 5
	p1.Points = &pts1
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf("update prediction u1: %v", err)
	}

	// User 2: one prediction with 3 points.
	p2 := &domain.Prediction{UserID: u2.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts2 := 3
	p2.Points = &pts2
	if err := predRepo.Update(context.Background(), p2); err != nil {
		t.Fatalf("update prediction u2: %v", err)
	}

	totals, err := predRepo.TotalPointsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(totals))
	}
	if totals[u1.ID] != 5 {
		t.Errorf("user1 total: got %d, want 5", totals[u1.ID])
	}
	if totals[u2.ID] != 3 {
		t.Errorf("user2 total: got %d, want 3", totals[u2.ID])
	}
}

func TestPredictionRepository_TotalPointsByQuiniela_ExcludesUnpaidMembers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	// u1 is paid; u2 is not paid.
	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, false)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := 3
	p1.Points = &pts
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	p2 := &domain.Prediction{UserID: u2.ID, MatchID: m.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts2 := 2
	p2.Points = &pts2
	if err := predRepo.Update(context.Background(), p2); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	totals, err := predRepo.TotalPointsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	// Only u1 (paid) should appear.
	if len(totals) != 1 {
		t.Fatalf("expected 1 entry (paid only), got %d", len(totals))
	}
	if _, ok := totals[u2.ID]; ok {
		t.Error("unpaid user must not appear in leaderboard totals")
	}
}

func TestPredictionRepository_TotalPointsByQuiniela_EmptyQuiniela_ReturnsEmptyMap(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	totals, err := predRepo.TotalPointsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 0 {
		t.Errorf("expected empty map for quiniela with no active paid members, got %v", totals)
	}
}

// ── PredictionRepository — TotalPointsByQuinielaAndPhase ─────────────────────

func TestPredictionRepository_TotalPointsByQuinielaAndPhase_MatchingPhase_ReturnsSumPerUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)

	// seedMatch uses PhaseGroupStage.
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts1 := 5
	p1.Points = &pts1
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf("update prediction u1: %v", err)
	}

	p2 := &domain.Prediction{UserID: u2.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 1}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts2 := 2
	p2.Points = &pts2
	if err := predRepo.Update(context.Background(), p2); err != nil {
		t.Fatalf("update prediction u2: %v", err)
	}

	totals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(totals))
	}
	if totals[u1.ID] != 5 {
		t.Errorf("user1 phase total: got %d, want 5", totals[u1.ID])
	}
	if totals[u2.ID] != 2 {
		t.Errorf("user2 phase total: got %d, want 2", totals[u2.ID])
	}
}

func TestPredictionRepository_TotalPointsByQuinielaAndPhase_NonMatchingPhase_ReturnsZeroForAll(t *testing.T) {
	// Predictions are on a group_stage match; querying the final phase must
	// return all active+paid members with 0 points (not an empty map).
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	m := seedMatch(t) // phase = group_stage
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := 5
	p.Points = &pts
	if err := predRepo.Update(context.Background(), p); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	// Query a different phase — the user has no predictions on final matches.
	totals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseFinal)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(totals) != 1 {
		t.Fatalf("expected 1 member with 0 points, got %d entries", len(totals))
	}
	if totals[u.ID] != 0 {
		t.Errorf("expected 0 points for non-matching phase, got %d", totals[u.ID])
	}
}

func TestPredictionRepository_TotalPointsByQuinielaAndPhase_ExcludesUnpaidMembers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, false) // unpaid

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	for _, u := range []*domain.User{u1, u2} {
		p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		pts := 3
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}

	totals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if _, ok := totals[u2.ID]; ok {
		t.Error("unpaid member must not appear in phase totals")
	}
	if totals[u1.ID] != 3 {
		t.Errorf("paid member total: got %d, want 3", totals[u1.ID])
	}
}

// TestPredictionRepository_TotalPointsByQuinielaAndPhase_CrossPhaseIsolation
// verifies that points scored on matches from a different phase are not
// included in the result. This is the correctness property guaranteed by the
// derived-table approach: only predictions whose match.phase equals the
// requested phase contribute to the aggregated total.
func TestPredictionRepository_TotalPointsByQuinielaAndPhase_CrossPhaseIsolation(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	mGroup := seedMatchWithPhase(t, domain.PhaseGroupStage)
	mFinal := seedMatchWithPhase(t, domain.PhaseFinal)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	// Prediction on the group-stage match: 5 points.
	pGroup := &domain.Prediction{UserID: u.ID, MatchID: mGroup.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), pGroup); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := 5
	pGroup.Points = &pts
	if err := predRepo.Update(context.Background(), pGroup); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	// Prediction on the final match: 2 points.
	pFinal := &domain.Prediction{UserID: u.ID, MatchID: mFinal.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), pFinal); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	finalPts := 2
	pFinal.Points = &finalPts
	if err := predRepo.Update(context.Background(), pFinal); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	// Querying group_stage must return only the 5 points; final points must
	// not bleed across the phase boundary.
	groupTotals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if groupTotals[u.ID] != 5 {
		t.Errorf("group_stage total: got %d, want 5 (final points must not bleed across phases)", groupTotals[u.ID])
	}

	// Querying final must return only the 2 points.
	finalTotals, err := predRepo.TotalPointsByQuinielaAndPhase(context.Background(), q.ID, domain.PhaseFinal)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if finalTotals[u.ID] != 2 {
		t.Errorf("final total: got %d, want 2 (group_stage points must not bleed across phases)", finalTotals[u.ID])
	}
}

// TestPredictionRepository_ListByUserAndQuiniela_ActiveMember_ReturnsPredictions
// verifies that an active member's predictions are returned when queried by
// user + quiniela.
func TestPredictionRepository_ListByUserAndQuiniela_ActiveMember_ReturnsPredictions(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 2, AwayScore: 1}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := predRepo.ListByUserAndQuiniela(context.Background(), u.ID, q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 1 {
		t.Fatalf(repoMsgExpect1Pred, len(preds))
	}
	if preds[0].UserID != u.ID {
		t.Errorf("expected user %d, got %d", u.ID, preds[0].UserID)
	}
	if preds[0].HomeScore != 2 || preds[0].AwayScore != 1 {
		t.Errorf("expected scores 2-1, got %d-%d", preds[0].HomeScore, preds[0].AwayScore)
	}
}

// TestPredictionRepository_ListByUserAndQuiniela_NonMember_ReturnsEmpty verifies
// that a user who has no membership record for the quiniela receives an empty
// slice (not an error).
func TestPredictionRepository_ListByUserAndQuiniela_NonMember_ReturnsEmpty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	// Deliberately do NOT seed a membership for u in q.
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := predRepo.ListByUserAndQuiniela(context.Background(), u.ID, q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 0 {
		t.Errorf("expected empty slice for non-member, got %d predictions", len(preds))
	}
}

// TestPredictionRepository_ListByUserAndQuiniela_InactiveMember_ReturnsEmpty
// verifies that the status = 'active' gate is enforced: a member whose
// membership status is not 'active' must not receive results.
func TestPredictionRepository_ListByUserAndQuiniela_InactiveMember_ReturnsEmpty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipLeft, false)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	preds, err := predRepo.ListByUserAndQuiniela(context.Background(), u.ID, q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(preds) != 0 {
		t.Errorf("expected empty slice for inactive member, got %d predictions", len(preds))
	}
}

// TestPredictionRepository_UpdateManyPoints_LargeBatch verifies that the
// UNNEST bulk-update path correctly persists points for a batch larger than
// the two-row case covered by the basic test. Each prediction in the batch
// must receive its own distinct point value.
func TestPredictionRepository_UpdateManyPoints_LargeBatch(t *testing.T) {
	const batchSize = 10
	cleanTables(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	// Create batchSize users, each with one prediction on the same match.
	m := seedMatch(t)
	preds := make([]*domain.Prediction, batchSize)
	for i := range batchSize {
		u := seedUser(t)
		p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: i, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf("create prediction %d: %v", i, err)
		}
		preds[i] = p
	}

	// Build the points map: prediction i receives i+1 points (all distinct).
	wantPoints := make(map[int]int, batchSize)
	for i, p := range preds {
		wantPoints[p.ID] = i + 1
	}

	if err := predRepo.UpdateManyPoints(context.Background(), wantPoints); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// Verify every prediction received the correct points value.
	for _, p := range preds {
		got, err := predRepo.GetByID(context.Background(), p.ID)
		if err != nil {
			t.Fatalf("get prediction %d: %v", p.ID, err)
		}
		want := wantPoints[p.ID]
		if got.Points == nil || *got.Points != want {
			t.Errorf("prediction %d: got points=%v, want %d", p.ID, got.Points, want)
		}
	}
}

// ── PredictionRepository — PredictionStatsByQuiniela ─────────────────────────

func TestPredictionRepository_PredictionStatsByQuiniela_ReturnsCountsPerUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)

	m1 := seedMatch(t)
	m2 := seedMatch(t)
	m3 := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	// u1: 5 pts (exact), 2 pts (correct), 0 pts (wrong) → correct=2, total=3, exact=1
	setPredPoints := func(userID, matchID, pts int) {
		p := &domain.Prediction{UserID: userID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}
	setPredPoints(u1.ID, m1.ID, domain.PointsExactScore)      // 5 pts
	setPredPoints(u1.ID, m2.ID, domain.PointsCorrectOutcome)  // 2 pts
	setPredPoints(u1.ID, m3.ID, domain.PointsIncorrectResult) // 0 pts

	// u2: 5 pts, 5 pts → correct=2, total=2, exact=2
	setPredPoints(u2.ID, m1.ID, domain.PointsExactScore) // 5 pts
	setPredPoints(u2.ID, m2.ID, domain.PointsExactScore) // 5 pts

	stats, err := predRepo.PredictionStatsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected stats for 2 users, got %d", len(stats))
	}

	s1 := stats[u1.ID]
	if s1 == nil {
		t.Fatal("expected stats for u1, got nil")
	}
	if s1.CorrectCount != 2 {
		t.Errorf("u1 CorrectCount: got %d, want 2", s1.CorrectCount)
	}
	if s1.TotalCount != 3 {
		t.Errorf("u1 TotalCount: got %d, want 3", s1.TotalCount)
	}
	if s1.ExactCount != 1 {
		t.Errorf("u1 ExactCount: got %d, want 1", s1.ExactCount)
	}

	s2 := stats[u2.ID]
	if s2 == nil {
		t.Fatal("expected stats for u2, got nil")
	}
	if s2.CorrectCount != 2 {
		t.Errorf("u2 CorrectCount: got %d, want 2", s2.CorrectCount)
	}
	if s2.TotalCount != 2 {
		t.Errorf("u2 TotalCount: got %d, want 2", s2.TotalCount)
	}
	if s2.ExactCount != 2 {
		t.Errorf("u2 ExactCount: got %d, want 2", s2.ExactCount)
	}
}

func TestPredictionRepository_PredictionStatsByQuiniela_ExcludesUnpaidMembers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, false) // unpaid

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	for _, uid := range []int{u1.ID, u2.ID} {
		p := &domain.Prediction{UserID: uid, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		pts := domain.PointsExactScore
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}

	stats, err := predRepo.PredictionStatsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected stats for 1 user (paid only), got %d", len(stats))
	}
	if _, ok := stats[u2.ID]; ok {
		t.Error("unpaid member must not appear in stats")
	}
}

func TestPredictionRepository_PredictionStatsByQuiniela_UnscoredPredictions_ExcludedFromCounts(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)

	m1 := seedMatch(t)
	m2 := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	// m1: scored with 5 pts; m2: unscored (points nil)
	p1 := &domain.Prediction{UserID: u.ID, MatchID: m1.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := domain.PointsExactScore
	p1.Points = &pts
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	p2 := &domain.Prediction{UserID: u.ID, MatchID: m2.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	// p2.Points intentionally left nil (match not yet scored)

	stats, err := predRepo.PredictionStatsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	s := stats[u.ID]
	if s == nil {
		t.Fatal("expected stats for user, got nil")
	}
	if s.TotalCount != 1 {
		t.Errorf("TotalCount: got %d, want 1 (unscored must be excluded)", s.TotalCount)
	}
	if s.ExactCount != 1 {
		t.Errorf("ExactCount: got %d, want 1", s.ExactCount)
	}
}

func TestPredictionRepository_PredictionStatsByQuiniela_NoMembers_ReturnsEmptyMap(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	stats, err := predRepo.PredictionStatsByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stats) != 0 {
		t.Errorf("expected empty map for quiniela with no active paid members, got %d entries", len(stats))
	}
}

func TestPredictionRepository_PredictionStatsByQuiniela_CancelledContext_ReturnsError(t *testing.T) {
	predRepo := repository.NewPostgresPredictionRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the query runs

	_, err := predRepo.PredictionStatsByQuiniela(ctx, 1)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── GetUserPredictionCounts ───────────────────────────────────────────────────

func TestPredictionRepository_GetUserPredictionCounts_ReturnsAggregates(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	m3 := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	createAndScore := func(matchID, pts int) {
		p := &domain.Prediction{UserID: u.ID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}
	// 5 pts (exact), 2 pts (correct), 0 pts (wrong)
	createAndScore(m1.ID, domain.PointsExactScore)
	createAndScore(m2.ID, domain.PointsCorrectOutcome)
	createAndScore(m3.ID, domain.PointsIncorrectResult)

	// unscored prediction — counts total but not scored
	p4 := &domain.Prediction{UserID: u.ID, MatchID: seedMatch(t).ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p4); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	counts, err := predRepo.GetUserPredictionCounts(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.TotalPredictions != 4 {
		t.Errorf("TotalPredictions: want 4, got %d", counts.TotalPredictions)
	}
	if counts.ScoredPredictions != 3 {
		t.Errorf("ScoredPredictions: want 3, got %d", counts.ScoredPredictions)
	}
	if counts.CorrectPredictions != 2 {
		t.Errorf("CorrectPredictions: want 2, got %d", counts.CorrectPredictions)
	}
	if counts.ExactPredictions != 1 {
		t.Errorf("ExactPredictions: want 1, got %d", counts.ExactPredictions)
	}
	// TotalPoints = 5 + 2 + 0 = 7
	if counts.TotalPoints != 7 {
		t.Errorf("TotalPoints: want 7, got %d", counts.TotalPoints)
	}
	if counts.LastPredictionAt == nil {
		t.Error("LastPredictionAt: want non-nil")
	}
}

func TestPredictionRepository_GetUserPredictionCounts_NoPredictions_ReturnsZeroes(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	counts, err := predRepo.GetUserPredictionCounts(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.TotalPredictions != 0 || counts.ScoredPredictions != 0 ||
		counts.CorrectPredictions != 0 || counts.ExactPredictions != 0 || counts.TotalPoints != 0 {
		t.Errorf("want all zeros for user with no predictions, got %+v", counts)
	}
	if counts.LastPredictionAt != nil {
		t.Error("LastPredictionAt: want nil for user with no predictions")
	}
}

func TestPredictionRepository_GetUserPredictionCounts_IsolatedPerUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	pts := domain.PointsExactScore
	p1.Points = &pts
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	counts, err := predRepo.GetUserPredictionCounts(context.Background(), u2.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.TotalPredictions != 0 {
		t.Errorf("u2 should have 0 predictions, got %d", counts.TotalPredictions)
	}
}

func TestPredictionRepository_GetUserPredictionCounts_CancelledContext_ReturnsError(t *testing.T) {
	predRepo := repository.NewPostgresPredictionRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := predRepo.GetUserPredictionCounts(ctx, 1)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── GetUserPointsByPhase ──────────────────────────────────────────────────────

func TestPredictionRepository_GetUserPointsByPhase_ReturnsPerPhasePoints(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	mGroup := seedMatchWithPhase(t, domain.PhaseGroupStage)
	mFinal := seedMatchWithPhase(t, domain.PhaseFinal)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	createAndScore := func(matchID, pts int) {
		p := &domain.Prediction{UserID: u.ID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}
	createAndScore(mGroup.ID, domain.PointsExactScore)     // 5 pts in group stage
	createAndScore(mFinal.ID, domain.PointsCorrectOutcome) // 2 pts in final

	byPhase, err := predRepo.GetUserPointsByPhase(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if byPhase[domain.PhaseGroupStage] != domain.PointsExactScore {
		t.Errorf("group stage: want %d, got %d", domain.PointsExactScore, byPhase[domain.PhaseGroupStage])
	}
	if byPhase[domain.PhaseFinal] != domain.PointsCorrectOutcome {
		t.Errorf("final: want %d, got %d", domain.PointsCorrectOutcome, byPhase[domain.PhaseFinal])
	}
}

func TestPredictionRepository_GetUserPointsByPhase_UnscoredExcluded(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t) // PhaseGroupStage by default
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	// no points set — prediction is unscored

	byPhase, err := predRepo.GetUserPointsByPhase(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(byPhase) != 0 {
		t.Errorf("want empty map for unscored predictions, got %v", byPhase)
	}
}

func TestPredictionRepository_GetUserPointsByPhase_NoPredictions_ReturnsEmptyMap(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	byPhase, err := predRepo.GetUserPointsByPhase(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(byPhase) != 0 {
		t.Errorf("want empty map, got %v", byPhase)
	}
}

func TestPredictionRepository_GetUserPointsByPhase_CancelledContext_ReturnsError(t *testing.T) {
	predRepo := repository.NewPostgresPredictionRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := predRepo.GetUserPointsByPhase(ctx, 1)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── ListUserScoredPointsChronological ────────────────────────────────────────

func TestPredictionRepository_ListUserScoredPointsChronological_ReturnsAllScoredPoints(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	m1 := seedMatch(t)
	m2 := seedMatch(t)
	m3 := seedMatch(t)

	createAndScore := func(matchID, pts int) {
		p := &domain.Prediction{UserID: u.ID, MatchID: matchID, HomeScore: 1, AwayScore: 0}
		if err := predRepo.Create(context.Background(), p); err != nil {
			t.Fatalf(fmtCreateErr, err)
		}
		p.Points = &pts
		if err := predRepo.Update(context.Background(), p); err != nil {
			t.Fatalf(fmtUpdatePredErr, err)
		}
	}
	createAndScore(m1.ID, domain.PointsExactScore)
	createAndScore(m2.ID, domain.PointsCorrectOutcome)
	createAndScore(m3.ID, domain.PointsIncorrectResult)

	pts, err := predRepo.ListUserScoredPointsChronological(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pts) != 3 {
		t.Fatalf("want 3 scored points, got %d", len(pts))
	}
	// Verify the multiset of returned values is correct without depending on
	// exact ordering (seedMatch uses time.Now() which may not differ enough
	// across rapid calls for a stable sort).
	sum := 0
	for _, p := range pts {
		sum += p
	}
	wantSum := domain.PointsExactScore + domain.PointsCorrectOutcome + domain.PointsIncorrectResult
	if sum != wantSum {
		t.Errorf("total points sum: want %d, got %d", wantSum, sum)
	}
}

func TestPredictionRepository_ListUserScoredPointsChronological_ExcludesUnscored(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	// m1: scored
	p1 := &domain.Prediction{UserID: u.ID, MatchID: m1.ID, HomeScore: 1, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	scored := domain.PointsExactScore
	p1.Points = &scored
	if err := predRepo.Update(context.Background(), p1); err != nil {
		t.Fatalf(fmtUpdatePredErr, err)
	}

	// m2: unscored (no points set)
	p2 := &domain.Prediction{UserID: u.ID, MatchID: m2.ID, HomeScore: 0, AwayScore: 0}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	pts, err := predRepo.ListUserScoredPointsChronological(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pts) != 1 {
		t.Fatalf("want 1 scored point entry, got %d", len(pts))
	}
	if pts[0] != domain.PointsExactScore {
		t.Errorf("pts[0]: want %d, got %d", domain.PointsExactScore, pts[0])
	}
}

func TestPredictionRepository_ListUserScoredPointsChronological_NoPredictions_ReturnsNil(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	pts, err := predRepo.ListUserScoredPointsChronological(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pts) != 0 {
		t.Errorf("want empty slice, got %v", pts)
	}
}

func TestPredictionRepository_ListUserScoredPointsChronological_CancelledContext_ReturnsError(t *testing.T) {
	predRepo := repository.NewPostgresPredictionRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := predRepo.ListUserScoredPointsChronological(ctx, 1)
	if err == nil {
		t.Fatal(repoMsgCancelledCtx)
	}
}

// ── TiebreakerConfigRepository ────────────────────────────────────────────────

func TestTiebreakerConfigRepository_Get_ReturnsNilWhenEmpty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg != nil {
		t.Errorf("expected nil before any question is set, got %+v", cfg)
	}
}

func TestTiebreakerConfigRepository_Upsert_CreatesAndUpdates(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.Upsert(context.Background(), "Total goals in the Final")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg.ID != 1 {
		t.Errorf("id: want 1, got %d", cfg.ID)
	}
	if cfg.Question != "Total goals in the Final" {
		t.Errorf("question: want 'Total goals in the Final', got %q", cfg.Question)
	}
	if cfg.Result != nil {
		t.Errorf("result: want nil before confirmation, got %v", cfg.Result)
	}

	// Update existing row.
	cfg2, err := repo.Upsert(context.Background(), "Total goals in the tournament")
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if cfg2.Question != "Total goals in the tournament" {
		t.Errorf("updated question: want updated text, got %q", cfg2.Question)
	}
}

func TestTiebreakerConfigRepository_Get_ReturnsAfterUpsert(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	_, err := repo.Upsert(context.Background(), repoTotalGoals)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	cfg, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg == nil {
		t.Fatal("expected config after upsert, got nil")
	}
	if cfg.Question != repoTotalGoals {
		t.Errorf("question: want 'Total goals', got %q", cfg.Question)
	}
}

func TestTiebreakerConfigRepository_SetResult_SetsResult(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	if _, err := repo.Upsert(context.Background(), repoTotalGoals); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := repo.SetResult(context.Background(), 42); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	cfg, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg.Result == nil || *cfg.Result != 42 {
		t.Errorf("result: want 42, got %v", cfg.Result)
	}
}

func TestTiebreakerConfigRepository_SetResult_NotFoundWhenNoConfig(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	err := repo.SetResult(context.Background(), 10)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── TournamentRepository ──────────────────────────────────────────────────────

func TestTournamentRepository_CreateSlot_ReturnsSlot(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	slot, err := repo.CreateSlot(context.Background(), "winner_group_a")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if slot.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if slot.Label != "winner_group_a" {
		t.Errorf("label: want winner_group_a, got %s", slot.Label)
	}
	if slot.Team != nil {
		t.Errorf("team: want nil on creation, got %v", slot.Team)
	}
}

func TestTournamentRepository_GetSlot_Found(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	created, err := repo.CreateSlot(context.Background(), "runner_up_group_a")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetSlot(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected slot, got nil")
	}
	if got.ID != created.ID {
		t.Errorf(fmtIDMismatch, got.ID, created.ID)
	}
}

func TestTournamentRepository_GetSlot_NotFoundReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	got, err := repo.GetSlot(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestTournamentRepository_ListSlots_ReturnsList(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	if _, err := repo.CreateSlot(context.Background(), "winner_group_b"); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if _, err := repo.CreateSlot(context.Background(), "runner_up_group_b"); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	slots, err := repo.ListSlots(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(slots) != 2 {
		t.Errorf("slots: want 2, got %d", len(slots))
	}
}

func TestTournamentRepository_ListSlots_EmptyWhenNone(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	slots, err := repo.ListSlots(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(slots) != 0 {
		t.Errorf("slots: want 0, got %d", len(slots))
	}
}

func TestTournamentRepository_ConfirmSlot_SetsTeam(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	created, err := repo.CreateSlot(context.Background(), "winner_group_c")
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	confirmed, err := repo.ConfirmSlot(context.Background(), created.ID, u.ID, repoMexico)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if confirmed.Team == nil || *confirmed.Team != repoMexico {
		t.Errorf("team: want Mexico, got %v", confirmed.Team)
	}
	if confirmed.ConfirmedAt == nil {
		t.Error("confirmed_at: want non-nil after confirmation")
	}
	if confirmed.ConfirmedByUserID == nil || *confirmed.ConfirmedByUserID != u.ID {
		t.Errorf("confirmed_by_user_id: want %d, got %v", u.ID, confirmed.ConfirmedByUserID)
	}
}

func TestTournamentRepository_ConfirmSlot_NotFoundWhenMissing(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	_, err := repo.ConfirmSlot(context.Background(), 99999, u.ID, repoMexico)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── helpers for new repositories ─────────────────────────────────────────────

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

func seedSystemParam(t *testing.T, key, value, category string) *domain.SystemParam {
	t.Helper()
	var p domain.SystemParam
	err := testDB.QueryRow(context.Background(),
		`INSERT INTO system_params (key, value, category)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		 RETURNING key, value, type, category, is_runtime, created_at, updated_at`,
		key, value, category,
	).Scan(&p.Key, &p.Value, &p.Type, &p.Category, &p.IsRuntime, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		t.Fatalf("seed system param: %v", err)
	}
	return &p
}

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

// ── AuditLogRepository ────────────────────────────────────────────────────────

func TestAuditLogRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	actorID := u.ID
	resType := "user"
	resID := u.ID
	entry := &domain.AuditLog{
		ActorID:      &actorID,
		Action:       "ban_user",
		ResourceType: &resType,
		ResourceID:   &resID,
		Metadata:     map[string]any{"reason": "spam"},
	}
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if entry.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestAuditLogRepository_Create_NilMetadataAndActor(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	entry := &domain.AuditLog{Action: "system_boot"}
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if entry.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestAuditLogRepository_ListByActor(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	actorID := u.ID
	for range 3 {
		e := &domain.AuditLog{ActorID: &actorID, Action: "some_action"}
		_ = repo.Create(context.Background(), e)
	}
	// unrelated entry from another actor
	other := seedUser(t)
	otherID := other.ID
	_ = repo.Create(context.Background(), &domain.AuditLog{ActorID: &otherID, Action: "other_action"})

	results, err := repo.ListByActor(context.Background(), u.ID, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 entries for actor %d, got %d", u.ID, len(results))
	}
}

func TestAuditLogRepository_ListByEntity(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	resType := repoResourceQuiniela
	resID := 42
	for range 2 {
		e := &domain.AuditLog{Action: "update", ResourceType: &resType, ResourceID: &resID}
		_ = repo.Create(context.Background(), e)
	}
	// noise: different resource type
	otherType := "user"
	_ = repo.Create(context.Background(), &domain.AuditLog{Action: "ban", ResourceType: &otherType, ResourceID: &resID})

	results, err := repo.ListByEntity(context.Background(), repoResourceQuiniela, 42, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 quiniela entries, got %d", len(results))
	}
}

func TestAuditLogRepository_ListByAction(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	for range 4 {
		_ = repo.Create(context.Background(), &domain.AuditLog{Action: "payment_validate"})
	}
	_ = repo.Create(context.Background(), &domain.AuditLog{Action: "payment_reject"})

	results, err := repo.ListByAction(context.Background(), "payment_validate", repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 validate entries, got %d", len(results))
	}
}

func TestAuditLogRepository_List_Pagination(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	for range 5 {
		_ = repo.Create(context.Background(), &domain.AuditLog{Action: "ping"})
	}

	page, err := repo.List(context.Background(), repository.AuditLogFilters{}, repository.Pagination{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page) != 2 {
		t.Errorf("expected 2 entries with limit=2, got %d", len(page))
	}
}

// ── PaymentRecordRepository ───────────────────────────────────────────────────

func TestPaymentRecordRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)

	pr := seedPaymentRecord(t, q.ID, u.ID)
	if pr.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if pr.Status != domain.PaymentStatusPending {
		t.Errorf("expected pending status, got %q", pr.Status)
	}
}

func TestPaymentRecordRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	created := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("expected ID %d, got %v", created.ID, got)
	}
}

func TestPaymentRecordRepository_GetByID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	got, err := repo.GetByID(context.Background(), 999999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestPaymentRecordRepository_ListByQuiniela(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	// noise: another quiniela
	u2 := seedUser(t)
	q2 := seedQuiniela(t, u2.ID)
	seedPaymentRecord(t, q2.ID, u2.ID)

	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	results, err := repo.ListByQuiniela(context.Background(), q.ID, repository.PaymentFilters{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 records for quiniela %d, got %d", q.ID, len(results))
	}
}

func TestPaymentRecordRepository_ListByQuiniela_FilterByStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	admin := seedUser(t)
	pr1 := seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	_, _ = repo.Validate(context.Background(), pr1.ID, admin.ID, "ok")

	status := domain.PaymentStatusPending
	pending, err := repo.ListByQuiniela(context.Background(), q.ID, repository.PaymentFilters{Status: &status})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending record, got %d", len(pending))
	}
}

func TestPaymentRecordRepository_ListByUser(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	results, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 records for user %d, got %d", u.ID, len(results))
	}
}

func TestPaymentRecordRepository_ListPending(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	admin := seedUser(t)
	pr1 := seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	_, _ = repo.Validate(context.Background(), pr1.ID, admin.ID, "paid")

	pending, err := repo.ListPending(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending record, got %d", len(pending))
	}
}

func TestPaymentRecordRepository_Validate_TransitionsToConfirmed(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	pr := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	result, err := repo.Validate(context.Background(), pr.ID, admin.ID, "verified manually")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if result.Status != domain.PaymentStatusConfirmed {
		t.Errorf("expected confirmed, got %q", result.Status)
	}
	if result.ReviewedBy == nil || *result.ReviewedBy != admin.ID {
		t.Errorf("expected reviewed_by %d, got %v", admin.ID, result.ReviewedBy)
	}
	if result.ConfirmedAt == nil {
		t.Error("expected confirmed_at to be set")
	}
}

func TestPaymentRecordRepository_Reject_TransitionsToRejected(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	pr := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	result, err := repo.Reject(context.Background(), pr.ID, admin.ID, repoFakeReceipt)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if result.Status != domain.PaymentStatusRejected {
		t.Errorf("expected rejected, got %q", result.Status)
	}
	if result.Notes != repoFakeReceipt {
		t.Errorf("expected notes %q, got %q", repoFakeReceipt, result.Notes)
	}
}

func TestPaymentRecordRepository_Validate_NotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	_, err := repo.Validate(context.Background(), 999999, u.ID, "")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestPaymentRecordRepository_Reject_AlreadyConfirmedIsNotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	pr := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	_, _ = repo.Validate(context.Background(), pr.ID, admin.ID, "ok")

	_, err := repo.Reject(context.Background(), pr.ID, admin.ID, "late reject")
	if !isNotFound(err) {
		t.Errorf("expected not-found for confirmed payment reject, got %v", err)
	}
}

// ── LeaderboardSnapshotRepository ────────────────────────────────────────────

func TestLeaderboardSnapshotRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	snapshot := &domain.LeaderboardSnapshot{
		QuinielaID: q.ID,
		TakenAt:    time.Now().UTC().Truncate(time.Microsecond),
		Entries: []domain.LeaderboardSnapshotEntry{
			{UserID: u.ID, Rank: 1, TotalPoints: 15, PrizeWinner: true},
		},
	}
	if err := repo.Create(context.Background(), snapshot); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if snapshot.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if len(snapshot.Entries) != 1 || snapshot.Entries[0].UserID != u.ID {
		t.Errorf("entries not round-tripped correctly: %+v", snapshot.Entries)
	}
}

func TestLeaderboardSnapshotRepository_Create_EmptyEntries(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	snapshot := &domain.LeaderboardSnapshot{
		QuinielaID: q.ID,
		TakenAt:    time.Now().UTC(),
		Entries:    []domain.LeaderboardSnapshotEntry{},
	}
	if err := repo.Create(context.Background(), snapshot); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if snapshot.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestLeaderboardSnapshotRepository_GetLatest(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	base := time.Now().UTC().Truncate(time.Microsecond)
	older := &domain.LeaderboardSnapshot{QuinielaID: q.ID, TakenAt: base.Add(-time.Hour), Entries: nil}
	newer := &domain.LeaderboardSnapshot{QuinielaID: q.ID, TakenAt: base, Entries: nil}
	_ = repo.Create(context.Background(), older)
	_ = repo.Create(context.Background(), newer)

	latest, err := repo.GetLatest(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if latest == nil || latest.ID != newer.ID {
		t.Errorf("expected latest ID %d, got %v", newer.ID, latest)
	}
}

func TestLeaderboardSnapshotRepository_GetLatest_NoneExists(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	snap, err := repo.GetLatest(context.Background(), 999999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if snap != nil {
		t.Errorf(fmtExpectNilGot, snap)
	}
}

func TestLeaderboardSnapshotRepository_ListByQuiniela_WithLimit(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 5 {
		s := &domain.LeaderboardSnapshot{QuinielaID: q.ID, TakenAt: base.Add(time.Duration(i) * time.Minute)}
		_ = repo.Create(context.Background(), s)
	}

	results, err := repo.ListByQuiniela(context.Background(), q.ID, 3)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 snapshots with limit=3, got %d", len(results))
	}
}

// ── UserRepository extensions (Ban/Unban/ListBanned) ─────────────────────────

func TestUserRepository_Ban_SetsFields(t *testing.T) {
	cleanTables(t)
	user := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	banned, err := repo.Ban(context.Background(), user.ID, admin.ID, repoPolicyViolation)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if banned.BannedAt == nil {
		t.Error("expected BannedAt to be set")
	}
	if banned.BannedBy == nil || *banned.BannedBy != admin.ID {
		t.Errorf("expected BannedBy %d, got %v", admin.ID, banned.BannedBy)
	}
	if banned.BanReason != repoPolicyViolation {
		t.Errorf("expected BanReason %q, got %q", repoPolicyViolation, banned.BanReason)
	}
}

func TestUserRepository_Ban_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	_, err := repo.Ban(context.Background(), 999999, 1, "reason")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUserRepository_Unban_ClearsFields(t *testing.T) {
	cleanTables(t)
	user := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	_, _ = repo.Ban(context.Background(), user.ID, admin.ID, "test")
	if err := repo.Unban(context.Background(), user.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), user.ID)
	if got.BannedAt != nil {
		t.Errorf("expected BannedAt nil after unban, got %v", got.BannedAt)
	}
}

func TestUserRepository_Unban_IsIdempotent(t *testing.T) {
	cleanTables(t)
	user := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	// user is not banned — should succeed silently
	if err := repo.Unban(context.Background(), user.ID); err != nil {
		t.Fatalf("unban on unbanned user should not error: %v", err)
	}
}

func TestUserRepository_ListBanned(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	_ = seedUser(t) // active, not banned
	admin := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	_, _ = repo.Ban(context.Background(), u1.ID, admin.ID, "r1")
	_, _ = repo.Ban(context.Background(), u2.ID, admin.ID, "r2")

	banned, err := repo.ListBanned(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(banned) != 2 {
		t.Errorf("expected 2 banned users, got %d", len(banned))
	}
}

// ── GroupMembershipRepository extension (RemoveByAdmin) ──────────────────────

func TestGroupMembershipRepository_RemoveByAdmin_SetsStatusLeft(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	admin := seedUser(t)
	m := seedActiveMembership(t, q.ID, member.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	if err := repo.RemoveByAdmin(context.Background(), m.ID, admin.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), m.ID)
	if got.Status != domain.MembershipLeft {
		t.Errorf("expected status %q, got %q", domain.MembershipLeft, got.Status)
	}
}

func TestGroupMembershipRepository_RemoveByAdmin_NotFoundWhenInactive(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	admin := seedUser(t)
	m := seedActiveMembership(t, q.ID, member.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	_ = repo.RemoveByAdmin(context.Background(), m.ID, admin.ID) // first removal
	err := repo.RemoveByAdmin(context.Background(), m.ID, admin.ID)
	if !isNotFound(err) {
		t.Errorf("expected not-found on second remove, got %v", err)
	}
}

// ── QuinielaRepository extensions ────────────────────────────────────────────

func TestQuinielaRepository_UpdateGroupSettings(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	maxM := 10
	updated, err := repo.UpdateGroupSettings(context.Background(), q.ID, &maxM, 5000)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.MaxMembers == nil || *updated.MaxMembers != 10 {
		t.Errorf("expected MaxMembers 10, got %v", updated.MaxMembers)
	}
	if updated.EntryFee != 5000 {
		t.Errorf("expected EntryFee 5000, got %d", updated.EntryFee)
	}
}

func TestQuinielaRepository_UpdateGroupSettings_NilMaxMembersRemovesCap(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	updated, err := repo.UpdateGroupSettings(context.Background(), q.ID, nil, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.MaxMembers != nil {
		t.Errorf("expected nil MaxMembers, got %v", updated.MaxMembers)
	}
}

func TestQuinielaRepository_UpdateGroupSettings_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	_, err := repo.UpdateGroupSettings(context.Background(), 999999, nil, 0)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestQuinielaRepository_DeleteByAdmin_SoftDeletes(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.DeleteByAdmin(context.Background(), q.ID, admin.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), q.ID)
	if got != nil {
		t.Errorf("expected nil after DeleteByAdmin, got %+v", got)
	}
}

func TestQuinielaRepository_DeleteByAdmin_NotFoundWhenAlreadyDeleted(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	_ = repo.DeleteByAdmin(context.Background(), q.ID, admin.ID)
	err := repo.DeleteByAdmin(context.Background(), q.ID, admin.ID)
	if !isNotFound(err) {
		t.Errorf("expected not-found on second delete, got %v", err)
	}
}

func TestUserRepository_Unban_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	err := repo.Unban(context.Background(), 999999)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestQuinielaRepository_Update_ConflictOnDuplicateName(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q2.Name = q1.Name
	err := repo.Update(context.Background(), q2)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error, got %v", err)
	}
}

func TestAuditLogRepository_Create_WithActorRole(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	actorID := u.ID
	role := domain.RoleAdmin
	entry := &domain.AuditLog{
		ActorID:   &actorID,
		ActorRole: &role,
		Action:    "ban_user",
	}
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if entry.ActorRole == nil || *entry.ActorRole != domain.RoleAdmin {
		t.Errorf("expected ActorRole %q, got %v", domain.RoleAdmin, entry.ActorRole)
	}
}

func TestAuditLogRepository_List_WithRoleAndMetadata(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	actorID := u.ID
	role := domain.RoleAdmin
	resType := repoResourceQuiniela
	resID := 1
	entry := &domain.AuditLog{
		ActorID:      &actorID,
		ActorRole:    &role,
		Action:       "delete_group",
		ResourceType: &resType,
		ResourceID:   &resID,
		Metadata:     map[string]any{"reason": "inactivity"},
	}
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	results, err := repo.List(context.Background(), repository.AuditLogFilters{}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(results))
	}
	got := results[0]
	if got.ActorRole == nil || *got.ActorRole != domain.RoleAdmin {
		t.Errorf("expected ActorRole %q, got %v", domain.RoleAdmin, got.ActorRole)
	}
	if got.Metadata["reason"] != "inactivity" {
		t.Errorf("expected metadata reason 'inactivity', got %v", got.Metadata["reason"])
	}
}

// ── GroupMembershipRepository admin extensions ────────────────────────────────

func TestGroupMembershipRepository_ListGroupIDsWithoutOwner_IncludesOrphan(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID) // no membership inserted
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	ids, err := repo.ListGroupIDsWithoutOwner(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	found := false
	for _, id := range ids {
		if id == q.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected quiniela %d to appear in orphan list, got %v", q.ID, ids)
	}
}

func TestGroupMembershipRepository_ListGroupIDsWithoutOwner_ExcludesWithOwner(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()
	m := &domain.GroupMembership{
		QuinielaID: q.ID,
		UserID:     owner.ID,
		Status:     domain.MembershipActive,
		Role:       domain.MembershipRoleCreateOwner,
		JoinedAt:   &now,
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed owner membership: %v", err)
	}

	ids, err := repo.ListGroupIDsWithoutOwner(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	for _, id := range ids {
		if id == q.ID {
			t.Errorf("quiniela %d with active owner should not appear in orphan list", q.ID)
		}
	}
}

func TestGroupMembershipRepository_ListStalePending_ReturnsPending(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedMembership(t, q.ID, member.ID, domain.MembershipPending, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	stale, err := repo.ListStalePending(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stale) != 1 {
		t.Errorf("expected 1 stale pending, got %d", len(stale))
	}
}

func TestGroupMembershipRepository_ListStalePending_ExcludesActive(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedActiveMembership(t, q.ID, member.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	stale, err := repo.ListStalePending(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stale) != 0 {
		t.Errorf("expected 0 stale (active member filtered out), got %d", len(stale))
	}
}

// ── PaymentRecordRepository admin extensions ──────────────────────────────────

func TestPaymentRecordRepository_List_NoFilter_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	results, err := repo.List(context.Background(), repository.PaymentFilters{}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 records, got %d", len(results))
	}
}

func TestPaymentRecordRepository_List_FilterByQuinielaID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q1.ID, u.ID)
	seedPaymentRecord(t, q2.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	results, err := repo.List(context.Background(), repository.PaymentFilters{QuinielaID: &q1.ID}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 record for quiniela %d, got %d", q1.ID, len(results))
	}
}

func TestPaymentRecordRepository_List_Pagination(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	results, err := repo.List(context.Background(), repository.PaymentFilters{}, repository.Pagination{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 records with limit=2 offset=1, got %d", len(results))
	}
}

func TestPaymentRecordRepository_ListStale_ReturnsPending(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	stale, err := repo.ListStale(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stale) != 1 {
		t.Errorf("expected 1 stale payment, got %d", len(stale))
	}
}

func TestPaymentRecordRepository_ListStale_ExcludesConfirmed(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	pr := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	_, _ = repo.Validate(context.Background(), pr.ID, admin.ID, "ok")

	stale, err := repo.ListStale(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stale) != 0 {
		t.Errorf("expected 0 stale (confirmed excluded), got %d", len(stale))
	}
}

// ── PredictionRepository admin extensions ─────────────────────────────────────

func TestPredictionRepository_ListAdmin_NoFilter_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAdmin(context.Background(), repository.PredictionAdminFilters{}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf(repoMsgExpect1Pred, len(results))
	}
}

func TestPredictionRepository_ListAdmin_FilterByUserID(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u1.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u2.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAdmin(context.Background(), repository.PredictionAdminFilters{UserID: &u1.ID}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].UserID != u1.ID {
		t.Errorf("expected 1 prediction for user %d, got %d", u1.ID, len(results))
	}
}

func TestPredictionRepository_ListAdmin_PaginationLimit(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u.ID, MatchID: m1.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u.ID, MatchID: m2.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAdmin(context.Background(), repository.PredictionAdminFilters{}, repository.Pagination{Limit: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 prediction with limit=1, got %d", len(results))
	}
}

func TestPredictionRepository_GlobalLeaderboard_RanksUsers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	p1 := &domain.Prediction{UserID: u1.ID, MatchID: m.ID, HomeScore: 1}
	if err := predRepo.Create(context.Background(), p1); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := predRepo.UpdateManyPoints(context.Background(), map[int]int{p1.ID: 10}); err != nil {
		t.Fatalf("update points: %v", err)
	}

	p2 := &domain.Prediction{UserID: u2.ID, MatchID: m.ID}
	if err := predRepo.Create(context.Background(), p2); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	entries, err := predRepo.GlobalLeaderboard(context.Background(), 10)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	if entries[0].UserID != u1.ID {
		t.Errorf("expected u1 first (most points), got userID=%d", entries[0].UserID)
	}
}

func TestPredictionRepository_GlobalLeaderboard_LimitRespected(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	m1 := seedMatch(t)
	m2 := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u1.ID, MatchID: m1.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Prediction{UserID: u2.ID, MatchID: m2.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	entries, err := repo.GlobalLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry with limit=1, got %d", len(entries))
	}
}

// ── QuinielaRepository admin extensions ──────────────────────────────────────

func TestQuinielaRepository_ListByIDs_ReturnsMatching(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	_ = seedQuiniela(t, u.ID) // not requested
	repo := repository.NewPostgresQuinielaRepository(testDB)

	results, err := repo.ListByIDs(context.Background(), []int{q1.ID, q2.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 quinielas, got %d", len(results))
	}
}

func TestQuinielaRepository_ListByIDs_EmptyInput_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	results, err := repo.ListByIDs(context.Background(), []int{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if results != nil {
		t.Errorf("expected nil for empty ids, got %v", results)
	}
}

// ── TiebreakerRepository admin extensions ────────────────────────────────────

func TestTiebreakerRepository_ListAll_ReturnsList(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u1.ID, Prediction: 3}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u2.ID, Prediction: 5}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAll(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 tiebreakers, got %d", len(results))
	}
}

func TestTiebreakerRepository_ListAll_PaginationLimit(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u1.ID, Prediction: 3}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u2.ID, Prediction: 5}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAll(context.Background(), repository.Pagination{Limit: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 tiebreaker with limit=1, got %d", len(results))
	}
}

// ── UserRepository admin extensions ──────────────────────────────────────────

func TestUserRepository_ListFiltered_NoFilter_ReturnsAll(t *testing.T) {
	cleanTables(t)
	seedUser(t)
	seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	results, err := repo.ListFiltered(context.Background(), repository.UserFilters{}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 users, got %d", len(results))
	}
}

func TestUserRepository_ListFiltered_FilterByBanned(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	_ = seedUser(t) // active
	admin := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)
	_, _ = repo.Ban(context.Background(), u1.ID, admin.ID, "test")

	banned := true
	results, err := repo.ListFiltered(context.Background(), repository.UserFilters{Banned: &banned}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].ID != u1.ID {
		t.Errorf("expected 1 banned user, got %d", len(results))
	}
}

func TestUserRepository_ListFiltered_FilterByRole(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	admin := &domain.User{Name: "Admin", Email: "admin@example.com", Role: domain.RoleAdmin}
	if err := repo.Create(context.Background(), admin); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	seedUser(t) // role = player

	role := domain.RoleAdmin
	results, err := repo.ListFiltered(context.Background(), repository.UserFilters{Role: &role}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].Role != domain.RoleAdmin {
		t.Errorf("expected 1 admin user, got %d", len(results))
	}
}

func TestUserRepository_ListFiltered_FilterBySearch(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	if err := repo.Create(context.Background(), &domain.User{Name: "alice", Email: "alice@example.com", Role: domain.RoleUser}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.User{Name: "bob", Email: "bob@example.com", Role: domain.RoleUser}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	search := "alic"
	results, err := repo.ListFiltered(context.Background(), repository.UserFilters{Search: &search}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].Name != "alice" {
		t.Errorf("expected 1 user matching 'alic', got %d", len(results))
	}
}

func TestUserRepository_ListFiltered_PaginationOffset(t *testing.T) {
	cleanTables(t)
	seedUser(t)
	seedUser(t)
	seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	results, err := repo.ListFiltered(context.Background(), repository.UserFilters{}, repository.Pagination{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 users with limit=2 offset=1, got %d", len(results))
	}
}

// ── PredictionRepository.ListQuinielaIDsByMatch ───────────────────────────────

func TestPredictionRepository_ListQuinielaIDsByMatch_ReturnsAffectedQuinielas(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q1 := seedQuiniela(t, u1.ID)
	q2 := seedQuiniela(t, u2.ID)

	// Both users are active + paid in their respective quinielas.
	seedMembership(t, q1.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q2.ID, u2.ID, domain.MembershipActive, true)

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	// Each user predicts on the same match.
	if err := predRepo.Create(context.Background(), &domain.Prediction{UserID: u1.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := predRepo.Create(context.Background(), &domain.Prediction{UserID: u2.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	ids, err := predRepo.ListQuinielaIDsByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 quiniela IDs, got %d: %v", len(ids), ids)
	}
}

func TestPredictionRepository_ListQuinielaIDsByMatch_ExcludesUnpaidMembers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)

	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipActive, false) // unpaid

	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	if err := predRepo.Create(context.Background(), &domain.Prediction{UserID: u1.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := predRepo.Create(context.Background(), &domain.Prediction{UserID: u2.ID, MatchID: m.ID}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	ids, err := predRepo.ListQuinielaIDsByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	// Only u1 (paid) counts; still one distinct quiniela_id.
	if len(ids) != 1 {
		t.Errorf("expected 1 quiniela ID (paid member only), got %d: %v", len(ids), ids)
	}
}

func TestPredictionRepository_ListQuinielaIDsByMatch_NoPredictions_ReturnsEmpty(t *testing.T) {
	cleanTables(t)
	m := seedMatch(t)
	predRepo := repository.NewPostgresPredictionRepository(testDB)

	ids, err := predRepo.ListQuinielaIDsByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty slice, got %v", ids)
	}
}

// ── UserRepository.GetStatusCounts ───────────────────────────────────────────

func TestUserRepository_GetStatusCounts_ReturnsCorrectTotals(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	admin := seedUser(t)

	u1 := seedUser(t)
	u2 := seedUser(t)
	if _, err := repo.Ban(context.Background(), u1.ID, admin.ID, "spam"); err != nil {
		t.Fatalf("ban: %v", err)
	}

	counts, err := repo.GetStatusCounts(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.Total < 3 {
		t.Errorf("expected Total ≥ 3, got %d", counts.Total)
	}
	if counts.Banned < 1 {
		t.Errorf("expected Banned ≥ 1, got %d", counts.Banned)
	}
	if counts.Active < 2 {
		t.Errorf("expected Active ≥ 2, got %d", counts.Active)
	}
	_ = u2
}

// ── QuinielaRepository.GetStatusCounts ───────────────────────────────────────

func TestQuinielaRepository_GetStatusCounts_ReturnsCorrectTotals(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	quinielaRepo := repository.NewPostgresQuinielaRepository(testDB)

	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	if err := quinielaRepo.DeleteByAdmin(context.Background(), q1.ID, admin.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	counts, err := quinielaRepo.GetStatusCounts(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.Total < 2 {
		t.Errorf("expected Total ≥ 2, got %d", counts.Total)
	}
	if counts.Deleted < 1 {
		t.Errorf("expected Deleted ≥ 1, got %d", counts.Deleted)
	}
	_ = q2
}

// ── QuinielaRepository.BulkDeleteByAdmin ─────────────────────────────────────

func TestQuinielaRepository_BulkDeleteByAdmin_DeletesAllIDs(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)

	succeeded, err := repo.BulkDeleteByAdmin(context.Background(), []int{q1.ID, q2.ID}, admin.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(succeeded))
	}
}

func TestQuinielaRepository_BulkDeleteByAdmin_AlreadyDeletedSkipped(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := seedQuiniela(t, u.ID)
	_, _ = repo.BulkDeleteByAdmin(context.Background(), []int{q.ID}, admin.ID)

	succeeded, err := repo.BulkDeleteByAdmin(context.Background(), []int{q.ID}, admin.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(succeeded) != 0 {
		t.Errorf("expected 0 succeeded for already-deleted ID, got %d", len(succeeded))
	}
}

// ── GroupMembershipRepository.BulkRemoveByAdmin ───────────────────────────────

func TestGroupMembershipRepository_BulkRemoveByAdmin_RemovesAllIDs(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	m1 := seedActiveMembership(t, q.ID, seedUser(t).ID)
	m2 := seedActiveMembership(t, q.ID, seedUser(t).ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	succeeded, err := repo.BulkRemoveByAdmin(context.Background(), []int{m1.ID, m2.ID}, admin.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(succeeded))
	}

	for _, id := range []int{m1.ID, m2.ID} {
		got, _ := repo.GetByID(context.Background(), id)
		if got == nil || got.Status != domain.MembershipLeft {
			t.Errorf("membership %d: expected status left, got %v", id, got)
		}
	}
}

func TestGroupMembershipRepository_BulkRemoveByAdmin_InactiveSkipped(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	m := seedActiveMembership(t, q.ID, seedUser(t).ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	_, _ = repo.BulkRemoveByAdmin(context.Background(), []int{m.ID}, admin.ID)

	succeeded, err := repo.BulkRemoveByAdmin(context.Background(), []int{m.ID}, admin.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(succeeded) != 0 {
		t.Errorf("expected 0 succeeded for already-inactive membership, got %d", len(succeeded))
	}
}

// ── PaymentRecordRepository.GetStatusCounts ───────────────────────────────────

func TestPaymentRecordRepository_GetStatusCounts_IncludesTotalCollected(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	paymentRepo := repository.NewPostgresPaymentRecordRepository(testDB)

	pr1 := seedPaymentRecord(t, q.ID, member.ID) // pending (10 000 centavos)
	pr2 := seedPaymentRecord(t, q.ID, owner.ID)  // will be confirmed

	if _, err := paymentRepo.Validate(context.Background(), pr2.ID, admin.ID, "ok"); err != nil {
		t.Fatalf("validate: %v", err)
	}

	counts, err := paymentRepo.GetStatusCounts(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.Pending < 1 {
		t.Errorf("expected Pending ≥ 1, got %d", counts.Pending)
	}
	if counts.Confirmed < 1 {
		t.Errorf("expected Confirmed ≥ 1, got %d", counts.Confirmed)
	}
	if counts.TotalCollected < 10000 {
		t.Errorf("expected TotalCollected ≥ 10000 (one confirmed at 10000 centavos), got %d", counts.TotalCollected)
	}
	_ = pr1
}

// ── GroupMembershipRepository.TransferOwnershipRoles ─────────────────────────

func TestGroupMembershipRepository_TransferOwnershipRoles_HappyPath_SwapsRoles(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)

	ownerMembership := seedMembership(t, q.ID, owner.ID, domain.MembershipActive, false)
	if err := repo.SetRole(context.Background(), ownerMembership.ID, domain.MembershipRoleCreateOwner); err != nil {
		t.Fatalf("seed owner role: %v", err)
	}
	memberMembership := seedActiveMembership(t, q.ID, member.ID)

	if err := repo.TransferOwnershipRoles(context.Background(), q.ID, memberMembership.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	demoted, err := repo.GetByID(context.Background(), ownerMembership.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if demoted.Role != domain.MembershipRoleMember {
		t.Errorf("expected old owner role=member after transfer, got %q", demoted.Role)
	}

	promoted, err := repo.GetByID(context.Background(), memberMembership.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if promoted.Role != domain.MembershipRoleCreateOwner {
		t.Errorf("expected new owner role=owner after transfer, got %q", promoted.Role)
	}
}

func TestGroupMembershipRepository_TransferOwnershipRoles_InvalidPromotee_RollsBackDemotion(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)

	ownerMembership := seedMembership(t, q.ID, owner.ID, domain.MembershipActive, false)
	if err := repo.SetRole(context.Background(), ownerMembership.ID, domain.MembershipRoleCreateOwner); err != nil {
		t.Fatalf("seed owner role: %v", err)
	}

	// Attempt to promote a membership ID that does not exist — the transaction
	// must roll back and the original owner must retain the owner role.
	if err := repo.TransferOwnershipRoles(context.Background(), q.ID, 999999); err == nil {
		t.Fatal("expected error for non-existent promotee membership, got nil")
	}

	unchanged, err := repo.GetByID(context.Background(), ownerMembership.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if unchanged.Role != domain.MembershipRoleCreateOwner {
		t.Errorf("rollback failed: expected role=owner after aborted transfer, got %q", unchanged.Role)
	}
}

func TestGroupMembershipRepository_TransferOwnershipRoles_CancelledContext_ReturnsError(t *testing.T) {
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.TransferOwnershipRoles(ctx, 1, 1); err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
