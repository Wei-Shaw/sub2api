//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func grokSettlementAccount() *Account {
	return &Account{ID: 313, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-secret", "base_url": "https://api.x.ai"}}
}

func TestGrokVideoSettlementWithoutClientPolling(t *testing.T) {
	repo := &videoTaskTestRepo{}
	upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"request_id":"accepted-grok"}`)}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, usageBillingRepo: repo, billingService: NewBillingService(&config.Config{}, nil)}
	account := grokSettlementAccount()
	groupID, price := int64(3), 0.20
	key := &APIKey{ID: 2, UserID: 1, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGrok, RateMultiplier: 1, Hydrated: true, VideoPrice480P: &price}}
	c, response := grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos/generations", nil)
	var quote *VideoTaskCostSnapshot
	result, err := svc.ForwardGrokVideoCreate(context.Background(), c, account, GrokMediaEndpointVideosGenerations, "",
		[]byte(`{"model":"grok-imagine-video-1.5","duration":6,"resolution":"480p","prompt":"test"}`), "application/json",
		func(created *OpenAIForwardResult) error {
			require.Empty(t, response.Body.String(), "acceptance must not precede persistence")
			return svc.PersistDurableVideoTask(context.Background(), created.ResponseID, key, account, nil, GrokVideoPendingBilling{Platform: PlatformGrok, Model: created.Model, BillingModel: created.BillingModel, VideoResolution: created.VideoResolution, VideoDurationSeconds: created.VideoDurationSeconds, CreatedAt: GrokVideoPendingCreatedAtNow()}, quote)
		}, func(normalized *OpenAIForwardResult) error {
			require.Empty(t, upstream.requests, "quote must precede upstream submission")
			var err error
			quote, err = svc.ResolveVideoTaskCost(context.Background(), &OpenAIRecordUsageInput{APIKey: key, Account: account, Result: normalized, QuotaPlatform: PlatformGrok})
			return err
		})
	require.NoError(t, err)
	require.Equal(t, "accepted-grok", result.ResponseID)
	require.JSONEq(t, `{"request_id":"accepted-grok"}`, response.Body.String())
	require.InDelta(t, 1.20, repo.task.CostSnapshot.Cost.ActualCost, 0.000001)
	require.NoError(t, ValidateDurableVideoTaskAccount(repo.task, account))
	price = 99 // Changes after acceptance cannot reprice this task.
	svc.accountRepo = videoTaskAccountRepo{account: account}
	upstream.response = grokMediaContentStatusResponse(`{"status":"done","video":{"url":"https://media.example/video.mp4","duration":6}}`)
	bills := 0
	processor := &videoTaskProcessor{repo: repo, poll: svc.pollDurableVideoTask, bill: func(_ context.Context, task *VideoTask) error {
		bills++
		input, err := durableVideoUsageInput(task)
		require.NoError(t, err)
		require.Equal(t, PlatformGrok, input.QuotaPlatform)
		require.Equal(t, "grok-imagine-video-1.5", input.Result.BillingModel)
		require.InDelta(t, 1.20, input.VideoTaskCost.Cost.ActualCost, 0.000001)
		return nil
	}}
	require.NoError(t, processor.process(context.Background(), repo.task))
	require.Equal(t, VideoTaskCompleted, repo.task.State)
	require.NoError(t, processor.process(context.Background(), repo.task))
	require.Equal(t, 1, bills)
	require.Len(t, upstream.requests, 2, "one create and one worker GET; no client poll")
	legacy := repo.task.Pending
	legacy.TrackingOnly = true
	require.Error(t, svc.PersistDurableVideoTask(context.Background(), "legacy", key, account, nil, legacy, quote))
}

func TestGrokVideoSettlementPreflightAndPersistenceFailure(t *testing.T) {
	for _, preflightFails := range []bool{true, false} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"request_id":"accepted-grok"}`)}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, response := grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos/generations", nil)
		_, err := svc.ForwardGrokVideoCreate(context.Background(), c, grokSettlementAccount(), GrokMediaEndpointVideosGenerations, "",
			[]byte(`{"model":"grok-imagine-video-1.5","duration":6}`), "application/json",
			func(*OpenAIForwardResult) error { return errors.New("database unavailable") },
			func(*OpenAIForwardResult) error {
				if preflightFails {
					return errors.New("no quote")
				}
				return nil
			})
		require.Error(t, err)
		if preflightFails {
			require.Empty(t, upstream.requests)
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
		} else {
			var unknown *OpenAIVideoCreateUnknownError
			require.ErrorAs(t, err, &unknown)
			require.Equal(t, "accepted-grok", unknown.TaskID)
			require.False(t, c.Writer.Written())
		}
	}
}

func TestGrokVideoSettlementAmbiguousAcceptanceDoesNotFailover(t *testing.T) {
	for _, status := range []int{200, 307, 408, 500, 502} {
		response := grokMediaContentStatusResponse(`{"status":"pending"}`)
		response.StatusCode = status
		upstream := &grokMediaContentUpstreamStub{response: response}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, _ := grokMediaContentTestContext(http.MethodPost, "https://site.example/v1/videos/generations", nil)
		_, err := svc.ForwardGrokVideoCreate(context.Background(), c, grokSettlementAccount(), GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"grok-imagine-video-1.5","duration":6}`), "application/json", func(*OpenAIForwardResult) error { t.Fatal("unknown task persisted"); return nil })
		var unknown *OpenAIVideoCreateUnknownError
		require.ErrorAs(t, err, &unknown)
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover))
		require.Len(t, upstream.requests, 1)
	}
}

func TestGrokVideoSettlementFailureAndUnknownNeverBill(t *testing.T) {
	for _, body := range []string{`{"status":"pending"}`, `{"status":"failed"}`, `{"status":"expired"}`, `{"status":"done"}`, `{"error":"unavailable"}`} {
		t.Run(body, func(t *testing.T) {
			account := grokSettlementAccount()
			task := videoTaskTestFixture()
			task.Pending.Platform = PlatformGrok
			task.RoutingFingerprint = videoTaskRoutingFingerprint(account)
			repo := &videoTaskTestRepo{task: task}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: videoTaskAccountRepo{account: account}, httpUpstream: &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(body)}}
			processor := &videoTaskProcessor{repo: repo, poll: svc.pollDurableVideoTask, bill: func(context.Context, *VideoTask) error { t.Fatal("non-success charged"); return nil }}
			_ = processor.process(context.Background(), task)
			require.NotEqual(t, VideoTaskCompleted, task.State)
			require.Nil(t, task.Completion)
		})
	}
}
