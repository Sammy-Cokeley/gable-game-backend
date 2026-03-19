package routes

import (
	"gable-backend/controllers"

	"github.com/gofiber/fiber/v2"
)

// PlatformRoutes mounts the versioned platform API at /api/v1.
// All endpoints are read-only and require no authentication.
func PlatformRoutes(app *fiber.App) {
	v1 := app.Group("/api/v1")

	// Wrestlers
	v1.Get("/wrestlers", controllers.V1GetWrestlers)
	v1.Get("/wrestlers/:id", controllers.V1GetWrestler)

	// Schools
	v1.Get("/schools", controllers.V1GetSchools)
	v1.Get("/schools/:slug", controllers.V1GetSchool)

	// Conferences
	v1.Get("/conferences", controllers.V1GetConferences)

	// Seasons
	v1.Get("/seasons", controllers.V1GetSeasons)

	// Rankings
	v1.Get("/rankings", controllers.V1GetRankings)
	v1.Get("/rankings/history/:wrestler_id", controllers.V1GetWrestlerRankingHistory)
}
