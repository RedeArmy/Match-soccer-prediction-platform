package outbox_test

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/migrations"
)

// testPool is shared across all tests in this package. Initialised once in
// TestMain so we only pay the container startup cost once.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	var cleanup func()
	testPool, cleanup = mustSetupDB()
	defer cleanup()
	os.Exit(m.Run())
}

func mustSetupDB() (*pgxpool.Pool, func()) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("outbox_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
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
		log.Fatalf("open pool: %v", err)
	}

	cleanup := func() {
		pool.Close()
		if err := container.Terminate(ctx); err != nil {
			log.Printf("terminate postgres container: %v", err)
		}
	}
	return pool, cleanup
}

// TestWriter_Write verifies that Write inserts a pending row into domain_outbox.
func TestWriter_Write(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	w := outbox.NewWriter(testPool)
	payload := notification.BankTransferPayload{
		UserID:      7,
		ProofID:     42,
		AmountCents: 25000,
		Currency:    "GTQ",
	}

	if err := w.Write(ctx,
		notification.EventAdminBankTransferPending,
		"bank_transfer_proof", "writer_write_42",
		payload,
	); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var count int
	err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM domain_outbox
		  WHERE event_type     = $1
		    AND aggregate_type = 'bank_transfer_proof'
		    AND aggregate_id   = 'writer_write_42'
		    AND status         = 'pending'`,
		string(notification.EventAdminBankTransferPending),
	).Scan(&count)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one outbox row; got 0")
	}
}

// TestWriter_WriteInTx verifies that WriteInTx participates in the caller's
// transaction: the row disappears when the transaction is rolled back.
func TestWriter_WriteInTx(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	w := outbox.NewWriter(testPool)
	payload := notification.WithdrawalPayload{
		UserID:      99,
		RequestID:   555,
		AmountCents: 10000,
		Currency:    "GTQ",
	}

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	if err := w.WriteInTx(ctx, tx,
		notification.EventAdminWithdrawalPending,
		"withdrawal_request", "writetx_555",
		payload,
	); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("WriteInTx: %v", err)
	}

	// Within the transaction the row must be visible.
	var countInTx int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM domain_outbox
		  WHERE event_type   = $1
		    AND aggregate_id = 'writetx_555'`,
		string(notification.EventAdminWithdrawalPending),
	).Scan(&countInTx); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("in-tx count: %v", err)
	}
	if countInTx == 0 {
		_ = tx.Rollback(ctx)
		t.Fatal("row not visible within the transaction")
	}

	// After rollback the row must not exist.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var countAfter int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM domain_outbox
		  WHERE event_type   = $1
		    AND aggregate_id = 'writetx_555'`,
		string(notification.EventAdminWithdrawalPending),
	).Scan(&countAfter); err != nil {
		t.Fatalf("post-rollback count: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("expected 0 rows after rollback; got %d", countAfter)
	}
}

// TestWriter_InvalidPayload verifies that a non-marshallable payload returns
// an error before touching the database.
func TestWriter_InvalidPayload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	w := outbox.NewWriter(testPool)
	// json.Marshal rejects channels.
	err := w.Write(ctx,
		notification.EventSystemCircuitBreakerOpened,
		"system", "invalid_payload",
		make(chan int),
	)
	if err == nil {
		t.Fatal("expected error for non-marshallable payload; got nil")
	}
}

// TestWriter_PayloadRoundtrip verifies that the JSONB stored in the database
// round-trips cleanly into the original payload struct.
func TestWriter_PayloadRoundtrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	w := outbox.NewWriter(testPool)
	original := notification.SystemAlertPayload{
		Component: "circuit_breaker",
		Detail:    "PayPal cert fetcher opened after 3 failures",
		Severity:  "critical",
	}

	if err := w.Write(ctx,
		notification.EventSystemCircuitBreakerOpened,
		"system", "roundtrip_paypal_cert",
		original,
	); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var rawPayload json.RawMessage
	if err := testPool.QueryRow(ctx,
		`SELECT payload FROM domain_outbox
		  WHERE event_type   = $1
		    AND aggregate_id = 'roundtrip_paypal_cert'
		  ORDER BY created_at DESC LIMIT 1`,
		string(notification.EventSystemCircuitBreakerOpened),
	).Scan(&rawPayload); err != nil {
		t.Fatalf("fetch payload: %v", err)
	}

	var decoded notification.SystemAlertPayload
	if err := json.Unmarshal(rawPayload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded.Component != original.Component {
		t.Errorf("Component: got %q; want %q", decoded.Component, original.Component)
	}
	if decoded.Detail != original.Detail {
		t.Errorf("Detail: got %q; want %q", decoded.Detail, original.Detail)
	}
}
