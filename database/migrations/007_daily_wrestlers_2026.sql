-- 007_daily_wrestlers_2026.sql
-- Seeds daily_wrestlers with the 330 2026 NCAA qualifiers in a fixed random order,
-- starting 2026-03-23 (day after the championship concluded).
-- Safe to re-run: uses ON CONFLICT DO NOTHING.

-- Drop the legacy FK to wrestlers_2025 so wrestler_id can hold WrestleStat IDs.
ALTER TABLE daily_wrestlers DROP CONSTRAINT IF EXISTS daily_wrestlers_wrestler_id_fkey;

DO $$
DECLARE
    r          RECORD;
    day_offset INT := 0;
BEGIN
    -- Fixed seed ensures the same shuffle on every run
    PERFORM setseed(0.2026);

    FOR r IN
        SELECT w.wrestlestat_id::INT AS wsid
        FROM core.wrestler w
        JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
        JOIN core.season se          ON se.id = ws.season_id AND se.year = 2026
        WHERE w.wrestlestat_id IS NOT NULL
        ORDER BY random()
    LOOP
        INSERT INTO daily_wrestlers (day, wrestler_id)
        VALUES ('2026-03-23'::date + day_offset, r.wsid)
        ON CONFLICT DO NOTHING;

        day_offset := day_offset + 1;
    END LOOP;
END $$;
