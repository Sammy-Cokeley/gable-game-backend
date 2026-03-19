-- 005_backfill_wrestlers_2025.sql
-- Widen win_percentage to unconstrained NUMERIC — the source column stores
-- values that may exceed the NUMERIC(5,4) precision set in 004_align_schema.
ALTER TABLE core.wrestler_season ALTER COLUMN win_percentage TYPE NUMERIC;
-- Populates core.* from the legacy wrestlers_2025 table.
-- Column names match the schema established by prior migrations on Render
-- (56052cd: primary_weight_class_id, wins/losses in wrestler_season).
-- Safe to re-run: all inserts use ON CONFLICT DO NOTHING.

-- ---------------------------------------------------------------------------
-- 1. Seed the 2025 season
-- ---------------------------------------------------------------------------
INSERT INTO core.season (year, label, start_date, end_date)
VALUES (2025, '2024-25', '2024-11-01', '2025-03-22')
ON CONFLICT (year) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. Seed conferences
-- ---------------------------------------------------------------------------
INSERT INTO core.conference (name, slug)
SELECT DISTINCT
    conference,
    lower(regexp_replace(conference, '[^a-zA-Z0-9]+', '-', 'g'))
FROM wrestlers_2025
WHERE conference IS NOT NULL AND conference <> ''
ON CONFLICT (name) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Seed schools
-- ---------------------------------------------------------------------------
INSERT INTO core.school (name, slug)
SELECT DISTINCT
    team,
    lower(regexp_replace(team, '[^a-zA-Z0-9]+', '-', 'g'))
FROM wrestlers_2025
WHERE team IS NOT NULL AND team <> ''
ON CONFLICT (name) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 4. Seed school ↔ conference ↔ season membership
-- ---------------------------------------------------------------------------
INSERT INTO core.school_conference_season (school_id, conference_id, season_id)
SELECT DISTINCT
    sc.id,
    co.id,
    se.id
FROM wrestlers_2025 w
JOIN core.school      sc ON sc.name = w.team
JOIN core.conference  co ON co.name = w.conference
JOIN core.season      se ON se.year = 2025
WHERE w.team IS NOT NULL AND w.conference IS NOT NULL
ON CONFLICT (school_id, season_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 5. Seed weight classes (sort_order added by 004_align_schema)
-- ---------------------------------------------------------------------------
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
-- 6. Seed wrestlers
-- ---------------------------------------------------------------------------
INSERT INTO core.wrestler (full_name, slug)
SELECT
    w.name,
    lower(regexp_replace(w.name, '[^a-zA-Z0-9]+', '-', 'g')) || '-' ||
    lower(regexp_replace(w.team, '[^a-zA-Z0-9]+', '-', 'g'))
FROM wrestlers_2025 w
ON CONFLICT (slug) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 7. Seed wrestler_season rows
--    Uses primary_weight_class_id and wins/losses per the existing schema.
-- ---------------------------------------------------------------------------
INSERT INTO core.wrestler_season (
    wrestler_id,
    season_id,
    school_id,
    primary_weight_class_id,
    class_year,
    wins,
    losses,
    win_percentage,
    ncaa_finish
)
SELECT
    wr.id,
    se.id,
    sc.id,
    wc.id,
    w.year,
    NULL,   -- record_wins not available in wrestlers_2025
    NULL,   -- record_losses not available in wrestlers_2025
    w.win_percentage::NUMERIC,
    NULLIF(w.ncaa_finish, '')
FROM wrestlers_2025 w
JOIN core.wrestler    wr ON wr.slug = lower(regexp_replace(w.name, '[^a-zA-Z0-9]+', '-', 'g'))
                                 || '-' ||
                                 lower(regexp_replace(w.team, '[^a-zA-Z0-9]+', '-', 'g'))
JOIN core.season      se ON se.year = 2025
JOIN core.school      sc ON sc.name = w.team
JOIN core.weight_class wc ON wc.label = w.weight_class
ON CONFLICT (wrestler_id, season_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 8. Populate legacy_wrestler_map
--    Existing table uses (legacy_table TEXT, legacy_id TEXT) composite key.
-- ---------------------------------------------------------------------------
INSERT INTO core.legacy_wrestler_map (legacy_table, legacy_id, wrestler_id)
SELECT
    'wrestlers_2025',
    w.id::TEXT,
    wr.id
FROM wrestlers_2025 w
JOIN core.wrestler wr ON wr.slug = lower(regexp_replace(w.name, '[^a-zA-Z0-9]+', '-', 'g'))
                              || '-' ||
                              lower(regexp_replace(w.team, '[^a-zA-Z0-9]+', '-', 'g'))
ON CONFLICT (legacy_table, legacy_id) DO NOTHING;
