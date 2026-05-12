package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresBalanceLedgerRepository is the PostgreSQL-backed implementation of
// BalanceLedgerRepository.
type PostgresBalanceLedgerRepository struct {
	db *pgxpool.Pool
}

// NewPostgresBalanceLedgerRepository constructs a PostgresBalanceLedgerRepository.
func NewPostgresBalanceLedgerRepository(db *pgxpool.Pool) *PostgresBalanceLedgerRepository {
	return &PostgresBalanceLedgerRepository{db: db}
}

// Credit adds deltaCents to balance_cents and inserts a ledger row atomically.
func (r *PostgresBalanceLedgerRepository) Credit(ctx context.Context, userID, deltaCents int, kind domain.BalanceLedgerKind, refID int64, refType string, creatorID int) error {
	return withTx(ctx, r.db, "BalanceLedgerRepository.Credit", func(tx pgx.Tx) error {
		var balanceAfter int
		err := tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents = balance_cents + $2,
			       updated_at    = NOW()
			 WHERE id = $1 AND deleted_at IS NULL
			 RETURNING balance_cents
		`, userID, deltaCents).Scan(&balanceAfter)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("user not found")
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		return insertLedgerTx(ctx, tx, ledgerRow{UserID: userID, DeltaCents: deltaCents, Kind: kind, BalanceAfter: balanceAfter, RefID: refID, RefType: refType, CreatorID: creatorID})
	})
}

// Debit subtracts deltaCents from the available balance
// (balance_cents - reserved_cents). Returns Conflict when insufficient.
func (r *PostgresBalanceLedgerRepository) Debit(ctx context.Context, userID, deltaCents int, kind domain.BalanceLedgerKind, refID int64, refType string, creatorID int) error {
	return withTx(ctx, r.db, "BalanceLedgerRepository.Debit", func(tx pgx.Tx) error {
		var balanceAfter int
		err := tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents = balance_cents - $2,
			       updated_at    = NOW()
			 WHERE id = $1
			   AND deleted_at IS NULL
			   AND (balance_cents - reserved_cents) >= $2
			 RETURNING balance_cents
		`, userID, deltaCents).Scan(&balanceAfter)
		if errors.Is(err, pgx.ErrNoRows) {
			return insufficientOrNotFound(ctx, tx, userID)
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		return insertLedgerTx(ctx, tx, ledgerRow{UserID: userID, DeltaCents: -deltaCents, Kind: kind, BalanceAfter: balanceAfter, RefID: refID, RefType: refType, CreatorID: creatorID})
	})
}

// Reserve moves amountCents from available to reserved_cents.
func (r *PostgresBalanceLedgerRepository) Reserve(ctx context.Context, userID, amountCents int, refID int64, refType string, creatorID int) error {
	return withTx(ctx, r.db, "BalanceLedgerRepository.Reserve", func(tx pgx.Tx) error {
		var balanceAfter int
		err := tx.QueryRow(ctx, `
			UPDATE users
			   SET reserved_cents = reserved_cents + $2,
			       updated_at     = NOW()
			 WHERE id = $1
			   AND deleted_at IS NULL
			   AND (balance_cents - reserved_cents) >= $2
			 RETURNING balance_cents
		`, userID, amountCents).Scan(&balanceAfter)
		if errors.Is(err, pgx.ErrNoRows) {
			return insufficientOrNotFound(ctx, tx, userID)
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		return insertLedgerTx(ctx, tx, ledgerRow{UserID: userID, DeltaCents: -amountCents, Kind: domain.LedgerKindWithdrawalReserve, BalanceAfter: balanceAfter, RefID: refID, RefType: refType, CreatorID: creatorID})
	})
}

// ReleaseReservation decrements reserved_cents, returning the hold to available.
func (r *PostgresBalanceLedgerRepository) ReleaseReservation(ctx context.Context, userID, amountCents int, refID int64, refType string, creatorID int) error {
	return withTx(ctx, r.db, "BalanceLedgerRepository.ReleaseReservation", func(tx pgx.Tx) error {
		var balanceAfter int
		err := tx.QueryRow(ctx, `
			UPDATE users
			   SET reserved_cents = reserved_cents - $2,
			       updated_at     = NOW()
			 WHERE id = $1
			   AND deleted_at IS NULL
			   AND reserved_cents >= $2
			 RETURNING balance_cents
		`, userID, amountCents).Scan(&balanceAfter)
		if errors.Is(err, pgx.ErrNoRows) {
			return insufficientOrNotFound(ctx, tx, userID)
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		return insertLedgerTx(ctx, tx, ledgerRow{UserID: userID, DeltaCents: amountCents, Kind: domain.LedgerKindWithdrawalRelease, BalanceAfter: balanceAfter, RefID: refID, RefType: refType, CreatorID: creatorID})
	})
}

// CommitReservation permanently deducts both balance_cents and reserved_cents.
func (r *PostgresBalanceLedgerRepository) CommitReservation(ctx context.Context, userID, amountCents int, refID int64, refType string, creatorID int) error {
	return withTx(ctx, r.db, "BalanceLedgerRepository.CommitReservation", func(tx pgx.Tx) error {
		var balanceAfter int
		err := tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents  = balance_cents  - $2,
			       reserved_cents = reserved_cents - $2,
			       updated_at     = NOW()
			 WHERE id = $1
			   AND deleted_at IS NULL
			   AND reserved_cents >= $2
			 RETURNING balance_cents
		`, userID, amountCents).Scan(&balanceAfter)
		if errors.Is(err, pgx.ErrNoRows) {
			return insufficientOrNotFound(ctx, tx, userID)
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		return insertLedgerTx(ctx, tx, ledgerRow{UserID: userID, DeltaCents: -amountCents, Kind: domain.LedgerKindWithdrawalDeduct, BalanceAfter: balanceAfter, RefID: refID, RefType: refType, CreatorID: creatorID})
	})
}

// ListByUser returns ledger entries for userID ordered by created_at DESC.
func (r *PostgresBalanceLedgerRepository) ListByUser(ctx context.Context, userID int, p Pagination) ([]*domain.BalanceLedger, error) {
	q := `SELECT id, user_id, delta_cents, kind, balance_after, ref_id, ref_type, created_by, created_at
		  FROM balance_ledger WHERE user_id = $1 ORDER BY created_at DESC`
	q, args, _ := applyPagination(q, []any{userID}, 2, p)
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, scanBalanceLedger)
}

// ledgerRow holds the payload for a single balance_ledger INSERT.
// Grouping these fields avoids exceeding the per-function parameter limit and
// makes call sites self-documenting via named fields.
type ledgerRow struct {
	UserID       int
	DeltaCents   int
	Kind         domain.BalanceLedgerKind
	BalanceAfter int
	RefID        int64  // 0 → stored as NULL
	RefType      string // "" → stored as NULL
	CreatorID    int    // 0 → stored as NULL (system/webhook origin)
}

// insertLedgerTx inserts one immutable balance_ledger row inside tx.
func insertLedgerTx(ctx context.Context, tx pgx.Tx, e ledgerRow) error {
	var creator *int
	if e.CreatorID != 0 {
		creator = &e.CreatorID
	}
	var rid *int64
	if e.RefID != 0 {
		rid = &e.RefID
	}
	var rtype *string
	if e.RefType != "" {
		rtype = &e.RefType
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO balance_ledger
		      (user_id, delta_cents, kind, balance_after, ref_id, ref_type, created_by)
		VALUES ($1,     $2,          $3,   $4,            $5,     $6,       $7)
	`, e.UserID, e.DeltaCents, e.Kind, e.BalanceAfter, rid, rtype, creator)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// insufficientOrNotFound checks whether the UPDATE matched 0 rows because the
// user does not exist (NotFound) or because the balance condition was not met
// (Conflict). Called inside a transaction after a conditional UPDATE returns
// ErrNoRows.
func insufficientOrNotFound(ctx context.Context, tx pgx.Tx, userID int) error {
	var exists bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NULL)`,
		userID,
	).Scan(&exists)
	if err != nil {
		return apperrors.Internal(err)
	}
	if !exists {
		return apperrors.NotFound("user not found")
	}
	return apperrors.Conflict("insufficient available balance")
}

func scanBalanceLedger(rows pgx.Rows) (*domain.BalanceLedger, error) {
	l := &domain.BalanceLedger{}
	if err := rows.Scan(
		&l.ID, &l.UserID, &l.DeltaCents, &l.Kind, &l.BalanceAfter,
		&l.RefID, &l.RefType, &l.CreatedBy, &l.CreatedAt,
	); err != nil {
		return nil, err
	}
	return l, nil
}

var _ BalanceLedgerRepository = (*PostgresBalanceLedgerRepository)(nil)
