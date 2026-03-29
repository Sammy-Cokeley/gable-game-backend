package rankings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) ImportSnapshot(ctx context.Context, snapshot SnapshotImport) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin ranking snapshot transaction: %w", err)
	}
	defer tx.Rollback()

	var sourceID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO ranking_source (name, is_active)
		VALUES ($1, TRUE)
		ON CONFLICT (name)
		DO UPDATE SET is_active = TRUE, updated_at = NOW()
		RETURNING id
	`, snapshot.SourceName).Scan(&sourceID); err != nil {
		return 0, fmt.Errorf("upsert ranking source: %w", err)
	}

	var snapshotID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO ranking_snapshot (season_id, ranking_date, source_id, weight_class)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, snapshot.SeasonID, snapshot.RankingDate, sourceID, snapshot.WeightClass).Scan(&snapshotID); err != nil {
		return 0, fmt.Errorf("insert ranking snapshot: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ranking_entry (snapshot_id, wrestler_id, rank, points, metadata)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare ranking entry insert: %w", err)
	}
	defer stmt.Close()

	for _, entry := range snapshot.Entries {
		metadata := entry.Metadata
		if len(metadata) == 0 {
			metadata = json.RawMessage(`{}`)
		}

		if _, err := stmt.ExecContext(ctx, snapshotID, entry.WrestlerID, entry.Rank, entry.Points, metadata); err != nil {
			return 0, fmt.Errorf("insert ranking entry wrestler_id=%d rank=%d: %w", entry.WrestlerID, entry.Rank, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit ranking snapshot transaction: %w", err)
	}

	return snapshotID, nil
}

var _ Repository = (*PostgresRepository)(nil)
