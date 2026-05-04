package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresPurger implements Purger against a PostgreSQL database.
type PostgresPurger struct {
	db *pgxpool.Pool
}

// NewPostgresPurger constructs a PostgresPurger backed by db.
func NewPostgresPurger(db *pgxpool.Pool) *PostgresPurger {
	return &PostgresPurger{db: db}
}

func (p *PostgresPurger) PurgeDeletedUsers(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := p.db.Exec(ctx,
		`DELETE FROM users WHERE deleted_at IS NOT NULL AND deleted_at < $1`, olderThan)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

func (p *PostgresPurger) PurgeDeletedQuinielas(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := p.db.Exec(ctx,
		`DELETE FROM quinielas WHERE deleted_at IS NOT NULL AND deleted_at < $1`, olderThan)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

var _ Purger = (*PostgresPurger)(nil)
