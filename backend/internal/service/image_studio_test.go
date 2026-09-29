//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type fakeImageStudioRepo struct {
	ImageStudioRepository
	unfinished int
	created    *ImageStudioJob
	finished   *ImageStudioJob
	assets     []ImageStudioAsset

	expiredBefore time.Time
}

func (r *fakeImageStudioRepo) CreateJob(_ context.Context, job *ImageStudioJob) error {
	job.ID = 7
	r.created = job
	return nil
}
func (r *fakeImageStudioRepo) CreateJobWithinLimit(ctx context.Context, job *ImageStudioJob, limit int) error {
	if r.unfinished >= limit {
		return ErrImageStudioBusy
	}
	return r.CreateJob(ctx, job)
}
func (r *fakeImageStudioRepo) FailStaleJobs(context.Context, int64, time.Time, string) error {
	return nil
}
func (r *fakeImageStudioRepo) CountUnfinishedJobs(context.Context, int64) (int, error) {
	return r.unfinished, nil
}
func (r *fakeImageStudioRepo) MarkJobRunning(context.Context, int64) error { return nil }
func (r *fakeImageStudioRepo) DeleteExpiredJobs(_ context.Context, before time.Time, _ int) ([]string, int, error) {
	r.expiredBefore = before
	return []string{"img/studio/10/1-0.png"}, 1, nil
}
func (r *fakeImageStudioRepo) FinishJob(_ context.Context, job *ImageStudioJob, assets []ImageStudioAsset) error {
	r.finished, r.assets = job, assets
	return nil
}

type fakeImageObjectStore struct {
	saved   []string
	failAt  int
	deleted []string
}

func (s *fakeImageObjectStore) Save(_ context.Context, key, _ string, _ []byte) (string, error) {
	if s.failAt > 0 && len(s.saved)+1 == s.failAt {
		return "", errors.New("bucket unavailable")
	}
	s.saved = append(s.saved, key)
	return "https://example.invalid/" + key, nil
}
func (s *fakeImageObjectStore) Open(context.Context, string) (io.ReadCloser, string, error) {
	return io.NopCloser(bytes.NewReader(nil)), "", nil
}
func (s *fakeImageObjectStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

type fakeImageStudioKeys map[int64]*APIKey

func (k fakeImageStudioKeys) GetByID(_ context.Context, id int64) (*APIKey, error) {
	if key, ok := k[id]; ok {
		return key, nil
	}
	return nil, errors.New("not found")
}

func newTestImageStudio(repo *fakeImageStudioRepo, store *fakeImageObjectStore) *ImageStudioService {
	uploader := NewImageResultUploader(store, "img/", 0, nil)
	return &ImageStudioService{
		repo: repo,
		apiKeys: fakeImageStudioKeys{
			1: {ID: 1, UserID: 10, Key: "sk-own", Group: &Group{Platform: PlatformOpenAI}},
			2: {ID: 2, UserID: 99, Key: "sk-other", Group: &Group{Platform: PlatformOpenAI}},
			3: {ID: 3, UserID: 10, Key: "sk-gemini", Group: &Group{Platform: PlatformGemini}},
		},
		resolve: func() (*ImageResultUploader, bool) { return uploader, true },
	}
}

func TestImageStudioSubmitValidatesOwnershipPlatformAndConcurrency(t *testing.T) {
	ctx := context.Background()
	repo := &fakeImageStudioRepo{}
	studio := newTestImageStudio(repo, &fakeImageObjectStore{})
	in := ImageStudioCreateInput{APIKeyID: 1, Model: "gpt-image-2", Prompt: "a pelican"}

	_, err := studio.Submit(ctx, 10, ImageStudioCreateInput{APIKeyID: 2, Model: "gpt-image-2", Prompt: "x"})
	require.ErrorContains(t, err, "API key not found", "another user's key must look nonexistent")

	_, err = studio.Submit(ctx, 10, ImageStudioCreateInput{APIKeyID: 3, Model: "gpt-image-2", Prompt: "x"})
	require.ErrorContains(t, err, "OpenAI and Grok")

	_, err = studio.Submit(ctx, 10, ImageStudioCreateInput{APIKeyID: 1, Model: "gpt-image-2", Prompt: "x", N: 5})
	require.Error(t, err)

	repo.unfinished = imageStudioMaxUnfinishedPerUser
	_, err = studio.Submit(ctx, 10, in)
	require.ErrorIs(t, err, ErrImageStudioBusy)

	repo.unfinished = 0
	in.InputImages = []string{"data:image/png;base64,AAAA"}
	in.Size = "1024x1024"
	sub, err := studio.Submit(ctx, 10, in)
	require.NoError(t, err)
	require.Equal(t, "/v1/images/edits", sub.Path)
	require.Equal(t, "sk-own", sub.APIKey)
	require.Equal(t, ImageStudioKindEdit, repo.created.Kind)
	require.Equal(t, "data:image/png;base64,AAAA", gjson.GetBytes(sub.Body, "images.0.image_url").String())
	require.Equal(t, "b64_json", gjson.GetBytes(sub.Body, "response_format").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(repo.created.Params, "size").String())
	require.NotContains(t, string(repo.created.Params), "sk-own")

	_, err = studio.Submit(ctx, 10, ImageStudioCreateInput{APIKeyID: 1, Model: "m", Prompt: "x", InputImages: []string{"https://evil.example/a.png"}})
	require.Error(t, err, "remote URLs are rejected (no SSRF surface)")
}

func TestImageStudioRunStoresAssetsAndHandlesFailures(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nfake"))
	twoImages := []byte(`{"data":[{"b64_json":"` + png + `","revised_prompt":"rp"},{"b64_json":"` + png + `"}]}`)
	newSub := func() *ImageStudioSubmission {
		return &ImageStudioSubmission{Job: &ImageStudioJob{ID: 7, UserID: 10, Params: []byte(`{"n":2}`)}, APIKey: "sk-own", Path: "/v1/images/generations"}
	}

	t.Run("success", func(t *testing.T) {
		repo, store := &fakeImageStudioRepo{}, &fakeImageObjectStore{}
		var gotKey string
		newTestImageStudio(repo, store).Run(newSub(), func(_ context.Context, _, apiKey string, _ []byte) (int, []byte) {
			gotKey = apiKey
			return http.StatusOK, twoImages
		})
		require.Equal(t, "sk-own", gotKey)
		require.Equal(t, ImageStudioStatusSucceeded, repo.finished.Status)
		require.Equal(t, 2, repo.finished.ImageCount)
		require.Len(t, repo.assets, 2)
		require.True(t, strings.HasPrefix(repo.assets[0].StorageKey, "img/studio/10/7-0"))
		require.Equal(t, "rp", repo.assets[0].RevisedPrompt)
	})

	t.Run("upstream error", func(t *testing.T) {
		repo := &fakeImageStudioRepo{}
		newTestImageStudio(repo, &fakeImageObjectStore{}).Run(newSub(), func(context.Context, string, string, []byte) (int, []byte) {
			return http.StatusForbidden, []byte(`{"error":{"message":"image generation not allowed"}}`)
		})
		require.Equal(t, ImageStudioStatusFailed, repo.finished.Status)
		require.Equal(t, "image generation not allowed", repo.finished.ErrorMessage)
		require.Empty(t, repo.assets)
	})

	t.Run("partial storage failure keeps saved images", func(t *testing.T) {
		repo := &fakeImageStudioRepo{}
		newTestImageStudio(repo, &fakeImageObjectStore{failAt: 2}).Run(newSub(), func(context.Context, string, string, []byte) (int, []byte) {
			return http.StatusOK, twoImages
		})
		require.Equal(t, ImageStudioStatusSucceeded, repo.finished.Status)
		require.Contains(t, repo.finished.Warning, "storage_failed")
		require.Len(t, repo.assets, 1)
	})
}

func TestImageStudioRetentionDeletesExpiredJobsAndObjects(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	repo, store := &fakeImageStudioRepo{}, &fakeImageObjectStore{}
	studio := newTestImageStudio(repo, store)

	studio.retention = func(context.Context) time.Duration { return 0 }
	deleted, err := studio.RunRetentionOnce(context.Background(), now)
	require.NoError(t, err)
	require.Zero(t, deleted, "retention 0 keeps images forever")
	require.True(t, repo.expiredBefore.IsZero())

	studio.retention = func(context.Context) time.Duration { return 30 * 24 * time.Hour }
	deleted, err = studio.RunRetentionOnce(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	require.Equal(t, now.Add(-30*24*time.Hour), repo.expiredBefore)
	require.Equal(t, []string{"img/studio/10/1-0.png"}, store.deleted)

	studio.resolve = func() (*ImageResultUploader, bool) { return nil, false }
	repo.expiredBefore = time.Time{}
	deleted, err = studio.RunRetentionOnce(context.Background(), now)
	require.NoError(t, err)
	require.Zero(t, deleted, "rows are kept while storage is off so objects are not orphaned")
}
