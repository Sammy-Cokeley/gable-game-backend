-- Align core.bout and core.event for TrackWrestling ingestion.
-- Adds missing columns; does not drop or rename existing ones.

-- ingest_batch: season_id is not known at batch creation time
ALTER TABLE core.ingest_batch ALTER COLUMN season_id DROP NOT NULL;

-- ingest_error: add row_number for CSV row tracking; relax entity_type
ALTER TABLE core.ingest_error ADD COLUMN IF NOT EXISTS row_number INT;
ALTER TABLE core.ingest_error ALTER COLUMN entity_type SET DEFAULT 'unknown';

-- event: add external identity fields for deduplication
ALTER TABLE core.event ADD COLUMN IF NOT EXISTS external_id  TEXT;
ALTER TABLE core.event ADD COLUMN IF NOT EXISTS source_name  TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE core.event ADD COLUMN IF NOT EXISTS event_date   DATE;
ALTER TABLE core.event ALTER COLUMN event_type DROP NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_event_source_external'
    ) THEN
        ALTER TABLE core.event
            ADD CONSTRAINT uq_event_source_external UNIQUE (source_name, external_id);
    END IF;
END$$;

-- bout: add per-bout detail columns used by the ingestion service
ALTER TABLE core.bout ADD COLUMN IF NOT EXISTS result_method TEXT;
ALTER TABLE core.bout ADD COLUMN IF NOT EXISTS score_a       INT;
ALTER TABLE core.bout ADD COLUMN IF NOT EXISTS score_b       INT;
ALTER TABLE core.bout ADD COLUMN IF NOT EXISTS match_time    TEXT;
ALTER TABLE core.bout ALTER COLUMN weight_class_id DROP NOT NULL;
