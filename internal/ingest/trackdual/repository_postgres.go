package trackdual

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
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

func (r *PostgresRepository) CreateIngestBatch(ctx context.Context, tx Tx) (string, error) {
	sqlTx := unwrapTx(tx)
	var id string
	err := sqlTx.QueryRowContext(ctx,
		`INSERT INTO core.ingest_batch (source_name, status) VALUES ($1, $2) RETURNING id`,
		SourceName, "processing",
	).Scan(&id)
	return id, err
}

func (r *PostgresRepository) FinalizeIngestBatch(ctx context.Context, tx Tx, batchID string, status string, summary ProcessResult) error {
	sqlTx := unwrapTx(tx)
	b, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	_, err = sqlTx.ExecContext(ctx,
		`UPDATE core.ingest_batch SET status=$2, summary=$3, completed_at=NOW() WHERE id=$1`,
		batchID, status, b,
	)
	return err
}

func (r *PostgresRepository) GetOrCreateSeason(ctx context.Context, tx Tx, year int) (string, error) {
	sqlTx := unwrapTx(tx)
	var id string
	err := sqlTx.QueryRowContext(ctx, `SELECT id FROM core.season WHERE year=$1`, year).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	err = sqlTx.QueryRowContext(ctx,
		`INSERT INTO core.season (year, label) VALUES ($1,$2)
		 ON CONFLICT (year) DO UPDATE SET label=EXCLUDED.label RETURNING id`,
		year, fmt.Sprintf("%d-%d", year, year+1),
	).Scan(&id)
	return id, err
}

func (r *PostgresRepository) GetWeightClassByLabel(ctx context.Context, tx Tx, label string) (string, error) {
	sqlTx := unwrapTx(tx)
	var id string
	err := sqlTx.QueryRowContext(ctx,
		`SELECT id FROM core.weight_class WHERE lower(label)=lower($1)`, label,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("weight class %q not found: %w", label, err)
	}
	return id, nil
}

func (r *PostgresRepository) GetOrCreateSchool(ctx context.Context, tx Tx, slug string) (string, error) {
	sqlTx := unwrapTx(tx)
	var id string
	err := sqlTx.QueryRowContext(ctx,
		`INSERT INTO core.school (slug, name) VALUES ($1, $2)
		 ON CONFLICT (slug) DO UPDATE SET slug=EXCLUDED.slug RETURNING id`,
		slug, humanizeSlug(slug),
	).Scan(&id)
	return id, err
}

func (r *PostgresRepository) GetOrCreateEvent(ctx context.Context, tx Tx, seasonID string, name, eventDate, eventType, dualID string) (string, error) {
	sqlTx := unwrapTx(tx)
	var id string
	err := sqlTx.QueryRowContext(ctx,
		`INSERT INTO core.event (season_id, name, event_date, event_type, external_id, source_name)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT ON CONSTRAINT uq_event_source_external DO UPDATE SET name=EXCLUDED.name
		 RETURNING id`,
		seasonID, name, eventDate, eventType, dualID, SourceName,
	).Scan(&id)
	return id, err
}

func (r *PostgresRepository) GetOrCreateWrestlerWithAlias(ctx context.Context, tx Tx, fullName string) (string, error) {
	sqlTx := unwrapTx(tx)

	// Check if we already have an alias for this name from this source.
	var wrestlerID string
	err := sqlTx.QueryRowContext(ctx,
		`SELECT wrestler_id FROM core.wrestler_alias
		 WHERE source_name=$1 AND lower(alias)=lower($2) LIMIT 1`,
		SourceName, fullName,
	).Scan(&wrestlerID)
	if err == nil {
		return wrestlerID, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	// Create a new wrestler. Try the clean slug first; append a random suffix on collision.
	slug := slugify(fullName)
	err = sqlTx.QueryRowContext(ctx,
		`INSERT INTO core.wrestler (full_name, slug) VALUES ($1, $2)
		 ON CONFLICT (slug) DO NOTHING RETURNING id`,
		fullName, slug,
	).Scan(&wrestlerID)
	if err == sql.ErrNoRows {
		// Slug collision — append a short random suffix.
		slug = fmt.Sprintf("%s-%06x", slug, rand.Intn(0xffffff))
		err = sqlTx.QueryRowContext(ctx,
			`INSERT INTO core.wrestler (full_name, slug) VALUES ($1, $2) RETURNING id`,
			fullName, slug,
		).Scan(&wrestlerID)
	}
	if err != nil {
		return "", err
	}

	_, err = sqlTx.ExecContext(ctx,
		`INSERT INTO core.wrestler_alias (wrestler_id, source_name, alias)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (source_name, alias) DO NOTHING`,
		wrestlerID, SourceName, fullName,
	)
	return wrestlerID, err
}

func (r *PostgresRepository) UpsertWrestlerSeason(ctx context.Context, tx Tx, wrestlerID, seasonID, schoolID, weightClassID string) error {
	sqlTx := unwrapTx(tx)
	_, err := sqlTx.ExecContext(ctx,
		`INSERT INTO core.wrestler_season (wrestler_id, season_id, school_id, primary_weight_class_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (wrestler_id, season_id) DO UPDATE
		   SET school_id=EXCLUDED.school_id,
		       primary_weight_class_id=EXCLUDED.primary_weight_class_id`,
		wrestlerID, seasonID, schoolID, weightClassID,
	)
	return err
}

func (r *PostgresRepository) InsertIngestError(ctx context.Context, tx Tx, batchID string, rowNumber int, reason string, payload CSVRecord) error {
	sqlTx := unwrapTx(tx)
	p, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = sqlTx.ExecContext(ctx,
		`INSERT INTO core.ingest_error (ingest_batch_id, row_number, entity_type, error_message, payload)
		 VALUES ($1, $2, 'bout', $3, $4)`,
		batchID, rowNumber, reason, p,
	)
	return err
}

func (r *PostgresRepository) InsertBout(ctx context.Context, tx Tx, in BoutInsertInput) (bool, error) {
	sqlTx := unwrapTx(tx)
	res, err := sqlTx.ExecContext(ctx, `
		INSERT INTO core.bout (
			event_id, season_id, weight_class_id,
			wrestler_a_id, wrestler_b_id, winner_id,
			result, result_method, score_a, score_b, match_time,
			source_name, source_match_id, identity_hash,
			ingest_batch_id, raw_payload
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $7, $8, $9, $10,
			$11, $12, $13,
			$14,
			jsonb_build_object(
				'dual_id',        $15,
				'bout_number',    $16,
				'winner_name',    $17,
				'wrestler_a_name',$18,
				'wrestler_b_name',$19,
				'weight_label',   $20,
				'event_date',     $21
			)
		) ON CONFLICT (source_name, identity_hash) DO NOTHING`,
		in.EventID, in.SeasonID, in.WeightClassID,
		in.WrestlerAID, in.WrestlerBID, in.WinnerID,
		in.ResultMethod, in.ScoreA, in.ScoreB, in.MatchTime,
		SourceName, in.SourceMatchID, in.IdentityHash,
		in.BatchID,
		in.DualID, in.BoutNumber, in.WinnerName,
		in.WrestlerAName, in.WrestlerBName, in.WeightLabel, in.EventDateISO,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func humanizeSlug(slug string) string {
	slug = strings.ReplaceAll(strings.TrimSpace(slug), "-", " ")
	words := strings.Fields(slug)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
