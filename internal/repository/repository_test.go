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
		`TRUNCATE group_memberships, tiebreakers, predictions, quinielas, matches, stadiums, users RESTART IDENTITY CASCADE`)
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
	u := &domain.User{Name: "User " + code, Email: code + "@example.com", Role: domain.RolePlayer}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func seedMatch(t *testing.T) *domain.Match {
	t.Helper()
	repo := repository.NewPostgresMatchRepository(testDB)
	m := &domain.Match{
		HomeTeam:  "Brazil",
		AwayTeam:  "Argentina",
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
		HomeTeam:  "Brazil",
		AwayTeam:  "Argentina",
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
	u := &domain.User{Name: "Bob", Email: "bob@example.com", Role: domain.RolePlayer}

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
	ghost := &domain.User{ID: 99999, Name: "Ghost", Email: "g@g.com", Role: domain.RolePlayer}

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
		HomeTeam:  "Brazil",
		AwayTeam:  "Argentina",
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
		t.Errorf("expected 1 prediction, got %d", len(preds))
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
		t.Errorf("expected 1 prediction, got %d", len(preds))
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
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	tb := &domain.Tiebreaker{UserID: u.ID, QuinielaID: q.ID, Prediction: 42}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if tb.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestTiebreakerRepository_GetByUserAndQuiniela_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, QuinielaID: q.ID, Prediction: 10}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	got, err := repo.GetByUserAndQuiniela(context.Background(), u.ID, q.ID)
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

func TestTiebreakerRepository_GetByUserAndQuiniela_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	got, err := repo.GetByUserAndQuiniela(context.Background(), 99999, 99999)
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
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, QuinielaID: q.ID, Prediction: 7}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	result := 9
	tb.Result = &result
	if err := repo.Update(context.Background(), tb); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if tb.Result == nil || *tb.Result != 9 {
		t.Errorf("result not updated: got %v", tb.Result)
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

func TestTiebreakerRepository_ListByQuiniela_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, QuinielaID: q.ID, Prediction: 3}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	tbs, err := repo.ListByQuiniela(context.Background(), q.ID)
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
		t.Errorf("status: got %q, want active", got.Status)
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
		t.Errorf("status: got %q, want active", got.Status)
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
		t.Errorf("status: got %q, want active", got.Status)
	}
}

func TestQuinielaRepository_UpdateStatus_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.UpdateStatus(context.Background(), 99999, domain.QuinielaStatusActive); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
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
		t.Fatalf("expected 1 prediction, got %d", len(preds))
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
