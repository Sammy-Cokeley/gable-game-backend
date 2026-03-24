-- 008_drop_user_guesses_wrestler_fkey.sql
-- Drops the legacy FK from user_guesses.wrestler_id → wrestlers_2025.
-- Since migration 007, wrestler_id holds WrestleStat IDs (integers matching
-- core.wrestler.wrestlestat_id::INT), so the old FK is invalid.

ALTER TABLE user_guesses DROP CONSTRAINT IF EXISTS user_guesses_wrestler_id_fkey;
