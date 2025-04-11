package controllers

import (
	"log"
	"time"

	"gable-backend/database"
	"gable-backend/models"
	"github.com/gofiber/fiber/v2"
)

func GetWrestlersByQuery(c *fiber.Ctx) error {
	log.Println("Getting daily wrestler")

	name := c.Query("name")

	if name == "" {
		rows, err := database.DB.Query("SELECT * FROM wrestlers_2025")
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()
	
		var wrestlers []models.Wrestler
		for rows.Next() {
			var w models.Wrestler
			if err := rows.Scan(&w.ID, &w.WeightClass, &w.Name, &w.Year, &w.Team, &w.Conference, &w.WinPercentage, &w.NCAAFinish); err != nil {
				return c.Status(500).SendString(err.Error())
			}
			wrestlers = append(wrestlers, w)
		}
		return c.JSON(wrestlers)
	}

	var w models.Wrestler
	err := database.DB.QueryRow("SELECT * FROM wrestlers_2025 WHERE LOWER(name) = LOWER($1)", name).
		Scan(&w.ID, &w.WeightClass, &w.Name, &w.Year, &w.Team, &w.Conference, &w.WinPercentage, &w.NCAAFinish)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	return c.JSON(w)
}

func GetDailyWrestler(c *fiber.Ctx) error {
	today := time.Now().UTC().Format("2006-01-02")


	var wrestlerID int
	err := database.DB.QueryRow("SELECT wrestler_id FROM daily_wrestlers WHERE day = $1::date", today).Scan(&wrestlerID)
	if err != nil {
		log.Println("Error when querying daily_wrestlers: ", err)
		return c.Status(404).SendString("Wrestler ID not found")
	}

	var w models.Wrestler
	err = database.DB.QueryRow("SELECT * FROM wrestlers_2025 WHERE id = $1", wrestlerID).
		Scan(&w.ID, &w.WeightClass, &w.Name, &w.Year, &w.Team, &w.Conference, &w.WinPercentage, &w.NCAAFinish)
	if err != nil {
		return c.Status(404).SendString("Wrestler ID did not match any ID in wrestlers table")
	}

	return c.JSON(w)
}
