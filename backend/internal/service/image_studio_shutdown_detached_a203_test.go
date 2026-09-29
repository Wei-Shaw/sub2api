//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// a203FinishRecorder records every FinishJob write-back so a late overwrite is visible.
type a203FinishRecorder struct {
	*fakeImageStudioRepo
	mu     sync.Mutex
	writes []ImageStudioJob
}

func (r *a203FinishRecorder) FinishJob(_ context.Context, job *ImageStudioJob, _ []ImageStudioAsset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, *job)
	return nil
}

func (r *a203FinishRecorder) snapshot() []ImageStudioJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ImageStudioJob(nil), r.writes...)
}

func TestImageStudioStopInterruptsJobWhoseExecutorIgnoresCancellationA203(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nfake"))
	repo := &a203FinishRecorder{fakeImageStudioRepo: &fakeImageStudioRepo{}}
	store := &fakeImageObjectStore{}
	studio := newTestImageStudio(repo.fakeImageStudioRepo, store)
	studio.repo = repo

	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseExecutor := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseExecutor)

	started, returned := make(chan struct{}), make(chan struct{})
	sub := &ImageStudioSubmission{Job: &ImageStudioJob{ID: 7, UserID: 10, Params: []byte(`{"n":1}`)}, APIKey: "sk-own", Path: "/v1/images/generations"}
	go func() {
		defer close(returned)
		studio.Run(sub, func(context.Context, string, string, []byte) (int, []byte) {
			// Like the real gateway replay: the upstream call is detached and ignores ctx.
			close(started)
			<-release
			return http.StatusOK, []byte(`{"data":[{"b64_json":"` + png + `"}]}`)
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
	case <-time.After(imageStudioShutdownWait + 3*time.Second):
		t.Fatal("Stop did not return")
	}
	writes := repo.snapshot()
	require.Len(t, writes, 1, "Stop must write back a job still running after its wait budget")
	require.Equal(t, int64(7), writes[0].ID)
	require.Equal(t, ImageStudioStatusFailed, writes[0].Status)
	require.Equal(t, imageStudioInterruptedMessage, writes[0].ErrorMessage)

	// The detached upstream call returns later; its result must not clobber the interrupted state.
	releaseExecutor()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after the executor finished")
	}
	require.Len(t, repo.snapshot(), 1, "a late result must not overwrite the interrupted write-back")
	require.Empty(t, store.saved, "images of a job already written back as interrupted must not be stored")
}
