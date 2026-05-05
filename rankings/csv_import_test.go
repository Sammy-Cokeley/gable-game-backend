package rankings

import (
	"strings"
	"testing"
)

func TestParseSnapshotCSVSuccess(t *testing.T) {
	csvData := "wrestler_id,rank,points,metadata\n" +
		"101,1,32.5,\"{\"\"note\"\":\"\"returning champ\"\"}\"\n" +
		"202,2,,\n"

	entries, err := ParseSnapshotCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("expected parse to succeed, got error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].WrestlerID != 101 || entries[0].Rank != 1 {
		t.Fatalf("unexpected first row parsed: %+v", entries[0])
	}
	if entries[0].Points == nil || *entries[0].Points != 32.5 {
		t.Fatalf("expected points to be set to 32.5, got %+v", entries[0].Points)
	}
	if string(entries[0].Metadata) != `{"note":"returning champ"}` {
		t.Fatalf("unexpected metadata: %s", string(entries[0].Metadata))
	}
	if entries[1].Points != nil {
		t.Fatalf("expected empty points cell to parse as nil")
	}
}

func TestParseSnapshotCSVMissingRequiredColumn(t *testing.T) {
	csvData := `rank,points
1,12
`

	_, err := ParseSnapshotCSV(strings.NewReader(csvData))
	if err == nil {
		t.Fatal("expected missing required column error, got nil")
	}
}
