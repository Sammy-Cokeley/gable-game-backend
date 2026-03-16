-- 003_ranking_schema.sql
-- Ranking ingestion and ranked-pool tables.
-- These replace the legacy rankings_releases / rankings_release_entries tables
-- for all new data going forward. Legacy tables remain intact for now.

-- ---------------------------------------------------------------------------
-- Ranking sources  (Flo, NCAA, TrackWrestling, etc.)
-- ---------------------------------------------------------------------------
CREATE TABLE core.ranking_source (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name      TEXT NOT NULL UNIQUE,
    slug      TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    notes     TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Ranking snapshots — one row per source × season × weight class × date
-- ---------------------------------------------------------------------------
CREATE TABLE core.ranking_snapshot (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id       UUID NOT NULL REFERENCES core.ranking_source(id),
    season_id       UUID NOT NULL REFERENCES core.season(id),
    weight_class_id UUID NOT NULL REFERENCES core.weight_class(id),
    snapshot_date   DATE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft', 'published')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_id, season_id, weight_class_id, snapshot_date)
);

CREATE INDEX ON core.ranking_snapshot (season_id, snapshot_date);

-- ---------------------------------------------------------------------------
-- Ranking entries — individual positions within a snapshot
-- ---------------------------------------------------------------------------
CREATE TABLE core.ranking_entry (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_id   UUID NOT NULL REFERENCES core.ranking_snapshot(id) ON DELETE CASCADE,
    wrestler_id   UUID NOT NULL REFERENCES core.wrestler(id),
    rank          INT  NOT NULL CHECK (rank > 0),
    previous_rank INT,          -- NULL = new to rankings
    metadata      JSONB,        -- source-specific extras (points, grade, etc.)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (snapshot_id, rank),
    UNIQUE (snapshot_id, wrestler_id)
);

CREATE INDEX ON core.ranking_entry (wrestler_id);

-- ---------------------------------------------------------------------------
-- Ranked pool rules — configurable criteria for building the game pool
-- ---------------------------------------------------------------------------
CREATE TABLE core.ranked_pool_rule (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    season_id   UUID NOT NULL REFERENCES core.season(id),
    -- Example rule_config:
    -- {"strategy": "top_n_in_sources", "n": 33, "min_sources": 1}
    -- {"strategy": "consensus", "n": 33, "scoring": "best_rank"}
    rule_config JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Ranked pool members — computed set of wrestlers eligible for a given week
-- ---------------------------------------------------------------------------
CREATE TABLE core.ranked_pool_member (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    season_id       UUID NOT NULL REFERENCES core.season(id),
    snapshot_date   DATE NOT NULL,
    weight_class_id UUID NOT NULL REFERENCES core.weight_class(id),
    wrestler_id     UUID NOT NULL REFERENCES core.wrestler(id),
    -- Why this wrestler was included (for debugging / transparency)
    reason          JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (season_id, snapshot_date, weight_class_id, wrestler_id)
);

CREATE INDEX ON core.ranked_pool_member (season_id, snapshot_date);
CREATE INDEX ON core.ranked_pool_member (wrestler_id);
