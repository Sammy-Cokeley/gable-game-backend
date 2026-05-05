CREATE TABLE IF NOT EXISTS ranking_source (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    notes TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ranking_snapshot (
    id BIGSERIAL PRIMARY KEY,
    season_id TEXT NOT NULL,
    ranking_date DATE NOT NULL,
    source_id BIGINT NOT NULL REFERENCES ranking_source(id),
    weight_class INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (season_id, ranking_date, source_id, weight_class)
);

CREATE TABLE IF NOT EXISTS ranking_entry (
    id BIGSERIAL PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES ranking_snapshot(id) ON DELETE CASCADE,
    wrestler_id INTEGER NOT NULL,
    rank INTEGER NOT NULL CHECK (rank > 0),
    points NUMERIC(8,2),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (snapshot_id, wrestler_id),
    UNIQUE (snapshot_id, rank)
);

CREATE INDEX IF NOT EXISTS idx_ranking_snapshot_lookup
    ON ranking_snapshot (season_id, ranking_date, source_id, weight_class);

CREATE INDEX IF NOT EXISTS idx_ranking_entry_snapshot
    ON ranking_entry (snapshot_id);
