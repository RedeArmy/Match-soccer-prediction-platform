-- tournament_slots tracks bracket positions confirmed by the system administrator.
-- Each row represents one named position in the bracket (e.g. "winner_group_a",
-- "best_3rd_1"). The admin fills in the team name after FIFA announces the
-- advancement, and optionally links the slot to the knockout match it feeds.
CREATE TABLE tournament_slots (
    id                   SERIAL PRIMARY KEY,
    label                VARCHAR(100) NOT NULL UNIQUE,
    team                 VARCHAR(100),
    confirmed_at         TIMESTAMPTZ,
    confirmed_by_user_id INT REFERENCES users(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
