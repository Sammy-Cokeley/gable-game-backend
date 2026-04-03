-- 011_daily_wrestlers_2024.sql
-- Seeds daily_wrestlers with 2024 NCAA qualifiers in a fixed random order,
-- starting 2024-03-23 (day after the 2024 championship concluded).
-- The 2024 archive window (2024-03-23 through 2025-03-22) does not overlap
-- with the 2025 archive window (2025-03-23 onward).
-- Safe to re-run: uses ON CONFLICT DO NOTHING.
--
-- PREREQUISITE: run the WrestleStat ingest for season 2024 first:
--   go run ./cmd/ingest_wrestlestat_qualifiers/main.go -season 2024
-- Verify coverage before running:
--   SELECT COUNT(*) FROM core.wrestler w
--   JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
--   JOIN core.season se ON se.id = ws.season_id AND se.year = 2024
--   WHERE w.wrestlestat_id IS NOT NULL;

DO $$
DECLARE
    r          RECORD;
    day_offset INT := 0;
BEGIN
    PERFORM setseed(0.2024);

    FOR r IN
        SELECT w.wrestlestat_id::INT AS wsid
        FROM core.wrestler w
        JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
        JOIN core.season se          ON se.id = ws.season_id AND se.year = 2024
        WHERE w.wrestlestat_id IS NOT NULL
        ORDER BY random()
    LOOP
        INSERT INTO daily_wrestlers (day, wrestler_id, season_year)
        VALUES ('2024-03-23'::date + day_offset, r.wsid, 2024)
        ON CONFLICT DO NOTHING;

        day_offset := day_offset + 1;
    END LOOP;
END $$;
