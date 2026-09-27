//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ctxCheckingImageStore fails saves whose context is already done, like a real object store would.
type ctxCheckingImageStore struct {
	*fakeImageObjectStore
}

func (s ctxCheckingImageStore) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return s.fakeImageObjectStore.Save(ctx, key, contentType, data)
}

// runUntilStopped starts a job whose executor blocks until its context is canceled, calls Stop,
// and returns the job as it was written back.
func runUntilStopped(t *testing.T, repo *fakeImageStudioRepo, studio *ImageStudioService, status int, body []byte) *ImageStudioJob {
	t.Helper()
	started := make(chan struct{})
	returned := make(chan struct{})
	sub := &ImageStudioSubmission{Job: &ImageStudioJob{ID: 7, UserID: 10, Params: []byte(`{"n":1}`)}, APIKey: "sk-own", Path: "/v1/images/generations"}
	go func() {
		defer close(returned)
		studio.Run(sub, func(ctx context.Context, _, _ string, _ []byte) (int, []byte) {
			close(started)
			<-ctx.Done()
			return status, body
		})
	}()
	<-started

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		studio.Stop()
	}()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return")
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Stop returned before the running job was written back")
	}
	require.NotNil(t, repo.finished, "the job must be written back before Stop returns")
	return repo.finished
}

func TestImageStudioStopInterruptsRunningJob(t *testing.T) {
	repo := &fakeImageStudioRepo{}
	studio := newTestImageStudio(repo, &fakeImageObjectStore{})

	job := runUntilStopped(t, repo, studio, http.StatusBadGateway, []byte(`{"error":{"message":"context canceled"}}`))

	require.Equal(t, ImageStudioStatusFailed, job.Status)
	require.Equal(t, imageStudioInterruptedMessage, job.ErrorMessage)

	// A job submitted after shutdown gets an already-canceled context instead of running untracked.
	var runCtxErr error
	studio.Run(&ImageStudioSubmission{Job: &ImageStudioJob{ID: 8, UserID: 10, Params: []byte(`{"n":1}`)}}, func(ctx context.Context, _, _ string, _ []byte) (int, []byte) {
		runCtxErr = ctx.Err()
		return 0, nil
	})
	require.True(t, errors.Is(runCtxErr, context.Canceled))
	require.Equal(t, imageStudioInterruptedMessage, repo.finished.ErrorMessage)
}

func TestImageStudioStopStillSavesImagesUpstreamAlreadyReturned(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nfake"))
	repo, store := &fakeImageStudioRepo{}, &fakeImageObjectStore{}
	studio := newTestImageStudio(repo, store)
	uploader := NewImageResultUploader(ctxCheckingImageStore{store}, "img/", 0, nil)
	studio.resolve = func() (*ImageResultUploader, bool) { return uploader, true }

	job := runUntilStopped(t, repo, studio, http.StatusOK, []byte(`{"data":[{"b64_json":"`+png+`"}]}`))

	require.Equal(t, ImageStudioStatusSucceeded, job.Status, job.ErrorMessage)
	require.Len(t, repo.assets, 1, "billed images must be stored even though the job context was canceled")
}
