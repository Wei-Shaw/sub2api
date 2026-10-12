//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type videoMonitorTestRepo struct {
	done        bool
	state, code string
	finishes    int
}

type videoMonitorWorkerTestRepo struct {
	videoTaskTestRepo
	videoMonitorTestRepo
	claimed chan struct{}
}

func (r *videoMonitorWorkerTestRepo) ClaimDueVideoTaskMonitor(ctx context.Context, _ time.Duration) (*VideoTask, error) {
	select {
	case r.claimed <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestVideoMonitorWorkerShutdownWaitsForIndependentMonitor(t *testing.T) {
	repo := &videoMonitorWorkerTestRepo{claimed: make(chan struct{}, 1)}
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	svc.StartVideoTaskWorker(&APIKeyService{})
	select {
	case <-repo.claimed:
	case <-time.After(time.Second):
		t.Fatal("independent monitor did not start")
	}
	svc.StopVideoTaskWorker()
	select {
	case <-svc.videoTaskDone:
	default:
		t.Fatal("worker stopped before monitor shutdown")
	}
}

func (*videoMonitorTestRepo) ClaimDueVideoTaskMonitor(context.Context, time.Duration) (*VideoTask, error) {
	return nil, nil
}
func (r *videoMonitorTestRepo) SyncVideoTaskMonitorUsage(context.Context, *VideoTask) (bool, error) {
	return r.done, nil
}
func (r *videoMonitorTestRepo) FinishVideoTaskMonitor(_ context.Context, _ *VideoTask, state, code string) error {
	r.state, r.code = state, code
	r.finishes++
	return nil
}

func TestVideoMonitorOnlyObservesAndReusesExistingUsage(t *testing.T) {
	for _, billed := range []bool{false, true} {
		task := videoTaskTestFixture()
		task.Pending.TrackingOnly = true
		task.CostSnapshot = nil // monitoring must never need a price or a billing input
		repo := &videoMonitorTestRepo{done: billed}
		polls := 0
		monitor := &videoTaskMonitor{repo: repo, poll: func(context.Context, *VideoTask) (string, string) {
			polls++
			return VideoTaskCompleted, ""
		}}
		require.NoError(t, monitor.process(context.Background(), task))
		if billed {
			require.Zero(t, polls, "existing usage avoids even an upstream lookup")
			require.Zero(t, repo.finishes)
		} else {
			require.Equal(t, 1, polls)
			require.Equal(t, VideoTaskCompleted, repo.state)
		}
		processor := &videoTaskProcessor{bill: func(context.Context, *VideoTask) error {
			t.Fatal("monitoring must never call billing")
			return nil
		}}
		require.ErrorContains(t, processor.process(context.Background(), task), "cannot enter settlement")
	}
}

func TestVideoMonitorDoesNotPollUnknownSubmissionsOrDurableBillingTasks(t *testing.T) {
	monitor := &videoTaskMonitor{poll: func(context.Context, *VideoTask) (string, string) {
		t.Fatal("unsafe task was queried")
		return "", ""
	}}
	task := videoTaskTestFixture()
	require.Error(t, monitor.process(context.Background(), task))
	task.Pending.TrackingOnly = true
	task.TaskID = "submission:unknown"
	require.Error(t, monitor.process(context.Background(), task))
}

func TestVideoMonitorPauseDoesNotDeclareFailure(t *testing.T) {
	task := videoTaskTestFixture()
	task.Pending.TrackingOnly = true
	task.Pending.CreatedAt = time.Now().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	task.Attempts = 20
	repo := &videoMonitorTestRepo{}
	monitor := &videoTaskMonitor{repo: repo, poll: func(context.Context, *VideoTask) (string, string) {
		return VideoTaskPending, "poll_not_found"
	}}
	require.NoError(t, monitor.process(context.Background(), task))
	require.Equal(t, VideoTaskPending, repo.state)
	require.Equal(t, "poll_exhausted", repo.code)
	// A slow but newly accepted task remains recoverable even after many checks.
	task.Pending.CreatedAt = time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
	require.NoError(t, monitor.process(context.Background(), task))
	require.Equal(t, "poll_not_found", repo.code)
}

func TestVideoTaskOldPollFailuresPauseWithoutChangingBillingRetries(t *testing.T) {
	task := videoTaskTestFixture()
	task.Pending.CreatedAt = time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)
	task.Attempts = 2600
	repo := &videoTaskTestRepo{task: task}
	processor := &videoTaskProcessor{repo: repo}
	require.Error(t, processor.retry(task, "poll_failed", errors.New("not found")))
	require.Equal(t, "poll_exhausted", repo.errorCode)
	require.Equal(t, VideoTaskPending, task.State)
	task.State = VideoTaskReady
	task.Completion = &VideoTaskCompletion{}
	require.Error(t, processor.retry(task, "billing_failed", errors.New("temporary database error")))
	require.Equal(t, "billing_failed", repo.errorCode, "existing settlement recovery remains unchanged")
}

func TestVideoMonitorPinnedNativeGETWithoutBillingOrDownload(t *testing.T) {
	for _, tc := range []struct {
		name, body, state, code string
		ark                     bool
		status                  int
	}{
		{"grok done", `{"status":"done","video":{"url":"https://video.invalid/private.mp4"}}`, VideoTaskCompleted, "", false, 200},
		{"grok queued", `{"status":"pending"}`, VideoTaskPending, "", false, 200},
		{"grok failed", `{"status":"failed"}`, VideoTaskFailed, "upstream_failed", false, 200},
		{"grok expired", `{"status":"expired"}`, VideoTaskPending, "upstream_expired", false, 200},
		{"not found", `{"error":"not found"}`, VideoTaskPending, "poll_not_found", false, 404},
		{"ark complete without tokens", `{"status":"succeeded","content":{"video_url":"https://video.invalid/private.mp4"}}`, VideoTaskCompleted, "", true, 200},
		{"ark cancelled", `{"status":"cancelled"}`, VideoTaskFailed, "upstream_cancelled", true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{ID: 313, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example/v1", "api_key": "test-only"}}
			task := videoTaskTestFixture()
			if tc.ark {
				account = seedanceTestAccount()
				task.AccountID = account.ID
				task.TaskID = "seedance:task-1"
			}
			task.Pending.Platform = account.Platform
			task.Pending.TrackingOnly = true
			task.RoutingFingerprint = videoTaskMonitorRoutingFingerprint(account)
			resp := grokMediaContentStatusResponse(tc.body)
			resp.StatusCode = tc.status
			upstream := &grokMediaContentUpstreamStub{response: resp}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: videoTaskAccountRepo{account: account}, httpUpstream: upstream}
			state, code := svc.pollVideoTaskMonitor(context.Background(), task)
			require.Equal(t, tc.state, state)
			require.Equal(t, tc.code, code)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, http.MethodGet, upstream.request.Method)
			require.NotRegexp(t, `/content$`, upstream.request.URL.Path)
			account.Credentials["base_url"] = "https://different.example"
			state, code = svc.pollVideoTaskMonitor(context.Background(), task)
			require.Equal(t, VideoTaskPending, state)
			require.Equal(t, "account_route_changed", code)
			require.Len(t, upstream.requests, 1, "route changes must prevent further requests")
		})
	}
}
