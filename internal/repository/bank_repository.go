package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// Bank is a row from gt_banks.
type Bank struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// BankRepository is the persistence interface for the gt_banks table.
type BankRepository interface {
	ListActive(ctx context.Context) ([]Bank, error)
	ListAll(ctx context.Context) ([]Bank, error)
	Create(ctx context.Context, name string) (Bank, error)
	SetActive(ctx context.Context, id int, active bool) (Bank, error)
}

// PostgresBankRepository implements BankRepository against Postgres.
type PostgresBankRepository struct {
	db *pgxpool.Pool
}

// NewPostgresBankRepository constructs a PostgresBankRepository.
func NewPostgresBankRepository(db *pgxpool.Pool) *PostgresBankRepository {
	return &PostgresBankRepository{db: db}
}

// ListActive returns all banks with active = TRUE ordered alphabetically.
func (r *PostgresBankRepository) ListActive(ctx context.Context) ([]Bank, error) {
	return r.list(ctx, true)
}

// ListAll returns all banks regardless of active status.
func (r *PostgresBankRepository) ListAll(ctx context.Context) ([]Bank, error) {
	return r.list(ctx, false)
}

func (r *PostgresBankRepository) list(ctx context.Context, onlyActive bool) ([]Bank, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()

	q := `SELECT id, name, active FROM gt_banks`
	if onlyActive {
		q += ` WHERE active = TRUE`
	}
	q += ` ORDER BY name ASC`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var banks []Bank
	for rows.Next() {
		var b Bank
		if err := rows.Scan(&b.ID, &b.Name, &b.Active); err != nil {
			return nil, err
		}
		banks = append(banks, b)
	}
	return banks, rows.Err()
}

// Create inserts a new active bank and returns the created row.
func (r *PostgresBankRepository) Create(ctx context.Context, name string) (Bank, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	var b Bank
	err := r.db.QueryRow(ctx,
		`INSERT INTO gt_banks (name, active) VALUES ($1, TRUE)
		 RETURNING id, name, active`, name).
		Scan(&b.ID, &b.Name, &b.Active)
	if err != nil {
		return Bank{}, apperrors.Internal(err)
	}
	return b, nil
}

// SetActive enables or disables a bank by ID and returns the updated row.
func (r *PostgresBankRepository) SetActive(ctx context.Context, id int, active bool) (Bank, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	var b Bank
	err := r.db.QueryRow(ctx,
		`UPDATE gt_banks SET active = $2, updated_at = NOW()
		 WHERE id = $1 RETURNING id, name, active`, id, active).
		Scan(&b.ID, &b.Name, &b.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Bank{}, apperrors.NotFound("bank not found")
	}
	if err != nil {
		return Bank{}, apperrors.Internal(err)
	}
	return b, nil
}
