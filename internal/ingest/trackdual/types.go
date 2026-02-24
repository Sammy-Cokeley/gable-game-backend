package trackdual

import "time"

const SourceName = "trackwrestling"

type CSVRecord struct {
	SeasonYear          int
	EventName           string
	EventDate           time.Time
	EventType           string
	DualID              string
	WeightLabel         string
	BoutNumber          int
	WrestlerAName       string
	WrestlerASchoolSlug string
	WrestlerBName       string
	WrestlerBSchoolSlug string
	WinnerName          string
	ResultMethod        string
	ScoreWinner         *int
	ScoreLoser          *int
	MatchTime           string
	SourceMatchID       string
}

type ProcessResult struct {
	RowsRead        int
	RowsSucceeded   int
	RowsFailed      int
	BoutsInserted   int
	BoutsDuplicated int
}

func (r ProcessResult) Status() string {
	if r.RowsFailed > 0 {
		return "completed_with_errors"
	}
	return "completed"
}
