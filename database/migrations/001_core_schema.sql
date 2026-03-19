-- 001_core_schema.sql
-- Canonical wrestling data model.
-- All tables live in the "core" schema to separate them from legacy game tables.

CREATE SCHEMA IF NOT EXISTS core;

-- Enable pgcrypto for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Seasons
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS core.season (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    year       INT  NOT NULL UNIQUE,   -- e.g. 2025 means the 2024-25 season
    label      TEXT NOT NULL,          -- e.g. "2024-25"
    start_date DATE,
    end_date   DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Conferences
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS core.conference (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    slug       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Schools
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS core.school (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    short_name TEXT,
    slug       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- School ↔ Conference ↔ Season membership
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS core.school_conference_season (
    school_id     UUID NOT NULL REFERENCES core.school(id),
    conference_id UUID NOT NULL REFERENCES core.conference(id),
    season_id     UUID NOT NULL REFERENCES core.season(id),
    PRIMARY KEY (school_id, season_id)
);

-- ---------------------------------------------------------------------------
-- Weight classes
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS core.weight_class (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label      TEXT NOT NULL UNIQUE,  -- e.g. "125", "HWT"
    pounds     INT,                   -- NULL for HWT
    sort_order INT  NOT NULL
);

INSERT INTO core.weight_class (label, pounds, sort_order) VALUES
    ('125',  125, 1),
    ('133',  133, 2),
    ('141',  141, 3),
    ('149',  149, 4),
    ('157',  157, 5),
    ('165',  165, 6),
    ('174',  174, 7),
    ('184',  184, 8),
    ('197',  197, 9),
    ('285',  285, 10)
ON CONFLICT (label) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Wrestlers (canonical identity — one row per person, ever)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS core.wrestler (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name      TEXT NOT NULL,
    slug           TEXT NOT NULL UNIQUE,   -- e.g. "robinson-vincent-penn-state"
    wrestlestat_id TEXT UNIQUE,            -- external ID from wrestlestat.com
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Wrestler aliases (alternative spellings / source variations)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS core.wrestler_alias (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wrestler_id UUID NOT NULL REFERENCES core.wrestler(id) ON DELETE CASCADE,
    alias       TEXT NOT NULL,
    source      TEXT,   -- e.g. "flo", "trackwrestling", "ncaa"
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (wrestler_id, alias)
);

-- ---------------------------------------------------------------------------
-- Wrestler seasons (roster entry — one row per wrestler per season)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS core.wrestler_season (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wrestler_id     UUID NOT NULL REFERENCES core.wrestler(id),
    season_id       UUID NOT NULL REFERENCES core.season(id),
    school_id       UUID NOT NULL REFERENCES core.school(id),
    weight_class_id UUID NOT NULL REFERENCES core.weight_class(id),
    class_year      TEXT,            -- FR, SO, JR, SR, RS-FR, etc.
    record_wins     INT,
    record_losses   INT,
    win_percentage  NUMERIC,         -- stored for convenience
    ncaa_finish     TEXT,            -- e.g. "1st", "All-American", "DNQ"
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (wrestler_id, season_id)
);

CREATE INDEX IF NOT EXISTS wrestler_season_season_id_idx    ON core.wrestler_season (season_id);
CREATE INDEX IF NOT EXISTS wrestler_season_school_id_idx    ON core.wrestler_season (school_id);
CREATE INDEX IF NOT EXISTS wrestler_season_wc_id_idx        ON core.wrestler_season (primary_weight_class_id);
