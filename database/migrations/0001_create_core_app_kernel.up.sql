CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS core;
CREATE SCHEMA IF NOT EXISTS app;

CREATE TABLE IF NOT EXISTS core.season (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    year INT UNIQUE NOT NULL,
    label TEXT UNIQUE NOT NULL,
    start_date DATE,
    end_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.school (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.conference (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.school_conference_season (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL REFERENCES core.school(id),
    conference_id UUID NOT NULL REFERENCES core.conference(id),
    season_id UUID NOT NULL REFERENCES core.season(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (school_id, season_id)
);

CREATE TABLE IF NOT EXISTS core.weight_class (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label TEXT UNIQUE NOT NULL,
    pounds INT,
    sort_order INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.wrestler (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wrestlestat_id INT UNIQUE NULL,
    full_name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.wrestler_alias (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wrestler_id UUID NOT NULL REFERENCES core.wrestler(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    source_name TEXT NOT NULL,
    confidence SMALLINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_name, alias)
);

CREATE TABLE IF NOT EXISTS core.wrestler_season (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wrestler_id UUID NOT NULL REFERENCES core.wrestler(id) ON DELETE CASCADE,
    season_id UUID NOT NULL REFERENCES core.season(id),
    school_id UUID NOT NULL REFERENCES core.school(id),
    primary_weight_class_id UUID NULL REFERENCES core.weight_class(id),
    class_year TEXT,
    wins INT,
    losses INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (wrestler_id, season_id)
);

CREATE TABLE IF NOT EXISTS core.event (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    season_id UUID NOT NULL REFERENCES core.season(id),
    name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    start_date DATE,
    end_date DATE,
    location TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name, season_id)
);

CREATE TABLE IF NOT EXISTS core.legacy_wrestler_map (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    legacy_table TEXT NOT NULL,
    legacy_id TEXT NOT NULL,
    wrestler_id UUID NOT NULL REFERENCES core.wrestler(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (legacy_table, legacy_id)
);

CREATE TABLE IF NOT EXISTS core.ingest_batch (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_name TEXT NOT NULL,
    season_id UUID NOT NULL REFERENCES core.season(id),
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    summary JSONB
);

CREATE TABLE IF NOT EXISTS core.ingest_error (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ingest_batch_id UUID REFERENCES core.ingest_batch(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_key TEXT,
    error_message TEXT NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.bout (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID REFERENCES core.event(id),
    season_id UUID NOT NULL REFERENCES core.season(id),
    round TEXT,
    bout_number INT,
    weight_class_id UUID NOT NULL REFERENCES core.weight_class(id),
    wrestler_a_id UUID NULL REFERENCES core.wrestler(id),
    wrestler_b_id UUID NULL REFERENCES core.wrestler(id),
    winner_id UUID REFERENCES core.wrestler(id),
    result TEXT,
    score TEXT,
    winner_score INT NULL,
    loser_score INT NULL,
    occurred_at TIMESTAMPTZ,
    source_name TEXT NOT NULL DEFAULT 'unknown',
    source_match_id TEXT NULL,
    identity_hash TEXT NOT NULL,
    ingest_batch_id UUID NULL REFERENCES core.ingest_batch(id),
    raw_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_name, source_match_id),
    UNIQUE (source_name, identity_hash)
);

CREATE INDEX IF NOT EXISTS idx_bout_season_weight_occurred ON core.bout (season_id, weight_class_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_bout_event_round ON core.bout (event_id, round);
CREATE INDEX IF NOT EXISTS idx_wrestler_season_wrestler ON core.wrestler_season (wrestler_id);
CREATE INDEX IF NOT EXISTS idx_wrestler_season_season ON core.wrestler_season (season_id);
CREATE INDEX IF NOT EXISTS idx_school_conf_season_school ON core.school_conference_season (school_id);
CREATE INDEX IF NOT EXISTS idx_school_conf_season_season ON core.school_conference_season (season_id);
CREATE INDEX IF NOT EXISTS idx_bout_wrestler_a ON core.bout (wrestler_a_id);
CREATE INDEX IF NOT EXISTS idx_bout_wrestler_b ON core.bout (wrestler_b_id);
CREATE INDEX IF NOT EXISTS idx_bout_winner ON core.bout (winner_id);
CREATE INDEX IF NOT EXISTS idx_bout_ingest_batch ON core.bout (ingest_batch_id);
CREATE INDEX IF NOT EXISTS idx_bout_event_id ON core.bout (event_id);
