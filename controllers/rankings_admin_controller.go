package controllers

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"gable-backend/database"

	"github.com/gofiber/fiber/v2"
	"github.com/lib/pq"
)

type CreateReleaseRequest struct {
	Source string `json:"source"`
	Season string `json:"season"`
	WeekOf string `json:"weekOf"` // YYYY-MM-DD
}

type ImportStagingRequest struct {
	WeightClass int    `json:"weightClass"`
	RawText     string `json:"rawText"`
}

type AttachRequest struct {
	WrestlestatID int `json:"wrestlestatId"`
}

type ReleaseRow struct {
	ID          int        `json:"id"`
	Source      string     `json:"source"`
	Season      string     `json:"season"`
	WeekOf      string     `json:"weekOf"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	PublishedAt *time.Time `json:"publishedAt"`
}

type StagingRow struct {
	ID            int    `json:"id"`
	ReleaseID     int    `json:"releaseId"`
	WeightClass   int    `json:"weightClass"`
	Rank          int    `json:"rank"`
	Name          string `json:"name"`
	School        string `json:"school"`
	PreviousRank  *int   `json:"previousRank"`
	WrestlestatID *int   `json:"wrestlestatId"`
	RowStatus     string `json:"status"`
	CreatedAt     string `json:"createdAt"`
}

type attachItem struct {
	RowID         int `json:"rowId"`
	WrestleStatID int `json:"wrestlestatId"`
}

type attachReq struct {
	Items []attachItem `json:"items"`
}

func CreateRankingsRelease(c *fiber.Ctx) error {
	var req CreateReleaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	req.Source = strings.TrimSpace(req.Source)
	req.Season = strings.TrimSpace(req.Season)
	req.WeekOf = strings.TrimSpace(req.WeekOf)

	if req.Source == "" || req.Season == "" || req.WeekOf == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "source, season, and weekOf are required"})
	}

	// validate weekOf date
	if _, err := time.Parse("2006-01-02", req.WeekOf); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "weekOf must be YYYY-MM-DD"})
	}

	var releaseID int
	err := database.DB.QueryRow(`
		INSERT INTO rankings_releases (source, season, week_of)
		VALUES ($1, $2, $3::date)
		RETURNING id
	`, req.Source, req.Season, req.WeekOf).Scan(&releaseID)

	if err != nil {
		// Unique constraint: (source, season, week_of)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Release already exists for this source/season/weekOf"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create release"})
	}

	return GetRankingsReleaseDetailByID(c, releaseID)
}

func ListRankingsReleases(c *fiber.Ctx) error {
	rows, err := database.DB.Query(`
		SELECT
			r.id, r.source, r.season, r.week_of, r.status, r.created_at, r.published_at,
			COALESCE(COUNT(s.id), 0) AS total,
			COALESCE(SUM(CASE WHEN s.row_status = 'resolved' THEN 1 ELSE 0 END), 0) AS resolved,
			COALESCE(SUM(CASE WHEN s.row_status = 'needs_review' THEN 1 ELSE 0 END), 0) AS needs_review
		FROM rankings_releases r
		LEFT JOIN rankings_release_staging_rows s
			ON s.release_id = r.id
		GROUP BY r.id
		ORDER BY r.week_of DESC, r.id DESC
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list releases"})
	}
	defer rows.Close()

	type ReleaseSummary struct {
		ID          int        `json:"id"`
		Source      string     `json:"source"`
		Season      string     `json:"season"`
		WeekOf      string     `json:"weekOf"`
		Status      string     `json:"status"`
		CreatedAt   time.Time  `json:"createdAt"`
		PublishedAt *time.Time `json:"publishedAt"`
		Total       int        `json:"total"`
		Resolved    int        `json:"resolved"`
		NeedsReview int        `json:"needsReview"`
	}

	summaries := make([]ReleaseSummary, 0)
	for rows.Next() {
		var s ReleaseSummary
		var weekOf time.Time
		if err := rows.Scan(&s.ID, &s.Source, &s.Season, &weekOf, &s.Status, &s.CreatedAt, &s.PublishedAt, &s.Total, &s.Resolved, &s.NeedsReview); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to parse releases"})
		}
		s.WeekOf = weekOf.Format("2006-01-02")
		summaries = append(summaries, s)
	}

	return c.JSON(summaries)
}

func GetRankingsReleaseDetail(c *fiber.Ctx) error {
	idStr := c.Params("id")
	releaseID, err := strconv.Atoi(idStr)
	if err != nil || releaseID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid release id"})
	}
	return GetRankingsReleaseDetailByID(c, releaseID)
}

func GetRankingsReleaseDetailByID(c *fiber.Ctx, releaseID int) error {
	var r ReleaseRow
	var weekOf time.Time
	err := database.DB.QueryRow(`
		SELECT id, source, season, week_of, status, created_at, published_at
		FROM rankings_releases
		WHERE id = $1
	`, releaseID).Scan(&r.ID, &r.Source, &r.Season, &weekOf, &r.Status, &r.CreatedAt, &r.PublishedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Release not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch release"})
	}
	r.WeekOf = weekOf.Format("2006-01-02")

	// counts
	var total, resolved, needsReview int
	err = database.DB.QueryRow(`
		SELECT
			COALESCE(COUNT(*), 0) AS total,
			COALESCE(SUM(CASE WHEN row_status = 'resolved' THEN 1 ELSE 0 END), 0) AS resolved,
			COALESCE(SUM(CASE WHEN row_status = 'needs_review' THEN 1 ELSE 0 END), 0) AS needs_review
		FROM rankings_release_staging_rows
		WHERE release_id = $1
	`, releaseID).Scan(&total, &resolved, &needsReview)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch counts"})
	}

	// staging rows
	stagingRows, err := fetchStagingRows(releaseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch staging rows"})
	}

	return c.JSON(fiber.Map{
		"release": r,
		"counts": fiber.Map{
			"total":       total,
			"resolved":    resolved,
			"needsReview": needsReview,
		},
		"stagingRows": stagingRows,
	})
}

func ImportRankingsStaging(c *fiber.Ctx) error {
	releaseID, err := strconv.Atoi(c.Params("id"))
	if err != nil || releaseID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid release id"})
	}

	var status string
	if err := database.DB.QueryRow(`SELECT status FROM rankings_releases WHERE id = $1`, releaseID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Release not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to validate release"})
	}
	if status != "draft" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Cannot import into a published release"})
	}

	var req ImportStagingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}
	if req.WeightClass <= 0 || strings.TrimSpace(req.RawText) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "weightClass and rawText are required"})
	}

	parsed, parseErrs := parseRankingsPaste(req.RawText)
	if len(parseErrs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Paste parsing failed",
			"details": parseErrs,
		})
	}
	if len(parsed) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "No rows parsed"})
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to start transaction"})
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO rankings_release_staging_rows
			(release_id, weight_class, rank, name, school, previous_rank, row_status)
		VALUES
			($1, $2, $3, $4, $5, $6, 'unresolved')
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to prepare insert"})
	}
	defer stmt.Close()

	for _, row := range parsed {
		_, err := stmt.Exec(releaseID, req.WeightClass, row.Rank, row.Name, row.School, row.PreviousRank)
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": "Duplicate rank exists for this release/weight class",
				})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to insert staging rows"})
		}
	}

	if err := tx.Commit(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to commit staging import"})
	}

	return GetRankingsReleaseDetailByID(c, releaseID)
}

func ClearRankingsStagingForWeight(c *fiber.Ctx) error {
	releaseID, err := strconv.Atoi(c.Params("id"))
	if err != nil || releaseID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid release id"})
	}

	weightStr := strings.TrimSpace(c.Query("weightClass"))
	weightClass, err := strconv.Atoi(weightStr)
	if err != nil || weightClass <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "weightClass query param is required and must be an integer"})
	}

	// Ensure release exists and is draft
	var status string
	if err := database.DB.QueryRow(`SELECT status FROM rankings_releases WHERE id = $1`, releaseID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Release not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to validate release"})
	}
	if status != "draft" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Cannot clear staging rows for a published release"})
	}

	_, err = database.DB.Exec(`
		DELETE FROM rankings_release_staging_rows
		WHERE release_id = $1 AND weight_class = $2
	`, releaseID, weightClass)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to clear staging rows"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

func AttachWrestleStatIDs(c *fiber.Ctx) error {
	var req attachReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}
	if len(req.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "items is required"})
	}

	// Validate items
	for _, it := range req.Items {
		if it.RowID <= 0 || it.WrestleStatID <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Each item must include positive rowId and wrestlestatId",
			})
		}
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to start transaction"})
	}
	defer tx.Rollback()

	// NOTE: if your column is row_status instead of status, change it here.
	stmt, err := tx.Prepare(`
		UPDATE rankings_release_staging_rows
		SET wrestlestat_id = $1,
		    row_status = 'resolved'
		WHERE id = $2
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to prepare update"})
	}
	defer stmt.Close()

	type itemResult struct {
		RowID int    `json:"rowId"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}

	results := make([]itemResult, 0, len(req.Items))

	for _, it := range req.Items {
		res, err := stmt.Exec(it.WrestleStatID, it.RowID)
		if err != nil {
			results = append(results, itemResult{RowID: it.RowID, OK: false, Error: err.Error()})
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			results = append(results, itemResult{RowID: it.RowID, OK: false, Error: "No row updated (invalid rowId?)"})
			continue
		}
		results = append(results, itemResult{RowID: it.RowID, OK: true})
	}

	if err := tx.Commit(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to commit attach"})
	}

	okCount := 0
	for _, r := range results {
		if r.OK {
			okCount++
		}
	}

	return c.JSON(fiber.Map{
		"ok":      okCount,
		"failed":  len(results) - okCount,
		"results": results,
	})
}

func PublishRankingsRelease(c *fiber.Ctx) error {
	releaseID, err := strconv.Atoi(c.Params("id"))
	if err != nil || releaseID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid release id"})
	}

	// Ensure release exists and is draft
	var status string
	err = database.DB.QueryRow(`SELECT status FROM rankings_releases WHERE id = $1`, releaseID).Scan(&status)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Release not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to validate release"})
	}
	if status != "draft" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Release is not in draft status"})
	}

	// Verify all staging rows resolved (and at least one row exists)
	var total, unresolved int
	err = database.DB.QueryRow(`
		SELECT
			COUNT(*) AS total,
			SUM(CASE WHEN (wrestlestat_id IS NULL OR row_status <> 'resolved') THEN 1 ELSE 0 END) AS unresolved
		FROM rankings_release_staging_rows
		WHERE release_id = $1
	`, releaseID).Scan(&total, &unresolved)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to validate staging rows"})
	}
	if total == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot publish an empty release"})
	}
	if unresolved > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Cannot publish: unresolved staging rows remain"})
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to start transaction"})
	}
	defer tx.Rollback()

	// Insert published entries
	_, err = tx.Exec(`
		INSERT INTO rankings_release_entries (release_id, weight_class, rank, previous_rank, wrestlestat_id)
		SELECT release_id, weight_class, rank, previous_rank, wrestlestat_id
		FROM rankings_release_staging_rows
		WHERE release_id = $1
	`, releaseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to write published entries"})
	}

	// Mark release published
	_, err = tx.Exec(`
		UPDATE rankings_releases
		SET status = 'published',
		    published_at = NOW()
		WHERE id = $1
	`, releaseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update release status"})
	}

	if err := tx.Commit(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to publish release"})
	}

	return GetRankingsReleaseDetailByID(c, releaseID)
}

// ------------------ helpers ------------------

type parsedPasteRow struct {
	Rank         int
	Name         string
	School       string
	PreviousRank *int
}

// ------------------ Paste parser (Flo-friendly) ------------------

func parseRankingsPaste(raw string) ([]parsedPasteRow, []string) {
	lines := strings.Split(raw, "\n")
	out := make([]parsedPasteRow, 0, len(lines))
	errs := make([]string, 0)

	seenRank := map[int]bool{}

	for _, ln := range lines {
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}

		// Skip header-like lines
		lower := strings.ToLower(line)
		if strings.Contains(lower, "rank") && strings.Contains(lower, "school") {
			continue
		}

		// Prefer tab-separated parsing (best for copied tables)
		if strings.Contains(line, "\t") {
			parts := trimAll(strings.Split(line, "\t"))

			// Supported shapes:
			// Flo:   [rank, grade, name, school, prev]
			// Basic: [rank, name, school, prev]
			if len(parts) == 5 || len(parts) == 4 {
				rank, err := strconv.Atoi(parts[0])
				if err != nil || rank <= 0 {
					errs = append(errs, `Invalid rank in line: "`+line+`"`)
					continue
				}
				if seenRank[rank] {
					errs = append(errs, `Duplicate rank in pasted block: `+strconv.Itoa(rank))
					continue
				}
				seenRank[rank] = true

				var name, school, prevRaw string
				if len(parts) == 5 {
					// rank, grade, name, school, prev
					name = parts[2]
					school = parts[3]
					prevRaw = parts[4]
				} else {
					// rank, name, school, prev
					name = parts[1]
					school = parts[2]
					prevRaw = parts[3]
				}

				name = strings.TrimSpace(name)
				school = strings.TrimSpace(school)

				if name == "" || school == "" {
					errs = append(errs, `Missing name/school in line: "`+line+`"`)
					continue
				}

				prevPtr, perr := parsePreviousRank(prevRaw, line)
				if perr != "" {
					errs = append(errs, perr)
					continue
				}

				out = append(out, parsedPasteRow{
					Rank:         rank,
					Name:         name,
					School:       school,
					PreviousRank: prevPtr,
				})
				continue
			}

			// If tabbed but unexpected columns, fall through to heuristic parsing below.
		}

		// Fallback: whitespace token parsing (less reliable)
		toks := trimAll(strings.Fields(line))
		if len(toks) < 4 {
			errs = append(errs, `Could not parse line (need at least 4 tokens): "`+line+`"`)
			continue
		}

		rank, err := strconv.Atoi(toks[0])
		if err != nil || rank <= 0 {
			errs = append(errs, `Invalid rank in line: "`+line+`"`)
			continue
		}
		if seenRank[rank] {
			errs = append(errs, `Duplicate rank in pasted block: `+strconv.Itoa(rank))
			continue
		}
		seenRank[rank] = true

		// last token is previous
		prevRaw := toks[len(toks)-1]
		prevPtr, perr := parsePreviousRank(prevRaw, line)
		if perr != "" {
			errs = append(errs, perr)
			continue
		}

		// Remove rank and previous
		core := toks[1 : len(toks)-1]

		// Drop grade if present (SO/SR/JR/FR/RS/etc.)
		if len(core) > 0 && looksLikeGrade(core[0]) {
			core = core[1:]
		}
		if len(core) < 2 {
			errs = append(errs, `Could not parse name/school in line: "`+line+`"`)
			continue
		}

		// Heuristic: school is last 2 tokens if possible, otherwise last 1.
		schoolTokens := 2
		if len(core)-schoolTokens < 2 {
			schoolTokens = 1
		}

		name := strings.Join(core[:len(core)-schoolTokens], " ")
		school := strings.Join(core[len(core)-schoolTokens:], " ")

		name = strings.TrimSpace(name)
		school = strings.TrimSpace(school)
		if name == "" || school == "" {
			errs = append(errs, `Missing name/school in line: "`+line+`"`)
			continue
		}

		out = append(out, parsedPasteRow{
			Rank:         rank,
			Name:         name,
			School:       school,
			PreviousRank: prevPtr,
		})
	}

	return out, errs
}

func parsePreviousRank(prevRaw string, line string) (*int, string) {
	prevRaw = strings.TrimSpace(prevRaw)
	if strings.EqualFold(prevRaw, "NR") {
		return nil, ""
	}
	prev, err := strconv.Atoi(prevRaw)
	if err != nil {
		return nil, `Invalid previous rank (use integer or NR) in line: "` + line + `"`
	}
	return &prev, ""
}

func looksLikeGrade(s string) bool {
	x := strings.ToUpper(strings.TrimSpace(s))
	// normalize common variants like "R-SO", "RS-SO", "So.", etc.
	x = strings.ReplaceAll(x, "-", "")
	x = strings.ReplaceAll(x, ".", "")
	switch x {
	case "FR", "SO", "JR", "SR", "RS", "RFR", "RSFR", "RSSO", "RSJR", "RSSR":
		return true
	default:
		return false
	}
}

func trimAll(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func fetchStagingRows(releaseID int) ([]StagingRow, error) {
	rows, err := database.DB.Query(`
		SELECT id, release_id, weight_class, rank, name, school, previous_rank, wrestlestat_id, row_status, created_at
		FROM rankings_release_staging_rows
		WHERE release_id = $1
		ORDER BY weight_class ASC, rank ASC
	`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]StagingRow, 0)
	for rows.Next() {
		var r StagingRow
		var prev sql.NullInt32
		var ws sql.NullInt32
		var createdAt time.Time

		if err := rows.Scan(&r.ID, &r.ReleaseID, &r.WeightClass, &r.Rank, &r.Name, &r.School, &prev, &ws, &r.RowStatus, &createdAt); err != nil {
			return nil, err
		}

		if prev.Valid {
			v := int(prev.Int32)
			r.PreviousRank = &v
		}
		if ws.Valid {
			v := int(ws.Int32)
			r.WrestlestatID = &v
		}

		r.CreatedAt = createdAt.Format(time.RFC3339)
		// Frontend expects "status" field naming; keep consistent with earlier scaffolds:
		r.RowStatus = normalizeRowStatus(r.RowStatus)

		out = append(out, StagingRow{
			ID:            r.ID,
			ReleaseID:     r.ReleaseID,
			WeightClass:   r.WeightClass,
			Rank:          r.Rank,
			Name:          r.Name,
			School:        r.School,
			PreviousRank:  r.PreviousRank,
			WrestlestatID: r.WrestlestatID,
			RowStatus:     r.RowStatus,
			CreatedAt:     r.CreatedAt,
		})
	}

	return out, nil
}

func normalizeRowStatus(s string) string {
	// keep as-is, but enforce expected values
	switch s {
	case "unresolved", "resolved", "needs_review":
		return s
	default:
		return "unresolved"
	}
}
