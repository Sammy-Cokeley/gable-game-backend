-- 009_daily_wrestlers_2025.sql
-- Seeds daily_wrestlers with 2025 NCAA qualifiers in a fixed random order,
-- starting 2025-03-23 (day after the 2025 championship concluded).
-- The 2025 archive window (2025-03-23 through 2026-03-22) does not overlap
-- with the 2026 live puzzle window (2026-03-23 onward).
-- Safe to re-run: uses ON CONFLICT DO NOTHING.
--
-- PREREQUISITE: core.wrestler rows for 2025 must have wrestlestat_id populated.
-- Verify coverage before running:
--   SELECT COUNT(*) FROM core.wrestler w
--   JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
--   JOIN core.season se ON se.id = ws.season_id AND se.year = 2025
--   WHERE w.wrestlestat_id IS NOT NULL;
-- If count is significantly below ~330, run the WrestleStat ingest for year 2025 first.

-- Add season_year so each puzzle date explicitly records which season it belongs to.
-- This avoids ambiguous season lookups for wrestlers who competed in multiple seasons.
ALTER TABLE daily_wrestlers ADD COLUMN IF NOT EXISTS season_year INT;

DO $$
DECLARE
    r          RECORD;
    day_offset INT := 0;
BEGIN
    -- Fixed seed ensures the same shuffle on every run
    PERFORM setseed(0.2025);

    FOR r IN
        SELECT w.wrestlestat_id::INT AS wsid
        FROM core.wrestler w
        JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
        JOIN core.season se          ON se.id = ws.season_id AND se.year = 2025
        WHERE w.wrestlestat_id IS NOT NULL
        ORDER BY random()
    LOOP
        INSERT INTO daily_wrestlers (day, wrestler_id, season_year)
        VALUES ('2025-03-23'::date + day_offset, r.wsid, 2025)
        ON CONFLICT DO NOTHING;

        day_offset := day_offset + 1;
    END LOOP;
END $$;
