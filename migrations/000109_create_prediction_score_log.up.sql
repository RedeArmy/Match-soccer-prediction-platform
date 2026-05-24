-- Audit trail for every ScoreMatch recalculation.
-- One row per prediction per scoring run; captures old and new points,
-- the match result, the prediction, and the full scoring config that was
-- active at the time so any points dispute can be reconstructed from this
-- table alone without touching the live predictions or matches rows.
CREATE TABLE prediction_score_log (
    id                   BIGSERIAL    PRIMARY KEY,
    prediction_id        INT          NOT NULL,
    match_id             INT          NOT NULL,
    user_id              INT          NOT NULL,
    scored_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    -- point transition
    old_points           SMALLINT,                          -- NULL on first scoring run
    new_points           SMALLINT     NOT NULL,
    delta                SMALLINT     GENERATED ALWAYS AS (new_points - COALESCE(old_points, 0)) STORED,
    -- match result snapshot
    match_home_score     SMALLINT     NOT NULL,
    match_away_score     SMALLINT     NOT NULL,
    match_win_method     TEXT,                              -- NULL for group-stage matches
    match_phase          TEXT         NOT NULL,
    -- prediction snapshot
    pred_home_score      SMALLINT     NOT NULL,
    pred_away_score      SMALLINT     NOT NULL,
    pred_win_method      TEXT,                              -- NULL when user did not submit a win-method guess
    -- scoring config snapshot (values that were active at scoring time)
    cfg_exact_score      SMALLINT     NOT NULL,
    cfg_correct_outcome  SMALLINT     NOT NULL,
    cfg_goal_diff        SMALLINT     NOT NULL,
    cfg_extra_time_bonus SMALLINT     NOT NULL,
    cfg_penalties_bonus  SMALLINT     NOT NULL
);

-- Fast lookup for the admin disputes view: "all scoring events for match M".
CREATE INDEX ON prediction_score_log (match_id);

-- Fast lookup for the user history view: "my scoring events, newest first".
CREATE INDEX ON prediction_score_log (user_id, scored_at DESC);
