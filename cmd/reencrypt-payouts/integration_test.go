//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/testutil"
	"github.com/rede/world-cup-quiniela/migrations"
	"github.com/rede/world-cup-quiniela/pkg/payoutenc"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testutil.SetupPostgres(t)
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.NewPool(ctx, database.Config{
		DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedUserForTest(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (name, email) VALUES ('Test User', $1) RETURNING id`,
		"reencrypt-test@example.com",
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedLegacyWithdrawal(t *testing.T, pool *pgxpool.Pool, userID int, payoutDetails string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO withdrawal_requests (user_id, amount_cents, currency, method, payout_details, gtq_reserved_cents)
		 VALUES ($1, 10000, 'GTQ', 'bank_gt', $2, 0)
		 RETURNING id`,
		userID, payoutDetails,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed withdrawal request: %v", err)
	}
	return id
}

func TestRun_MigratesLegacyPlaintextRow(t *testing.T) {
	pool := setupTestDB(t)
	userID := seedUserForTest(t, pool)
	legacy := `{"account_number":"1234567890","bank_name":"Banrural"}`
	id := seedLegacyWithdrawal(t, pool, userID, legacy)

	enc, err := payoutenc.NewAESGCM(testKeyHex)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}

	res, err := run(context.Background(), pool, enc, false)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.migrated != 1 || res.failed != 0 {
		t.Fatalf("result: got %+v, want migrated=1 failed=0", res)
	}

	var stored string
	if err := pool.QueryRow(context.Background(),
		`SELECT payout_details::text FROM withdrawal_requests WHERE id = $1`, id,
	).Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored == legacy {
		t.Fatal("payout_details was not modified — remediation did not run")
	}

	decoded, err := payoutenc.Unmarshal(enc, []byte(stored))
	if err != nil {
		t.Fatalf("Unmarshal stored value: %v", err)
	}
	if decoded["account_number"] != "1234567890" || decoded["bank_name"] != "Banrural" {
		t.Errorf("decoded fields do not match original: %v", decoded)
	}
}

func TestRun_DryRun_DoesNotModifyRows(t *testing.T) {
	pool := setupTestDB(t)
	userID := seedUserForTest(t, pool)
	legacy := `{"account_number":"9999999999","bank_name":"BAM"}`
	id := seedLegacyWithdrawal(t, pool, userID, legacy)

	enc, err := payoutenc.NewAESGCM(testKeyHex)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}

	res, err := run(context.Background(), pool, enc, true)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.migrated != 1 {
		t.Fatalf("dry-run should still report the row as migratable, got %+v", res)
	}

	var stored string
	if err := pool.QueryRow(context.Background(),
		`SELECT payout_details::text FROM withdrawal_requests WHERE id = $1`, id,
	).Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored != legacy {
		t.Errorf("dry-run must not modify the row; got %q, want unchanged %q", stored, legacy)
	}
}

func TestRun_AlreadyEncryptedRow_IsSkippedBySQLFilter(t *testing.T) {
	pool := setupTestDB(t)
	userID := seedUserForTest(t, pool)

	enc, err := payoutenc.NewAESGCM(testKeyHex)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	encrypted, err := payoutenc.Marshal(enc, map[string]string{"account_number": "555"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	id := seedLegacyWithdrawal(t, pool, userID, string(encrypted))

	res, err := run(context.Background(), pool, enc, false)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.migrated != 0 {
		t.Errorf("already-encrypted row must not be touched, got migrated=%d", res.migrated)
	}

	var stored string
	if err := pool.QueryRow(context.Background(),
		`SELECT payout_details::text FROM withdrawal_requests WHERE id = $1`, id,
	).Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored != string(encrypted) {
		t.Error("already-encrypted row must remain byte-for-byte unchanged")
	}
}

func TestRun_NoLegacyRows_ReturnsZeroResult(t *testing.T) {
	pool := setupTestDB(t)
	enc, err := payoutenc.NewAESGCM(testKeyHex)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	res, err := run(context.Background(), pool, enc, false)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.migrated != 0 || res.failed != 0 {
		t.Errorf("expected zero result on empty table, got %+v", res)
	}
}
