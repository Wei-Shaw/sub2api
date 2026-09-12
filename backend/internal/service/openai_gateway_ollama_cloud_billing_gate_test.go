//go:build unit

package service

// B2-④（请求期 ollama_cloud 定价缺失显式拒绝）与 S1（计费候选过滤纳入
// ollama_cloud）的回归测试。
//
// 关键顺序约束的实证：S1 只在「定价已就位」的前提下才安全——若过滤把候选滤空
// 而定价缺失，会从误计变全零计费。本文件用真实 fallbackPrices（含 ollama 价卡）
// 证明：ollama 真实模型名（不含 claude-*）永不被过滤，候选不会滤空。

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFilterCNProviderBillingModelCandidates_OllamaCloud(t *testing.T) {
	svc := &OpenAIGatewayService{} // resolver 为 nil → 无显式分组/渠道定价
	apiKey := &APIKey{Group: &Group{ID: 1, Platform: PlatformOllamaCloud}}
	ollamaAccount := &Account{ID: 4, Platform: PlatformOllamaCloud}

	// claude-* 被移除，ollama 真实模型名保留 → 候选不会滤空。
	filtered := svc.filterCNProviderBillingModelCandidates(context.Background(), ollamaAccount, apiKey,
		[]string{"claude-sonnet-4-5", "gpt-oss:120b-cloud", "qwen3.5:397b"})
	require.Equal(t, []string{"gpt-oss:120b-cloud", "qwen3.5:397b"}, filtered,
		"ollama 真实模型名必须保留，只有 claude-* 被过滤")

	// 全 claude 候选才会被清空（上层走 B2-④ 显式拒绝，而非误计 Anthropic 价）。
	allClaude := svc.filterCNProviderBillingModelCandidates(context.Background(), ollamaAccount, apiKey,
		[]string{"claude-sonnet-4-5", "claude-opus-4-1"})
	require.Empty(t, allClaude)

	// 未定价的非 claude 名也不过滤（过滤只针对 claude-*，不查价）。
	unpriced := svc.filterCNProviderBillingModelCandidates(context.Background(), ollamaAccount, apiKey,
		[]string{"totally-unpriced-model"})
	require.Equal(t, []string{"totally-unpriced-model"}, unpriced)

	// 既有豁免语义回归：运营者显式配置 claude 渠道价时 claude 名保留。
	const groupID = int64(778)
	svcWithChannel := &OpenAIGatewayService{billingService: NewBillingService(nil, nil)}
	svcWithChannel.channelService = newChannelServiceWithPricings(groupID, []ChannelModelPricing{
		tokenPricingForModels([]string{"claude-sonnet-4-5"}, 3.0),
	})
	svcWithChannel.resolver = NewModelPricingResolver(svcWithChannel.channelService, svcWithChannel.billingService)
	apiKeyWithGroup := &APIKey{Group: &Group{ID: groupID, Platform: PlatformOllamaCloud}}
	kept := svcWithChannel.filterCNProviderBillingModelCandidates(context.Background(), ollamaAccount, apiKeyWithGroup,
		[]string{"claude-sonnet-4-5", "gpt-oss:120b-cloud"})
	require.Equal(t, []string{"claude-sonnet-4-5", "gpt-oss:120b-cloud"}, kept,
		"显式渠道定价的 claude-* 候选必须保留（既有豁免语义）")
}

// B2-④ 主断言：ollama_cloud + 未定价模型 → 400 MODEL_PRICING_MISSING、
// 不落零成本记录（usage/billing 均不写库、日志无 pricing_missing_record_zero_cost
// 对应的落账路径）。
func TestOpenAIGatewayRecordUsage_OllamaCloudUnpricedModelRejected(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_ollama_unpriced",
			Usage: OpenAIUsage{
				InputTokens:  1_000_000,
				OutputTokens: 0,
			},
			Model:    "totally-unpriced-model",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 2},
		Account: &Account{ID: 3, Platform: PlatformOllamaCloud},
	})
	requireModelPricingMissing(t, err, "totally-unpriced-model")
	require.Zero(t, usageRepo.calls, "拒绝路径不得写 usage log（零成本落账被收窄掉）")
	require.Zero(t, billingRepo.calls, "拒绝路径不得触发计费入账")
}

// claude-* 请求打到 ollama_cloud：候选被 S1 滤空 → ErrModelPricingUnavailable →
// B2-④ 显式拒绝。这是「按 Anthropic 价静默误计」被修复后的目标行为。
func TestOpenAIGatewayRecordUsage_OllamaCloudClaudeNameRejected(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_ollama_claude_name",
			Usage: OpenAIUsage{
				InputTokens:  1_000_000,
				OutputTokens: 0,
			},
			Model:    "claude-sonnet-4-5",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 2},
		Account: &Account{ID: 3, Platform: PlatformOllamaCloud},
	})
	requireModelPricingMissing(t, err)
	require.Zero(t, usageRepo.calls)
}

// 已定价模型（真实 :tag 请求形态）→ 正常落账且 actual_cost > 0：
// 证明 S1 过滤 + B2-① tag fallback + B2-② 价卡组合后，ollama 正常请求计费完整。
func TestOpenAIGatewayRecordUsage_OllamaCloudPricedModelBilledPositive(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_ollama_priced",
			Usage: OpenAIUsage{
				InputTokens:  1_000_000,
				OutputTokens: 0,
			},
			Model:    "gpt-oss:120b-cloud",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 2},
		Account: &Account{ID: 3, Platform: PlatformOllamaCloud},
	})
	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Greater(t, usageRepo.lastLog.ActualCost, 0.0,
		"gpt-oss:120b-cloud 经 :tag fallback 命中 ollama 价卡，actual_cost 必须为正")
}

// 只收窄 ollama_cloud 的回归：其它平台的定价缺失保持既有 fail-open
// （零成本落账 + Warn），不得被本批改动波及。
func TestOpenAIGatewayRecordUsage_NonOllamaPlatformZeroCostFallbackPreserved(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_openai_unpriced_zero_cost",
			Usage: OpenAIUsage{
				InputTokens:  1_000_000,
				OutputTokens: 0,
			},
			Model:    "totally-unpriced-model",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 2},
		Account: &Account{ID: 3, Platform: PlatformOpenAI},
	})
	require.NoError(t, err, "非 ollama 平台的定价缺失必须保持既有零成本落账行为")
	require.Equal(t, 1, usageRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 0.0, usageRepo.lastLog.ActualCost)
}
