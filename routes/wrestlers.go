package routes

import (
	"gable-backend/controllers"
	"gable-backend/middleware"

	"github.com/gofiber/fiber/v2"
)

func WrestlerRoutes(app *fiber.App) {
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Backend is up!")
	})

	api := app.Group("/api")

	api.Get("/wrestlers", controllers.GetWrestlersByQuery)
	api.Get("/daily", controllers.GetDailyWrestler)
	api.Post("/register", controllers.Register)
	api.Post("/login", controllers.Login)
	api.Get("/me", middleware.RequireAuth, controllers.GetMe)
	api.Post("/verify-email", controllers.VerifyEmail)
	api.Post("/resend-verification", controllers.ResendVerification)
}
