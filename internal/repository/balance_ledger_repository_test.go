package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// seedUserWithBalance creates a user and sets their balance_cents directly.
func seedUserWithBalance(t *testing.T, balanceCents int) *domain.User {
	t.Helper()
	u := seedUser(t)
	_, err := testDB.Exec(context.Background(),
		`UPDATE users SET balance_cents = $1 WHERE id = $2`, balanceCents, u.ID)
	if err != nil {
		t.Fatalf("seedUserWithBalance: %v", err)
	}
	u.BalanceCents = balanceCents
	return u
}

func TestBalanceLedgerRepository_Credit_IncreasesBalance(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 0)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	if err := repo.Credit(context.Background(), u.ID, 5000, domain.LedgerKindBankTransfer, 0, "", 0); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 5000 {
		t.Errorf("balance: got %d, want 5000", bal)
	}
}

func TestBalanceLedgerRepository_Credit_CreatesLedgerEntry(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	if err := repo.Credit(context.Background(), u.ID, 3000, domain.LedgerKindBankTransfer, 42, "bank_transfer_proof", admin.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	entries, err := repo.ListByUser(context.Background(), u.ID, repository.Unbounded())
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].DeltaCents != 3000 {
		t.Errorf("delta_cents: got %d, want 3000", entries[0].DeltaCents)
	}
	if entries[0].Kind != domain.LedgerKindBankTransfer {
		t.Errorf("kind: got %q, want %q", entries[0].Kind, domain.LedgerKindBankTransfer)
	}
}

func TestBalanceLedgerRepository_Credit_UserNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	err := repo.Credit(context.Background(), 999999, 1000, domain.LedgerKindBankTransfer, 0, "", 0)
	if !isNotFound(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestBalanceLedgerRepository_Debit_DecreasesBalance(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 10000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	if err := repo.Debit(context.Background(), u.ID, 3000, domain.LedgerKindEntryFee, 0, "", 0); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 7000 {
		t.Errorf("balance: got %d, want 7000", bal)
	}
}

func TestBalanceLedgerRepository_Debit_InsufficientBalance(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 1000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	err := repo.Debit(context.Background(), u.ID, 5000, domain.LedgerKindEntryFee, 0, "", 0)
	if err == nil {
		t.Fatal("expected conflict error for insufficient balance, got nil")
	}
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error, got %v", err)
	}
}

func TestBalanceLedgerRepository_Debit_UserNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	err := repo.Debit(context.Background(), 999999, 100, domain.LedgerKindEntryFee, 0, "", 0)
	if !isNotFound(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestBalanceLedgerRepository_Reserve_MovesToReserved(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 8000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	if err := repo.Reserve(context.Background(), u.ID, 3000, 1, "withdrawal_request", 0); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, reserved, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 8000 {
		t.Errorf("balance_cents unchanged: got %d, want 8000", bal)
	}
	if reserved != 3000 {
		t.Errorf("reserved_cents: got %d, want 3000", reserved)
	}
}

func TestBalanceLedgerRepository_Reserve_InsufficientBalance(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 1000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	err := repo.Reserve(context.Background(), u.ID, 5000, 1, "withdrawal_request", 0)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict, got %v", err)
	}
}

func TestBalanceLedgerRepository_ReleaseReservation_FreesReserved(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 8000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	// First reserve, then release.
	if err := repo.Reserve(context.Background(), u.ID, 3000, 1, "withdrawal_request", 0); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := repo.ReleaseReservation(context.Background(), u.ID, 3000, 1, "withdrawal_request", 0); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	_, reserved, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if reserved != 0 {
		t.Errorf("reserved_cents after release: got %d, want 0", reserved)
	}
}

func TestBalanceLedgerRepository_CommitReservation_DeductsBalance(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 8000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	if err := repo.Reserve(context.Background(), u.ID, 3000, 1, "withdrawal_request", 0); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := repo.CommitReservation(context.Background(), u.ID, 3000, 1, "withdrawal_request", 0); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, reserved, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 5000 {
		t.Errorf("balance_cents after commit: got %d, want 5000", bal)
	}
	if reserved != 0 {
		t.Errorf("reserved_cents after commit: got %d, want 0", reserved)
	}
}

func TestBalanceLedgerRepository_CommitReservation_InsufficientReserved(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 8000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	err := repo.CommitReservation(context.Background(), u.ID, 5000, 1, "withdrawal_request", 0)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict for no reserved balance, got %v", err)
	}
}

func TestBalanceLedgerRepository_ListByUser_OrderedDescByCreatedAt(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 20000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	_ = repo.Credit(context.Background(), u.ID, 1000, domain.LedgerKindBankTransfer, 0, "", 0)
	_ = repo.Credit(context.Background(), u.ID, 2000, domain.LedgerKindBankTransfer, 0, "", 0)
	_ = repo.Credit(context.Background(), u.ID, 3000, domain.LedgerKindBankTransfer, 0, "", 0)

	entries, err := repo.ListByUser(context.Background(), u.ID, repository.Unbounded())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Most recent first
	if entries[0].DeltaCents != 3000 {
		t.Errorf("first entry delta: got %d, want 3000 (most recent)", entries[0].DeltaCents)
	}
}

func TestBalanceLedgerRepository_ListByUser_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	entries, err := repo.ListByUser(context.Background(), u.ID, repository.Unbounded())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ── CreditIdempotent ──────────────────────────────────────────────────────────

func TestBalanceLedgerRepository_CreditIdempotent_HappyPath(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	credited, err := repo.CreditIdempotent(context.Background(), u.ID, 3000, domain.LedgerKindWebhookRecurrente, "REF-IDEM-001", "", 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !credited {
		t.Error("expected credited=true for first call, got false")
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 3000 {
		t.Errorf("balance: got %d, want 3000", bal)
	}
}

func TestBalanceLedgerRepository_CreditIdempotent_DuplicateIsNoop(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	if _, err := repo.CreditIdempotent(context.Background(), u.ID, 2000, domain.LedgerKindWebhookRecurrente, "REF-IDEM-DUP", "", 0); err != nil {
		t.Fatalf("first call: %v", err)
	}

	credited, err := repo.CreditIdempotent(context.Background(), u.ID, 2000, domain.LedgerKindWebhookRecurrente, "REF-IDEM-DUP", "", 0)
	if err != nil {
		t.Fatalf("duplicate call: %v", err)
	}
	if credited {
		t.Error("expected credited=false for duplicate reference, got true")
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 2000 {
		t.Errorf("balance: got %d, want 2000 (must not double-credit)", bal)
	}
}

func TestBalanceLedgerRepository_CreditIdempotent_NDeliveriesProduceOneLedgerRow(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	for i := 0; i < 5; i++ {
		if _, err := repo.CreditIdempotent(context.Background(), u.ID, 1500, domain.LedgerKindWebhookRecurrente, "REF-SINGLE", "", 0); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	entries, err := repo.ListByUser(context.Background(), u.ID, repository.Unbounded())
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 ledger entry for 5 deliveries, got %d", len(entries))
	}
}

func TestBalanceLedgerRepository_CreditIdempotent_EmptyReferenceReturnsValidation(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	_, err := repo.CreditIdempotent(context.Background(), u.ID, 1000, domain.LedgerKindWebhookRecurrente, "", "", 0)
	if err == nil {
		t.Fatal("expected validation error for empty reference, got nil")
	}
}

func TestBalanceLedgerRepository_CreditIdempotent_UserNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	_, err := repo.CreditIdempotent(context.Background(), 999999, 1000, domain.LedgerKindWebhookRecurrente, "REF-GHOST", "", 0)
	if !isNotFound(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// TestBalanceLedgerRepository_ConcurrentCreditAndDebit verifies that N
// concurrent Credit and Debit calls on the same user produce a correct final
// balance and one ledger row per call, with no duplicate or lost rows.
//
// Each goroutine either credits 1000 or debits 1000.  Starting balance is set
// high enough (N*1000) so no Debit fails with insufficient-funds.
// ── SumTransactionsByUserAndPeriod ────────────────────────────────────────────
//
// These tests exercise the AML velocity-check SQL directly against PostgreSQL.
// The KYC gate (kyc_gate.go) relies on this function for every deposit,
// withdrawal, and KYC review — a bug in the query would silently bypass all
// rolling-window velocity limits.

// TestBalanceLedgerRepository_SumTransactionsByUserAndPeriod_FiltersKind verifies
// that only ledger rows whose kind matches one of the requested kinds are
// included in the sum. Rows of other kinds within the same time window must
// not contribute to the result.
func TestBalanceLedgerRepository_SumTransactionsByUserAndPeriod_FiltersKind(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUserWithBalance(t, 10_000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	// Credit 3 000 via bank_transfer and 2 000 via prize — both are deposit-kind.
	if err := repo.Credit(ctx, u.ID, 3_000, domain.LedgerKindBankTransfer, 0, "", 0); err != nil {
		t.Fatalf("credit bank_transfer: %v", err)
	}
	if err := repo.Credit(ctx, u.ID, 2_000, domain.LedgerKindPrize, 0, "", 0); err != nil {
		t.Fatalf("credit prize: %v", err)
	}
	// Debit 1 000 via entry_fee — must be excluded from the deposit-kinds query.
	if err := repo.Debit(ctx, u.ID, 1_000, domain.LedgerKindEntryFee, 0, "", 0); err != nil {
		t.Fatalf("debit entry_fee: %v", err)
	}

	sum, err := repo.SumTransactionsByUserAndPeriod(ctx, u.ID,
		[]domain.BalanceLedgerKind{domain.LedgerKindBankTransfer, domain.LedgerKindPrize},
		time.Now().Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	// Only bank_transfer (3 000) + prize (2 000) should be counted.
	const want int64 = 5_000
	if sum != want {
		t.Errorf("sum = %d, want %d (entry_fee row must be excluded)", sum, want)
	}
}

// TestBalanceLedgerRepository_SumTransactionsByUserAndPeriod_EnforcesTimeWindow
// verifies that rows created before the `since` threshold are excluded even
// when their kind matches. Only rows within the rolling window must be counted.
func TestBalanceLedgerRepository_SumTransactionsByUserAndPeriod_EnforcesTimeWindow(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUserWithBalance(t, 0)

	// Insert a row with a backdated created_at that falls outside the 24-hour
	// window. Inserted directly to control the timestamp; no balance_cents
	// consistency required for this read-only aggregate query.
	if _, err := testDB.Exec(ctx,
		`INSERT INTO balance_ledger (user_id, delta_cents, kind, balance_after, created_at)
		 VALUES ($1, 9000, 'bank_transfer', 9000, $2)`,
		u.ID, time.Now().Add(-25*time.Hour),
	); err != nil {
		t.Fatalf("insert old ledger row: %v", err)
	}

	// Insert a recent row within the window.
	if _, err := testDB.Exec(ctx,
		`INSERT INTO balance_ledger (user_id, delta_cents, kind, balance_after, created_at)
		 VALUES ($1, 4000, 'bank_transfer', 13000, $2)`,
		u.ID, time.Now().Add(-30*time.Minute),
	); err != nil {
		t.Fatalf("insert recent ledger row: %v", err)
	}

	sum, err := repository.NewPostgresBalanceLedgerRepository(testDB).
		SumTransactionsByUserAndPeriod(ctx, u.ID,
			[]domain.BalanceLedgerKind{domain.LedgerKindBankTransfer},
			time.Now().Add(-24*time.Hour),
		)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	// Only the recent row (4 000) should be summed; the 25-hour-old row is outside
	// the window.
	const want int64 = 4_000
	if sum != want {
		t.Errorf("sum = %d, want %d (old row must be excluded by time filter)", sum, want)
	}
}

// TestBalanceLedgerRepository_SumTransactionsByUserAndPeriod_ReturnsZeroForNoRows
// verifies that the function returns 0 when no rows match (empty table, wrong
// kind, or time window before all data). This guards against NULL propagation
// from SUM() that would bypass velocity checks by making comparisons undefined.
func TestBalanceLedgerRepository_SumTransactionsByUserAndPeriod_ReturnsZeroForNoRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	sum, err := repo.SumTransactionsByUserAndPeriod(ctx, u.ID,
		[]domain.BalanceLedgerKind{domain.LedgerKindBankTransfer},
		time.Now().Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if sum != 0 {
		t.Errorf("sum = %d, want 0 for empty table", sum)
	}
}

// TestBalanceLedgerRepository_SumTransactionsByUserAndPeriod_IsolatesUser verifies
// that transactions belonging to other users are never included in the sum,
// even when their kind and timestamp match the query parameters.
func TestBalanceLedgerRepository_SumTransactionsByUserAndPeriod_IsolatesUser(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	userA := seedUserWithBalance(t, 10_000)
	userB := seedUserWithBalance(t, 10_000)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	// Both users receive a bank_transfer credit.
	if err := repo.Credit(ctx, userA.ID, 5_000, domain.LedgerKindBankTransfer, 0, "", 0); err != nil {
		t.Fatalf("credit userA: %v", err)
	}
	if err := repo.Credit(ctx, userB.ID, 7_000, domain.LedgerKindBankTransfer, 0, "", 0); err != nil {
		t.Fatalf("credit userB: %v", err)
	}

	// Query only for userA — userB's 7 000 must not appear in the result.
	sum, err := repo.SumTransactionsByUserAndPeriod(ctx, userA.ID,
		[]domain.BalanceLedgerKind{domain.LedgerKindBankTransfer},
		time.Now().Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	const want int64 = 5_000
	if sum != want {
		t.Errorf("sum = %d, want %d (userB row must be excluded)", sum, want)
	}
}

func TestBalanceLedgerRepository_ConcurrentCreditAndDebit(t *testing.T) {
	if testDB == nil {
		t.Skip("integration test: no test database available")
	}
	cleanTables(t)

	const workers = 10
	const amountCents = 1_000
	startBalance := workers * amountCents // ensure debits never underflow

	u := seedUserWithBalance(t, startBalance)
	repo := repository.NewPostgresBalanceLedgerRepository(testDB)

	var wg sync.WaitGroup
	wg.Add(workers * 2) // workers credits + workers debits

	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			if err := repo.Credit(context.Background(), u.ID, amountCents, domain.LedgerKindBankTransfer, int64(i), "concurrent_credit", 0); err != nil {
				t.Errorf("Credit[%d]: %v", i, err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			if err := repo.Debit(context.Background(), u.ID, amountCents, domain.LedgerKindEntryFee, int64(i), "concurrent_debit", 0); err != nil {
				t.Errorf("Debit[%d]: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	userRepo := repository.NewPostgresUserRepository(testDB)
	finalBalance, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	// N credits and N debits of equal amount cancel out; balance equals startBalance.
	if finalBalance != startBalance {
		t.Errorf("final balance: got %d, want %d (started %d, %d credits, %d debits each %d cents)",
			finalBalance, startBalance, startBalance, workers, workers, amountCents)
	}

	entries, err := repo.ListByUser(context.Background(), u.ID, repository.Unbounded())
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	wantRows := workers * 2
	if len(entries) != wantRows {
		t.Errorf("ledger rows: got %d, want %d (no duplicates, no missing)", len(entries), wantRows)
	}
}
