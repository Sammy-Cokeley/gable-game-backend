package controllers

import (
	"database/sql"
	"log"
	"strconv"
	"strings"

	"gable-backend/database"

	"github.com/gofiber/fiber/v2"
)

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

type RankingEntryResponse struct {
	Rank         int    `json:"rank"`
	PreviousRank *int   `json:"previous_rank"`
	WrestlerID   string `json:"wrestler_id"`
	WrestlerName string `json:"wrestler_name"`
	WrestlerSlug string `json:"wrestler_slug"`
	School       string `json:"school"`
	SchoolSlug   string `json:"school_slug"`
	WeightClass  string `json:"weight_class"`
	SourceName   string `json:"source"`
	SourceSlug   string `json:"source_slug"`
	SnapshotDate string `json:"snapshot_date"`
	SeasonYear   int    `json:"season_year"`
}

type RankingHistoryEntry struct {
	Rank         int    `json:"rank"`
	PreviousRank *int   `json:"previous_rank"`
	SnapshotDate string `json:"snapshot_date"`
	WeightClass  string `json:"weight_class"`
	SourceName   string `json:"source"`
	SourceSlug   string `json:"source_slug"`
	SeasonYear   int    `json:"season_year"`
}

// ---------------------------------------------------------------------------
// GET /api/v1/rankings
// Query params: season (year), source (slug), date (YYYY-MM-DD), weight_class
// Returns all published ranking entries matching the filters.
// ---------------------------------------------------------------------------
func V1GetRankings(c *fiber.Ctx) error {
	args := []interface{}{}
	where := []string{"rs.status = 'published'"}
	i := 1

	if s := c.Query("season"); s != "" {
		args = append(args, s)
		where = append(where, "se.year = $"+itoa(i))
		i++
	}
	if src := c.Query("source"); src != "" {
		args = append(args, src)
		where = append(where, "src.slug = $"+itoa(i))
		i++
	}
	if d := c.Query("date"); d != "" {
		args = append(args, d)
		where = append(where, "rs.snapshot_date = $"+itoa(i))
		i++
	}
	if wc := c.Query("weight_class"); wc != "" {
		args = append(args, wc)
		where = append(where, "wc.label = $"+itoa(i))
		i++
	}

	whereClause := "WHERE " + strings.Join(where, " AND ")

	rows, err := database.DB.Query(`
		SELECT
			re.rank, re.previous_rank,
			w.id, w.full_name, w.slug,
			sc.name, sc.slug,
			wc.label,
			src.name, src.slug,
			rs.snapshot_date::TEXT,
			se.year
		FROM core.ranking_entry re
		JOIN core.ranking_snapshot rs ON rs.id = re.snapshot_id
		JOIN core.ranking_source src ON src.id = rs.source_id
		JOIN core.wrestler w ON w.id = re.wrestler_id
		JOIN core.weight_class wc ON wc.id = rs.weight_class_id
		JOIN core.season se ON se.id = rs.season_id
		LEFT JOIN core.wrestler_season ws ON ws.wrestler_id = w.id AND ws.season_id = rs.season_id
		LEFT JOIN core.school sc ON sc.id = ws.school_id
		`+whereClause+`
		ORDER BY se.year DESC, rs.snapshot_date DESC, wc.sort_order, re.rank
	`, args...)
	if err != nil {
		log.Printf("platform_rankings error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	entries := []RankingEntryResponse{}
	for rows.Next() {
		var e RankingEntryResponse
		var school, schoolSlug sql.NullString
		if err := rows.Scan(
			&e.Rank, &e.PreviousRank,
			&e.WrestlerID, &e.WrestlerName, &e.WrestlerSlug,
			&school, &schoolSlug,
			&e.WeightClass,
			&e.SourceName, &e.SourceSlug,
			&e.SnapshotDate, &e.SeasonYear,
		); err != nil {
			log.Printf("platform_rankings error: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		if school.Valid {
			e.School = school.String
		}
		if schoolSlug.Valid {
			e.SchoolSlug = schoolSlug.String
		}
		entries = append(entries, e)
	}

	return c.JSON(entries)
}

// ---------------------------------------------------------------------------
// GET /api/v1/rankings/history/:wrestler_slug
// Returns every published ranking entry for this wrestler across all sources
// and seasons, most recent first.
// ---------------------------------------------------------------------------
func V1GetWrestlerRankingHistory(c *fiber.Ctx) error {
	wrestlerID := c.Params("wrestler_id")

	// Confirm wrestler exists
	var exists bool
	err := database.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM core.wrestler WHERE id = $1)`, wrestlerID,
	).Scan(&exists)
	if err != nil {
		log.Printf("platform_rankings error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "wrestler not found"})
	}

	rows, err := database.DB.Query(`
		SELECT
			re.rank, re.previous_rank,
			rs.snapshot_date::TEXT,
			wc.label,
			src.name, src.slug,
			se.year
		FROM core.ranking_entry re
		JOIN core.ranking_snapshot rs ON rs.id = re.snapshot_id
		JOIN core.ranking_source src ON src.id = rs.source_id
		JOIN core.weight_class wc ON wc.id = rs.weight_class_id
		JOIN core.season se ON se.id = rs.season_id
		WHERE re.wrestler_id = $1
		  AND rs.status = 'published'
		ORDER BY rs.snapshot_date DESC, src.name
	`, wrestlerID)
	if err != nil {
		log.Printf("platform_rankings error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	history := []RankingHistoryEntry{}
	for rows.Next() {
		var e RankingHistoryEntry
		if err := rows.Scan(
			&e.Rank, &e.PreviousRank,
			&e.SnapshotDate, &e.WeightClass,
			&e.SourceName, &e.SourceSlug,
			&e.SeasonYear,
		); err != nil {
			log.Printf("platform_rankings error: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		history = append(history, e)
	}

	return c.JSON(history)
}

// itoa converts an int to its decimal string representation.
func itoa(i int) string {
	return strconv.Itoa(i)
}
