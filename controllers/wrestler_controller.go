package controllers

import (
	"log"
	"time"

	"gable-backend/database"
	"gable-backend/models"

	"github.com/gofiber/fiber/v2"
)

// coreWrestlerQuery is the base SELECT that reads wrestler attributes from core.*,
// bridging through core.legacy_wrestler_map so the returned id is still the
// legacy wrestlers_2025 INT id that the game's guess submission expects.
const coreWrestlerQuery = `
	SELECT lm.legacy_id::INT, wc.label, w.full_name, COALESCE(ws.class_year, ''),
	       sc.name, COALESCE(co.name, ''),
	       COALESCE(ws.win_percentage::TEXT, ''), COALESCE(ws.ncaa_finish, '')
	FROM core.legacy_wrestler_map lm
	JOIN core.wrestler w          ON w.id  = lm.wrestler_id
	JOIN core.wrestler_season ws  ON ws.wrestler_id = w.id
	JOIN core.season se           ON se.id = ws.season_id AND se.year = 2025
	JOIN core.weight_class wc     ON wc.id = ws.primary_weight_class_id
	JOIN core.school sc           ON sc.id = ws.school_id
	LEFT JOIN core.school_conference_season scs
	                              ON scs.school_id = sc.id AND scs.season_id = ws.season_id
	LEFT JOIN core.conference co  ON co.id = scs.conference_id
	WHERE lm.legacy_table = 'wrestlers_2025'
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

	var legacyID int
	err := database.DB.QueryRow(
		"SELECT wrestler_id FROM daily_wrestlers WHERE day = $1::date", today,
	).Scan(&legacyID)
	if err != nil {
		log.Println("Error querying daily_wrestlers:", err)
		return c.Status(404).SendString("Wrestler not found for today")
	}

	w, err := scanWrestler(database.DB.QueryRow(
		coreWrestlerQuery+" AND lm.legacy_id = $1::TEXT", legacyID,
	))
	if err != nil {
		log.Println("Error fetching daily wrestler from core:", err)
		return c.Status(404).SendString("Wrestler not found")
	}
	return c.JSON(w)
}
