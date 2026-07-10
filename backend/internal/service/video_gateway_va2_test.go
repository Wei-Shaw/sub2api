package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// fakeVideoAdapter is a configurable in-memory adapter for VA2 worker tests. It
// reports the mock provider so a test can replace the registered mock adapter via
// the white-box svc.adapters map and drive deterministic running/succeeded paths.
type fakeVideoAdapter struct {
	pollsUntilSucceed int           // succeed on/after this poll number; <=0 => never terminal
	pollDelay         time.Duration // optional per-poll sleep to simulate a slow provider
	pollResult        *VideoAdapterResult
	polls             int
}

func (a *fakeVideoAdapter) Provider() string { return VideoProviderMock }

func (a *fakeVideoAdapter) CreateTask(_ context.Context, _ *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{
		UpstreamTaskID: fmt.Sprintf("fake-%d", task.ID),
		Status:         VideoStatusSubmitted,
	}, nil
}

func (a *fakeVideoAdapter) PollTask(_ context.Context, _ *VideoProviderAccount, _ *VideoTask) (*VideoAdapterResult, error) {
	if a.pollDelay > 0 {
		time.Sleep(a.pollDelay)
	}
	a.polls++
	if a.pollsUntilSucceed > 0 && a.polls >= a.pollsUntilSucceed {
		if a.pollResult != nil {
			return cloneVideoAdapterResult(a.pollResult), nil
		}
		return &VideoAdapterResult{
			Status:    VideoStatusSucceeded,
			ResultURL: "https://mock.sub2api.local/video/fake.mp4",
		}, nil
	}
	return &VideoAdapterResult{Status: VideoStatusRunning}, nil
}

func (a *fakeVideoAdapter) CancelTask(_ context.Context, _ *VideoProviderAccount, _ *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{Status: VideoStatusCancelled}, nil
}

func (a *fakeVideoAdapter) NormalizeStatus(upstream string) string {
	return normalizeVideoStatus(upstream)
}

func (a *fakeVideoAdapter) BuildCreatePayload(_ *VideoProviderAccount, _ *VideoTask) map[string]any {
	return map[string]any{}
}

func cloneVideoAdapterResult(in *VideoAdapterResult) *VideoAdapterResult {
	if in == nil {
		return nil
	}
	out := *in
	if in.UsageTotalTokens != nil {
		v := *in.UsageTotalTokens
		out.UsageTotalTokens = &v
	}
	if in.ActualDuration != nil {
		v := *in.ActualDuration
		out.ActualDuration = &v
	}
	if in.Payload != nil {
		out.Payload = make(map[string]any, len(in.Payload))
		for k, v := range in.Payload {
			out.Payload[k] = v
		}
	}
	return &out
}

func cfgWithMaxPolls(maxPolls int) *config.Config {
	return &config.Config{VideoGateway: config.VideoGatewayConfig{MaxPollAttempts: maxPolls}}
}

// TestVideoPollWindowMeetsMinimum proves the design numbers: at the default poll
// interval the per-task poll cap yields a window ≥360s and ≥2× the observed ~170s
// Seedance generation time, and the wall-clock timeout is the outer backstop.
func TestVideoPollWindowMeetsMinimum(t *testing.T) {
	interval := videoDefaultPollInterval
	maxPolls := videoDefaultMaxPollAttempts
	window := time.Duration(maxPolls) * interval
	if window < 360*time.Second {
		t.Fatalf("poll window %s (%d × %s) < required 360s", window, maxPolls, interval)
	}
	const seedanceGenSeconds = 170
	if window < 2*seedanceGenSeconds*time.Second {
		t.Fatalf("poll window %s < 2× generation time (%ds)", window, 2*seedanceGenSeconds)
	}
	timeout := 15 * time.Minute // NewVideoGatewayWorker default
	if timeout < window {
		t.Fatalf("task timeout %s must be ≥ poll window %s (outer backstop)", timeout, window)
	}
}

// TestVideoWorkerPollsPatientlyUntilSucceeded mirrors "running ~170s then
// succeeded": a task that stays running for 34 polls (~170s at 5s) still reaches
// succeeded because the 72-poll cap is well beyond it.
func TestVideoWorkerPollsPatientlyUntilSucceeded(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil) // nil cfg => default cap 72
	const runningPolls = 34                                           // ~170s at the 5s interval
	svc.adapters[VideoProviderMock] = &fakeVideoAdapter{pollsUntilSucceed: runningPolls}

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "patient long-running render",
		CreatedBy:         9,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	// 1 submit + runningPolls polls (+ slack); terminal tasks are skipped after.
	for range runningPolls + 5 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process: %v", err)
		}
	}
	got, _, err := svc.GetTask(ctx, task.ID, 9, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != VideoStatusSucceeded {
		t.Fatalf("expected succeeded after %d running polls, got %s", runningPolls, got.Status)
	}
	if got.PollCount != runningPolls {
		t.Fatalf("expected poll_count=%d, got %d", runningPolls, got.PollCount)
	}
	if got.ResultURL == "" {
		t.Fatal("expected result url on success")
	}
}

// TestVideoWorkerPollCapTerminatesNeverSucceeding mirrors "never succeeded → hits
// cap, terminated as failed, not infinite": with a small cap and an adapter that
// never reaches a terminal status, the task is failed exactly at the cap.
func TestVideoWorkerPollCapTerminatesNeverSucceeding(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	const maxPolls = 3
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfgWithMaxPolls(maxPolls))
	svc.adapters[VideoProviderMock] = &fakeVideoAdapter{pollsUntilSucceed: 0} // never terminal

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "runaway never-succeeds render",
		CreatedBy:         11,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	// Drive exactly: 1 submit tick + maxPolls poll ticks. At this point the task has
	// been polled EXACTLY maxPolls times and is still running (cap reached, but the
	// task is not over-polled beyond the cap).
	useTimeout := time.Hour // large so the wall-clock timeout never fires; only the cap can
	for range 1 + maxPolls {
		if err := svc.ProcessRunnableTasks(ctx, 10, useTimeout); err != nil {
			t.Fatalf("process: %v", err)
		}
	}
	atCap, _, err := svc.GetTask(ctx, task.ID, 11, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if atCap.PollCount != maxPolls {
		t.Fatalf("expected exactly %d polls at the cap, got %d", maxPolls, atCap.PollCount)
	}
	if atCap.Status != VideoStatusRunning {
		t.Fatalf("after exactly %d polls the task should still be running (not over-polled), got %s", maxPolls, atCap.Status)
	}

	// One more tick: the cap is observed BEFORE a (maxPolls+1)-th poll, so the task is
	// failed without ever polling again.
	if err := svc.ProcessRunnableTasks(ctx, 10, useTimeout); err != nil {
		t.Fatalf("process (cap tick): %v", err)
	}
	got, _, err := svc.GetTask(ctx, task.ID, 11, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != VideoStatusFailed {
		t.Fatalf("expected failed after poll cap, got %s", got.Status)
	}
	if got.PollCount != maxPolls {
		t.Fatalf("expected poll_count to stop exactly at the cap %d (no extra poll), got %d (runaway?)", maxPolls, got.PollCount)
	}
	if !strings.Contains(got.ErrorMessage, "exceeded max poll attempts") {
		t.Fatalf("expected poll-cap error message, got %q", got.ErrorMessage)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected completed_at to be set on poll-cap termination")
	}
	if len(repo.usage) != 1 {
		t.Fatalf("expected exactly one usage log for the terminated task, got %d", len(repo.usage))
	}

	// Drive further: a terminal task must never be polled or charged again.
	for range 5 {
		if err := svc.ProcessRunnableTasks(ctx, 10, useTimeout); err != nil {
			t.Fatalf("process (post-terminal): %v", err)
		}
	}
	final, _, err := svc.GetTask(ctx, task.ID, 11, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if final.PollCount != maxPolls {
		t.Fatalf("terminal task was re-polled: poll_count drifted to %d", final.PollCount)
	}
}

// TestVideoTaskProcessBudgetExceedsProviderCall is the VA2 tick-context invariant:
// the per-task processing budget must exceed the provider call timeout so the DB
// writes that persist a poll's result always have time after a maximally-slow (but
// completing) provider round-trip. The loop also no longer imposes the short poll
// interval as a deadline, so this budget is the real bound on a single task.
func TestVideoTaskProcessBudgetExceedsProviderCall(t *testing.T) {
	if videoTaskProcessBudget <= videoProviderCallTimeout {
		t.Fatalf("per-task budget %s must exceed provider call timeout %s so a slow poll's writes still persist",
			videoTaskProcessBudget, videoProviderCallTimeout)
	}
}

// TestVideoWorkerSlowPollPersistsResult: a provider poll slower than the worker's
// tick cadence still reaches succeeded, because the loop imposes no short deadline
// and the per-task budget is generous.
func TestVideoWorkerSlowPollPersistsResult(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	svc.adapters[VideoProviderMock] = &fakeVideoAdapter{pollsUntilSucceed: 1, pollDelay: 120 * time.Millisecond}

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "slow provider poll",
		CreatedBy:         5,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	for range 3 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process: %v", err)
		}
	}
	got, _, err := svc.GetTask(ctx, task.ID, 5, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != VideoStatusSucceeded {
		t.Fatalf("expected succeeded after a slow poll, got %s", got.Status)
	}
}

// TestVideoWorkerHonorsCancelledContext: once the parent context is cancelled
// (shutdown requested), processTask must NOT start provider work — the task is left
// untouched for a later tick instead of being processed after shutdown.
func TestVideoWorkerHonorsCancelledContext(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	task, err := svc.CreateTask(context.Background(), VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "cancel before work",
		CreatedBy:         6,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled => simulates shutdown in progress
	_ = svc.ProcessRunnableTasks(cancelled, 10, time.Minute)

	got, _, err := svc.GetTask(context.Background(), task.ID, 6, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != VideoStatusQueued {
		t.Fatalf("expected task left queued under a cancelled ctx (shutdown honored), got %s", got.Status)
	}
}
