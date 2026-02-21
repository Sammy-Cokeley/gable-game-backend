package rankings

import (
	"encoding/json"
	"time"
)

type RankingSource struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Notes    string `json:"notes,omitempty"`
	IsActive bool   `json:"isActive"`
}

type RankingSnapshot struct {
	ID          int64     `json:"id"`
	SeasonID    string    `json:"seasonId"`
	RankingDate time.Time `json:"rankingDate"`
	SourceID    int64     `json:"sourceId"`
	WeightClass int       `json:"weightClass"`
}

type RankingEntry struct {
	SnapshotID int64           `json:"snapshotId"`
	WrestlerID int             `json:"wrestlerId"`
	Rank       int             `json:"rank"`
	Points     *float64        `json:"points,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

type SnapshotImport struct {
	SourceName  string         `json:"sourceName"`
	SeasonID    string         `json:"seasonId"`
	RankingDate time.Time      `json:"rankingDate"`
	WeightClass int            `json:"weightClass"`
	Entries     []RankingEntry `json:"entries"`
}
