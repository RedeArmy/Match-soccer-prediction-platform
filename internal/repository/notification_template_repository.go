package repository

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

type templateCacheKey struct{ eventType, locale string }

// postgresNotificationTemplateRepository implements NotificationTemplateRepository.
// It eagerly loads all rows into an in-memory map on first access and refreshes
// every ttl.  Writes immediately invalidate the cache so the next read
// re-fetches from the database.
type postgresNotificationTemplateRepository struct {
	pool *pgxpool.Pool
	ttl  time.Duration

	mu       sync.RWMutex
	cache    map[templateCacheKey]*domain.NotificationTemplate
	loadedAt time.Time
}

// NewPostgresNotificationTemplateRepository constructs a caching repository
// backed by the given connection pool.  ttl controls how long the in-memory
// cache is considered fresh; use domain.DefaultNotifyTemplateCacheTTLSec * time.Second
// when no system-param override is available.
func NewPostgresNotificationTemplateRepository(pool *pgxpool.Pool, ttl time.Duration) NotificationTemplateRepository {
	if ttl <= 0 {
		ttl = time.Duration(domain.DefaultNotifyTemplateCacheTTLSec) * time.Second
	}
	return &postgresNotificationTemplateRepository{pool: pool, ttl: ttl}
}

func (r *postgresNotificationTemplateRepository) cacheHit(key templateCacheKey) (*domain.NotificationTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cache == nil || time.Since(r.loadedAt) >= r.ttl {
		return nil, false
	}
	t, ok := r.cache[key]
	return t, ok
}

// warm loads all rows from the database and replaces the cache.
// Must be called with mu write-locked.
func (r *postgresNotificationTemplateRepository) warm(ctx context.Context) error {
	const q = `
SELECT event_type, locale, title_tmpl, body_tmpl, action_url_tmpl,
       email_subject_tmpl, email_html_tmpl, updated_by, updated_at
FROM   notification_templates
ORDER  BY event_type, locale
`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return apperrors.Internal(err)
	}
	defer rows.Close()

	m := make(map[templateCacheKey]*domain.NotificationTemplate)
	for rows.Next() {
		var t domain.NotificationTemplate
		if err := rows.Scan(
			&t.EventType, &t.Locale,
			&t.TitleTmpl, &t.BodyTmpl, &t.ActionURLTmpl,
			&t.EmailSubjectTmpl, &t.EmailHTMLTmpl, &t.UpdatedBy, &t.UpdatedAt,
		); err != nil {
			return apperrors.Internal(err)
		}
		m[templateCacheKey{t.EventType, t.Locale}] = &t
	}
	if err := rows.Err(); err != nil {
		return apperrors.Internal(err)
	}

	r.cache = m
	r.loadedAt = time.Now()
	return nil
}

func (r *postgresNotificationTemplateRepository) Get(ctx context.Context, eventType, locale string) (*domain.NotificationTemplate, error) {
	key := templateCacheKey{eventType, locale}
	if t, ok := r.cacheHit(key); ok {
		return t, nil // may be nil (negative cache entry is not stored — miss = not found)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Re-check after acquiring write lock: another goroutine may have warmed the cache.
	if r.cache != nil && time.Since(r.loadedAt) < r.ttl {
		return r.cache[key], nil
	}
	if err := r.warm(ctx); err != nil {
		return nil, err
	}
	return r.cache[key], nil
}

func (r *postgresNotificationTemplateRepository) List(ctx context.Context) ([]*domain.NotificationTemplate, error) {
	r.mu.Lock()
	if r.cache == nil || time.Since(r.loadedAt) >= r.ttl {
		if err := r.warm(ctx); err != nil {
			r.mu.Unlock()
			return nil, err
		}
	}
	out := make([]*domain.NotificationTemplate, 0, len(r.cache))
	for _, t := range r.cache {
		cp := *t
		out = append(out, &cp)
	}
	r.mu.Unlock()
	return out, nil
}

func (r *postgresNotificationTemplateRepository) Upsert(ctx context.Context, t *domain.NotificationTemplate) error {
	const q = `
INSERT INTO notification_templates
    (event_type, locale, title_tmpl, body_tmpl, action_url_tmpl, email_subject_tmpl, email_html_tmpl, updated_by, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (event_type, locale) DO UPDATE SET
    title_tmpl         = EXCLUDED.title_tmpl,
    body_tmpl          = EXCLUDED.body_tmpl,
    action_url_tmpl    = EXCLUDED.action_url_tmpl,
    email_subject_tmpl = EXCLUDED.email_subject_tmpl,
    email_html_tmpl    = EXCLUDED.email_html_tmpl,
    updated_by         = EXCLUDED.updated_by,
    updated_at         = now()
`
	if _, err := r.pool.Exec(ctx, q,
		t.EventType, t.Locale,
		t.TitleTmpl, t.BodyTmpl, t.ActionURLTmpl, t.EmailSubjectTmpl, t.EmailHTMLTmpl,
		t.UpdatedBy,
	); err != nil {
		return apperrors.Internal(err)
	}
	r.invalidate()
	return nil
}

func (r *postgresNotificationTemplateRepository) Delete(ctx context.Context, eventType, locale string) error {
	const q = `DELETE FROM notification_templates WHERE event_type = $1 AND locale = $2`
	if _, err := r.pool.Exec(ctx, q, eventType, locale); err != nil {
		return apperrors.Internal(err)
	}
	r.invalidate()
	return nil
}

func (r *postgresNotificationTemplateRepository) invalidate() {
	r.mu.Lock()
	r.loadedAt = time.Time{}
	r.mu.Unlock()
}

func (r *postgresNotificationTemplateRepository) ListHistory(ctx context.Context, eventType, locale string, limit int) ([]*domain.NotificationTemplateHistory, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	const q = `
SELECT id, event_type, locale, title_tmpl, body_tmpl, action_url_tmpl,
       email_subject_tmpl, email_html_tmpl, changed_by, changed_at
FROM   notification_template_history
WHERE  event_type = $1 AND locale = $2
ORDER  BY id DESC
LIMIT  $3
`
	rows, err := r.pool.Query(ctx, q, eventType, locale, limit)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var out []*domain.NotificationTemplateHistory
	for rows.Next() {
		h := &domain.NotificationTemplateHistory{}
		if err := rows.Scan(
			&h.ID, &h.EventType, &h.Locale,
			&h.TitleTmpl, &h.BodyTmpl, &h.ActionURLTmpl,
			&h.EmailSubjectTmpl, &h.EmailHTMLTmpl,
			&h.ChangedBy, &h.ChangedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return out, nil
}

func (r *postgresNotificationTemplateRepository) GetHistoryEntry(ctx context.Context, id int64, eventType, locale string) (*domain.NotificationTemplateHistory, error) {
	const q = `
SELECT id, event_type, locale, title_tmpl, body_tmpl, action_url_tmpl,
       email_subject_tmpl, email_html_tmpl, changed_by, changed_at
FROM   notification_template_history
WHERE  id = $1 AND event_type = $2 AND locale = $3
`
	h := &domain.NotificationTemplateHistory{}
	err := r.pool.QueryRow(ctx, q, id, eventType, locale).Scan(
		&h.ID, &h.EventType, &h.Locale,
		&h.TitleTmpl, &h.BodyTmpl, &h.ActionURLTmpl,
		&h.EmailSubjectTmpl, &h.EmailHTMLTmpl,
		&h.ChangedBy, &h.ChangedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("notification template history entry not found")
		}
		return nil, apperrors.Internal(err)
	}
	return h, nil
}

var _ NotificationTemplateRepository = (*postgresNotificationTemplateRepository)(nil)
