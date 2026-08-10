package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type imageTaskMemoryStore struct {
	task     *ImageTaskRecord
	ttl      time.Duration
	saveErr  error
	getErr   error
	cleanup  []ImageTaskCleanup
	deleted  bool
	listed   []*ImageTaskRecord
	mutation sync.Mutex
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

func (s *imageTaskMemoryStore) ListByUser(_ context.Context, userID int64, _ int) ([]*ImageTaskRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.listed != nil {
		out := make([]*ImageTaskRecord, 0, len(s.listed))
		for _, task := range s.listed {
			copy := *task
			out = append(out, &copy)
		}
		return out, nil
	}
	if s.task == nil || s.task.UserID != userID {
		return []*ImageTaskRecord{}, nil
	}
	copy := *s.task
	return []*ImageTaskRecord{&copy}, nil
}

func (s *imageTaskMemoryStore) ListForAdmin(_ context.Context, offset int64, limit int) ([]*ImageTaskRecord, int, error) {
	all := s.listed
	if len(all) == 0 && s.task != nil {
		all = []*ImageTaskRecord{s.task}
	}
	if offset >= int64(len(all)) {
		return []*ImageTaskRecord{}, len(all), nil
	}
	end := int(offset) + limit
	if end > len(all) {
		end = len(all)
	}
	return append([]*ImageTaskRecord(nil), all[int(offset):end]...), len(all), nil
}

func (s *imageTaskMemoryStore) AdminStorageStats(context.Context) (int, int64, error) {
	all := s.listed
	if len(all) == 0 && s.task != nil {
		all = []*ImageTaskRecord{s.task}
	}
	var images int
	var bytes int64
	for _, task := range all {
		images += len(task.StorageKeys)
		bytes += sumImageStorageSizes(task.StorageSizes)
	}
	return images, bytes, nil
}

func (s *imageTaskMemoryStore) Delete(_ context.Context, _ string) error {
	s.task = nil
	s.deleted = true
	return nil
}

func (s *imageTaskMemoryStore) ScheduleCleanup(_ context.Context, cleanup ImageTaskCleanup) error {
	for i := range s.cleanup {
		if s.cleanup[i].TaskID == cleanup.TaskID {
			s.cleanup[i] = cleanup
			return nil
		}
	}
	s.cleanup = append(s.cleanup, cleanup)
	return nil
}

func (s *imageTaskMemoryStore) GetCleanup(_ context.Context, id string) (*ImageTaskCleanup, error) {
	for i := range s.cleanup {
		if s.cleanup[i].TaskID == id {
			copy := s.cleanup[i]
			return &copy, nil
		}
	}
	return nil, ErrImageTaskNotFound
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

func (s *imageTaskMemoryStore) TryLock(context.Context, string, string, time.Duration) (bool, error) {
	return s.mutation.TryLock(), nil
}

func (s *imageTaskMemoryStore) Unlock(context.Context, string, string) error {
	s.mutation.Unlock()
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
	require.Equal(t, owner.UserID, store.task.UserID)
	require.Equal(t, owner.APIKeyID, store.task.APIKeyID)
	require.Zero(t, created.ExpiresAt, "retention starts only after generation finishes")
	require.Equal(t, 70*time.Minute, store.ttl, "processing TTL covers execution plus result retention")

	_, err = svc.Get(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 10}, created.ID)
	require.ErrorIs(t, err, ErrImageTaskNotFound)

	result := json.RawMessage(`{"created":123,"data":[{"url":"https://example.test/image.png"}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	completed, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusCompleted, completed.Status)
	require.Equal(t, http.StatusOK, completed.HTTPStatus)
	require.Empty(t, completed.ImageURL)
	require.Zero(t, completed.ImageCount, "a legacy result without private storage keys must not be advertised as downloadable")
	require.NotContains(t, string(completed.Result), "example.test")
	require.NotNil(t, completed.CompletedAt)
	require.InDelta(t, *completed.CompletedAt+int64(time.Hour/time.Second), completed.ExpiresAt, 1)
}

func TestImageTaskServiceNeverExposesLegacyStoredURLs(t *testing.T) {
	store := &imageTaskMemoryStore{task: &ImageTaskRecord{
		ID: "legacy", UserID: 7, APIKeyID: 9, Status: ImageTaskStatusCompleted,
		Result:      json.RawMessage(`{"data":[{"url":"https://public.example/images/legacy.png","b64_json":"secret","revised_prompt":"kept"}]}`),
		StorageKeys: []string{"images/legacy-0.png"},
	}}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)

	task, err := svc.Get(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 9}, "legacy")
	require.NoError(t, err)
	require.Equal(t, 1, task.ImageCount)
	require.NotContains(t, string(task.Result), "public.example")
	require.NotContains(t, string(task.Result), "b64_json")
	require.Contains(t, string(task.Result), "revised_prompt")
}

func TestImageTaskServiceStripsNonstandardImageURLsBeforePersistence(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{}
	svc := NewImageTaskServiceWithUploader(store, NewImageResultUploader(storage, "images/", 0, nil), time.Hour, time.Minute)
	owner := ImageTaskOwner{UserID: 7, APIKeyID: 9}
	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	result := json.RawMessage(`{"data":[{"b64_json":"` + b64 + `","metadata":{"image_url":"https://public.example/image.png","download_url":"https://public.example/download"}}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	require.NotContains(t, string(store.task.Result), "public.example")
	require.NotContains(t, string(store.task.Result), "image_url")
	require.NotContains(t, string(store.task.Result), "download_url")
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

func TestImageTaskServiceListForUserFiltersForeignRecords(t *testing.T) {
	store := &imageTaskMemoryStore{listed: []*ImageTaskRecord{
		{ID: "own-new", UserID: 7, CreatedAt: 3, Status: ImageTaskStatusCompleted},
		{ID: "foreign", UserID: 8, CreatedAt: 2, Status: ImageTaskStatusCompleted},
		{ID: "own-old", UserID: 7, CreatedAt: 1, Status: ImageTaskStatusFailed},
	}}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)

	tasks, err := svc.ListForUser(context.Background(), 7, 24)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, "own-new", tasks[0].ID)
	require.Equal(t, "own-old", tasks[1].ID)

	tasks, err = svc.ListForUser(context.Background(), 0, 24)
	require.NoError(t, err)
	require.Empty(t, tasks)
}

func TestImageTaskServiceDownloadForUser(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{}
	svc := NewImageTaskServiceWithUploader(store, NewImageResultUploader(storage, "images/", 0, nil), time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 9})
	require.NoError(t, err)

	_, err = svc.DownloadForUser(context.Background(), 7, created.ID, "0")
	require.ErrorIs(t, err, ErrImageTaskNotReady)

	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`{"data":[{"b64_json":"`+b64+`"}]}`)))

	_, err = svc.DownloadForUser(context.Background(), 8, created.ID, "0")
	require.ErrorIs(t, err, ErrImageTaskNotFound)
	_, err = svc.DownloadForUser(context.Background(), 7, created.ID, "1")
	require.ErrorIs(t, err, ErrImageTaskImageNotFound)

	download, err := svc.DownloadForUser(context.Background(), 7, created.ID, "0")
	require.NoError(t, err)
	require.Equal(t, pngBytes, download.Data)
	require.Equal(t, "image/png", download.ContentType)
	require.Equal(t, created.ID+"-1.png", download.Filename)
}

func TestImageTaskServiceDownloadRequiresSubmittingAPIKey(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{}
	svc := NewImageTaskServiceWithUploader(store, NewImageResultUploader(storage, "images/", 0, nil), time.Hour, time.Minute)
	owner := ImageTaskOwner{UserID: 7, APIKeyID: 9}
	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`{"data":[{"b64_json":"`+b64+`"}]}`)))

	_, err = svc.Download(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 10}, created.ID, "0")
	require.ErrorIs(t, err, ErrImageTaskNotFound)
	_, err = svc.Download(context.Background(), ImageTaskOwner{UserID: 8, APIKeyID: 9}, created.ID, "0")
	require.ErrorIs(t, err, ErrImageTaskNotFound)
	download, err := svc.Download(context.Background(), owner, created.ID, "0")
	require.NoError(t, err)
	require.Equal(t, pngBytes, download.Data)
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

func TestImageTaskServiceDeleteSingleImageUpdatesTaskAndCleanup(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{}
	svc := NewImageTaskServiceWithUploader(store, NewImageResultUploader(storage, "images/", 0, nil), time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 9})
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	result := json.RawMessage(`{"data":[{"b64_json":"` + b64 + `","revised_prompt":"first"},{"b64_json":"` + b64 + `","revised_prompt":"second"}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))
	require.Equal(t, []int64{int64(len(pngBytes)), int64(len(pngBytes))}, store.task.StorageSizes)

	firstImageID := imageTaskImageID(created.ID, "images/"+created.ID+"-0.png")
	updated, err := svc.DeleteImageForUser(context.Background(), 7, created.ID, firstImageID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, updated.ImageCount)
	require.Equal(t, []int64{int64(len(pngBytes))}, updated.ImageSizes)
	require.Equal(t, []string{"images/" + created.ID + "-1.png"}, store.task.StorageKeys)
	require.NotContains(t, string(store.task.Result), "first")
	require.Contains(t, string(store.task.Result), "second")
	require.Equal(t, []string{"images/" + created.ID + "-1.png"}, store.cleanup[0].Keys)
	require.Equal(t, []string{"images/" + created.ID + "-0.png"}, storage.deleted)

	updated, err = svc.DeleteImageForUser(context.Background(), 7, created.ID, firstImageID)
	require.ErrorIs(t, err, ErrImageTaskImageNotFound)
	require.Nil(t, updated)
	require.Equal(t, []string{"images/" + created.ID + "-1.png"}, store.task.StorageKeys,
		"a stale stable image ID must not drift to the remaining array index")

	updated, err = svc.DeleteImageForUser(context.Background(), 7, created.ID, "0")
	require.NoError(t, err)
	require.Nil(t, updated)
	require.True(t, store.deleted)
	require.Empty(t, store.cleanup)
	require.Equal(t, []string{
		"images/" + created.ID + "-0.png",
		"images/" + created.ID + "-1.png",
	}, storage.deleted)
}

func TestImageTaskServiceSerializesConcurrentStableImageDeletes(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &fakeImageStorage{}
	svc := NewImageTaskServiceWithUploader(store, NewImageResultUploader(storage, "images/", 0, nil), time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 9})
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(
		`{"data":[{"b64_json":"`+b64+`"},{"b64_json":"`+b64+`"}]}`,
	)))
	imageIDs := imageTaskToPublic(store.task).ImageIDs
	require.Len(t, imageIDs, 2)

	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for _, imageID := range imageIDs {
		wait.Add(1)
		go func(ref string) {
			defer wait.Done()
			_, deleteErr := svc.DeleteImageForUser(context.Background(), 7, created.ID, ref)
			errors <- deleteErr
		}(imageID)
	}
	wait.Wait()
	close(errors)
	for deleteErr := range errors {
		require.NoError(t, deleteErr)
	}
	require.True(t, store.deleted)
	require.ElementsMatch(t, []string{
		"images/" + created.ID + "-0.png",
		"images/" + created.ID + "-1.png",
	}, storage.deleted)
}

func TestImageTaskServiceListForAdminPaginatesAndTotalsStorage(t *testing.T) {
	store := &imageTaskMemoryStore{listed: []*ImageTaskRecord{
		{ID: "new", UserID: 7, APIKeyID: 9, CreatedAt: 3, Status: ImageTaskStatusCompleted, StorageKeys: []string{"new-0", "new-1"}, StorageSizes: []int64{100, 200}},
		{ID: "old", UserID: 8, APIKeyID: 10, CreatedAt: 2, Status: ImageTaskStatusCompleted, StorageKeys: []string{"old-0"}, StorageSizes: []int64{300}},
	}}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)

	page, err := svc.ListForAdmin(context.Background(), 2, 1)
	require.NoError(t, err)
	require.Equal(t, 2, page.Total)
	require.Equal(t, 3, page.TotalImages)
	require.Equal(t, int64(600), page.StorageBytes)
	require.Len(t, page.Tasks, 1)
	require.Equal(t, "old", page.Tasks[0].Task.ID)
	require.Equal(t, int64(300), page.Tasks[0].StorageBytes)
}

func TestImageTaskServiceListForAdminHandlesExtremePageWithoutOverflow(t *testing.T) {
	store := &imageTaskMemoryStore{listed: []*ImageTaskRecord{{
		ID: "only", CreatedAt: 1, Status: ImageTaskStatusCompleted,
	}}}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)

	page, err := svc.ListForAdmin(context.Background(), math.MaxInt, 100)
	require.NoError(t, err)
	require.Equal(t, math.MaxInt, page.Page)
	require.Equal(t, 1, page.Total)
	require.Empty(t, page.Tasks)
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
