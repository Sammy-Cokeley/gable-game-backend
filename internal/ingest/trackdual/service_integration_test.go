package trackdual

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type fakeTx struct{}

func (f *fakeTx) Commit() error   { return nil }
func (f *fakeTx) Rollback() error { return nil }

type fakeRepo struct {
	nextID int
	bouts  map[string]struct{}
	errors int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{nextID: 1, bouts: map[string]struct{}{}} }

func (f *fakeRepo) BeginTx(context.Context) (Tx, error)                    { return &fakeTx{}, nil }
func (f *fakeRepo) CreateIngestBatch(context.Context, Tx) (string, error)   { return "batch-1", nil }
func (f *fakeRepo) FinalizeIngestBatch(context.Context, Tx, string, string, ProcessResult) error {
	return nil
}
func (f *fakeRepo) GetOrCreateSeason(context.Context, Tx, int) (string, error) {
	return "season-uuid", nil
}
func (f *fakeRepo) GetWeightClassByLabel(context.Context, Tx, string) (string, error) {
	return "wc-uuid", nil
}
func (f *fakeRepo) GetOrCreateSchool(context.Context, Tx, string) (string, error) {
	return "school-uuid", nil
}
func (f *fakeRepo) GetOrCreateEvent(context.Context, Tx, string, string, string, string, string) (string, error) {
	return "event-uuid", nil
}
func (f *fakeRepo) GetOrCreateWrestlerWithAlias(context.Context, Tx, string) (string, error) {
	f.nextID++
	return fmt.Sprintf("wrestler-%d", f.nextID), nil
}
func (f *fakeRepo) UpsertWrestlerSeason(context.Context, Tx, string, string, string, string) error {
	return nil
}
func (f *fakeRepo) InsertIngestError(context.Context, Tx, string, int, string, CSVRecord) error {
	f.errors++
	return nil
}
func (f *fakeRepo) InsertBout(_ context.Context, _ Tx, input BoutInsertInput) (bool, error) {
	if _, ok := f.bouts[input.IdentityHash]; ok {
		return false, nil
	}
	f.bouts[input.IdentityHash] = struct{}{}
	return true, nil
}

func TestService_IdempotentBoutInsertAndValidationErrors(t *testing.T) {
	scoreW, scoreL := 5, 2
	row := CSVRecord{
		SeasonYear: 2025, EventName: "Event", EventDate: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), EventType: "Dual", DualID: "D1",
		WeightLabel: "149", BoutNumber: 3, WrestlerAName: "A Name", WrestlerASchoolSlug: "iowa", WrestlerBName: "B Name", WrestlerBSchoolSlug: "osu",
		WinnerName: "A Name", ResultMethod: "DEC", ScoreWinner: &scoreW, ScoreLoser: &scoreL, MatchTime: "7:00", SourceMatchID: "M1",
	}
	badRow := row
	badRow.WinnerName = "Unknown"

	repo := newFakeRepo()
	svc := NewService(repo)
	result1, err := svc.Process(context.Background(), []CSVRecord{row, badRow})
	if err != nil {
		t.Fatalf("unexpected error process #1: %v", err)
	}
	if result1.BoutsInserted != 1 || result1.BoutsDuplicated != 0 || result1.RowsFailed != 1 {
		t.Fatalf("unexpected first result: %+v", result1)
	}

	result2, err := svc.Process(context.Background(), []CSVRecord{row})
	if err != nil {
		t.Fatalf("unexpected error process #2: %v", err)
	}
	if result2.BoutsInserted != 0 || result2.BoutsDuplicated != 1 {
		t.Fatalf("expected duplicate on second run, got %+v", result2)
	}
}
