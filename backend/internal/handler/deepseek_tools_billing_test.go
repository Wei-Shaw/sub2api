//go:build unit

package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type deepseekToolsQuotaRepo struct {
	service.UserPlatformQuotaRepository
	usage map[service.UserPlatformQuotaKey]float64
}

func (r *deepseekToolsQuotaRepo) GetByUserPlatform(_ context.Context, id int64, platform string) (*service.UserPlatformQuotaRecord, error) {
	if platform != service.PlatformDeepseek {
		return nil, nil
	}
	limit := 2 * 0.00075
	now := time.Now()
	return &service.UserPlatformQuotaRecord{
		UserID: id, Platform: platform, DailyLimitUSD: &limit, DailyWindowStart: &now,
		DailyUsageUSD: r.usage[service.UserPlatformQuotaKey{UserID: id, Platform: platform}],
	}, nil
}

func (r *deepseekToolsQuotaRepo) IncrementUsageWithReset(_ context.Context, id int64, platform string, cost float64, _ time.Time) error {
	r.usage[service.UserPlatformQuotaKey{UserID: id, Platform: platform}] += cost
	return nil
}

func TestDeepseekToolsIdenticalRequestsAndCompositeQuota(t *testing.T) {
	cfg := &config.Config{}
	account := service.Account{
		ID: 11, GroupIDs: []int64{163}, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 2, Credentials: map[string]any{"api_key": "test"},
	}
	accounts := &openAIWSFailoverHandlerAccountRepoStub{accounts: []service.Account{account}}
	quotas := &deepseekToolsQuotaRepo{usage: map[service.UserPlatformQuotaKey]float64{}}
	billing := service.NewBillingCacheService(nil, &cachedBillingUserRepo{balance: 5}, nil, nil, nil, nil, cfg, quotas)
	t.Cleanup(billing.Stop)
	commands := &compatStreamUsageBillingRepo{}
	logs := &cachedBillingUsageRepo{}
	up := &fileOwnerTestUpstream{status: http.StatusOK}
	gateway := service.NewOpenAIGatewayService(accounts, logs, commands, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, up, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, quotas)
	h := &OpenAIGatewayHandler{gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(nil), SSEPingFormatNone, 0), cfg: cfg}
	const body = `{"query":"same query"}`
	call := func(wantStatus int) {
		t.Helper()
		c, w := fileOwnerTestContext("POST", "/v1/tools/search", body, service.PlatformComposite, "", 7)
		key, _ := middleware.GetAPIKeyFromContext(c)
		key.Group.RateMultiplier = 1
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, "same-client-id"))
		h.DeepseekTools(c, "search")
		require.Equal(t, wantStatus, w.Code, w.Body.String())
	}

	up.reply = `{"results":[]}`
	call(http.StatusOK)
	require.Empty(t, commands.commands)
	require.Empty(t, logs.logs)
	up.reply = `{"results":[{"title":"found"}]}`
	call(http.StatusOK)
	call(http.StatusOK)
	require.Len(t, commands.commands, 2)
	require.Len(t, logs.logs, 2)
	for i, cmd := range commands.commands {
		require.Equal(t, service.HashUsageRequestPayload([]byte(body)), cmd.RequestPayloadHash)
		require.True(t, strings.HasPrefix(cmd.RequestID, "deepseek_tools:"))
		require.LessOrEqual(t, len(cmd.RequestID), 64)
		require.Equal(t, cmd.RequestID, logs.logs[i].RequestID)
		require.InDelta(t, 0.00075, cmd.BalanceCost, 1e-12)
	}
	require.NotEqual(t, commands.commands[0].RequestID, commands.commands[1].RequestID)
	require.Equal(t, map[service.UserPlatformQuotaKey]float64{{UserID: 7, Platform: service.PlatformDeepseek}: 0.0015}, quotas.usage)
	call(http.StatusTooManyRequests)
	require.Len(t, up.paths, 3, "the exhausted quota must reject the next request before forwarding")
	require.Len(t, commands.commands, 2)
}

func TestDeepseekFilesHaveNoReservation(t *testing.T) {
	for _, endpoint := range []string{"files_create", "files_list", "files_get/file-1", "files_delete/file-1"} {
		require.Zero(t, deepseekToolsInflightEstimate(endpoint), endpoint)
	}
}
