package controllers

import (
	"log"
	"strconv"
	"time"

	"gable-backend/database"
	"gable-backend/models"

	"github.com/gofiber/fiber/v2"
)

// coreWrestlerQuery is the base SELECT that reads 2026 wrestler attributes from core.*.
// The returned id is the core.wrestler.wrestlestat_id cast to INT for use by the
// game's guess submission endpoint.
const coreWrestlerQuery = `
	SELECT w.wrestlestat_id::INT, wc.label, w.full_name, COALESCE(ws.class_year, ''),
	       sc.name, COALESCE(co.name, ''),
	       COALESCE(ws.win_percentage::TEXT, ''), COALESCE(ws.ncaa_finish, '')
	FROM core.wrestler_season ws
	JOIN core.wrestler w      ON w.id  = ws.wrestler_id
	JOIN core.season se       ON se.id = ws.season_id AND se.year = 2026
	JOIN core.weight_class wc ON wc.id = ws.primary_weight_class_id
	JOIN core.school sc       ON sc.id = ws.school_id
	LEFT JOIN core.school_conference_season scs
	                          ON scs.school_id = sc.id AND scs.season_id = ws.season_id
	LEFT JOIN core.conference co ON co.id = scs.conference_id
	WHERE w.wrestlestat_id IS NOT NULL
`

func scanWrestler(row interface{ Scan(...any) error }) (models.Wrestler, error) {
	var w models.Wrestler
	err := row.Scan(
		&w.ID, &w.WeightClass, &w.Name, &w.Year,
		&w.Team, &w.Conference, &w.WinPercentage, &w.NCAAFinish,
	)
	return w, err
}

func GetWrestlersByQuery(c *fiber.Ctx) error {
	name := c.Query("name")

	if name == "" {
		rows, err := database.DB.Query(coreWrestlerQuery + " ORDER BY w.full_name")
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		var wrestlers []models.Wrestler
		for rows.Next() {
			w, err := scanWrestler(rows)
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}
			wrestlers = append(wrestlers, w)
		}
		return c.JSON(wrestlers)
	}

	w, err := scanWrestler(database.DB.QueryRow(
		coreWrestlerQuery+" AND LOWER(w.full_name) = LOWER($1)", name,
	))
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	return c.JSON(w)
}

func GetDailyWrestler(c *fiber.Ctx) error {
	loc, _ := time.LoadLocation("America/New_York")
	today := time.Now().In(loc).Format("2006-01-02")

	dateStr, err := resolveGameDate(c, today)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	seasonYear, err := seasonYearForDate(c.Context(), dateStr)
	if err != nil {
		log.Printf("GetDailyWrestler: no puzzle for date %s: %v", dateStr, err)
		return c.Status(404).SendString("Wrestler not found for date")
	}

	var legacyID int
	err = database.DB.QueryRowContext(c.Context(),
		"SELECT wrestler_id FROM daily_wrestlers WHERE day = $1::date", dateStr,
	).Scan(&legacyID)
	if err != nil {
		log.Printf("GetDailyWrestler daily_wrestlers: %v", err)
		return c.Status(404).SendString("Wrestler not found for date")
	}

	w, err := scanWrestler(database.DB.QueryRowContext(c.Context(),
		dynamicCoreWrestlerQuery+" AND w.wrestlestat_id = $2",
		seasonYear, strconv.Itoa(legacyID),
	))
	if err != nil {
		log.Printf("GetDailyWrestler core query: %v", err)
		return c.Status(404).SendString("Wrestler not found")
	}
	return c.JSON(w)
}
