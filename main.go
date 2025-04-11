package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	//"github.com/joho/godotenv"
	"gable-backend/database"
	"gable-backend/routes"
)

func main() {
	// Load env vars from .env file
	// if err := godotenv.Load(); err != nil {
	// 	log.Fatal("Error loading .env file")
	// }

	// Load environment variables
	database.ConnectDB()

	// Create Fiber app
	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	routes.WrestlerRoutes(app)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	fmt.Println("Server running on port " + port)
	log.Fatal(app.Listen(":" + port))
}
