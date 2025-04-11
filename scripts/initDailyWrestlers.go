package main

import (
	"database/sql"
	"os"
	"fmt"
	"log"
	"math/rand"
	"time"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}
	// Connect to the database
	ConnectDB()

	// Call the function to initialize daily wrestlers
	err := initDailyWrestlers()
	if err != nil {
		log.Fatal("Error initializing daily wrestlers:", err)
	}
}

func ConnectDB() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	var err error
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatal("Database not reachable:", err)
	}

	fmt.Println("Connected to PostgreSQL successfully!")
}

func initDailyWrestlers() error {
	// Get all wrestler IDs
	rows, err := DB.Query("SELECT id FROM wrestlers_2025")
	if err != nil {
		return err
	}
	defer rows.Close()

	var wrestlerIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		wrestlerIDs = append(wrestlerIDs, id)
	}

	// Shuffle wrestler IDs
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(wrestlerIDs), func(i, j int) { wrestlerIDs[i], wrestlerIDs[j] = wrestlerIDs[j], wrestlerIDs[i] })

	// Insert into daily_wrestlers table
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO daily_wrestlers (day, wrestler_id) VALUES ($1, $2)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	startDate := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)

	for i, id := range wrestlerIDs {
		day := startDate.AddDate(0, 0, i)
		_, err := stmt.Exec(day, id)
		if err != nil {
			return fmt.Errorf("Error inserting wrestler ID %d on day %d: %v", id, i, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	fmt.Println("Daily wrestlers initialized successfully")
	return nil
}
