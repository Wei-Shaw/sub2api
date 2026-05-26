package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactPrefersSupportedOverUnknown
// 验证 compact 调度时显式支持 (tier=2) 优先于未探测 (tier=1)。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactPrefersSupportedOverUnknown(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91001)
	accounts := []Account{
		{
			ID:          71001,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{}, // unknown
		},
		{
			ID:          71002,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{"openai_compact_supported": true}, // tier=2
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

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71002), selection.Account.ID, "compact-supported account should win over unknown")
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRejectsExplicitlyUnsupported
// 验证 force_off / 已探测不支持 (tier=0) 的账号不会被 compact 请求选中。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactRejectsExplicitlyUnsupported(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91002)
	accounts := []Account{
		{
			ID:          71010,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{"openai_compact_mode": OpenAICompactModeForceOff},
		},
		{
			ID:          71011,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{"openai_compact_supported": false},
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

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableCompactAccounts), "compact-only accounts should rejected explicitly unsupported and return compact error")
	require.Nil(t, selection)
}

// TestOpenAIGatewayService_SelectAccountWithScheduler_CompactFallsBackToUnknown
// 验证当没有"已知支持"账号时，compact 请求会回退到"未探测"账号。
func TestOpenAIGatewayService_SelectAccountWithScheduler_CompactFallsBackToUnknown(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	ctx := context.Background()
	groupID := int64(91003)
	accounts := []Account{
		{
			ID:          71020,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{"openai_compact_supported": false}, // tier=0
		},
		{
			ID:          71021,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			Extra:       map[string]any{}, // unknown -> tier=1
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

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(71021), selection.Account.ID, "unknown account should be picked when no supported account available")
}

// TestOpenAICompactSupportTier 验证 tier 分类逻辑。
func TestOpenAICompactSupportTier(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    int
	}{
		{name: "nil", account: nil, want: 0},
		{name: "non openai", account: &Account{Platform: PlatformAnthropic}, want: 0},
		{name: "openai unknown", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{}}, want: 1},
		{name: "openai supported", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_supported": true}}, want: 2},
		{name: "openai unsupported", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_supported": false}}, want: 0},
		{name: "force on", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_mode": OpenAICompactModeForceOn}}, want: 2},
		{name: "force off overrides probe true", account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_compact_mode": OpenAICompactModeForceOff, "openai_compact_supported": true}}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openAICompactSupportTier(tt.account); got != tt.want {
				t.Fatalf("openAICompactSupportTier(...) = %d, want %d", got, tt.want)
			}
		})
	}
}
