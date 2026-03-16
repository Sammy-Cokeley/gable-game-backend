-- 004_backfill_wrestlers_2025.sql
-- Populates core.* from the legacy wrestlers_2025 table.
-- Safe to run on any environment; uses INSERT ... ON CONFLICT DO NOTHING
-- so re-running is harmless.

-- ---------------------------------------------------------------------------
-- 1. Seed the 2025 season
-- ---------------------------------------------------------------------------
INSERT INTO core.season (year, label, start_date, end_date)
VALUES (2025, '2024-25', '2024-11-01', '2025-03-22')
ON CONFLICT (year) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. Seed conferences (distinct values from wrestlers_2025)
-- ---------------------------------------------------------------------------
INSERT INTO core.conference (name, slug)
SELECT DISTINCT
    conference,
    lower(regexp_replace(conference, '[^a-zA-Z0-9]+', '-', 'g'))
FROM wrestlers_2025
WHERE conference IS NOT NULL AND conference <> ''
ON CONFLICT (name) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Seed schools (distinct team values from wrestlers_2025)
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
-- 5. Seed wrestlers (canonical identity)
--    Slug: "firstname-lastname" lowercased, special chars stripped.
--    wrestlestat_id is unknown at this point — left NULL, enriched later.
-- ---------------------------------------------------------------------------
INSERT INTO core.wrestler (full_name, slug)
SELECT
    w.name,
    lower(regexp_replace(w.name, '[^a-zA-Z0-9]+', '-', 'g')) || '-' ||
    lower(regexp_replace(w.team, '[^a-zA-Z0-9]+', '-', 'g'))
FROM wrestlers_2025 w
ON CONFLICT (slug) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 6. Seed wrestler_season rows
-- ---------------------------------------------------------------------------
INSERT INTO core.wrestler_season (
    wrestler_id,
    season_id,
    school_id,
    weight_class_id,
    class_year,
    win_percentage,
    ncaa_finish
)
SELECT
    wr.id,
    se.id,
    sc.id,
    wc.id,
    w.year,
    CASE
        WHEN w.win_percentage ~ '^[0-9]+(\.[0-9]+)?$'
        THEN w.win_percentage::NUMERIC
        ELSE NULL
    END,
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
-- 7. Populate legacy_wrestler_map (wrestlers_2025.id → core.wrestler.id)
-- ---------------------------------------------------------------------------
INSERT INTO core.legacy_wrestler_map (legacy_id, wrestler_id)
SELECT
    w.id,
    wr.id
FROM wrestlers_2025 w
JOIN core.wrestler wr ON wr.slug = lower(regexp_replace(w.name, '[^a-zA-Z0-9]+', '-', 'g'))
                              || '-' ||
                              lower(regexp_replace(w.team, '[^a-zA-Z0-9]+', '-', 'g'))
ON CONFLICT (legacy_id) DO NOTHING;
