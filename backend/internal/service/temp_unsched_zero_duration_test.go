//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func zeroDurationAccount(statusCode int, keyword string) *Account {
	return newTempUnschedScopeAccount("account", map[string]any{
		"error_code":       statusCode,
		"keywords":         []any{keyword},
		"duration_minutes": 0,
	})
}

func TestGetTempUnschedulableRules_ZeroRequiresExplicitValidDuration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		duration any
		valid    bool
	}{
		{"integer zero", 0, true},
		{"JSON zero", float64(0), true},
		{"string zero", "0", true},
		{"number zero", json.Number("0"), true},
		{"positive", 10, true},
		{"negative", -1, false},
		{"missing", nil, false},
		{"invalid", "invalid", false},
		{"boolean", false, false},
		{"fraction", 0.5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := zeroDurationAccount(403, "not_allowed")
			rule := account.Credentials["temp_unschedulable_rules"].([]any)[0].(map[string]any)
			if tc.duration == nil {
				delete(rule, "duration_minutes")
			} else {
				rule["duration_minutes"] = tc.duration
			}
			require.Equal(t, tc.valid, len(account.GetTempUnschedulableRules()) == 1)
		})
	}
}

func TestTempUnschedulableZeroDuration_IgnoresSchedulingState(t *testing.T) {
	for _, keyword := range []string{"reasoning_effort_not_allowed", "service_tier_not_allowed"} {
		for _, model := range []string{"", "gpt-5.4"} {
			for _, pool := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/model=%s/pool=%t", keyword, model, pool), func(t *testing.T) {
					account := zeroDurationAccount(http.StatusForbidden, keyword)
					account.Credentials["pool_mode"] = pool
					repo := &errorPolicyRepoStub{}
					rateLimits := &RateLimitService{accountRepo: repo}
					gateway := &OpenAIGatewayService{rateLimitService: rateLimits}
					body := []byte(`{"error":{"code":"` + keyword + `","message":"request parameter not allowed"}}`)

					require.Equal(t, ErrorPolicySkipped, rateLimits.CheckErrorPolicy(context.Background(), account, 403, body, model))
					require.False(t, rateLimits.HandleTempUnschedulable(context.Background(), account, 403, body, model))
					require.False(t, rateLimits.HandleUpstreamError(context.Background(), account, 403, nil, body, model))
					require.False(t, gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 403, nil, body, model))
					require.Zero(t, repo.tempCalls)
					require.Zero(t, repo.setErrCalls)
					require.Empty(t, repo.modelRateLimitCalls)
					require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
					require.True(t, account.IsSchedulable())
				})
			}
		}
	}
}

func TestTempUnschedulableZeroDuration_DoesNotRecordTransientFailures(t *testing.T) {
	account := zeroDurationAccount(http.StatusServiceUnavailable, "unavailable")
	repo := &errorPolicyRepoStub{}
	rateLimits := &RateLimitService{accountRepo: repo}
	gateway := &OpenAIGatewayService{rateLimitService: rateLimits}
	body := []byte(`{"error":{"message":"upstream unavailable"}}`)
	for range 3 {
		require.False(t, gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 503, nil, body, "gpt-5.4"))
	}
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
	require.False(t, gateway.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.4"))
}

func TestTempUnschedulableZeroDuration_PreservesRuleOrder(t *testing.T) {
	for _, ignoreFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("ignoreFirst=%t", ignoreFirst), func(t *testing.T) {
			account := zeroDurationAccount(403, "not_allowed")
			ignored := account.Credentials["temp_unschedulable_rules"].([]any)[0]
			cooldown := map[string]any{"error_code": 403, "keywords": []any{"not_allowed"}, "duration_minutes": 10}
			if ignoreFirst {
				account.Credentials["temp_unschedulable_rules"] = []any{ignored, cooldown}
			} else {
				account.Credentials["temp_unschedulable_rules"] = []any{cooldown, ignored}
			}
			repo := &errorPolicyRepoStub{}
			svc := &RateLimitService{accountRepo: repo}
			body := []byte(`{"error":{"code":"service_tier_not_allowed"}}`)
			require.Equal(t, !ignoreFirst, svc.HandleUpstreamError(context.Background(), account, 403, nil, body, "gpt-5.4"))
			require.Len(t, repo.modelRateLimitCalls, map[bool]int{true: 0, false: 1}[ignoreFirst])
			require.Zero(t, repo.setErrCalls)
		})
	}
}

func TestTempUnschedulableZeroDuration_PreservesUnmatchedAndCustomPolicies(t *testing.T) {
	account := zeroDurationAccount(403, "not_allowed")
	repo := &errorPolicyRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	require.True(t, svc.HandleUpstreamError(context.Background(), account, 403, nil, []byte(`{"error":{"message":"account suspended"}}`)))
	require.Equal(t, 1, repo.setErrCalls)

	account.Credentials["custom_error_codes_enabled"] = true
	account.Credentials["custom_error_codes"] = []any{float64(403)}
	require.Equal(t, ErrorPolicyMatched, svc.CheckErrorPolicy(context.Background(), account, 403, []byte(`{"error":{"code":"service_tier_not_allowed"}}`)))
}

func TestTempUnschedulableZeroDuration_GrokSkipsBodyCooldown(t *testing.T) {
	account := zeroDurationAccount(403, "spending limit")
	account.Platform = PlatformGrok
	repo := &grokQuotaAccountRepo{}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.handleGrokAccountUpstreamError(context.Background(), account, 403, nil, []byte(`{"error":{"message":"spending limit reached"}}`))
	require.Zero(t, repo.tempUnschedCalls)
	require.Nil(t, account.TempUnschedulableUntil)
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
}

func TestTempUnschedulableZeroDuration_AntigravitySkipsDefaultPolicy(t *testing.T) {
	account := zeroDurationAccount(503, "quota")
	account.Platform = PlatformAntigravity
	gateway := &AntigravityGatewayService{}
	handled, code, err := gateway.applyErrorPolicy(antigravityRetryLoopParams{
		ctx: context.Background(), account: account,
	}, 503, nil, []byte(`{"error":{"message":"quota exhausted"}}`))
	require.True(t, handled)
	require.Equal(t, 503, code)
	require.NoError(t, err)
}

func TestTempUnschedulableZeroDuration_SkipsPoolHealthBreaker(t *testing.T) {
	account := zeroDurationAccount(503, "unavailable")
	account.Credentials["pool_mode"] = true
	encoded, err := json.Marshal(OpenAIAPIKeyHealthBreakerSettings{Enabled: true, WindowMinutes: 1, FailureThreshold: 3, CooldownMinutes: 5})
	require.NoError(t, err)
	settingRepo := &openAIAPIKeyHealthSettingRepo{value: string(encoded)}
	cache := &openAIAPIKeyHealthCacheStub{tripped: true}
	repo := &openAIAPIKeyHealthAccountRepo{}
	blocker := &openAIAPIKeyHealthRuntimeBlocker{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)
	svc.SetSettingService(NewSettingService(settingRepo, &config.Config{}))
	svc.SetOpenAIAPIKeyHealthCache(cache)
	svc.SetAccountRuntimeBlocker(blocker)
	for range 3 {
		require.False(t, svc.ObserveOpenAIAPIKeyHealthFailure(context.Background(), account, &UpstreamFailoverError{
			StatusCode: 503, ResponseBody: []byte(`{"error":{"message":"upstream unavailable"}}`),
		}))
	}
	require.Zero(t, settingRepo.getCalls)
	require.Zero(t, cache.recordCalls)
	require.Zero(t, repo.setCalls)
	require.Zero(t, blocker.calls)
	require.Nil(t, account.TempUnschedulableUntil)
}
