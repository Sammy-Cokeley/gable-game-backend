package routes

import (
	"github.com/gofiber/fiber/v2"
	"gable-backend/controllers"
)

func WrestlerRoutes(app *fiber.App) {
	api := app.Group("/api")

	api.Get("/wrestlers", controllers.GetWrestlersByQuery)
	api.Get("/daily", controllers.GetDailyWrestler)
}
