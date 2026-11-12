//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIPreferredAccountSharedPool(t *testing.T) {
	for _, mode := range []string{"legacy", "legacy-without-load-batch", "advanced", "advanced-weighted-sticky"} {
		t.Run(mode, func(t *testing.T) {
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			advanced, weighted := "false", "false"
			if mode == "advanced" || mode == "advanced-weighted-sticky" {
				advanced = "true"
			}
			if mode == "advanced-weighted-sticky" {
				weighted = "true"
			}
			cfg := newSchedulerTestOpenAIWSV2Config()
			cfg.Gateway.Scheduling.LoadBatchEnabled = mode != "legacy-without-load-batch"
			cfg.Gateway.OpenAIWS.LBTopK = 1
			cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1

			group := &Group{ID: 12, Platform: PlatformOpenAI, PreferUserAccounts: true}
			aliceID, bobID := int64(101), int64(102)
			account := func(id int64, priority int, owner *int64) Account {
				return Account{
					ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: priority,
					PreferredUserID: owner, GroupIDs: []int64{group.ID},
					Extra: map[string]any{"openai_oauth_responses_websockets_v2_enabled": true},
				}
			}
			accounts := []Account{account(1, 100, &aliceID), account(2, 50, &bobID), account(3, 1, nil)}
			cache := &schedulerTestGatewayCache{}
			slots := map[int64]bool{}
			svc := &OpenAIGatewayService{
				cfg: cfg, cache: cache,
				accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: slots}),
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(advanced, weighted),
			}
			ctx := withOpenAIQuotaAutoPauseSettings(context.Background(), OpsOpenAIAccountQuotaAutoPauseSettings{DefaultThreshold7d: 1})
			caller := func(userID int64) context.Context {
				return WithOpenAIPreferredAccountUser(ctx, &APIKey{UserID: userID, Group: group})
			}
			selectAccount := func(ctx context.Context, previous, session string, transport OpenAIUpstreamTransport, excluded map[int64]struct{}, want int64) {
				t.Helper()
				got, _, err := svc.SelectAccountWithSchedulerForCapability(
					ctx, &group.ID, previous, session, "gpt-5.4-mini", excluded,
					transport, OpenAIEndpointCapabilityResponses, false, false, true, PlatformOpenAI,
				)
				require.NoError(t, err)
				require.NotNil(t, got)
				require.True(t, got.Acquired, "an available shared account should be used without waiting")
				defer got.ReleaseFunc()
				require.Equal(t, want, got.Account.ID)
			}
			require.Equal(t, advanced == "true", svc.isOpenAIAdvancedSchedulerEnabled(ctx))

			selectAccount(ctx, "", "existing-shared", OpenAIUpstreamTransportHTTPSSE, nil, 3)
			selectAccount(caller(aliceID), "", "existing-shared", OpenAIUpstreamTransportHTTPSSE, nil, 3)
			selectAccount(caller(aliceID), "", "alice-fresh", OpenAIUpstreamTransportHTTPSSE, nil, 1)
			selectAccount(caller(bobID), "", "bob-fresh", OpenAIUpstreamTransportResponsesWebsocketV2, nil, 2)
			selectAccount(caller(aliceID), "", "alice-retry", OpenAIUpstreamTransportHTTPSSE, map[int64]struct{}{1: {}}, 3)

			slots[1] = false
			selectAccount(caller(aliceID), "", "alice-busy", OpenAIUpstreamTransportHTTPSSE, nil, 3)
			delete(slots, 1)

			accounts[0].Extra["codex_7d_used_percent"] = 100.0
			accounts[0].Extra["codex_7d_reset_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
			accounts[0].Extra["codex_usage_updated_at"] = time.Now().Format(time.RFC3339)
			selectAccount(caller(aliceID), "", "alice-quota", OpenAIUpstreamTransportHTTPSSE, nil, 3)
			accounts[0].Extra["codex_7d_used_percent"] = 0.0
			selectAccount(caller(aliceID), "", "alice-quota", OpenAIUpstreamTransportHTTPSSE, nil, 3)
			selectAccount(caller(aliceID), "", "alice-reset-new", OpenAIUpstreamTransportHTTPSSE, nil, 1)

			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, group.ID, "resp_shared", 3, time.Hour))
			selectAccount(caller(aliceID), "resp_shared", "", OpenAIUpstreamTransportResponsesWebsocketV2, nil, 3)

			group.PreferUserAccounts = false
			selectAccount(caller(aliceID), "", "disabled", OpenAIUpstreamTransportHTTPSSE, nil, 3)
		})
	}
}
