package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"gable-backend/database"
	"gable-backend/internal/ingest/trackdual"
)

func main() {
	csvPath := flag.String("csv", "", "Path to Trackwrestling dual box score CSV")
	flag.Parse()

	if *csvPath == "" {
		log.Fatal("missing required -csv argument")
	}

	f, err := os.Open(*csvPath)
	if err != nil {
		log.Fatalf("open csv: %v", err)
	}
	defer f.Close()

	records, err := trackdual.ParseCSV(f)
	if err != nil {
		log.Fatalf("parse csv: %v", err)
	}

	database.ConnectDB()
	repo := trackdual.NewPostgresRepository(database.DB)
	service := trackdual.NewService(repo)

	result, err := service.Process(context.Background(), records)
	if err != nil {
		log.Fatalf("process ingest: %v", err)
	}

	fmt.Printf("Ingestion complete: rows=%d success=%d failed=%d inserted=%d duplicate=%d\n",
		result.RowsRead, result.RowsSucceeded, result.RowsFailed, result.BoutsInserted, result.BoutsDuplicated)
}
