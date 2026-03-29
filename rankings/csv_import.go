package rankings

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func ParseSnapshotCSV(reader io.Reader) ([]RankingEntry, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv must include a header row and at least one data row")
	}

	headers := make(map[string]int, len(records[0]))
	for idx, col := range records[0] {
		headers[strings.ToLower(strings.TrimSpace(col))] = idx
	}

	required := []string{"wrestler_id", "rank"}
	for _, col := range required {
		if _, ok := headers[col]; !ok {
			return nil, fmt.Errorf("missing required column %q", col)
		}
	}

	entries := make([]RankingEntry, 0, len(records)-1)
	for i, record := range records[1:] {
		lineNo := i + 2

		wrestlerID, err := readInt(record, headers["wrestler_id"])
		if err != nil {
			return nil, fmt.Errorf("line %d wrestler_id: %w", lineNo, err)
		}
		rank, err := readInt(record, headers["rank"])
		if err != nil {
			return nil, fmt.Errorf("line %d rank: %w", lineNo, err)
		}

		var points *float64
		if idx, ok := headers["points"]; ok {
			value := strings.TrimSpace(readValue(record, idx))
			if value != "" {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return nil, fmt.Errorf("line %d points: invalid float %q", lineNo, value)
				}
				points = &parsed
			}
		}

		metadata := json.RawMessage(`{}`)
		if idx, ok := headers["metadata"]; ok {
			value := strings.TrimSpace(readValue(record, idx))
			if value != "" {
				if !json.Valid([]byte(value)) {
					return nil, fmt.Errorf("line %d metadata: invalid json", lineNo)
				}
				metadata = json.RawMessage(value)
			}
		}

		entries = append(entries, RankingEntry{
			WrestlerID: wrestlerID,
			Rank:       rank,
			Points:     points,
			Metadata:   metadata,
		})
	}

	return entries, nil
}

func readInt(record []string, idx int) (int, error) {
	value := strings.TrimSpace(readValue(record, idx))
	if value == "" {
		return 0, fmt.Errorf("value is required")
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", value)
	}
	return parsed, nil
}

func readValue(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return record[idx]
}
