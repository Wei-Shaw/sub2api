package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestNormalizeOpenAICompatiblePlatform_TypeSafeKeepsOwnValue 回归保护：
// typesafe 必须归一为自身。若归一为 openai，OpenAI 格式调度器的
// account.Platform != NormalizeOpenAICompatiblePlatform(req.Platform) 判定会把
// typesafe 账号全部过滤，/v1/systemone 永远选不到账号。
func TestNormalizeOpenAICompatiblePlatform_TypeSafeKeepsOwnValue(t *testing.T) {
	t.Parallel()
	require.Equal(t, PlatformTypeSafe, NormalizeOpenAICompatiblePlatform(PlatformTypeSafe))
	// 其余既有平台语义不变。
	require.Equal(t, PlatformOpenAI, NormalizeOpenAICompatiblePlatform(PlatformOpenAI))
	require.Equal(t, PlatformGrok, NormalizeOpenAICompatiblePlatform(PlatformGrok))
	require.Equal(t, PlatformKimi, NormalizeOpenAICompatiblePlatform(PlatformKimi))
	require.Equal(t, PlatformZhipu, NormalizeOpenAICompatiblePlatform(PlatformZhipu))
	require.Equal(t, PlatformDeepseek, NormalizeOpenAICompatiblePlatform(PlatformDeepseek))
	require.Equal(t, PlatformMiniMax, NormalizeOpenAICompatiblePlatform(PlatformMiniMax))
	require.Equal(t, PlatformOpenCodeGo, NormalizeOpenAICompatiblePlatform(PlatformOpenCodeGo))
	require.Equal(t, PlatformOpenAI, NormalizeOpenAICompatiblePlatform(PlatformAnthropic))
	require.Equal(t, PlatformOpenAI, NormalizeOpenAICompatiblePlatform("something-else"))
}

func newTypeSafeSchedulingTestService(accounts []Account) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	return &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
}

func typesafeSchedulingTestAccounts() []Account {
	return []Account{
		{
			ID:          72001,
			Platform:    PlatformTypeSafe,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"api_key": "ts-key", "base_url": "https://api.typesafe.ai"},
		},
		{
			ID:          72002,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-key"},
		},
	}
}

// TestSelectAccountWithSchedulerForCapability_TypeSafePlatformIsExactMatch 双向钉住：
// platform=typesafe 的请求只选 typesafe 账号，platform=openai 的请求只选 openai 账号，
// 两个池绝不互相穿越。
func TestSelectAccountWithSchedulerForCapability_TypeSafePlatformIsExactMatch(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(92001)
	svc := newTypeSafeSchedulingTestService(typesafeSchedulingTestAccounts())

	// typesafe 请求 → 命中 typesafe 账号。
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "", "jev-judge-v1", nil,
		OpenAIUpstreamTransportHTTPSSE, "", false, false, true, PlatformTypeSafe,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(72001), selection.Account.ID, "typesafe 请求必须选中 typesafe 账号")
	require.Equal(t, PlatformTypeSafe, selection.Account.Platform)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	// openai 请求 → 命中 openai 账号，绝不命中 typesafe 账号。
	selection, _, err = svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "", "gpt-5.4", nil,
		OpenAIUpstreamTransportHTTPSSE, "", false, false, true, PlatformOpenAI,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(72002), selection.Account.ID, "openai 请求不得选中 typesafe 账号")
	require.Equal(t, PlatformOpenAI, selection.Account.Platform)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

// TestSelectAccountWithSchedulerForCapability_TypeSafePoolIsNotShared 反向：只有 typesafe
// 账号时 platform=openai 选不到账号；只有 openai 账号时 platform=typesafe 选不到账号。
func TestSelectAccountWithSchedulerForCapability_TypeSafePoolIsNotShared(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(92002)
	accounts := typesafeSchedulingTestAccounts()

	typesafeOnly := newTypeSafeSchedulingTestService(accounts[:1])
	selection, _, err := typesafeOnly.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "", "gpt-5.4", nil,
		OpenAIUpstreamTransportHTTPSSE, "", false, false, true, PlatformOpenAI,
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableAccounts), "typesafe 账号不得被 openai 请求选中: %v", err)
	require.Nil(t, selection)

	openaiOnly := newTypeSafeSchedulingTestService(accounts[1:])
	selection, _, err = openaiOnly.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "", "jev-judge-v1", nil,
		OpenAIUpstreamTransportHTTPSSE, "", false, false, true, PlatformTypeSafe,
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableAccounts), "openai 账号不得被 typesafe 请求选中: %v", err)
	require.Nil(t, selection)
}
