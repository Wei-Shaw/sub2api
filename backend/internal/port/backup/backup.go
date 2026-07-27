// Package backup contains the port interfaces for the database-backup
// bounded context: pg_dump/restore streaming plus S3-compatible object
// storage of backup files. The contracts reference only domain/stdlib types
// so the repository layer can implement them without importing
// internal/service. The service package keeps type aliases to the interfaces
// so existing call sites and test stubs continue to satisfy the contracts.
package backup

import (
	"context"
	"io"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// DBDumper abstracts database dump/restore operations.
type DBDumper interface {
	Dump(ctx context.Context) (io.ReadCloser, error)
	Restore(ctx context.Context, data io.Reader) error
}

// BackupObjectStore abstracts object storage for backup files.
type BackupObjectStore interface {
	Upload(ctx context.Context, key string, body io.Reader, contentType string) (sizeBytes int64, err error)
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	PresignURL(ctx context.Context, key string, expiry time.Duration) (string, error)
	HeadBucket(ctx context.Context) error
}

// BackupObjectStoreFactory creates an object store from S3 config.
type BackupObjectStoreFactory func(ctx context.Context, cfg *domain.BackupS3Config) (BackupObjectStore, error)
