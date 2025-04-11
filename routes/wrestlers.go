package routes

import (
	"github.com/gofiber/fiber/v2"
	"gable-backend/controllers"
)

func WrestlerRoutes(app *fiber.App) {
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Backend is up!")
	})

	api := app.Group("/api")

	api.Get("/wrestlers", controllers.GetWrestlersByQuery)
	api.Get("/daily", controllers.GetDailyWrestler)
}
