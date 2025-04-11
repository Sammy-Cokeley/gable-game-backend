package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
	"gable-backend/database"
	"gable-backend/routes"
)

func main() {
	// Load env vars from .env file
	if os.Getenv("RENDER") == "" {
		err := godotenv.Load()
		if err != nil {
			log.Println("No .env file found, continuing with system environment variables")
		}
	}
	// Load environment variables
	database.ConnectDB()

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT environment variable not set")
	}

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	routes.WrestlerRoutes(app)

	// Start server
	log.Println("Server running on port " + port)
	log.Fatal(app.Listen(":" + port))
}
