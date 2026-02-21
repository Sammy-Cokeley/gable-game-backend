# V2 Chunk 1 Local Verification Checklist

## 1) Apply migrations

Apply the new SQL migration files with your preferred migration runner (or manually in psql):

- `database/migrations/000001_create_ranking_ingestion_tables.up.sql`
- Rollback path: `database/migrations/000001_create_ranking_ingestion_tables.down.sql`

## 2) Run unit tests

```bash
go test ./rankings -v
```

## 3) Smoke test CSV importer

Example CSV (`/tmp/rankings.csv`):

```csv
wrestler_id,rank,points,metadata
101,1,32.5,{"note":"returning champ"}
202,2,,{}
```

Run importer:

```bash
go run ./cmd/import_rankings_snapshot \
  --csv /tmp/rankings.csv \
  --source coaches \
  --season 2025 \
  --date 2025-01-15 \
  --weight-class 125
```

Expected result: command prints a snapshot ID and inserted row count.
