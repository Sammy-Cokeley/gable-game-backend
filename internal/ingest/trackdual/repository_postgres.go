package trackdual

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

type pgTx struct{ tx *sql.Tx }

func (p *pgTx) Commit() error   { return p.tx.Commit() }
func (p *pgTx) Rollback() error { return p.tx.Rollback() }

func unwrapTx(tx Tx) *sql.Tx { return tx.(*pgTx).tx }

func (r *PostgresRepository) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &pgTx{tx: tx}, nil
}

func (r *PostgresRepository) CreateIngestBatch(ctx context.Context, tx Tx) (int64, error) {
	sqlTx := unwrapTx(tx)
	var id int64
	err := sqlTx.QueryRowContext(ctx, `INSERT INTO core.ingest_batch (source_name, status) VALUES ($1, $2) RETURNING id`, SourceName, "processing").Scan(&id)
	return id, err
}
func (r *PostgresRepository) FinalizeIngestBatch(ctx context.Context, tx Tx, batchID int64, status string, summary ProcessResult) error {
	sqlTx := unwrapTx(tx)
	b, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	_, err = sqlTx.ExecContext(ctx, `UPDATE core.ingest_batch SET status=$2, summary=$3, finished_at=NOW() WHERE id=$1`, batchID, status, b)
	return err
}
func (r *PostgresRepository) GetOrCreateSeason(ctx context.Context, tx Tx, year int) (int64, error) {
	sqlTx := unwrapTx(tx)
	var id int64
	err := sqlTx.QueryRowContext(ctx, `SELECT id FROM core.season WHERE year=$1`, year).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	err = sqlTx.QueryRowContext(ctx, `INSERT INTO core.season (year, label) VALUES ($1,$2) ON CONFLICT (year) DO UPDATE SET label=EXCLUDED.label RETURNING id`, year, fmt.Sprintf("%d-%d", year, year+1)).Scan(&id)
	return id, err
}
func (r *PostgresRepository) GetWeightClassByLabel(ctx context.Context, tx Tx, label string) (int64, error) {
	sqlTx := unwrapTx(tx)
	var id int64
	err := sqlTx.QueryRowContext(ctx, `SELECT id FROM core.weight_class WHERE lower(label)=lower($1)`, label).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("weight class %q not found: %w", label, err)
	}
	return id, nil
}
func (r *PostgresRepository) GetOrCreateSchool(ctx context.Context, tx Tx, slug string) (int64, error) {
	sqlTx := unwrapTx(tx)
	var id int64
	err := sqlTx.QueryRowContext(ctx, `INSERT INTO core.school (slug,name) VALUES ($1,$2) ON CONFLICT (slug) DO UPDATE SET slug=EXCLUDED.slug RETURNING id`, slug, humanizeSlug(slug)).Scan(&id)
	return id, err
}
func (r *PostgresRepository) GetOrCreateEvent(ctx context.Context, tx Tx, seasonID int64, name, eventDate, eventType, dualID string) (int64, error) {
	sqlTx := unwrapTx(tx)
	var id int64
	err := sqlTx.QueryRowContext(ctx, `INSERT INTO core.event (season_id,name,event_date,event_type,external_id,source_name) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (source_name,external_id) DO UPDATE SET name=EXCLUDED.name RETURNING id`, seasonID, name, eventDate, eventType, dualID, SourceName).Scan(&id)
	return id, err
}
func (r *PostgresRepository) GetOrCreateWrestlerWithAlias(ctx context.Context, tx Tx, fullName string) (int64, error) {
	sqlTx := unwrapTx(tx)
	var wrestlerID int64
	err := sqlTx.QueryRowContext(ctx, `SELECT wrestler_id FROM core.wrestler_alias WHERE source_name=$1 AND lower(alias_name)=lower($2) LIMIT 1`, SourceName, fullName).Scan(&wrestlerID)
	if err == nil {
		return wrestlerID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	err = sqlTx.QueryRowContext(ctx, `INSERT INTO core.wrestler (full_name,slug) VALUES ($1,regexp_replace(lower($1), '[^a-z0-9]+', '-', 'g')) RETURNING id`, fullName).Scan(&wrestlerID)
	if err != nil {
		return 0, err
	}
	_, err = sqlTx.ExecContext(ctx, `INSERT INTO core.wrestler_alias (wrestler_id,source_name,alias_name) VALUES ($1,$2,$3) ON CONFLICT (source_name,alias_name) DO NOTHING`, wrestlerID, SourceName, fullName)
	return wrestlerID, err
}
func (r *PostgresRepository) UpsertWrestlerSeason(ctx context.Context, tx Tx, wrestlerID, seasonID, schoolID, weightClassID int64) error {
	sqlTx := unwrapTx(tx)
	_, err := sqlTx.ExecContext(ctx, `INSERT INTO core.wrestler_season (wrestler_id,season_id,school_id,primary_weight_class_id) VALUES ($1,$2,$3,$4) ON CONFLICT (wrestler_id,season_id) DO UPDATE SET school_id=EXCLUDED.school_id, primary_weight_class_id=EXCLUDED.primary_weight_class_id`, wrestlerID, seasonID, schoolID, weightClassID)
	return err
}
func (r *PostgresRepository) InsertIngestError(ctx context.Context, tx Tx, batchID int64, rowNumber int, reason string, payload CSVRecord) error {
	sqlTx := unwrapTx(tx)
	p, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = sqlTx.ExecContext(ctx, `INSERT INTO core.ingest_error (ingest_batch_id,row_number,error_message,payload) VALUES ($1,$2,$3,$4)`, batchID, rowNumber, reason, p)
	return err
}
func (r *PostgresRepository) InsertBout(ctx context.Context, tx Tx, in BoutInsertInput) (bool, error) {
	sqlTx := unwrapTx(tx)
	res, err := sqlTx.ExecContext(ctx, `INSERT INTO core.bout (event_id,season_id,weight_class_id,wrestler_a_id,wrestler_b_id,winner_id,result_method,score_a,score_b,match_time,source_name,source_match_id,identity_hash,metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,jsonb_build_object('dual_id',$14,'bout_number',$15,'winner_name',$16,'wrestler_a_name',$17,'wrestler_b_name',$18,'weight_label',$19,'event_date',$20)) ON CONFLICT (source_name, identity_hash) DO NOTHING`, in.EventID, in.SeasonID, in.WeightClassID, in.WrestlerAID, in.WrestlerBID, in.WinnerID, in.ResultMethod, in.ScoreA, in.ScoreB, in.MatchTime, SourceName, in.SourceMatchID, in.IdentityHash, in.DualID, in.BoutNumber, in.WinnerName, in.WrestlerAName, in.WrestlerBName, in.WeightLabel, in.EventDateISO)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func humanizeSlug(slug string) string {
	slug = strings.ReplaceAll(strings.TrimSpace(slug), "-", " ")
	return strings.Title(slug)
}
