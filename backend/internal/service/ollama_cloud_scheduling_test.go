package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 本文件覆盖 §11.3 ② 组（C1 + C2 + C6 + C7）与 C10：让 ollama_cloud 分组能
// 真正调度到账号。缺任一条的表现都是静默空池（候选池按 platform 精确匹配）。

// C2：漏了 IsOllamaCloud() 会被 openai_account_scheduler.go 的
// !IsOpenAICompatible() 过滤掉（filterStats: platform_mismatch）。
func TestOllamaCloudPlatform_IsOpenAICompatible(t *testing.T) {
	t.Parallel()

	acc := &Account{Platform: PlatformOllamaCloud, Type: AccountTypeAPIKey}
	require.True(t, acc.IsOpenAICompatible(), "ollama_cloud 账号必须进入 OpenAI 网关候选池")
	require.True(t, acc.IsOllamaCloud())

	// 其他平台的既有语义不变。
	require.True(t, (&Account{Platform: PlatformOpenAI}).IsOpenAICompatible())
	require.False(t, (&Account{Platform: PlatformAnthropic}).IsOpenAICompatible())
	require.False(t, (*Account)(nil).IsOpenAICompatible())
}

// C6 + F2：canonical 桶必须覆盖 ollama_cloud，否则 rebuild 不写该桶、缓存永不命中。
func TestOllamaCloudPlatform_SchedulerSnapshotPlatforms(t *testing.T) {
	t.Parallel()

	platforms := schedulerSnapshotPlatforms()
	require.Len(t, platforms, 11)
	require.Contains(t, platforms[:], PlatformOllamaCloud)

	seen := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		require.NotEmpty(t, platform)
		_, dup := seen[platform]
		require.Falsef(t, dup, "duplicate platform in schedulerSnapshotPlatforms: %s", platform)
		seen[platform] = struct{}{}
	}

	// 桶数必须与平台数组长度一致：每平台 single+forced，anthropic/gemini 各多一个 mixed。
	const groupID int64 = 7701
	require.Equal(t, len(platforms)*2+2, schedulerCanonicalBucketCount())
	require.Equal(t, 24, schedulerCanonicalBucketCount())

	buckets := schedulerCanonicalBuckets(groupID)
	require.Len(t, buckets, schedulerCanonicalBucketCount())
	require.Contains(t, buckets, SchedulerBucket{GroupID: groupID, Platform: PlatformOllamaCloud, Mode: SchedulerModeSingle})
	require.Contains(t, buckets, SchedulerBucket{GroupID: groupID, Platform: PlatformOllamaCloud, Mode: SchedulerModeForced})
}

// C1 + C2 端到端：ollama_cloud 分组的请求必须能在候选池里选中 ollama_cloud 账号。
// 若 C1 回退（归一为 openai），mock repo 按 platform 查询会返回空池 →
// ErrNoAvailableAccounts；若 C2 回退，候选过滤会以 platform_mismatch 排除。
func TestOllamaCloudPlatform_EntersOpenAISchedulerCandidatePool(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(7702)
	accounts := []Account{
		{
			ID:          77021,
			Platform:    PlatformOllamaCloud,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Credentials: map[string]any{"api_key": "ollama-key", "base_url": "https://ollama.com/v1"},
			// 运行时白名单生效后，空 mapping 且无清单的 ollama_cloud 账号是
			// deny-all（保存期门禁本就拒绝该配置），夹具必须声明清单才能入选。
			Extra: map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b"}},
		},
		{
			ID:          77022,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"",
		"",
		"gpt-oss:120b",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityChatCompletions,
		false,
		false,
		false,
		PlatformOllamaCloud,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(77021), selection.Account.ID, "ollama_cloud 分组必须命中 ollama_cloud 账号而不是 openai 账号")
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)

	// 反向控制：openai 分组的池里不得出现 ollama_cloud 账号（平台精确匹配）。
	openAISelection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"",
		"",
		"gpt-oss:120b",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityChatCompletions,
		false,
		false,
		false,
		PlatformOpenAI,
	)
	require.NoError(t, err)
	require.NotNil(t, openAISelection)
	require.NotNil(t, openAISelection.Account)
	require.Equal(t, int64(77022), openAISelection.Account.ID, "openai 分组不得命中 ollama_cloud 账号")
}

// C10：TestCredentials 的 default 分支会返回 unsupported platform → 管理端
// 「测试凭证」对新平台必定失败。
func TestOllamaCloudPlatform_TestCredentialsSupported(t *testing.T) {
	t.Parallel()

	svc := &AccountService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{
			{ID: 77031, Platform: PlatformOllamaCloud, Type: AccountTypeAPIKey},
		}},
	}
	require.NoError(t, svc.TestCredentials(context.Background(), 77031))
}
