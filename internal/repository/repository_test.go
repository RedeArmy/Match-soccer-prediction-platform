package repository_test

import (
	"context"
	"errors"
	"log"
	"os"
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

const (
	dbImage    = "postgres:17-alpine"
	dbName     = "quiniela_test"
	dbUser     = "test"
	dbPassword = "test"
)

// testDB is shared across all tests in this package. It is initialised once in
// TestMain to avoid starting a container per test function.
var testDB *pgxpool.Pool

func TestMain(m *testing.M) {
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
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			log.Printf("terminate postgres container: %v", err)
		}
	}()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	testDB, err = database.NewPool(ctx, database.Config{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		log.Fatalf("new pool: %v", err)
	}
	defer testDB.Close()

	os.Exit(m.Run())
}

// cleanTables truncates every table in reverse foreign-key order so each test
// starts with an empty database. RESTART IDENTITY resets serial sequences.
func cleanTables(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(context.Background(),
		`TRUNCATE tiebreakers, predictions, quinielas, matches, users RESTART IDENTITY CASCADE`)
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
	u := &domain.User{Name: "Alice", Email: "alice@example.com", PasswordHash: "hash", Role: domain.RolePlayer}
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
		KickoffAt: time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond),
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed match: %v", err)
	}
	return m
}

func seedQuiniela(t *testing.T, ownerID int) *domain.Quiniela {
	t.Helper()
	repo := repository.NewPostgresQuinielaRepository(testDB)
	q := &domain.Quiniela{Name: "Oficina 2026", OwnerID: ownerID}
	if err := repo.Create(context.Background(), q); err != nil {
		t.Fatalf("seed quiniela: %v", err)
	}
	return q
}

// ── UserRepository ────────────────────────────────────────────────────────────

func TestUserRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	u := &domain.User{Name: "Bob", Email: "bob@example.com", PasswordHash: "h", Role: domain.RolePlayer}

	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID == 0 {
		t.Error("expected non-zero ID after create")
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
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing user, got %+v", got)
	}
}

func TestUserRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	u.Name = "Alice Updated"
	if err := repo.Update(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestUserRepository_Delete_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	if err := repo.Delete(context.Background(), u.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestUserRepository_List_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	users, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		KickoffAt: time.Now().Add(48 * time.Hour).UTC(),
	}

	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
}

func TestMatchRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	created := seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
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
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestMatchRepository_List_ReturnsAll(t *testing.T) {
	cleanTables(t)
	seedMatch(t)
	repo := repository.NewPostgresMatchRepository(testDB)

	matches, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
	}
	if len(live) != 1 {
		t.Errorf("expected 1 live match, got %d", len(live))
	}

	scheduled, err := repo.ListByStatus(context.Background(), domain.MatchStatusScheduled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scheduled) != 0 {
		t.Errorf("expected 0 scheduled matches, got %d", len(scheduled))
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
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
}

func TestPredictionRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 0}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("create: %v", err)
	}

	pts := 5
	p.Points = &pts
	if err := repo.Update(context.Background(), p); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestPredictionRepository_GetByUserAndMatch_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 3, AwayScore: 2}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByUserAndMatch(context.Background(), u.ID, m.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestPredictionRepository_ListByUser_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	m := seedMatch(t)
	repo := repository.NewPostgresPredictionRepository(testDB)
	p := &domain.Prediction{UserID: u.ID, MatchID: m.ID, HomeScore: 1, AwayScore: 1}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("create: %v", err)
	}

	preds, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("create: %v", err)
	}

	preds, err := repo.ListByMatch(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preds) != 1 {
		t.Errorf("expected 1 prediction, got %d", len(preds))
	}
}

// ── QuinielaRepository ────────────────────────────────────────────────────────

func TestQuinielaRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := &domain.Quiniela{Name: "Test Pool", OwnerID: u.ID}
	if err := repo.Create(context.Background(), q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
}

func TestQuinielaRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
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
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestQuinielaRepository_Delete_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.Delete(context.Background(), q.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestQuinielaRepository_ListByOwner_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	quinielas, err := repo.ListByOwner(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
	}
	if tb.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
}

func TestTiebreakerRepository_GetByUserAndQuiniela_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, QuinielaID: q.ID, Prediction: 10}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByUserAndQuiniela(context.Background(), u.ID, q.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestTiebreakerRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, QuinielaID: q.ID, Prediction: 7}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf("create: %v", err)
	}

	result := 9
	tb.Result = &result
	if err := repo.Update(context.Background(), tb); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestTiebreakerRepository_ListByQuiniela_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, QuinielaID: q.ID, Prediction: 3}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf("create: %v", err)
	}

	tbs, err := repo.ListByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tbs) != 1 {
		t.Errorf("expected 1 tiebreaker, got %d", len(tbs))
	}
}
