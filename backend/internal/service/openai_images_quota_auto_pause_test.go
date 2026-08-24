//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Codex 文本用量（5h/7d 窗口）与 gpt-image 生图额度在上游是两套独立配额，
// 文本窗口触发自动暂停不应连带让账号无法服务 /v1/images/*（issue #1886）。

func TestShouldAutoPauseOpenAIAccountByQuota_ImagesEndpointExempt(t *testing.T) {
	account := &Account{
		ID:       188601,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent":   100.0,
			"auto_pause_5h_threshold": 0.95,
		},
	}

	paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
	require.True(t, paused, "文本请求仍按 5h 窗口自动暂停")
	require.Equal(t, "5h", decision.window)

	paused, _ = shouldAutoPauseOpenAIAccountByQuota(WithOpenAIImagesEndpoint(context.Background()), account)
	require.False(t, paused, "/v1/images/* 消耗独立生图配额，不受文本窗口暂停影响")

	// Responses 端点内嵌生图工具的请求同样吃文本额度，只带生图意图不构成豁免。
	paused, _ = shouldAutoPauseOpenAIAccountByQuota(WithOpenAIImageGenerationIntent(context.Background()), account)
	require.True(t, paused, "仅生图意图（非 /v1/images/* 入站）不豁免")
}

func TestShouldAutoPauseOpenAIAccountByQuota_ImagesEndpointExemptFor7dWindow(t *testing.T) {
	account := &Account{
		ID:       188602,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_7d_used_percent":   99.0,
			"auto_pause_7d_threshold": 0.9,
		},
	}

	paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
	require.True(t, paused)
	require.Equal(t, "7d", decision.window)

	paused, _ = shouldAutoPauseOpenAIAccountByQuota(WithOpenAIImagesEndpoint(context.Background()), account)
	require.False(t, paused)
}

func TestSelectAccountWithSchedulerForImages_TextQuotaExhaustedAccountStaysSelectable(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	exhausted := Account{
		ID:          188603,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra: map[string]any{
			"codex_7d_used_percent":   99.0,
			"auto_pause_7d_threshold": 0.95,
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{exhausted}},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	_, _, err := svc.SelectAccountWithScheduler(
		context.Background(), nil, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false,
	)
	require.Error(t, err, "文本调度仍应因 7d 窗口打满而无可用账号")

	selection, _, err := svc.SelectAccountWithSchedulerForImages(
		context.Background(), nil, "", "gpt-image-2", nil, OpenAIImagesCapabilityBasic,
	)
	require.NoError(t, err, "生图调度必须仍能选中该账号")
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(188603), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
