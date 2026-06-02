package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ExchangeRateRepository persists and retrieves exchange_rate_history rows.
type ExchangeRateRepository interface {
	// Save inserts a new rate record.  The inserted row's ID and CreatedAt are
	// not required by callers; they are populated by the database defaults.
	Save(ctx context.Context, rec *domain.ExchangeRateRecord) error

	// GetLatest returns the most recent row ordered by effective_at DESC.
	// Returns nil, nil when the table is empty (e.g. fresh deployment before
	// the first RefreshRate call).
	GetLatest(ctx context.Context) (*domain.ExchangeRateRecord, error)

	// GetHistory returns rows ordered by effective_at DESC.
	// limit/offset implement simple page-based pagination; the caller is
	// responsible for supplying sensible defaults.
	GetHistory(ctx context.Context, limit, offset int) ([]*domain.ExchangeRateRecord, error)

	// GetOverrides returns admin override rows ordered by effective_at DESC.
	GetOverrides(ctx context.Context, limit int) ([]*domain.ExchangeRateRecord, error)
}

type postgresExchangeRateRepository struct {
	db *pgxpool.Pool
}

// NewPostgresExchangeRateRepository constructs the Postgres implementation.
func NewPostgresExchangeRateRepository(db *pgxpool.Pool) ExchangeRateRepository {
	return &postgresExchangeRateRepository{db: db}
}

func (r *postgresExchangeRateRepository) Save(ctx context.Context, rec *domain.ExchangeRateRecord) error {
	const q = `
		INSERT INTO exchange_rate_history
		    (reference_rate, buy_rate, sell_rate, buy_margin_pct, sell_margin_pct,
		     source, api_version, stale, is_override, override_reason, override_by,
		     fetched_at, effective_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	_, err := r.db.Exec(ctx, q,
		rec.ReferenceRate.String(),
		rec.BuyRate.String(),
		rec.SellRate.String(),
		rec.BuyMarginPct.String(),
		rec.SellMarginPct.String(),
		rec.Source,
		rec.ApiVersion,
		rec.Stale,
		rec.IsOverride,
		nullableString(rec.OverrideReason),
		rec.OverrideBy,
		rec.FetchedAt,
		rec.EffectiveAt,
	)
	if err != nil {
		return fmt.Errorf("exchange_rate_history: save: %w", err)
	}
	return nil
}

func (r *postgresExchangeRateRepository) GetLatest(ctx context.Context) (*domain.ExchangeRateRecord, error) {
	const q = `
		SELECT id, reference_rate, buy_rate, sell_rate, buy_margin_pct, sell_margin_pct,
		       source, api_version, stale, is_override,
		       COALESCE(override_reason, '') AS override_reason,
		       override_by, fetched_at, effective_at, created_at
		FROM exchange_rate_history
		ORDER BY effective_at DESC
		LIMIT 1`

	return r.scanOne(ctx, q)
}

func (r *postgresExchangeRateRepository) GetHistory(ctx context.Context, limit, offset int) ([]*domain.ExchangeRateRecord, error) {
	const q = `
		SELECT id, reference_rate, buy_rate, sell_rate, buy_margin_pct, sell_margin_pct,
		       source, api_version, stale, is_override,
		       COALESCE(override_reason, '') AS override_reason,
		       override_by, fetched_at, effective_at, created_at
		FROM exchange_rate_history
		ORDER BY effective_at DESC
		LIMIT $1 OFFSET $2`

	return r.scanMany(ctx, q, limit, offset)
}

func (r *postgresExchangeRateRepository) GetOverrides(ctx context.Context, limit int) ([]*domain.ExchangeRateRecord, error) {
	const q = `
		SELECT id, reference_rate, buy_rate, sell_rate, buy_margin_pct, sell_margin_pct,
		       source, api_version, stale, is_override,
		       COALESCE(override_reason, '') AS override_reason,
		       override_by, fetched_at, effective_at, created_at
		FROM exchange_rate_history
		WHERE is_override = TRUE
		ORDER BY effective_at DESC
		LIMIT $1`

	return r.scanMany(ctx, q, limit)
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func (r *postgresExchangeRateRepository) scanOne(ctx context.Context, q string, args ...any) (*domain.ExchangeRateRecord, error) {
	row := r.db.QueryRow(ctx, q, args...)
	rec, err := scanExchangeRateRecord(row)
	if err != nil {
		return nil, fmt.Errorf("exchange_rate_history: scan: %w", err)
	}
	return rec, nil
}

func (r *postgresExchangeRateRepository) scanMany(ctx context.Context, q string, args ...any) ([]*domain.ExchangeRateRecord, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("exchange_rate_history: query: %w", err)
	}
	defer rows.Close()

	var out []*domain.ExchangeRateRecord
	for rows.Next() {
		rec, err := scanExchangeRateRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("exchange_rate_history: scan row: %w", err)
		}
		if rec != nil {
			out = append(out, rec)
		}
	}
	return out, rows.Err()
}

// scanner is satisfied by pgx Row and Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanExchangeRateRecord(s scanner) (*domain.ExchangeRateRecord, error) {
	var (
		rec        domain.ExchangeRateRecord
		refStr     string
		buyStr     string
		sellStr    string
		buyMrgStr  string
		sellMrgStr string
	)
	err := s.Scan(
		&rec.ID,
		&refStr, &buyStr, &sellStr, &buyMrgStr, &sellMrgStr,
		&rec.Source, &rec.ApiVersion, &rec.Stale, &rec.IsOverride,
		&rec.OverrideReason, &rec.OverrideBy,
		&rec.FetchedAt, &rec.EffectiveAt, &rec.CreatedAt,
	)
	if err != nil {
		// pgx returns pgx.ErrNoRows when a QueryRow scan finds no row.
		// Return nil, nil so callers can distinguish "not found" from errors.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var parseErr error
	rec.ReferenceRate, parseErr = decimal.NewFromString(refStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse reference_rate %q: %w", refStr, parseErr)
	}
	rec.BuyRate, parseErr = decimal.NewFromString(buyStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse buy_rate %q: %w", buyStr, parseErr)
	}
	rec.SellRate, parseErr = decimal.NewFromString(sellStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse sell_rate %q: %w", sellStr, parseErr)
	}
	rec.BuyMarginPct, parseErr = decimal.NewFromString(buyMrgStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse buy_margin_pct %q: %w", buyMrgStr, parseErr)
	}
	rec.SellMarginPct, parseErr = decimal.NewFromString(sellMrgStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse sell_margin_pct %q: %w", sellMrgStr, parseErr)
	}

	return &rec, nil
}

// nullableString returns nil for an empty string, so INSERT stores NULL.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
