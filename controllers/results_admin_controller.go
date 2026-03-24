package controllers

import (
	"context"
	"log"

	"gable-backend/database"
	"gable-backend/internal/ingest/trackdual"

	"github.com/gofiber/fiber/v2"
)

// ImportTrackDualCSV accepts a multipart CSV upload and runs the TrackWrestling
// dual box score ingestion pipeline.
//
// POST /api/admin/results/import/trackdual
func ImportTrackDualCSV(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "missing file field"})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not open uploaded file"})
	}
	defer f.Close()

	records, err := trackdual.ParseCSV(f)
	if err != nil {
		return c.Status(422).JSON(fiber.Map{"error": err.Error()})
	}

	repo := trackdual.NewPostgresRepository(database.DB)
	svc := trackdual.NewService(repo)

	result, err := svc.Process(context.Background(), records)
	if err != nil {
		log.Printf("trackdual import error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "ingestion failed"})
	}

	return c.JSON(fiber.Map{
		"rows_read":        result.RowsRead,
		"rows_succeeded":   result.RowsSucceeded,
		"rows_failed":      result.RowsFailed,
		"bouts_inserted":   result.BoutsInserted,
		"bouts_duplicated": result.BoutsDuplicated,
		"status":           result.Status(),
	})
}
