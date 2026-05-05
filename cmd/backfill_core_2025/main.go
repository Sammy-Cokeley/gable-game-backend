package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type rowMap map[string]any

type report struct {
	StartedAt   time.Time        `json:"started_at"`
	FinishedAt  time.Time        `json:"finished_at"`
	Summary     map[string]int   `json:"summary"`
	Exceptions  []string         `json:"exceptions"`
	EventID     string           `json:"event_id"`
	SampleBouts []map[string]any `json:"sample_bouts,omitempty"`
}

func main() {
	if os.Getenv("RENDER") == "" {
		_ = godotenv.Load()
	}

	dsn := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dsn) == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	r := &report{StartedAt: time.Now().UTC(), Summary: map[string]int{}}
	if err := runBackfill(db, r); err != nil {
		log.Fatalf("backfill failed: %v", err)
	}
	r.FinishedAt = time.Now().UTC()

	if err := writeReport(r); err != nil {
		log.Fatalf("write report: %v", err)
	}

	log.Printf("backfill complete: %+v", r.Summary)
}

func runBackfill(db *sql.DB, r *report) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	seasonID, err := ensureSeason(tx)
	if err != nil {
		return err
	}

	wrestlers2025, err := fetchTable(tx, "public", "wrestlers_2025")
	if err != nil {
		return fmt.Errorf("fetch wrestlers_2025: %w", err)
	}
	r.Summary["source_wrestlers_2025"] = len(wrestlers2025)

	schools, conferences, schoolConferencePairs := collectDimensions(wrestlers2025)
	schoolIDs, err := upsertNamedDimension(tx, "core.school", schools)
	if err != nil {
		return err
	}
	conferenceIDs, err := upsertNamedDimension(tx, "core.conference", conferences)
	if err != nil {
		return err
	}
	r.Summary["schools"] = len(schoolIDs)
	r.Summary["conferences"] = len(conferenceIDs)

	for pair := range schoolConferencePairs {
		school := pair[:strings.Index(pair, "||")]
		conference := pair[strings.Index(pair, "||")+2:]
		if _, err := tx.Exec(`
			INSERT INTO core.school_conference_season (school_id, conference_id, season_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (school_id, conference_id, season_id) DO NOTHING
		`, schoolIDs[school], conferenceIDs[conference], seasonID); err != nil {
			r.Exceptions = append(r.Exceptions, fmt.Sprintf("school_conference_season %s/%s: %v", school, conference, err))
		}
	}
	r.Summary["school_conference_season"] = len(schoolConferencePairs)

	weightClasses := collectWeightClasses(wrestlers2025)
	weightClassIDs, err := upsertWeightClasses(tx, weightClasses)
	if err != nil {
		return err
	}
	r.Summary["weight_classes"] = len(weightClassIDs)

	profileByName, err := loadProfileByName(tx)
	if err != nil {
		return err
	}
	seasonByWSID, err := loadWrestlerSeasonByID(tx)
	if err != nil {
		return err
	}

	wrestlerIDByLegacy := map[string]uuid.UUID{}
	wrestlerIDByName := map[string]uuid.UUID{}

	for _, wr := range wrestlers2025 {
		legacyID := asString(wr["id"])
		name := strings.TrimSpace(asString(wr["name"]))
		if name == "" {
			r.Exceptions = append(r.Exceptions, fmt.Sprintf("wrestlers_2025 id=%s missing name", legacyID))
			continue
		}
		team := strings.TrimSpace(asString(wr["team"]))
		weightLabel := strings.TrimSpace(asString(wr["weight_class"]))
		classYear := strings.TrimSpace(asString(wr["year"]))

		normName := normalizeName(name)
		p, hasProfile := profileByName[normName]
		var wsID sql.NullInt64
		canonicalName := name
		if hasProfile {
			wsID = sql.NullInt64{Int64: p.WrestleStatID, Valid: true}
			if strings.TrimSpace(p.Name) != "" {
				canonicalName = strings.TrimSpace(p.Name)
			}
		}

		wrestlerID, err := upsertWrestler(tx, wsID, canonicalName)
		if err != nil {
			r.Exceptions = append(r.Exceptions, fmt.Sprintf("wrestler upsert failed legacy=%s name=%s err=%v", legacyID, name, err))
			continue
		}

		wrestlerIDByLegacy[legacyID] = wrestlerID
		wrestlerIDByName[normName] = wrestlerID

		if legacyID != "" {
			_, _ = tx.Exec(`
				INSERT INTO core.legacy_wrestler_map (legacy_table, legacy_id, wrestler_id)
				VALUES ('wrestlers_2025', $1, $2)
				ON CONFLICT (legacy_table, legacy_id) DO UPDATE SET wrestler_id = EXCLUDED.wrestler_id
			`, legacyID, wrestlerID)
		}

		var seasonRec wsSeason
		if wsID.Valid {
			if s, ok := seasonByWSID[wsID.Int64]; ok {
				seasonRec = s
				if seasonRec.ClassYear != "" {
					classYear = seasonRec.ClassYear
				}
				if seasonRec.Team != "" {
					team = seasonRec.Team
				}
			}
		}

		var schoolID any
		if team != "" {
			schoolID = schoolIDs[team]
		}
		var wcID any
		if weightLabel != "" {
			wcID = weightClassIDs[weightLabel]
		}

		wins := nullableInt(seasonRec.Wins)
		losses := nullableInt(seasonRec.Losses)

		_, err = tx.Exec(`
			INSERT INTO core.wrestler_season (wrestler_id, season_id, school_id, weight_class_id, class_year, wins, losses)
			VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7)
			ON CONFLICT (wrestler_id, season_id)
			DO UPDATE SET
				school_id = EXCLUDED.school_id,
				weight_class_id = EXCLUDED.weight_class_id,
				class_year = COALESCE(EXCLUDED.class_year, core.wrestler_season.class_year),
				wins = COALESCE(EXCLUDED.wins, core.wrestler_season.wins),
				losses = COALESCE(EXCLUDED.losses, core.wrestler_season.losses),
				updated_at = NOW()
		`, wrestlerID, seasonID, schoolID, wcID, classYear, wins, losses)
		if err != nil {
			r.Exceptions = append(r.Exceptions, fmt.Sprintf("wrestler_season upsert failed legacy=%s name=%s err=%v", legacyID, name, err))
		}
	}

	var eventID uuid.UUID
	err = tx.QueryRow(`
		INSERT INTO core.event (season_id, name, event_type, start_date, end_date, location)
		VALUES ($1, 'NCAA Championships 2025', 'championship', '2025-03-20', '2025-03-22', 'Philadelphia, PA')
		ON CONFLICT (name, season_id)
		DO UPDATE SET event_type = EXCLUDED.event_type
		RETURNING id
	`, seasonID).Scan(&eventID)
	if err != nil {
		return fmt.Errorf("upsert event: %w", err)
	}
	r.EventID = eventID.String()

	matches, err := fetchTable(tx, "public", "ncaa_matches_2025")
	if err != nil {
		return fmt.Errorf("fetch ncaa_matches_2025: %w", err)
	}
	r.Summary["source_matches_2025"] = len(matches)

	insertedBouts := 0
	for i, m := range matches {
		boutNum := nullableInt(parseInt(m["bout_number"], parseInt(m["match_number"], i+1)))
		weightLabel := chooseNonEmpty(asString(m["weight_class"]), asString(m["weight"]), asString(m["wt"]))
		winnerID := lookupWrestlerID(m, wrestlerIDByLegacy, wrestlerIDByName, "winner")
		loserID := lookupWrestlerID(m, wrestlerIDByLegacy, wrestlerIDByName, "loser")

		sourceMatchID := chooseNonEmpty(asString(m["id"]), strconv.Itoa(i+1))
		rawPayload, _ := json.Marshal(m)

		_, err := tx.Exec(`
			INSERT INTO core.bout (
				event_id, season_id, round, bout_number, weight_class_id,
				wrestler1_id, wrestler2_id, winner_id, result, decision_type, score,
				source_match_id, raw_payload
			) VALUES (
				$1, $2, NULLIF($3,''), $4, $5,
				$6, $7, $8, NULLIF($9,''), NULLIF($10,''), NULLIF($11,''),
				$12, $13
			)
			ON CONFLICT (source_match_id) DO UPDATE SET
				event_id = EXCLUDED.event_id,
				season_id = EXCLUDED.season_id,
				round = EXCLUDED.round,
				bout_number = EXCLUDED.bout_number,
				weight_class_id = EXCLUDED.weight_class_id,
				wrestler1_id = EXCLUDED.wrestler1_id,
				wrestler2_id = EXCLUDED.wrestler2_id,
				winner_id = EXCLUDED.winner_id,
				result = EXCLUDED.result,
				decision_type = EXCLUDED.decision_type,
				score = EXCLUDED.score,
				raw_payload = EXCLUDED.raw_payload
		`, eventID, seasonID, chooseNonEmpty(asString(m["round"]), asString(m["round_name"])), boutNum, weightClassIDs[weightLabel],
			winnerID, loserID, winnerID,
			chooseNonEmpty(asString(m["result"]), asString(m["method"])),
			chooseNonEmpty(asString(m["decision_type"]), asString(m["result_type"])),
			chooseNonEmpty(asString(m["score"]), asString(m["final_score"])),
			sourceMatchID, rawPayload)
		if err != nil {
			r.Exceptions = append(r.Exceptions, fmt.Sprintf("bout upsert failed match=%s err=%v", sourceMatchID, err))
			continue
		}
		insertedBouts++
		if len(r.SampleBouts) < 5 {
			r.SampleBouts = append(r.SampleBouts, map[string]any{"source_match_id": sourceMatchID, "winner_id": winnerID, "loser_id": loserID})
		}
	}
	r.Summary["bouts_upserted"] = insertedBouts
	r.Summary["exceptions"] = len(r.Exceptions)

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func ensureSeason(tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRow(`
		INSERT INTO core.season (code, start_date, end_date)
		VALUES ('2025', '2024-11-01', '2025-03-31')
		ON CONFLICT (code)
		DO UPDATE SET code = EXCLUDED.code
		RETURNING id
	`).Scan(&id)
	return id, err
}

func fetchTable(tx *sql.Tx, schema, table string) ([]rowMap, error) {
	cols, err := tableColumns(tx, schema, table)
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table %s.%s has no columns", schema, table)
	}
	quoted := make([]string, 0, len(cols))
	for _, c := range cols {
		quoted = append(quoted, pqQuoteIdentifier(c))
	}
	q := fmt.Sprintf("SELECT %s FROM %s.%s", strings.Join(quoted, ", "), pqQuoteIdentifier(schema), pqQuoteIdentifier(table))
	rows, err := tx.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []rowMap
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := rowMap{}
		for i, c := range cols {
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func tableColumns(tx *sql.Tx, schema, table string) ([]string, error) {
	rows, err := tx.Query(`
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func collectDimensions(rows []rowMap) (map[string]struct{}, map[string]struct{}, map[string]struct{}) {
	schools := map[string]struct{}{}
	conferences := map[string]struct{}{}
	pairs := map[string]struct{}{}
	for _, r := range rows {
		school := strings.TrimSpace(asString(r["team"]))
		conference := strings.TrimSpace(asString(r["conference"]))
		if school != "" {
			schools[school] = struct{}{}
		}
		if conference != "" {
			conferences[conference] = struct{}{}
		}
		if school != "" && conference != "" {
			pairs[school+"||"+conference] = struct{}{}
		}
	}
	return schools, conferences, pairs
}

func collectWeightClasses(rows []rowMap) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		label := strings.TrimSpace(asString(r["weight_class"]))
		if label == "" {
			continue
		}
		out[label] = parseLeadingInt(label)
	}
	return out
}

func upsertNamedDimension(tx *sql.Tx, table string, values map[string]struct{}) (map[string]int64, error) {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ids := map[string]int64{}
	for _, name := range keys {
		var id int64
		q := fmt.Sprintf(`
			INSERT INTO %s (name, slug)
			VALUES ($1, $2)
			ON CONFLICT (name)
			DO UPDATE SET slug = EXCLUDED.slug
			RETURNING id
		`, table)
		if err := tx.QueryRow(q, name, slugify(name)).Scan(&id); err != nil {
			return nil, err
		}
		ids[name] = id
	}
	return ids, nil
}

func upsertWeightClasses(tx *sql.Tx, values map[string]int) (map[string]int64, error) {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ids := map[string]int64{}
	for _, label := range keys {
		var id int64
		if err := tx.QueryRow(`
			INSERT INTO core.weight_class (label, pounds)
			VALUES ($1, NULLIF($2, 0))
			ON CONFLICT (label)
			DO UPDATE SET pounds = EXCLUDED.pounds
			RETURNING id
		`, label, values[label]).Scan(&id); err != nil {
			return nil, err
		}
		ids[label] = id
	}
	return ids, nil
}

type wrestlerProfile struct {
	WrestleStatID int64
	Name          string
}

type wsSeason struct {
	ClassYear string
	Team      string
	Wins      int
	Losses    int
}

func loadProfileByName(tx *sql.Tx) (map[string]wrestlerProfile, error) {
	rows, err := tx.Query(`SELECT wrestlestat_id, name FROM public.wrestler_profiles`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]wrestlerProfile{}
	for rows.Next() {
		var p wrestlerProfile
		if err := rows.Scan(&p.WrestleStatID, &p.Name); err != nil {
			return nil, err
		}
		out[normalizeName(p.Name)] = p
	}
	return out, rows.Err()
}

func loadWrestlerSeasonByID(tx *sql.Tx) (map[int64]wsSeason, error) {
	rows, err := tx.Query(`
		SELECT wrestlestat_id, class_year, team, COALESCE(wins,0), COALESCE(losses,0)
		FROM public.wrestler_seasons
		WHERE season = '2025'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]wsSeason{}
	for rows.Next() {
		var id int64
		var s wsSeason
		if err := rows.Scan(&id, &s.ClassYear, &s.Team, &s.Wins, &s.Losses); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

func upsertWrestler(tx *sql.Tx, wrestlestatID sql.NullInt64, canonicalName string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(`
		INSERT INTO core.wrestler (wrestlestat_id, canonical_name)
		VALUES ($1, $2)
		ON CONFLICT (wrestlestat_id)
		DO UPDATE SET canonical_name = EXCLUDED.canonical_name, updated_at = NOW()
		RETURNING id
	`, wrestlestatID, canonicalName).Scan(&id)
	if err == nil {
		return id, nil
	}
	if wrestlestatID.Valid {
		return uuid.Nil, err
	}
	// Name-only fallback path.
	err = tx.QueryRow(`
		SELECT id FROM core.wrestler
		WHERE wrestlestat_id IS NULL AND lower(canonical_name) = lower($1)
		LIMIT 1
	`, canonicalName).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return uuid.Nil, err
	}
	err = tx.QueryRow(`
		INSERT INTO core.wrestler (wrestlestat_id, canonical_name)
		VALUES (NULL, $1)
		RETURNING id
	`, canonicalName).Scan(&id)
	return id, err
}

func lookupWrestlerID(match rowMap, byLegacy map[string]uuid.UUID, byName map[string]uuid.UUID, prefix string) any {
	for _, col := range []string{prefix + "_id", prefix + "_wrestler_id"} {
		legacyID := strings.TrimSpace(asString(match[col]))
		if legacyID == "" {
			continue
		}
		if id, ok := byLegacy[legacyID]; ok {
			return id
		}
	}
	for _, col := range []string{prefix + "_name", prefix} {
		name := strings.TrimSpace(asString(match[col]))
		if name == "" {
			continue
		}
		if id, ok := byName[normalizeName(name)]; ok {
			return id
		}
	}
	return nil
}

func writeReport(r *report) error {
	if err := os.MkdirAll("reports", 0o755); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102_150405")
	jsonPath := filepath.Join("reports", "backfill_core_2025_summary_"+stamp+".json")
	exPath := filepath.Join("reports", "backfill_core_2025_exceptions_"+stamp+".log")

	blob, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, blob, 0o644); err != nil {
		return err
	}

	exText := strings.Join(r.Exceptions, "\n")
	if exText == "" {
		exText = "No exceptions captured."
	}
	if err := os.WriteFile(exPath, []byte(exText+"\n"), 0o644); err != nil {
		return err
	}

	fmt.Printf("Summary report: %s\n", jsonPath)
	fmt.Printf("Exceptions report: %s\n", exPath)
	return nil
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

func parseInt(v any, fallback int) int {
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func parseLeadingInt(label string) int {
	re := regexp.MustCompile(`\d+`)
	match := re.FindString(label)
	if match == "" {
		return 0
	}
	n, _ := strconv.Atoi(match)
	return n
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ".", "")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "&", " and ")
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func pqQuoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func chooseNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func nullableInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}
