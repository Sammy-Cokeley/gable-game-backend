package rankings

import "context"

type Repository interface {
	ImportSnapshot(ctx context.Context, snapshot SnapshotImport) (int64, error)
}
