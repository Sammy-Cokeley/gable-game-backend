package trackdual

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Tx interface {
	Commit() error
	Rollback() error
}

type Repository interface {
	BeginTx(ctx context.Context) (Tx, error)
	CreateIngestBatch(ctx context.Context, tx Tx) (string, error)
	FinalizeIngestBatch(ctx context.Context, tx Tx, batchID string, status string, summary ProcessResult) error
	GetOrCreateSeason(ctx context.Context, tx Tx, year int) (string, error)
	GetWeightClassByLabel(ctx context.Context, tx Tx, label string) (string, error)
	GetOrCreateSchool(ctx context.Context, tx Tx, slug string) (string, error)
	GetOrCreateEvent(ctx context.Context, tx Tx, seasonID string, name string, eventDate string, eventType string, dualID string) (string, error)
	GetOrCreateWrestlerWithAlias(ctx context.Context, tx Tx, fullName string) (string, error)
	UpsertWrestlerSeason(ctx context.Context, tx Tx, wrestlerID string, seasonID string, schoolID string, weightClassID string) error
	InsertIngestError(ctx context.Context, tx Tx, batchID string, rowNumber int, reason string, payload CSVRecord) error
	InsertBout(ctx context.Context, tx Tx, input BoutInsertInput) (bool, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Process(ctx context.Context, rows []CSVRecord) (ProcessResult, error) {
	result := ProcessResult{RowsRead: len(rows)}
	if len(rows) == 0 {
		return result, nil
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return result, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	batchID, err := s.repo.CreateIngestBatch(ctx, tx)
	if err != nil {
		return result, fmt.Errorf("create ingest batch: %w", err)
	}
	for i, row := range rows {
		rowNum := i + 2
		if !winnerMatches(row) {
			result.RowsFailed++
			_ = s.repo.InsertIngestError(ctx, tx, batchID, rowNum, "winner_name must match wrestler_a_name or wrestler_b_name", row)
			continue
		}
		if err := s.processRow(ctx, tx, batchID, row, &result); err != nil {
			result.RowsFailed++
			_ = s.repo.InsertIngestError(ctx, tx, batchID, rowNum, err.Error(), row)
			continue
		}
		result.RowsSucceeded++
	}
	if err := s.repo.FinalizeIngestBatch(ctx, tx, batchID, result.Status(), result); err != nil {
		return result, fmt.Errorf("finalize ingest batch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit tx: %w", err)
	}
	return result, nil
}

func (s *Service) processRow(ctx context.Context, tx Tx, batchID string, row CSVRecord, result *ProcessResult) error {
	seasonID, err := s.repo.GetOrCreateSeason(ctx, tx, row.SeasonYear)
	if err != nil {
		return fmt.Errorf("season: %w", err)
	}
	weightClassID, err := s.repo.GetWeightClassByLabel(ctx, tx, row.WeightLabel)
	if err != nil {
		return fmt.Errorf("weight class: %w", err)
	}
	aSchoolID, err := s.repo.GetOrCreateSchool(ctx, tx, row.WrestlerASchoolSlug)
	if err != nil {
		return fmt.Errorf("wrestler A school: %w", err)
	}
	bSchoolID, err := s.repo.GetOrCreateSchool(ctx, tx, row.WrestlerBSchoolSlug)
	if err != nil {
		return fmt.Errorf("wrestler B school: %w", err)
	}
	eventID, err := s.repo.GetOrCreateEvent(ctx, tx, seasonID, row.EventName, row.EventDate.Format("2006-01-02"), row.EventType, row.DualID)
	if err != nil {
		return fmt.Errorf("event: %w", err)
	}
	wrestlerAID, err := s.repo.GetOrCreateWrestlerWithAlias(ctx, tx, row.WrestlerAName)
	if err != nil {
		return fmt.Errorf("wrestler A: %w", err)
	}
	wrestlerBID, err := s.repo.GetOrCreateWrestlerWithAlias(ctx, tx, row.WrestlerBName)
	if err != nil {
		return fmt.Errorf("wrestler B: %w", err)
	}
	_ = s.repo.UpsertWrestlerSeason(ctx, tx, wrestlerAID, seasonID, aSchoolID, weightClassID)
	_ = s.repo.UpsertWrestlerSeason(ctx, tx, wrestlerBID, seasonID, bSchoolID, weightClassID)
	winnerID := wrestlerAID
	if !nameEqual(row.WinnerName, row.WrestlerAName) {
		winnerID = wrestlerBID
	}
	scoreA, scoreB := assignScores(row)
	inserted, err := s.repo.InsertBout(ctx, tx, BoutInsertInput{
		BatchID:       batchID,
		EventID:       eventID,
		SeasonID:      seasonID,
		WeightClassID: weightClassID,
		WrestlerAID:   wrestlerAID,
		WrestlerBID:   wrestlerBID,
		WinnerID:      winnerID,
		ResultMethod:  row.ResultMethod,
		ScoreA:        scoreA,
		ScoreB:        scoreB,
		MatchTime:     row.MatchTime,
		SourceMatchID: row.SourceMatchID,
		IdentityHash:  ComputeIdentityHash(row),
		DualID:        row.DualID,
		BoutNumber:    row.BoutNumber,
		WrestlerAName: row.WrestlerAName,
		WrestlerBName: row.WrestlerBName,
		WinnerName:    row.WinnerName,
		WeightLabel:   row.WeightLabel,
		EventDateISO:  row.EventDate.Format("2006-01-02"),
	})
	if err != nil {
		return fmt.Errorf("insert bout: %w", err)
	}
	if inserted {
		result.BoutsInserted++
	} else {
		result.BoutsDuplicated++
	}
	return nil
}

func ComputeIdentityHash(row CSVRecord) string {
	base := strings.Join([]string{
		strings.TrimSpace(row.DualID),
		row.EventDate.Format("2006-01-02"),
		strings.ToLower(strings.TrimSpace(row.WeightLabel)),
		fmt.Sprintf("%d", row.BoutNumber),
		strings.ToLower(strings.TrimSpace(row.WrestlerAName)),
		strings.ToLower(strings.TrimSpace(row.WrestlerBName)),
		strings.ToLower(strings.TrimSpace(row.ResultMethod)),
		normalizeScore(row.ScoreWinner),
		normalizeScore(row.ScoreLoser),
	}, "|")
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])
}

func normalizeScore(v *int) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%d", *v)
}

func winnerMatches(row CSVRecord) bool {
	return nameEqual(row.WinnerName, row.WrestlerAName) || nameEqual(row.WinnerName, row.WrestlerBName)
}

func nameEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func assignScores(row CSVRecord) (*int, *int) {
	if nameEqual(row.WinnerName, row.WrestlerAName) {
		return row.ScoreWinner, row.ScoreLoser
	}
	return row.ScoreLoser, row.ScoreWinner
}
