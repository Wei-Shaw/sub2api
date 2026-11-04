package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *BackupService) beginBackupRestore(ctx context.Context, id string) (*BackupRecord, error) {
	ctx, release, err := s.lockBackupRecordUpdates(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	records, err := s.loadRecordsLocked(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].ID != id {
			continue
		}
		if records[i].Status != "completed" {
			return nil, infraerrors.BadRequest("BACKUP_NOT_COMPLETED", "can only restore from a completed backup")
		}
		if records[i].RestoreStatus == "running" {
			return nil, ErrRestoreInProgress
		}
		records[i].RestoreStatus = "running"
		records[i].RestoreStartedAt = time.Now().Format(time.RFC3339)
		records[i].RestoreError = ""
		if err := s.saveRecordsLocked(ctx, records); err != nil {
			return nil, err
		}
		return &records[i], nil
	}
	return nil, ErrBackupNotFound
}

// Failed/panicked restores must not resurrect a removed record. A successful
// full database restore may itself roll back settings and remove its own record,
// so that completion keeps the existing explicit re-registration behavior.
func (s *BackupService) saveRestoreRecord(ctx context.Context, record *BackupRecord) error {
	ctx, release, err := s.lockBackupRecordUpdates(ctx)
	if err != nil {
		return err
	}
	defer release()
	records, err := s.loadRecordsLocked(ctx)
	if err != nil {
		return err
	}
	for i := range records {
		if records[i].ID != record.ID {
			continue
		}
		if records[i].RestoreStartedAt != record.RestoreStartedAt &&
			(record.RestoreStatus != "completed" || (records[i].RestoreStatus == "running" && records[i].RestoreStartedAt != "")) {
			return infraerrors.Conflict("BACKUP_RESTORE_STATE_CHANGED", "a newer restore operation owns this backup")
		}
		if record.RestoreStatus == "completed" {
			// Restoring the full database may roll the backup record itself back
			// to its pre-upload state. Re-register the completed snapshot while
			// retaining any archive protection already persisted in the database.
			snapshot := *record
			if records[i].MonthlyArchive != nil {
				snapshot.MonthlyArchive = records[i].MonthlyArchive
				snapshot.ExpiresAt = ""
			}
			records[i] = snapshot
			return s.saveRecordsLocked(ctx, records)
		}
		// Only the restore operation's fields change, preserving current archive
		// metadata and any unrelated progress recorded by another writer.
		records[i].RestoreStatus = record.RestoreStatus
		records[i].RestoreError = record.RestoreError
		records[i].RestoredAt = record.RestoredAt
		return s.saveRecordsLocked(ctx, records)
	}
	if record.RestoreStatus == "completed" {
		return s.saveRecordsLocked(ctx, append(records, *record))
	}
	return ErrBackupNotFound
}
