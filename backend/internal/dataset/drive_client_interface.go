package dataset

import "context"

// DriveClientInterface abstracts Google Drive upload operations for testing.
type DriveClientInterface interface {
	UploadBatch(ctx context.Context, entries []DatasetEntry) (string, error)
}
