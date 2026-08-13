CREATE TABLE services (
    id              UUID            PRIMARY KEY,
    name            TEXT            NOT NULL UNIQUE,
    description     TEXT            NOT NULL DEFAULT '',
    owner_team      TEXT            NOT NULL,
    repo_url        TEXT            NOT NULL,
    tier            INT             NOT NULL DEFAULT 3 CHECK (tier BETWEEN 1 AND 3),
    created_at      TIMESTAMPTZ     NOT NULL,
    updated_at      TIMESTAMPTZ     NOT NULL
);

CREATE TABLE scorecard_results (
    id              UUID            PRIMARY KEY,
    service_id      UUID            NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    overall_score   INT             NOT NULL CHECK (overall_score BETWEEN 0 AND 100),
    level           TEXT            NOT NULL CHECK (level IN ('bronze', 'silver', 'gold')),
    evaluated_at    TIMESTAMPTZ     NOT NULL
);

-- Supports "latest scorecard for this service", the hot read path.
CREATE INDEX idx_scorecard_results_service_evaluated
    ON scorecard_results (service_id, evaluated_at DESC);

CREATE TABLE check_results (
    id              UUID            PRIMARY KEY,
    result_id       UUID            NOT NULL REFERENCES scorecard_results (id) ON DELETE CASCADE,
    check_key       TEXT            NOT NULL,
    passed          BOOLEAN         NOT NULL,
    weight          INT             NOT NULL CHECK (weight > 0),
    detail          TEXT            NOT NULL DEFAULT '',
    -- SQL rows have no inherent order; this preserves the engine's ordering
    position        INT             NOT NULL
);


CREATE  INDEX idx_check_results_result ON check_results (result_id);
