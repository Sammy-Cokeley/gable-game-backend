package controllers

import (
	"time"

	"gable-backend/database"
	"gable-backend/models"
	"github.com/gofiber/fiber/v2"
)

func GetWrestlersByQuery(c *fiber.Ctx) error {

	name := c.Query("name")

	if name == "" {
		rows, err := database.DB.Query("SELECT * FROM wrestlers")
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()
	
		var wrestlers []models.Wrestler
		for rows.Next() {
			var w models.Wrestler
			if err := rows.Scan(&w.ID, &w.Name, &w.Team, &w.Conference, &w.Weight, &w.Wins, &w.Losses); err != nil {
				return c.Status(500).SendString(err.Error())
			}
			wrestlers = append(wrestlers, w)
		}
		return c.JSON(wrestlers)
	}

	var w models.Wrestler
	err := database.DB.QueryRow("SELECT * FROM wrestlers WHERE LOWER(name) = LOWER($1)", name).
		Scan(&w.ID, &w.Name, &w.Team, &w.Conference, &w.Weight, &w.Wins, &w.Losses)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	return c.JSON(w)
}

func GetDailyWrestler(c *fiber.Ctx) error {
	gameStartDate := time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC)
	today := time.Now().UTC()
	daysSinceStart := int(today.Sub(gameStartDate).Hours() / 24)

	var wrestlerID int
	err := database.DB.QueryRow("SELECT wrestler_id FROM daily_wrestlers WHERE day = $1", daysSinceStart%330).Scan(&wrestlerID)
	if err != nil {
		return c.Status(404).SendString("Wrestler ID not found")
	}

	var w models.Wrestler
	err = database.DB.QueryRow("SELECT * FROM wrestlers WHERE id = $1", wrestlerID).
		Scan(&w.ID, &w.Name, &w.Team, &w.Conference, &w.Weight, &w.Wins, &w.Losses)
	if err != nil {
		return c.Status(404).SendString("Wrestler ID did not match any ID in wrestlers table")
	}

	return c.JSON(w)
}
