-- group_memberships is the pivot table between users and quinielas.
--
-- Each row records one user's membership in one quiniela. The status column
-- tracks the membership lifecycle:
--   pending  — invitation sent, user has not yet confirmed.
--   active   — user is a confirmed participant; their predictions count.
--   left     — user voluntarily exited; kept for audit history.
--
-- The owner of a quiniela receives an 'active' membership automatically at
-- creation time so they appear in the leaderboard without extra steps.
--
-- joined_at records when the status last transitioned to 'active'. It is
-- NULL for pending memberships and populated when the user confirms.

CREATE TYPE membership_status AS ENUM ('pending', 'active', 'left');

CREATE TABLE group_memberships (
    id           SERIAL          PRIMARY KEY,
    quiniela_id  INTEGER         NOT NULL REFERENCES quinielas(id) ON DELETE CASCADE,
    user_id      INTEGER         NOT NULL REFERENCES users(id)     ON DELETE CASCADE,
    status       membership_status NOT NULL DEFAULT 'pending',
    joined_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT group_memberships_unique_member UNIQUE (quiniela_id, user_id)
);

CREATE INDEX idx_group_memberships_quiniela ON group_memberships (quiniela_id);
CREATE INDEX idx_group_memberships_user     ON group_memberships (user_id);
