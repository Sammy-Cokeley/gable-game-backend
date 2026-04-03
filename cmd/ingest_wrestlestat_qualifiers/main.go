package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"time"

	"gable-backend/database"
	"gable-backend/internal/wrestlestat"

	"github.com/joho/godotenv"
)

var weightClasses = []int{125, 133, 141, 149, 157, 165, 174, 184, 197, 285}

func main() {
	season := flag.Int("season", 2026, "Season year to ingest (e.g. 2026)")
	delay := flag.Duration("delay", 500*time.Millisecond, "Delay between HTTP requests")
	weight := flag.Int("weight", 0, "Only process one weight class (0 = all)")
	limit := flag.Int("limit", 0, "Max wrestlers to check per weight class (0 = all)")
	skipFinish := flag.Bool("skip-finish", false, "Skip profile fetch for NCAA finish (imports all starters, not just qualifiers)")
	flag.Parse()

	if os.Getenv("RENDER") == "" {
		_ = godotenv.Load()
	}
	database.ConnectDB()
	client := &http.Client{Timeout: 15 * time.Second}

	total, failed, skipped := 0, 0, 0
	ctx := context.Background()

	// Collect entries: historical years use a single all-weights page;
	// current season (default 2026) uses one request per weight class.
	var allEntries []wrestlestat.WrestlerEntry

	if *season > 0 && *season < 2026 {
		log.Printf("=== Fetching year-indexed starters for season %d ===", *season)
		entries, err := wrestlestat.ScrapeAllStartersByYear(client, *season, *weight)
		if err != nil {
			log.Fatalf("ERROR scraping season %d: %v", *season, err)
		}
		log.Printf("Scraped %d wrestlers across all weight classes", len(entries))
		allEntries = entries
	} else {
		weights := weightClasses
		if *weight > 0 {
			weights = []int{*weight}
		}
		for _, w := range weights {
			log.Printf("=== Weight class %d ===", w)
			entries, err := wrestlestat.ScrapeStartersByWeight(client, w)
			if err != nil {
				log.Printf("  ERROR scraping %d: %v", w, err)
				continue
			}
			log.Printf("  Scraped %d wrestlers", len(entries))
			time.Sleep(*delay)
			allEntries = append(allEntries, entries...)
		}
	}

	if *limit > 0 && len(allEntries) > *limit {
		allEntries = allEntries[:*limit]
	}

	for i := range allEntries {
		e := &allEntries[i]

		if !*skipFinish {
			if err := wrestlestat.FetchNCAAFinish(client, e, *season); err != nil {
				log.Printf("  WARN: profile fetch failed for %s (wsid=%d): %v", e.Name, e.WrestleStatID, err)
			}
			time.Sleep(*delay)

			// Only import wrestlers who competed at NCAAs
			if e.NCAAFinish == "" {
				skipped++
				continue
			}
		}

		if err := upsertQualifier(ctx, database.DB, *season, e); err != nil {
			log.Printf("  ERROR upserting %s: %v", e.Name, err)
			failed++
		} else {
			log.Printf("  OK: %s (%s, %s, %s, %s, %s W%d-L%d)",
				e.Name, e.WeightClass, e.ClassYear, e.School, e.Conference, e.NCAAFinish, e.Wins, e.Losses)
			total++
		}
	}

	fmt.Printf("\n=== Done ===\n")
	fmt.Printf("Upserted: %d\n", total)
	fmt.Printf("Failed:   %d\n", failed)
	fmt.Printf("Skipped (no NCAA finish): %d\n", skipped)
}

// upsertQualifier writes a single qualifier's data into core.* tables within a transaction.
func upsertQualifier(ctx context.Context, db *sql.DB, seasonYear int, e *wrestlestat.WrestlerEntry) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Season
	seasonID, err := getOrCreateSeason(ctx, tx, seasonYear)
	if err != nil {
		return fmt.Errorf("season: %w", err)
	}

	// 2. School
	schoolSlug := e.SchoolSlug
	if schoolSlug == "" {
		schoolSlug = wrestlestat.Slugify(e.School)
	}
	schoolID, err := getOrCreateSchool(ctx, tx, schoolSlug, e.School)
	if err != nil {
		return fmt.Errorf("school: %w", err)
	}

	// 3. Conference + school-conference-season link
	if e.Conference != "" {
		confID, err := getOrCreateConference(ctx, tx, e.Conference)
		if err != nil {
			return fmt.Errorf("conference: %w", err)
		}
		if err := linkSchoolConferenceSeason(ctx, tx, schoolID, confID, seasonID); err != nil {
			return fmt.Errorf("school_conference_season: %w", err)
		}
	}

	// 4. Weight class
	weightClassID, err := getWeightClassByLabel(ctx, tx, e.WeightClass)
	if err != nil {
		return fmt.Errorf("weight class %q: %w", e.WeightClass, err)
	}

	// 5. Wrestler (match by wrestlestat_id, then create)
	wrestlerID, err := getOrCreateWrestler(ctx, tx, e)
	if err != nil {
		return fmt.Errorf("wrestler: %w", err)
	}

	// 6. Wrestler season
	if err := upsertWrestlerSeason(ctx, tx, wrestlerID, seasonID, schoolID, weightClassID, e); err != nil {
		return fmt.Errorf("wrestler_season: %w", err)
	}

	return tx.Commit()
}

// ---- DB helpers -------------------------------------------------------------

func getOrCreateSeason(ctx context.Context, tx *sql.Tx, year int) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM core.season WHERE year=$1`, year).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO core.season (year, label)
		 VALUES ($1, $2)
		 ON CONFLICT (year) DO UPDATE SET label=EXCLUDED.label
		 RETURNING id`,
		year, fmt.Sprintf("%d-%d", year-1, year),
	).Scan(&id)
	return id, err
}

func getOrCreateSchool(ctx context.Context, tx *sql.Tx, slug, displayName string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx,
		`INSERT INTO core.school (slug, name)
		 VALUES ($1, $2)
		 ON CONFLICT (slug) DO UPDATE SET slug=EXCLUDED.slug
		 RETURNING id`,
		slug, displayName,
	).Scan(&id)
	return id, err
}

func getOrCreateConference(ctx context.Context, tx *sql.Tx, name string) (string, error) {
	slug := wrestlestat.Slugify(name)
	var id string
	err := tx.QueryRowContext(ctx,
		`INSERT INTO core.conference (name, slug)
		 VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name
		 RETURNING id`,
		name, slug,
	).Scan(&id)
	return id, err
}

func linkSchoolConferenceSeason(ctx context.Context, tx *sql.Tx, schoolID, confID, seasonID string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO core.school_conference_season (school_id, conference_id, season_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (school_id, season_id) DO UPDATE SET conference_id=EXCLUDED.conference_id`,
		schoolID, confID, seasonID,
	)
	return err
}

func getWeightClassByLabel(ctx context.Context, tx *sql.Tx, label string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM core.weight_class WHERE lower(label)=lower($1)`, label,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("not found: %w", err)
	}
	return id, nil
}

func getOrCreateWrestler(ctx context.Context, tx *sql.Tx, e *wrestlestat.WrestlerEntry) (string, error) {
	wsid := fmt.Sprintf("%d", e.WrestleStatID)

	// Try to find by wrestlestat_id first
	var id string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM core.wrestler WHERE wrestlestat_id=$1`, wsid,
	).Scan(&id)
	if err == nil {
		// Update name if it has changed
		_, _ = tx.ExecContext(ctx,
			`UPDATE core.wrestler SET full_name=$1 WHERE id=$2`,
			e.Name, id,
		)
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	// Create new wrestler
	slug := wrestlestat.Slugify(e.Name)
	err = tx.QueryRowContext(ctx,
		`INSERT INTO core.wrestler (full_name, slug, wrestlestat_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (slug) DO NOTHING
		 RETURNING id`,
		e.Name, slug, wsid,
	).Scan(&id)
	if err == sql.ErrNoRows {
		// Slug collision — append random suffix
		slug = fmt.Sprintf("%s-%06x", slug, rand.IntN(0xffffff))
		err = tx.QueryRowContext(ctx,
			`INSERT INTO core.wrestler (full_name, slug, wrestlestat_id)
			 VALUES ($1, $2, $3)
			 RETURNING id`,
			e.Name, slug, wsid,
		).Scan(&id)
	}
	return id, err
}

func upsertWrestlerSeason(ctx context.Context, tx *sql.Tx, wrestlerID, seasonID, schoolID, weightClassID string, e *wrestlestat.WrestlerEntry) error {
	var winPct *float64
	if e.Wins+e.Losses > 0 {
		v := float64(e.Wins) / float64(e.Wins+e.Losses) * 100
		winPct = &v
	}

	ncaaFinish := nullIfEmpty(e.NCAAFinish)
	classYear := nullIfEmpty(e.ClassYear)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO core.wrestler_season
			(wrestler_id, season_id, school_id, primary_weight_class_id,
			 class_year, wins, losses, win_percentage, ncaa_finish)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (wrestler_id, season_id) DO UPDATE SET
			school_id               = EXCLUDED.school_id,
			primary_weight_class_id = EXCLUDED.primary_weight_class_id,
			class_year              = EXCLUDED.class_year,
			wins                    = EXCLUDED.wins,
			losses                  = EXCLUDED.losses,
			win_percentage          = EXCLUDED.win_percentage,
			ncaa_finish             = EXCLUDED.ncaa_finish`,
		wrestlerID, seasonID, schoolID, weightClassID,
		classYear, e.Wins, e.Losses, winPct, ncaaFinish,
	)
	return err
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
