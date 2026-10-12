package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type videoTaskTestRepo struct {
	UsageBillingRepository
	mu        sync.Mutex
	task      *VideoTask
	saveErr   error
	finishErr error
	events    []string
	delay     time.Duration
	errorCode string
}

func (r *videoTaskTestRepo) CreateVideoTask(_ context.Context, t *VideoTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *t
	copy.State = VideoTaskPending
	r.task = &copy
	return nil
}
func (r *videoTaskTestRepo) GetVideoTask(_ context.Context, id string, user, key int64) (*VideoTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.task == nil || r.task.TaskID != id || r.task.UserID != user || r.task.APIKeyID != key {
		return nil, nil
	}
	copy := *r.task
	return &copy, nil
}
func (r *videoTaskTestRepo) ClaimDueVideoTask(context.Context, time.Duration) (*VideoTask, error) {
	return nil, nil
}
func (r *videoTaskTestRepo) ClaimOwnedVideoTask(ctx context.Context, id string, user, key int64, _ time.Duration) (*VideoTask, error) {
	t, err := r.GetVideoTask(ctx, id, user, key)
	if t != nil && t.LeaseToken != "" {
		return nil, nil
	}
	return t, err
}
func (r *videoTaskTestRepo) SaveVideoTaskCompletion(_ context.Context, t *VideoTask, c *VideoTaskCompletion) error {
	r.events = append(r.events, "save")
	if r.saveErr != nil {
		return r.saveErr
	}
	r.task.Completion = c
	r.task.State = VideoTaskReady
	return nil
}
func (r *videoTaskTestRepo) FinishVideoTaskAttempt(_ context.Context, t *VideoTask, state string, delay time.Duration, code string) error {
	r.events = append(r.events, state)
	r.delay = delay
	if r.finishErr != nil {
		return r.finishErr
	}
	r.task.State = state
	r.task.LastError = code
	r.errorCode = code
	return nil
}

func videoTaskTestFixture() *VideoTask {
	group, sub := int64(3), int64(4)
	return &VideoTask{TaskID: "video-1", UserID: 1, APIKeyID: 2, GroupID: &group, AccountID: 313, SubscriptionID: &sub, State: VideoTaskPending, Attempts: 1, CostSnapshot: &VideoTaskCostSnapshot{Version: 1}, Pending: GrokVideoPendingBilling{Model: "provider-video", BillingModel: "priced-video", OriginalModel: "client-video", VideoResolution: "720p", VideoDurationSeconds: 5, CreatedAt: "2026-09-15T06:00:00Z"}}
}

func TestVideoTaskRetryUsesSavedCompletionBeforeBilling(t *testing.T) {
	task := videoTaskTestFixture()
	repo := &videoTaskTestRepo{task: task}
	completion := &VideoTaskCompletion{Model: "provider-video", BillingModel: "priced-video", VideoDurationSeconds: 5, VideoResolution: "720p"}
	polls, bills := 0, 0
	p := &videoTaskProcessor{repo: repo, poll: func(context.Context, *VideoTask) (*VideoTaskCompletion, string, error) {
		polls++
		return completion, "", nil
	}, bill: func(_ context.Context, job *VideoTask) error {
		bills++
		repo.events = append(repo.events, "bill")
		require.Same(t, completion, job.Completion)
		if bills == 1 {
			return errors.New("transaction rolled back")
		}
		return nil
	}}
	require.Error(t, p.process(context.Background(), task))
	require.Equal(t, VideoTaskReady, task.State)
	require.Equal(t, []string{"save", "bill", "ready"}, repo.events)
	require.NoError(t, p.process(context.Background(), task))
	require.Equal(t, 1, polls, "a ready task must settle without another upstream request")
	require.Equal(t, 2, bills)
	require.Equal(t, VideoTaskCompleted, task.State)
	require.NoError(t, p.process(context.Background(), task))
	require.Equal(t, 2, bills, "completed task short circuits")
}

func TestVideoTaskCompletionMustPersistBeforeBilling(t *testing.T) {
	task := videoTaskTestFixture()
	repo := &videoTaskTestRepo{task: task, saveErr: ErrVideoTaskLeaseLost}
	p := &videoTaskProcessor{repo: repo, poll: func(context.Context, *VideoTask) (*VideoTaskCompletion, string, error) {
		return &VideoTaskCompletion{}, "", nil
	}, bill: func(context.Context, *VideoTask) error { t.Fatal("charged without durable completion"); return nil }}
	require.ErrorIs(t, p.process(context.Background(), task), ErrVideoTaskLeaseLost)
	require.Equal(t, VideoTaskPending, task.State)
}

func TestVideoTaskPendingFailureAndFingerprintConflict(t *testing.T) {
	for _, tc := range []struct {
		name       string
		outcome    string
		completion *VideoTaskCompletion
		billErr    error
		state      string
	}{
		{name: "pending", state: VideoTaskPending},
		{name: "upstream failed", outcome: "upstream_failed", state: VideoTaskFailed},
		{name: "upstream cancelled", outcome: "upstream_cancelled", state: VideoTaskFailed},
		{name: "pricing conflict", completion: &VideoTaskCompletion{}, billErr: ErrUsageBillingRequestConflict, state: VideoTaskReady},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := videoTaskTestFixture()
			repo := &videoTaskTestRepo{task: task}
			p := &videoTaskProcessor{repo: repo, poll: func(context.Context, *VideoTask) (*VideoTaskCompletion, string, error) {
				return tc.completion, tc.outcome, nil
			}, bill: func(context.Context, *VideoTask) error { require.NotNil(t, tc.completion); return tc.billErr }}
			err := p.process(context.Background(), task)
			if tc.billErr != nil {
				require.ErrorIs(t, err, tc.billErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.state, task.State)
			if tc.state == VideoTaskFailed {
				require.Equal(t, tc.outcome, repo.errorCode)
			}
			if tc.state == VideoTaskPending {
				require.Equal(t, videoTaskPollInterval, repo.delay)
			}
		})
	}
}

func TestVideoTaskMetadataAndAtomicInput(t *testing.T) {
	task := videoTaskTestFixture()
	now := time.Date(2026, 9, 15, 6, 1, 0, 0, time.UTC)
	c, err := durableVideoCompletion(task, &OpenAIForwardResult{VideoCount: 1, ResponseID: "wrong-upstream-id", VideoDurationSeconds: 120, VideoResolution: "1440p", VideoURL: "https://secret.example/?token=private", Usage: OpenAIUsage{OutputTokens: 13}}, now)
	require.NoError(t, err)
	require.Equal(t, 120, c.VideoDurationSeconds)
	require.Equal(t, "720p", c.VideoResolution, "preserve create-time tier")
	require.Equal(t, time.Minute, c.Duration)
	task.Completion = c
	input, err := durableVideoUsageInput(task)
	require.NoError(t, err)
	require.True(t, input.RequireAtomicVideo)
	require.Same(t, task.CostSnapshot, input.VideoTaskCost)
	require.Equal(t, "grok-video:video-1", input.Result.RequestID)
	require.Equal(t, "video-1", input.Result.ResponseID)
	require.Equal(t, HashUsageRequestPayload([]byte("video-1")), input.RequestPayloadHash)
	require.Equal(t, 13, input.Result.Usage.OutputTokens)
	require.Equal(t, "2026-09-15T06:00:00Z", input.PricingAt.Format(time.RFC3339))
	encoded, err := json.Marshal(task)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret.example")
	require.NotContains(t, string(encoded), "private")
	for _, field := range []string{"seconds", "resolution", "model"} {
		t.Run(field, func(t *testing.T) {
			x := videoTaskTestFixture()
			switch field {
			case "seconds":
				x.Pending.VideoDurationSeconds = 0
			case "resolution":
				x.Pending.VideoResolution = ""
			case "model":
				x.Pending.Model = ""
				x.Pending.OriginalModel = ""
			}
			_, err := durableVideoCompletion(x, &OpenAIForwardResult{VideoCount: 1}, now)
			require.Error(t, err, "must not invent Grok defaults")
		})
	}
}

func TestVideoTaskPersistenceOwnerAndRoute(t *testing.T) {
	repo := &videoTaskTestRepo{}
	svc := &OpenAIGatewayService{usageBillingRepo: repo, billingService: NewBillingService(&config.Config{}, nil)}
	task := videoTaskTestFixture()
	a := &Account{ID: 313, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example", "api_key": "secret"}, Extra: map[string]any{"openai_video_protocol": "grok"}}
	key := &APIKey{ID: 2, UserID: 1, GroupID: task.GroupID, Key: "client-secret"}
	price := 0.2
	key.Group = &Group{ID: *task.GroupID, Platform: PlatformOpenAI, RateMultiplier: 1, SubscriptionType: SubscriptionTypeSubscription, Hydrated: true, VideoPrice720P: &price}
	sub := &UserSubscription{ID: 4, UserID: 1, GroupID: 3}
	quote, err := svc.ResolveVideoTaskCost(context.Background(), &OpenAIRecordUsageInput{
		RequireAtomicVideo: true, APIKey: key, Account: a, Subscription: sub, RequestPayloadHash: "pre-send-body-hash",
		Result:        &OpenAIForwardResult{Model: task.Pending.Model, BillingModel: task.Pending.BillingModel, VideoCount: 1, VideoResolution: task.Pending.VideoResolution, VideoDurationSeconds: task.Pending.VideoDurationSeconds},
		QuotaPlatform: PlatformOpenAI,
	})
	require.NoError(t, err)
	price = 99               // the live card changes while the upstream create is in flight
	svc.billingService = nil // persistence must not call the calculator again
	require.Error(t, svc.PersistOpenAIVideoTask(context.Background(), task.TaskID, key, a, sub, task.Pending, nil))
	wrongOwner := *quote
	wrongOwner.Command.UserID = 99
	wrongOwner.Log.UserID = 99
	require.Error(t, svc.PersistOpenAIVideoTask(context.Background(), task.TaskID, key, a, sub, task.Pending, &wrongOwner))
	wrongUnits := task.Pending
	wrongUnits.VideoDurationSeconds++
	require.Error(t, svc.PersistOpenAIVideoTask(context.Background(), task.TaskID, key, a, sub, wrongUnits, quote))
	wrongUnits = task.Pending
	wrongUnits.VideoResolution = "1080p"
	require.Error(t, svc.PersistOpenAIVideoTask(context.Background(), task.TaskID, key, a, sub, wrongUnits, quote))
	require.NoError(t, svc.PersistOpenAIVideoTask(context.Background(), task.TaskID, key, a, sub, task.Pending, quote))
	loaded, err := svc.LoadDurableVideoTask(context.Background(), task.TaskID, 1, 2)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, sub.ID, *loaded.SubscriptionID)
	require.NotNil(t, loaded.CostSnapshot)
	require.InDelta(t, 1.0, loaded.CostSnapshot.Cost.ActualCost, 0.000001)
	require.NotSame(t, quote, loaded.CostSnapshot)
	require.Equal(t, "pre-send-body-hash", quote.Command.RequestPayloadHash)
	require.Equal(t, HashUsageRequestPayload([]byte(task.TaskID)), loaded.CostSnapshot.Command.RequestPayloadHash)
	quote.Cost.ActualCost = 99 // caller mutation cannot change the stored quote
	require.InDelta(t, 1.0, loaded.CostSnapshot.Cost.ActualCost, 0.000001)
	require.NoError(t, ValidateDurableVideoTaskAccount(loaded, a))
	encoded, err := json.Marshal(loaded)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret")
	require.NotContains(t, string(encoded), "relay.example")
	other, err := svc.LoadDurableVideoTask(context.Background(), task.TaskID, 99, 2)
	require.NoError(t, err)
	require.Nil(t, other)
	a.Credentials["base_url"] = "https://changed.example"
	require.Error(t, ValidateDurableVideoTaskAccount(loaded, a))
	a.Credentials["base_url"] = "https://relay.example"
	a.Extra["openai_video_protocol"] = "zhongy"
	require.Error(t, ValidateDurableVideoTaskAccount(loaded, a))
	sub.UserID = 99
	require.Error(t, svc.PersistOpenAIVideoTask(context.Background(), task.TaskID, key, a, sub, task.Pending, quote))
	a.Platform = PlatformGrok
	require.Error(t, svc.PersistOpenAIVideoTask(context.Background(), task.TaskID, key, a, nil, task.Pending, quote))
}

func TestVideoTaskReconcileNoLegacyAndBusyLease(t *testing.T) {
	task := videoTaskTestFixture()
	task.LeaseToken = "another-process"
	repo := &videoTaskTestRepo{task: task}
	svc := &OpenAIGatewayService{usageBillingRepo: repo, videoTaskProcessor: &videoTaskProcessor{repo: repo}}
	require.NoError(t, svc.ReconcileOwnedVideoTask(context.Background(), "legacy-task", 1, 2))
	require.NoError(t, svc.ReconcileOwnedVideoTask(context.Background(), task.TaskID, 99, 2))
	require.ErrorIs(t, svc.ReconcileOwnedVideoTask(context.Background(), task.TaskID, 1, 2), ErrVideoTaskBusy)
	task.State = VideoTaskCompleted
	require.NoError(t, svc.ReconcileOwnedVideoTask(context.Background(), task.TaskID, 1, 2))
}

func TestVideoTaskWorkerStartStop(t *testing.T) {
	svc := &OpenAIGatewayService{usageBillingRepo: &videoTaskTestRepo{}}
	svc.StartVideoTaskWorker(&APIKeyService{})
	done := svc.videoTaskDone
	require.NotNil(t, done)
	svc.StartVideoTaskWorker(&APIKeyService{})
	require.Equal(t, done, svc.videoTaskDone)
	svc.CloseOpenAIWSPool()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	svc.StopVideoTaskWorker()
	svc.StartVideoTaskWorker(&APIKeyService{})
	require.Equal(t, done, svc.videoTaskDone, "must not restart during gateway teardown")
}

type videoTaskAccountRepo struct {
	AccountRepository
	account *Account
}

func (r videoTaskAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func TestVideoTaskPollPinnedOpenAIProtocolsWithoutContentFetch(t *testing.T) {
	for _, protocol := range []string{"", "grok", "zhongy"} {
		t.Run(protocol, func(t *testing.T) {
			a := openAIVideoTestAccount()
			a.ID = 313
			a.Extra = map[string]any{"openai_video_protocol": protocol}
			task := videoTaskTestFixture()
			task.RoutingFingerprint = videoTaskRoutingFingerprint(a)
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"video-1","status":"completed"}`)}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: videoTaskAccountRepo{account: a}, httpUpstream: upstream}
			completion, outcome, err := svc.pollDurableVideoTask(context.Background(), task)
			require.NoError(t, err)
			require.Empty(t, outcome)
			require.NotNil(t, completion)
			require.Equal(t, 5, completion.VideoDurationSeconds)
			require.Equal(t, "720p", completion.VideoResolution)
			require.Equal(t, http.MethodGet, upstream.request.Method)
			require.Contains(t, upstream.request.URL.String(), "video-1")
			require.NotContains(t, upstream.request.URL.Path, "/content")
			a.Credentials["base_url"] = "https://untrusted-change.example"
			_, _, err = svc.pollDurableVideoTask(context.Background(), task)
			require.Error(t, err)
			require.Len(t, upstream.requests, 1, "route change must fail before any new upstream request")
		})
	}
}

func TestVideoTaskRetryAfterBillingCommitButTaskUpdateFailure(t *testing.T) {
	task := videoTaskTestFixture()
	task.State = VideoTaskReady
	task.Completion = &VideoTaskCompletion{Model: "video", BillingModel: "video", VideoResolution: "720p", VideoDurationSeconds: 5}
	repo := &videoTaskTestRepo{task: task, finishErr: errors.New("task update connection lost")}
	bills := 0
	p := &videoTaskProcessor{repo: repo, poll: func(context.Context, *VideoTask) (*VideoTaskCompletion, string, error) {
		t.Fatal("saved completion must not poll")
		return nil, "", nil
	}, bill: func(_ context.Context, job *VideoTask) error {
		bills++
		input, err := durableVideoUsageInput(job)
		require.NoError(t, err)
		require.Equal(t, "grok-video:video-1", input.Result.RequestID)
		require.True(t, input.RequireAtomicVideo)
		return nil // first commit, then the billing repository's verified dedup
	}}
	require.Error(t, p.process(context.Background(), task))
	require.Equal(t, VideoTaskReady, task.State)
	repo.finishErr = nil
	require.NoError(t, p.process(context.Background(), task))
	require.Equal(t, 2, bills)
	require.Equal(t, VideoTaskCompleted, task.State)
}

func TestVideoTaskCanceledAttemptReleasesLeaseWithBoundedBackoff(t *testing.T) {
	task := videoTaskTestFixture()
	task.Attempts = 1000
	repo := &videoTaskTestRepo{task: task}
	p := &videoTaskProcessor{repo: repo, poll: func(ctx context.Context, _ *VideoTask) (*VideoTaskCompletion, string, error) {
		return nil, "", ctx.Err()
	}, bill: func(context.Context, *VideoTask) error { t.Fatal("must not bill canceled lookup"); return nil }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, p.process(ctx, task), context.Canceled)
	require.Equal(t, VideoTaskPending, task.State)
	require.Equal(t, 4*time.Minute, repo.delay)
	require.Equal(t, "poll_failed", repo.errorCode, "local cancellation is not provider cancellation")
}

func TestVideoTaskPollTimeoutRemainsRecoverable(t *testing.T) {
	for _, cause := range []error{
		context.DeadlineExceeded,
		&url.Error{Op: "Get", URL: "https://secret.example/?token=private", Err: context.DeadlineExceeded},
		errors.New("provider failed: private prompt https://secret.example"),
	} {
		t.Run(fmt.Sprintf("%T", cause), func(t *testing.T) {
			task := videoTaskTestFixture()
			task.Pending.CreatedAt = time.Now().Add(-3 * time.Hour).Format(time.RFC3339Nano)
			repo := &videoTaskTestRepo{task: task}
			p := &videoTaskProcessor{repo: repo, poll: func(context.Context, *VideoTask) (*VideoTaskCompletion, string, error) {
				return nil, "", cause
			}, bill: func(context.Context, *VideoTask) error { t.Fatal("must not bill a failed lookup"); return nil }}
			require.ErrorIs(t, p.process(context.Background(), task), cause)
			require.Equal(t, VideoTaskPending, task.State)
			want := "poll_failed"
			if errors.Is(cause, context.DeadlineExceeded) {
				want = "poll_timeout"
			}
			require.Equal(t, want, repo.errorCode)
			require.Positive(t, repo.delay)
			// Recovery settles the same task using the existing atomic billing input.
			p.poll = func(context.Context, *VideoTask) (*VideoTaskCompletion, string, error) {
				return &VideoTaskCompletion{Model: "video", BillingModel: "video", VideoResolution: "720p", VideoDurationSeconds: 5}, "", nil
			}
			bills := 0
			p.bill = func(context.Context, *VideoTask) error { bills++; return nil }
			require.NoError(t, p.process(context.Background(), task))
			require.NoError(t, p.process(context.Background(), task))
			require.Equal(t, 1, bills)
			require.Equal(t, VideoTaskCompleted, task.State)
			require.Empty(t, repo.errorCode)
		})
	}
}

func TestVideoTaskProviderOutcomesAreAllowlisted(t *testing.T) {
	for _, protocol := range []string{"", "grok", "zhongy"} {
		for _, tc := range []struct {
			name, body, state, code string
			httpStatus              int
		}{
			{"failed", `{"status":"failed","error":{"message":"private prompt https://secret.example"}}`, VideoTaskFailed, "upstream_failed", 200},
			{"cancelled", `{"status":"cancelled"}`, VideoTaskFailed, "upstream_cancelled", 200},
			{"canceled", `{"status":"canceled"}`, VideoTaskFailed, "upstream_cancelled", 200},
			{"expired", `{"status":"expired"}`, VideoTaskPending, "upstream_expired", 200},
			{"pending", `{"status":"pending"}`, VideoTaskPending, "", 200},
			{"unknown", `{"status":"failed: private prompt"}`, VideoTaskPending, "poll_failed", 200},
			{"missing status", `{"error":{"code":"failed"}}`, VideoTaskPending, "poll_failed", 200},
			{"malformed", `{"status":"failed"`, VideoTaskPending, "poll_failed", 200},
			{"unauthorized", `{"status":"failed"}`, VideoTaskPending, "poll_failed", 401},
			{"not found", `{"status":"expired"}`, VideoTaskPending, "poll_failed", 404},
			{"rate limited", `{"status":"failed"}`, VideoTaskPending, "poll_failed", 429},
			{"server error", `{"status":"failed"}`, VideoTaskPending, "poll_failed", 500},
			{"gateway timeout", `{"status":"failed"}`, VideoTaskPending, "poll_failed", 504},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				a := openAIVideoTestAccount()
				a.ID = 313
				a.Extra = map[string]any{"openai_video_protocol": protocol}
				task := videoTaskTestFixture()
				task.Pending.CreatedAt = time.Now().Add(-3 * time.Hour).Format(time.RFC3339Nano)
				task.RoutingFingerprint = videoTaskRoutingFingerprint(a)
				repo := &videoTaskTestRepo{task: task}
				upstream := &grokMediaContentUpstreamStub{response: &http.Response{StatusCode: tc.httpStatus, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: videoTaskAccountRepo{account: a}, httpUpstream: upstream}
				p := &videoTaskProcessor{repo: repo, poll: svc.pollDurableVideoTask, bill: func(context.Context, *VideoTask) error { t.Fatal("must not bill without completion"); return nil }}
				err := p.process(context.Background(), task)
				if tc.state == VideoTaskFailed || tc.code == "" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.Positive(t, repo.delay)
				}
				require.Equal(t, tc.state, task.State)
				require.Equal(t, tc.code, repo.errorCode)
				require.Len(t, upstream.requests, 1)
				require.Equal(t, http.MethodGet, upstream.request.Method)
				require.NotContains(t, upstream.request.URL.Path, "/content")
			})
		}
	}
}
