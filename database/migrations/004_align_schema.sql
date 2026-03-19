-- 004_align_schema.sql
-- Adds columns that were absent from the schema created by prior migrations
-- (committed in 56052cd). All statements are safe to run on a fresh DB too.

-- core.wrestler_season is missing win_percentage and ncaa_finish
ALTER TABLE core.wrestler_season ADD COLUMN IF NOT EXISTS win_percentage NUMERIC;
ALTER TABLE core.wrestler_season ADD COLUMN IF NOT EXISTS ncaa_finish TEXT;

-- core.school is missing short_name
ALTER TABLE core.school ADD COLUMN IF NOT EXISTS short_name TEXT;
