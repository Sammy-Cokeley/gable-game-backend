-- 010_daily_wrestlers_season_year.sql
-- Backfills season_year for the 2026 live puzzle rows that were seeded by
-- migration 007 before the season_year column existed (added in migration 009).
--
-- The column already exists after migration 009 runs. This migration only
-- needs to set season_year on the pre-existing 2026 rows.

UPDATE daily_wrestlers SET season_year = 2026 WHERE day >= '2026-03-23' AND season_year IS NULL;
