package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gable-backend/database"
	"gable-backend/rankings"

	"github.com/joho/godotenv"
)

func main() {
	if os.Getenv("RENDER") == "" {
		_ = godotenv.Load()
	}

	var (
		csvPath     = flag.String("csv", "", "Path to CSV file containing ranking entries")
		sourceName  = flag.String("source", "", "Ranking source identifier (e.g. coaches, media)")
		seasonID    = flag.String("season", "", "Season id/label (e.g. 2025)")
		rankingDate = flag.String("date", "", "Snapshot date in YYYY-MM-DD format")
		weightClass = flag.Int("weight-class", 0, "Weight class for this snapshot")
	)
	flag.Parse()

	if err := validateFlags(*csvPath, *sourceName, *seasonID, *rankingDate, *weightClass); err != nil {
		log.Fatal(err)
	}

	date, err := time.Parse("2006-01-02", *rankingDate)
	if err != nil {
		log.Fatalf("invalid --date value: %v", err)
	}

	file, err := os.Open(*csvPath)
	if err != nil {
		log.Fatalf("open csv file: %v", err)
	}
	defer file.Close()

	entries, err := rankings.ParseSnapshotCSV(file)
	if err != nil {
		log.Fatalf("parse csv: %v", err)
	}

	database.ConnectDB()
	repo := rankings.NewPostgresRepository(database.DB)

	snapshotID, err := repo.ImportSnapshot(context.Background(), rankings.SnapshotImport{
		SourceName:  strings.TrimSpace(*sourceName),
		SeasonID:    strings.TrimSpace(*seasonID),
		RankingDate: date,
		WeightClass: *weightClass,
		Entries:     entries,
	})
	if err != nil {
		log.Fatalf("import snapshot: %v", err)
	}

	fmt.Printf("Imported snapshot %d with %d ranking entries\n", snapshotID, len(entries))
}

func validateFlags(csvPath, sourceName, seasonID, rankingDate string, weightClass int) error {
	if strings.TrimSpace(csvPath) == "" {
		return fmt.Errorf("--csv is required")
	}
	if strings.TrimSpace(sourceName) == "" {
		return fmt.Errorf("--source is required")
	}
	if strings.TrimSpace(seasonID) == "" {
		return fmt.Errorf("--season is required")
	}
	if strings.TrimSpace(rankingDate) == "" {
		return fmt.Errorf("--date is required")
	}
	if weightClass <= 0 {
		return fmt.Errorf("--weight-class must be greater than 0")
	}
	return nil
}
