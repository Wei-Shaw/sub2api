package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type imageTaskRepoMemory struct {
	tasks map[string]*ImageTask
}

func newImageTaskRepoMemory() *imageTaskRepoMemory {
	return &imageTaskRepoMemory{tasks: make(map[string]*ImageTask)}
}

func (r *imageTaskRepoMemory) Create(_ context.Context, task *ImageTask) error {
	cp := *task
	r.tasks[task.TaskID] = &cp
	return nil
}

func (r *imageTaskRepoMemory) GetByTaskID(_ context.Context, taskID string) (*ImageTask, error) {
	task, ok := r.tasks[taskID]
	if !ok {
		return nil, ErrImageTaskNotFound
	}
	cp := *task
	return &cp, nil
}

func (r *imageTaskRepoMemory) MarkRunning(_ context.Context, taskID string) error {
	task, ok := r.tasks[taskID]
	if !ok {
		return ErrImageTaskNotFound
	}
	task.Status = ImageTaskStatusRunning
	return nil
}

func (r *imageTaskRepoMemory) MarkSucceeded(_ context.Context, taskID, filePath, mimeType string, byteSize int64, expiresAt time.Time) error {
	task, ok := r.tasks[taskID]
	if !ok {
		return ErrImageTaskNotFound
	}
	task.Status = ImageTaskStatusSucceeded
	task.FilePath = filePath
	task.MimeType = mimeType
	task.ByteSize = byteSize
	task.ExpiresAt = expiresAt
	return nil
}

func (r *imageTaskRepoMemory) MarkFailed(_ context.Context, taskID, message string, expiresAt time.Time) error {
	task, ok := r.tasks[taskID]
	if !ok {
		return ErrImageTaskNotFound
	}
	task.Status = ImageTaskStatusFailed
	task.ErrorMessage = message
	task.ExpiresAt = expiresAt
	return nil
}

func (r *imageTaskRepoMemory) DeleteExpired(_ context.Context, now time.Time) ([]ImageTask, error) {
	var deleted []ImageTask
	for id, task := range r.tasks {
		if !now.Before(task.ExpiresAt) {
			deleted = append(deleted, *task)
			delete(r.tasks, id)
		}
	}
	return deleted, nil
}

func (r *imageTaskRepoMemory) MarkStaleRunningFailed(_ context.Context, message string) error {
	for _, task := range r.tasks {
		if task.Status == ImageTaskStatusPending || task.Status == ImageTaskStatusRunning {
			task.Status = ImageTaskStatusFailed
			task.ErrorMessage = message
		}
	}
	return nil
}

func TestImageTaskServiceCreatePendingReturnsTaskID(t *testing.T) {
	repo := newImageTaskRepoMemory()
	svc := NewImageTaskServiceWithOptions(repo, t.TempDir(), time.Hour)
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	task, err := svc.CreatePending(context.Background(), 7, 9, "/v1/images/generations", "gpt-image-2", "draw a cat")
	require.NoError(t, err)
	require.NotEmpty(t, task.TaskID)
	require.Equal(t, ImageTaskStatusPending, task.Status)
	require.Equal(t, now.Add(time.Hour), task.ExpiresAt)
}

func TestImageTaskServiceSaveResultWritesTemporaryFileAndMarksSucceeded(t *testing.T) {
	repo := newImageTaskRepoMemory()
	dir := t.TempDir()
	svc := NewImageTaskServiceWithOptions(repo, dir, time.Hour)

	task, err := svc.CreatePending(context.Background(), 7, 9, "/v1/images/generations", "gpt-image-2", "draw a cat")
	require.NoError(t, err)

	saved, err := svc.SaveResult(context.Background(), task.TaskID, []byte("png-bytes"), "image/png")
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusSucceeded, saved.Status)
	require.Equal(t, filepath.Join(dir, task.TaskID+".png"), saved.FilePath)
	require.Equal(t, int64(9), saved.ByteSize)

	got, err := os.ReadFile(saved.FilePath)
	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), got)
}

func TestImageTaskServiceGetForUserHidesOtherUsersTasks(t *testing.T) {
	repo := newImageTaskRepoMemory()
	svc := NewImageTaskServiceWithOptions(repo, t.TempDir(), time.Hour)
	task, err := svc.CreatePending(context.Background(), 7, 9, "/v1/images/generations", "gpt-image-2", "draw a cat")
	require.NoError(t, err)

	_, err = svc.GetForUser(context.Background(), task.TaskID, 8)
	require.ErrorIs(t, err, ErrImageTaskNotFound)
}

func TestImageTaskServiceGetForAPIKeyRequiresSameAPIKey(t *testing.T) {
	repo := newImageTaskRepoMemory()
	svc := NewImageTaskServiceWithOptions(repo, t.TempDir(), time.Hour)
	task, err := svc.CreatePending(context.Background(), 7, 9, "/v1/images/generations", "gpt-image-2", "draw a cat")
	require.NoError(t, err)

	_, err = svc.GetForAPIKey(context.Background(), task.TaskID, 7, 10)
	require.ErrorIs(t, err, ErrImageTaskNotFound)

	got, err := svc.GetForAPIKey(context.Background(), task.TaskID, 7, 9)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, got.TaskID)
}

func TestImageTaskServiceCleanupExpiredDeletesFilesAndRecords(t *testing.T) {
	repo := newImageTaskRepoMemory()
	dir := t.TempDir()
	svc := NewImageTaskServiceWithOptions(repo, dir, time.Hour)
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	task, err := svc.CreatePending(context.Background(), 7, 9, "/v1/images/generations", "gpt-image-2", "draw a cat")
	require.NoError(t, err)
	saved, err := svc.SaveResult(context.Background(), task.TaskID, []byte("png-bytes"), "image/png")
	require.NoError(t, err)

	svc.now = func() time.Time { return now.Add(time.Hour + time.Second) }
	deleted, err := svc.CleanupExpired(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	require.NoFileExists(t, saved.FilePath)

	_, err = repo.GetByTaskID(context.Background(), task.TaskID)
	require.True(t, errors.Is(err, ErrImageTaskNotFound))
}
