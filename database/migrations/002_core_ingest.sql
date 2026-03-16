-- 002_core_ingest.sql
-- Observability tables for data ingestion, plus the legacy ID bridge.

-- ---------------------------------------------------------------------------
-- Ingest batches — one row per import job
-- ---------------------------------------------------------------------------
CREATE TABLE core.ingest_batch (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source       TEXT NOT NULL,   -- e.g. "wrestlestat", "csv_rankings", "manual"
    season_id    UUID REFERENCES core.season(id),
    status       TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    stats        JSONB,           -- e.g. {"inserted": 42, "skipped": 3, "errors": 1}
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Ingest errors — one row per failed record within a batch
-- ---------------------------------------------------------------------------
CREATE TABLE core.ingest_error (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id      UUID NOT NULL REFERENCES core.ingest_batch(id) ON DELETE CASCADE,
    entity_type   TEXT NOT NULL,   -- e.g. "wrestler", "ranking_entry"
    raw_data      JSONB,           -- the raw input that caused the error
    error_message TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON core.ingest_error (batch_id);

-- ---------------------------------------------------------------------------
-- Legacy wrestler map — bridges wrestlers_2025.id → core.wrestler.id
-- Allows the legacy game tables to stay unchanged while canonical data grows.
-- ---------------------------------------------------------------------------
CREATE TABLE core.legacy_wrestler_map (
    legacy_id   INT  NOT NULL UNIQUE,   -- wrestlers_2025.id
    wrestler_id UUID NOT NULL REFERENCES core.wrestler(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON core.legacy_wrestler_map (wrestler_id);
