package trackdual

import (
	"strings"
	"testing"
)

func TestParseCSV_Success(t *testing.T) {
	csv := `season_year,event_name,event_date,event_type,dual_id,weight_label,bout_number,wrestler_a_name,wrestler_a_school_slug,wrestler_b_name,wrestler_b_school_slug,winner_name,result_method,score_winner,score_loser,match_time,source_match_id
2025,Big Match,2025-01-05,Dual,D123,157,7,John A,iowa,Max B,osu,John A,DEC,8,3,7:00,M001`

	records, err := ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].SeasonYear != 2025 {
		t.Fatalf("expected season 2025, got %d", records[0].SeasonYear)
	}
	if records[0].WinnerName != "John A" {
		t.Fatalf("expected winner John A, got %q", records[0].WinnerName)
	}
	if records[0].ScoreWinner == nil {
		t.Fatalf("expected score_winner to be populated")
	}
}

func TestParseCSV_BadHeader(t *testing.T) {
	csv := `season,event_name,event_date,event_type,dual_id,weight_label,bout_number,wrestler_a_name,wrestler_a_school_slug,wrestler_b_name,wrestler_b_school_slug,winner_name,result_method,score_winner,score_loser,match_time,source_match_id`
	_, err := ParseCSV(strings.NewReader(csv))
	if err == nil {
		t.Fatalf("expected error for invalid header")
	}
	if !strings.Contains(err.Error(), "invalid header") {
		t.Fatalf("expected invalid header error, got %v", err)
	}
}
