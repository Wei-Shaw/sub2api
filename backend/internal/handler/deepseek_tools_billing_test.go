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

func TestDeepseekTTSHandlerReservesWhatItSettles(t *testing.T) {
	cache := newHandlerInflightCache(5)
	var reserved float64
	up := &deepseekTestTransport{respond: func(_ *http.Request, _ int64) (*http.Response, error) {
		cache.mu.Lock()
		require.Len(t, cache.res, 1)
		for _, v := range cache.res {
			reserved = v
		}
		cache.mu.Unlock()
		r := deepseekTestReply(200, strings.NewReader("audio"))
		r.Header.Set("Content-Type", "audio/wav")
		return r, nil
	}}
	h, _, billing := newDeepseekProxyHandler(t, 1, up, nil)
	h.cfg.Billing.InflightReservation.Enabled = true
	h.billingCacheService = service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, h.cfg, nil)
	t.Cleanup(h.billingCacheService.Stop)
	for i, text := range []string{" x ", strings.Repeat("你", 5000)} {
		c, rec := fileOwnerTestContext("POST", "/v1/tools/tts", `{"text":"`+text+`"}`, service.PlatformDeepseek, "", 7)
		key, _ := middleware.GetAPIKeyFromContext(c)
		key.Group.RateMultiplier = 2
		h.DeepseekTools(c, "tts")
		require.Equal(t, 200, rec.Code)
		require.Len(t, billing.commands, i+1)
		want := []float64{3, 10000}[i] * 2 * .45 / 10000
		require.InDelta(t, want, reserved, 1e-12)
		require.InDelta(t, reserved, billing.commands[i].BalanceCost, 1e-12)
		require.Zero(t, cache.count())
	}
}

func TestDeepseekFilesHaveNoReservation(t *testing.T) {
	for _, endpoint := range []string{"files_create", "files_list", "files_get/file-1", "files_delete/file-1"} {
		require.Zero(t, deepseekToolsInflightEstimate(endpoint, nil), endpoint)
	}
}
