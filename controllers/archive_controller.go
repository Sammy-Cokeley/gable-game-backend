package controllers

import (
	"context"
	"fmt"
	"log"

	"gable-backend/database"

	"github.com/gofiber/fiber/v2"
)

// ArchiveSeasonItem describes a single season available for archive play.
type ArchiveSeasonItem struct {
	Year          int    `json:"year"`
	Label         string `json:"label"`
	EarliestDate  string `json:"earliest_date"`
	LatestDate    string `json:"latest_date"`
	WrestlerCount int    `json:"wrestler_count"`
}

// GetArchiveSeasons returns all seasons that have at least one past puzzle in
// daily_wrestlers. Today's puzzle is excluded — it belongs to the live daily mode.
func GetArchiveSeasons(c *fiber.Ctx) error {
	seasons, err := queryArchiveSeasons(c.Context())
	if err != nil {
		log.Printf("GetArchiveSeasons: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.JSON(fiber.Map{"seasons": seasons})
}

func queryArchiveSeasons(ctx context.Context) ([]ArchiveSeasonItem, error) {
	rows, err := database.DB.QueryContext(ctx, `
		SELECT
			se.year,
			se.label,
			MIN(dw.day)::TEXT  AS earliest_date,
			MAX(dw.day)::TEXT  AS latest_date,
			COUNT(dw.day)      AS wrestler_count
		FROM daily_wrestlers dw
		JOIN core.wrestler w         ON w.wrestlestat_id::INT = dw.wrestler_id
		JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
		JOIN core.season se          ON se.id = ws.season_id
		WHERE dw.day < CURRENT_DATE
		GROUP BY se.year, se.label
		ORDER BY se.year DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("queryArchiveSeasons: %w", err)
	}
	defer rows.Close()

	var seasons []ArchiveSeasonItem
	for rows.Next() {
		var s ArchiveSeasonItem
		if err := rows.Scan(&s.Year, &s.Label, &s.EarliestDate, &s.LatestDate, &s.WrestlerCount); err != nil {
			return nil, fmt.Errorf("queryArchiveSeasons scan: %w", err)
		}
		seasons = append(seasons, s)
	}
	return seasons, nil
}
