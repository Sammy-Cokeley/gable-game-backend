package controllers

import (
	"context"
	"fmt"
	"time"

	"gable-backend/database"

	"github.com/gofiber/fiber/v2"
)

// dynamicCoreWrestlerQuery is the base SELECT for wrestler attributes, parameterized
// on season year as $1. Additional predicates must use $2, $3, etc.
const dynamicCoreWrestlerQuery = `
	SELECT w.wrestlestat_id::INT, wc.label, w.full_name, COALESCE(ws.class_year, ''),
	       sc.name, COALESCE(co.name, ''),
	       COALESCE(ws.win_percentage::TEXT, ''), COALESCE(ws.ncaa_finish, '')
	FROM core.wrestler_season ws
	JOIN core.wrestler w      ON w.id  = ws.wrestler_id
	JOIN core.season se       ON se.id = ws.season_id AND se.year = $1
	JOIN core.weight_class wc ON wc.id = ws.primary_weight_class_id
	JOIN core.school sc       ON sc.id = ws.school_id
	LEFT JOIN core.school_conference_season scs
	                          ON scs.school_id = sc.id AND scs.season_id = ws.season_id
	LEFT JOIN core.conference co ON co.id = scs.conference_id
	WHERE w.wrestlestat_id IS NOT NULL
`

// resolveGameDate returns the date to use for a game request.
// If the ?date= param is absent, today (ET) is returned.
// If present, it must be YYYY-MM-DD and must be strictly before today.
func resolveGameDate(c *fiber.Ctx, today string) (string, error) {
	dateParam := c.Query("date")
	if dateParam == "" {
		return today, nil
	}
	if _, err := time.Parse("2006-01-02", dateParam); err != nil {
		return "", fmt.Errorf("invalid date format, use YYYY-MM-DD")
	}
	if dateParam >= today {
		return "", fmt.Errorf("date must be in the past for archive mode")
	}
	return dateParam, nil
}

// seasonYearForDate returns the core.season.year for the season whose wrestlers
// are scheduled on the given date in daily_wrestlers.
// Returns an error if no puzzle exists for that date.
func seasonYearForDate(ctx context.Context, dateStr string) (int, error) {
	var year int
	err := database.DB.QueryRowContext(ctx, `
		SELECT se.year
		FROM daily_wrestlers dw
		JOIN core.wrestler w         ON w.wrestlestat_id::INT = dw.wrestler_id
		JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
		JOIN core.season se          ON se.id = ws.season_id
		WHERE dw.day = $1::date
		ORDER BY se.year ASC
		LIMIT 1
	`, dateStr).Scan(&year)
	if err != nil {
		return 0, fmt.Errorf("seasonYearForDate %s: %w", dateStr, err)
	}
	return year, nil
}
