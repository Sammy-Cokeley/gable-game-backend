package trackdual

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

var expectedHeaders = []string{
	"season_year",
	"event_name",
	"event_date",
	"event_type",
	"dual_id",
	"weight_label",
	"bout_number",
	"wrestler_a_name",
	"wrestler_a_school_slug",
	"wrestler_b_name",
	"wrestler_b_school_slug",
	"winner_name",
	"result_method",
	"score_winner",
	"score_loser",
	"match_time",
	"source_match_id",
}

func ParseCSV(reader io.Reader) ([]CSVRecord, error) {
	c := csv.NewReader(reader)
	c.TrimLeadingSpace = true

	headers, err := c.Read()
	if err != nil {
		return nil, fmt.Errorf("read headers: %w", err)
	}

	if err := validateHeaders(headers); err != nil {
		return nil, err
	}

	var records []CSVRecord
	for {
		row, err := c.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}

		record, err := parseRow(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, nil
}

func validateHeaders(headers []string) error {
	if len(headers) != len(expectedHeaders) {
		return fmt.Errorf("invalid header count: got %d, expected %d", len(headers), len(expectedHeaders))
	}
	for idx, h := range headers {
		if strings.TrimSpace(h) != expectedHeaders[idx] {
			return fmt.Errorf("invalid header at index %d: got %q, expected %q", idx, h, expectedHeaders[idx])
		}
	}
	return nil
}

func parseRow(row []string) (CSVRecord, error) {
	if len(row) != len(expectedHeaders) {
		return CSVRecord{}, fmt.Errorf("invalid row width: got %d, expected %d", len(row), len(expectedHeaders))
	}

	seasonYear, err := strconv.Atoi(strings.TrimSpace(row[0]))
	if err != nil {
		return CSVRecord{}, fmt.Errorf("invalid season_year %q: %w", row[0], err)
	}

	eventDate, err := time.Parse("2006-01-02", strings.TrimSpace(row[2]))
	if err != nil {
		return CSVRecord{}, fmt.Errorf("invalid event_date %q: %w", row[2], err)
	}

	boutNumber, err := strconv.Atoi(strings.TrimSpace(row[6]))
	if err != nil {
		return CSVRecord{}, fmt.Errorf("invalid bout_number %q: %w", row[6], err)
	}

	scoreWinner, err := parseOptionalInt(row[13])
	if err != nil {
		return CSVRecord{}, fmt.Errorf("invalid score_winner %q: %w", row[13], err)
	}

	scoreLoser, err := parseOptionalInt(row[14])
	if err != nil {
		return CSVRecord{}, fmt.Errorf("invalid score_loser %q: %w", row[14], err)
	}

	return CSVRecord{
		SeasonYear:          seasonYear,
		EventName:           strings.TrimSpace(row[1]),
		EventDate:           eventDate,
		EventType:           strings.TrimSpace(row[3]),
		DualID:              strings.TrimSpace(row[4]),
		WeightLabel:         strings.TrimSpace(row[5]),
		BoutNumber:          boutNumber,
		WrestlerAName:       strings.TrimSpace(row[7]),
		WrestlerASchoolSlug: strings.TrimSpace(row[8]),
		WrestlerBName:       strings.TrimSpace(row[9]),
		WrestlerBSchoolSlug: strings.TrimSpace(row[10]),
		WinnerName:          strings.TrimSpace(row[11]),
		ResultMethod:        strings.TrimSpace(row[12]),
		ScoreWinner:         scoreWinner,
		ScoreLoser:          scoreLoser,
		MatchTime:           strings.TrimSpace(row[15]),
		SourceMatchID:       strings.TrimSpace(row[16]),
	}, nil
}

func parseOptionalInt(s string) (*int, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
