ALTER TABLE users
    ADD COLUMN banned_at  TIMESTAMPTZ,
    ADD COLUMN banned_by  INT REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN ban_reason TEXT NOT NULL DEFAULT '';

-- Partial index: only banned users are indexed, keeping the index tiny.
CREATE INDEX idx_users_banned_at ON users (banned_at) WHERE banned_at IS NOT NULL;
