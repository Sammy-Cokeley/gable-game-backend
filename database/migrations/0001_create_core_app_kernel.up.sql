CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS core;
CREATE SCHEMA IF NOT EXISTS app;

CREATE TABLE IF NOT EXISTS core.season (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    start_date DATE,
    end_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.school (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    slug TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.conference (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    slug TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.school_conference_season (
    id BIGSERIAL PRIMARY KEY,
    school_id BIGINT NOT NULL REFERENCES core.school(id),
    conference_id BIGINT NOT NULL REFERENCES core.conference(id),
    season_id BIGINT NOT NULL REFERENCES core.season(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (school_id, conference_id, season_id)
);

CREATE TABLE IF NOT EXISTS core.weight_class (
    id BIGSERIAL PRIMARY KEY,
    label TEXT NOT NULL,
    pounds INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (label)
);

CREATE TABLE IF NOT EXISTS core.wrestler (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wrestlestat_id BIGINT UNIQUE,
    canonical_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.wrestler_alias (
    id BIGSERIAL PRIMARY KEY,
    wrestler_id UUID NOT NULL REFERENCES core.wrestler(id) ON DELETE CASCADE,
    alias_name TEXT NOT NULL,
    source TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (wrestler_id, alias_name)
);

CREATE TABLE IF NOT EXISTS core.wrestler_season (
    id BIGSERIAL PRIMARY KEY,
    wrestler_id UUID NOT NULL REFERENCES core.wrestler(id) ON DELETE CASCADE,
    season_id BIGINT NOT NULL REFERENCES core.season(id),
    school_id BIGINT REFERENCES core.school(id),
    weight_class_id BIGINT REFERENCES core.weight_class(id),
    class_year TEXT,
    wins INT,
    losses INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (wrestler_id, season_id)
);

CREATE TABLE IF NOT EXISTS core.event (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    season_id BIGINT REFERENCES core.season(id),
    name TEXT NOT NULL,
    event_type TEXT,
    start_date DATE,
    end_date DATE,
    location TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name, season_id)
);

CREATE TABLE IF NOT EXISTS core.bout (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID REFERENCES core.event(id),
    season_id BIGINT REFERENCES core.season(id),
    round TEXT,
    bout_number INT,
    weight_class_id BIGINT REFERENCES core.weight_class(id),
    wrestler1_id UUID REFERENCES core.wrestler(id),
    wrestler2_id UUID REFERENCES core.wrestler(id),
    winner_id UUID REFERENCES core.wrestler(id),
    result TEXT,
    decision_type TEXT,
    score TEXT,
    occurred_at TIMESTAMPTZ,
    source_match_id TEXT UNIQUE,
    raw_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.legacy_wrestler_map (
    id BIGSERIAL PRIMARY KEY,
    legacy_table TEXT NOT NULL,
    legacy_id TEXT NOT NULL,
    wrestler_id UUID NOT NULL REFERENCES core.wrestler(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (legacy_table, legacy_id)
);

CREATE TABLE IF NOT EXISTS core.ingest_batch (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT NOT NULL,
    season_id BIGINT REFERENCES core.season(id),
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    summary JSONB
);

CREATE TABLE IF NOT EXISTS core.ingest_error (
    id BIGSERIAL PRIMARY KEY,
    batch_id UUID REFERENCES core.ingest_batch(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_key TEXT,
    error_message TEXT NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
