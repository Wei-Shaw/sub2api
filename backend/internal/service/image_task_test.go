package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type imageTaskMemoryStore struct {
	task    *ImageTaskRecord
	ttl     time.Duration
	saveErr error
	getErr  error
	cleanup []ImageTaskCleanup
	deleted bool
}

func (s *imageTaskMemoryStore) Save(_ context.Context, task *ImageTaskRecord, ttl time.Duration) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	copy := *task
	s.task = &copy
	s.ttl = ttl
	return nil
}

func (s *imageTaskMemoryStore) Get(_ context.Context, _ string) (*ImageTaskRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.task == nil {
		return nil, ErrImageTaskNotFound
	}
	copy := *s.task
	return &copy, nil
}

func (s *imageTaskMemoryStore) Delete(_ context.Context, _ string) error {
	s.task = nil
	s.deleted = true
	return nil
}

func (s *imageTaskMemoryStore) ScheduleCleanup(_ context.Context, cleanup ImageTaskCleanup) error {
	s.cleanup = append(s.cleanup, cleanup)
	return nil
}

func (s *imageTaskMemoryStore) ListDueCleanup(_ context.Context, now time.Time, limit int) ([]ImageTaskCleanup, error) {
	out := make([]ImageTaskCleanup, 0)
	for _, cleanup := range s.cleanup {
		if cleanup.ExpiresAt <= now.Unix() && len(out) < limit {
			out = append(out, cleanup)
		}
	}
	return out, nil
}

func (s *imageTaskMemoryStore) DeleteCleanup(_ context.Context, id string) error {
	s.cleanup = slices.DeleteFunc(s.cleanup, func(cleanup ImageTaskCleanup) bool { return cleanup.TaskID == id })
	return nil
}

func TestImageTaskServiceLifecycleAndOwnership(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, 10*time.Minute)
	owner := ImageTaskOwner{UserID: 7, APIKeyID: 9}

	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusProcessing, created.Status)
	require.Equal(t, created.ID, created.TaskID)
	require.Equal(t, "image.generation.task", created.Object)
	require.Equal(t, time.Hour, store.ttl)
	require.Equal(t, owner.UserID, store.task.UserID)
	require.Equal(t, owner.APIKeyID, store.task.APIKeyID)
	require.InDelta(t, created.CreatedAt+int64(time.Hour/time.Second), created.ExpiresAt, 1)

	_, err = svc.Get(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 10}, created.ID)
	require.ErrorIs(t, err, ErrImageTaskNotFound)

	result := json.RawMessage(`{"created":123,"data":[{"url":"https://example.test/image.png"}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	completed, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusCompleted, completed.Status)
	require.Equal(t, http.StatusOK, completed.HTTPStatus)
	require.Equal(t, "https://example.test/image.png", completed.ImageURL)
	require.JSONEq(t, string(result), string(completed.Result))
	require.NotNil(t, completed.CompletedAt)
	require.InDelta(t, *completed.CompletedAt+int64(time.Hour/time.Second), completed.ExpiresAt, 1)
}

func TestImageTaskServiceInvalidResultBecomesFailed(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2})
	require.NoError(t, err)

	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`not-json`)))
	got, err := svc.Get(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2}, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusFailed, got.Status)
	require.Equal(t, http.StatusBadGateway, got.HTTPStatus)
	require.Contains(t, string(got.Error), "non-JSON")
}

func TestImageTaskServiceMapsStoreFailures(t *testing.T) {
	store := &imageTaskMemoryStore{saveErr: errors.New("redis down")}
	svc := NewImageTaskService(store)

	_, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2})
	require.ErrorIs(t, err, ErrImageTaskUnavailable)
}

func TestImageTaskServiceDashboardMetadataAndUserOwnership(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)
	created, err := svc.CreateWithMetadata(
		context.Background(),
		ImageTaskOwner{UserID: 7, APIKeyID: 9},
		ImageTaskMetadata{GroupID: 3, Platform: PlatformOpenAI, Model: "gpt-image-2", PromptPreview: "draw a lighthouse", InputImageCount: 2},
	)
	require.NoError(t, err)
	require.Equal(t, int64(3), created.GroupID)
	require.Equal(t, PlatformOpenAI, created.Platform)
	require.Equal(t, "gpt-image-2", created.Model)
	require.Equal(t, 2, created.InputImageCount)
	require.Equal(t, "draw a lighthouse", created.PromptPreview)

	got, err := svc.GetForUser(context.Background(), 7, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)

	_, err = svc.GetForUser(context.Background(), 8, created.ID)
	require.ErrorIs(t, err, ErrImageTaskNotFound)
}

func TestImageTaskServiceDownloadForUser(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-data"))
	}))
	defer imageServer.Close()

	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 9})
	require.NoError(t, err)

	_, err = svc.DownloadForUser(context.Background(), 7, created.ID, 0)
	require.ErrorIs(t, err, ErrImageTaskNotReady)

	result, err := json.Marshal(map[string]any{"data": []map[string]string{{"url": imageServer.URL + "/image.png"}}})
	require.NoError(t, err)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	_, err = svc.DownloadForUser(context.Background(), 8, created.ID, 0)
	require.ErrorIs(t, err, ErrImageTaskNotFound)
	_, err = svc.DownloadForUser(context.Background(), 7, created.ID, 1)
	require.ErrorIs(t, err, ErrImageTaskImageNotFound)

	download, err := svc.DownloadForUser(context.Background(), 7, created.ID, 0)
	require.NoError(t, err)
	require.Equal(t, []byte("png-data"), download.Data)
	require.Equal(t, "image/png", download.ContentType)
	require.Equal(t, created.ID+"-1.png", download.Filename)
}

func TestImageTaskServiceDeleteForUserRemovesStoredFilesAndRecord(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{}
	uploader := NewImageResultUploader(storage, "images/", 0, nil)
	svc := NewImageTaskServiceWithUploader(store, uploader, time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 9})
	require.NoError(t, err)

	err = svc.DeleteForUser(context.Background(), 7, created.ID)
	require.ErrorIs(t, err, ErrImageTaskDeleteNotReady)
	require.False(t, store.deleted)

	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`{"data":[{"b64_json":"`+b64+`"}]}`)))
	require.Len(t, store.cleanup, 1)
	require.Equal(t, []string{"images/" + created.ID + "-0.png"}, store.cleanup[0].Keys)

	err = svc.DeleteForUser(context.Background(), 8, created.ID)
	require.ErrorIs(t, err, ErrImageTaskNotFound)
	require.NotNil(t, store.task)

	require.NoError(t, svc.DeleteForUser(context.Background(), 7, created.ID))
	require.True(t, store.deleted)
	require.Empty(t, store.cleanup)
	require.Equal(t, []string{"images/" + created.ID + "-0.png"}, storage.deleted)
}

func TestImageTaskServiceDeleteFailureKeepsTaskForRetry(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{deleteErr: errors.New("bucket unavailable")}
	uploader := NewImageResultUploader(storage, "images/", 0, nil)
	svc := NewImageTaskServiceWithUploader(store, uploader, time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 9})
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`{"data":[{"b64_json":"`+b64+`"}]}`)))

	err = svc.DeleteForUser(context.Background(), 7, created.ID)
	require.ErrorIs(t, err, ErrImageTaskUnavailable)
	require.NotNil(t, store.task)
	require.False(t, store.deleted)
	require.Len(t, store.cleanup, 1)
}

func TestImageTaskServiceCleanupDeletesExpiredObjects(t *testing.T) {
	store := &imageTaskMemoryStore{cleanup: []ImageTaskCleanup{
		{TaskID: "expired", Keys: []string{"images/expired-0.png"}, ExpiresAt: 100},
		{TaskID: "future", Keys: []string{"images/future-0.png"}, ExpiresAt: 300},
	}}
	storage := &fakeImageStorage{}
	svc := NewImageTaskServiceWithUploader(store, NewImageResultUploader(storage, "images/", 0, nil), time.Hour, time.Minute)

	cleaned, err := svc.RunCleanupOnce(context.Background(), time.Unix(200, 0))
	require.NoError(t, err)
	require.Equal(t, 1, cleaned)
	require.Equal(t, []string{"images/expired-0.png"}, storage.deleted)
	require.Equal(t, []ImageTaskCleanup{{TaskID: "future", Keys: []string{"images/future-0.png"}, ExpiresAt: 300}}, store.cleanup)
}

func TestImageTaskServiceRefusesDeleteAfterStorageTargetChanges(t *testing.T) {
	store := &imageTaskMemoryStore{}
	oldStorage := &fakeImageStorage{}
	oldUploader := NewImageResultUploader(oldStorage, "images/", 0, nil)
	oldUploader.storageIdentity = "s3:old"
	currentUploader := oldUploader
	svc := NewImageTaskServiceWithResolver(store, func() (*ImageResultUploader, bool, time.Duration) {
		return currentUploader, true, time.Hour
	}, time.Hour, time.Minute)

	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 9})
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`{"data":[{"b64_json":"`+b64+`"}]}`)))
	require.Equal(t, "s3:old", store.task.StorageIdentity)
	require.Equal(t, "s3:old", store.cleanup[0].StorageIdentity)

	newStorage := &fakeImageStorage{}
	currentUploader = NewImageResultUploader(newStorage, "images/", 0, nil)
	currentUploader.storageIdentity = "s3:new"

	err = svc.DeleteForUser(context.Background(), 7, created.ID)
	require.ErrorIs(t, err, ErrImageTaskUnavailable)
	require.NotNil(t, store.task)
	require.Empty(t, newStorage.deleted)

	cleaned, err := svc.RunCleanupOnce(context.Background(), time.Unix(store.task.ExpiresAt+1, 0))
	require.NoError(t, err)
	require.Zero(t, cleaned)
	require.Len(t, store.cleanup, 1)
	require.Empty(t, newStorage.deleted)
}
