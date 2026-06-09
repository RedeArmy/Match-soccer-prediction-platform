package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// BankAccountType is a row from bank_account_types.
type BankAccountType struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// BankAccountTypeRepository is the persistence interface for bank_account_types.
type BankAccountTypeRepository interface {
	ListActive(ctx context.Context) ([]BankAccountType, error)
	ListAll(ctx context.Context) ([]BankAccountType, error)
	Create(ctx context.Context, name string) (BankAccountType, error)
	SetActive(ctx context.Context, id int, active bool) (BankAccountType, error)
}

// PostgresBankAccountTypeRepository implements BankAccountTypeRepository.
type PostgresBankAccountTypeRepository struct {
	db *pgxpool.Pool
}

// NewPostgresBankAccountTypeRepository constructs a PostgresBankAccountTypeRepository.
func NewPostgresBankAccountTypeRepository(db *pgxpool.Pool) *PostgresBankAccountTypeRepository {
	return &PostgresBankAccountTypeRepository{db: db}
}

// ListActive returns all account types with active = TRUE ordered alphabetically.
func (r *PostgresBankAccountTypeRepository) ListActive(ctx context.Context) ([]BankAccountType, error) {
	return r.list(ctx, true)
}

// ListAll returns all account types regardless of active status.
func (r *PostgresBankAccountTypeRepository) ListAll(ctx context.Context) ([]BankAccountType, error) {
	return r.list(ctx, false)
}

func (r *PostgresBankAccountTypeRepository) list(ctx context.Context, onlyActive bool) ([]BankAccountType, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()

	q := `SELECT id, name, active FROM bank_account_types`
	if onlyActive {
		q += ` WHERE active = TRUE`
	}
	q += ` ORDER BY name ASC`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []BankAccountType
	for rows.Next() {
		var t BankAccountType
		if err := rows.Scan(&t.ID, &t.Name, &t.Active); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

// Create inserts a new active account type and returns the created row.
func (r *PostgresBankAccountTypeRepository) Create(ctx context.Context, name string) (BankAccountType, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	var t BankAccountType
	err := r.db.QueryRow(ctx,
		`INSERT INTO bank_account_types (name, active) VALUES ($1, TRUE)
		 RETURNING id, name, active`, name).
		Scan(&t.ID, &t.Name, &t.Active)
	if err != nil {
		return BankAccountType{}, apperrors.Internal(err)
	}
	return t, nil
}

// SetActive enables or disables an account type by ID and returns the updated row.
func (r *PostgresBankAccountTypeRepository) SetActive(ctx context.Context, id int, active bool) (BankAccountType, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	var t BankAccountType
	err := r.db.QueryRow(ctx,
		`UPDATE bank_account_types SET active = $2, updated_at = NOW()
		 WHERE id = $1 RETURNING id, name, active`, id, active).
		Scan(&t.ID, &t.Name, &t.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return BankAccountType{}, apperrors.NotFound("bank account type not found")
	}
	if err != nil {
		return BankAccountType{}, apperrors.Internal(err)
	}
	return t, nil
}
