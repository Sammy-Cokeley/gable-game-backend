package routes

import (
	"time"

	"gable-backend/controllers"
	"gable-backend/middleware"

	"github.com/gofiber/fiber/v2"

	"github.com/gofiber/fiber/v2/middleware/limiter"
)

func WrestlerRoutes(app *fiber.App) {
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Backend is up!")
	})

	api := app.Group("/api")

	//GET Requests
	api.Get("/wrestlers", controllers.GetWrestlersByQuery)
	api.Get("/daily", controllers.GetDailyWrestler)
	api.Get("/me", middleware.RequireAuth, controllers.GetMe)
	api.Get("/user/guesses", middleware.RequireAuth, controllers.GetUserGuesses)
	api.Get("/user/stats", middleware.RequireAuth, controllers.GetUserStats)

	//POST Requests
	api.Post("/register", controllers.Register)
	api.Post("/login", controllers.Login)
	api.Post("/verify-email", controllers.VerifyEmail)
	api.Post("/resend-verification", controllers.ResendVerification)
	api.Post("/user/guess", middleware.RequireAuth, controllers.SubmitUserGuess)
	api.Post("/user/stats", middleware.RequireAuth, controllers.UpdateUserStats)
	api.Post("/contact", middleware.RequireAuth, limiter.New(limiter.Config{
		Max:        1,
		Expiration: time.Minute,
	}), controllers.ContactHandler)
}
