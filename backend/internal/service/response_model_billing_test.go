//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 夹具模型必须同时满足两个条件，否则测的就不是想测的那条规则：
//  1. 两者价格不同——否则"更便宜才采纳"的断言退化成恒真；
//  2. 两者都能被 HasIdentifiedTokenPricing 确定性识别（即价格表里的精确条目），
//     否则请求会先被"响应模型必须可识别"这道更靠前的门挡掉，成本比较根本走不到。
//
// claude-opus-4 / gpt-5.1 之类的名字不满足条件 2（前者不是 fallback 精确键，
// 后者与 gpt-5.5 共用同一条 gpt-5.4 价格因而也不满足条件 1）。
const (
	anthropicCheapFixtureModel  = "claude-sonnet-4"
	anthropicPriceyFixtureModel = "claude-opus-4.8"
	openAICheapFixtureModel     = "gpt-5.4-nano"
	openAIPriceyFixtureModel    = "gpt-5.5"
)

// orderedResponseBillingModels 返回 (cheaper, pricier) 及各自成本，按当前价格表排序，
// 使断言不依赖两个具体模型的价格大小关系（价格表调整时测试仍然自洽）。
func orderedResponseBillingModels(t *testing.T, svc *BillingService, tokens UsageTokens, a, b string) (string, string, *CostBreakdown, *CostBreakdown) {
	t.Helper()
	costA, err := svc.CalculateCost(a, tokens, 1.1)
	require.NoError(t, err)
	costB, err := svc.CalculateCost(b, tokens, 1.1)
	require.NoError(t, err)
	require.NotEqual(t, costA.TotalCost, costB.TotalCost, "fixture prices for %s and %s must differ", a, b)
	require.True(t, svc.HasIdentifiedTokenPricing(a), "fixture model %s must be identifiable in the pricing table", a)
	require.True(t, svc.HasIdentifiedTokenPricing(b), "fixture model %s must be identifiable in the pricing table", b)
	if costA.TotalCost < costB.TotalCost {
		return a, b, costA, costB
	}
	return b, a, costB, costA
}

// --- Anthropic gateway (GatewayService.RecordUsage) ---

func TestGatewayServiceRecordUsage_ResponseModelBillsCheaperResponseModel(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	cheaper, pricier, cheaperCost, _ := orderedResponseBillingModels(t, svc.billingService, tokens, anthropicCheapFixtureModel, anthropicPriceyFixtureModel)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:             "gateway_response_model_downgrade",
			Usage:                 ClaudeUsage{InputTokens: 100, OutputTokens: 50},
			Model:                 pricier,
			UpstreamResponseModel: cheaper, // upstream declared a runtime downgrade
			Duration:              time.Second,
		},
		APIKey:  &APIKey{ID: 501, Quota: 100},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      pricier,
			ChannelMappedModel: pricier,
			BillingModelSource: BillingModelSourceResponse,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, cheaperCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, cheaperCost.ActualCost, userRepo.lastAmount, 1e-12)
	require.True(t, usageRepo.lastLog.ActualCost > 0, "cost must not be zero")
	// 审计链完整保留：请求/发送模型不因计费切换被改写，响应模型与 mismatch 记录在案。
	require.Equal(t, pricier, usageRepo.lastLog.Model)
	require.Equal(t, pricier, usageRepo.lastLog.RequestedModel)
	require.NotNil(t, usageRepo.lastLog.UpstreamResponseModel)
	require.Equal(t, cheaper, *usageRepo.lastLog.UpstreamResponseModel)
	require.NotNil(t, usageRepo.lastLog.UpstreamModelMismatch)
	require.True(t, *usageRepo.lastLog.UpstreamModelMismatch)
}

func TestGatewayServiceRecordUsage_ResponseModelRejectsPricierResponseModel(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	cheaper, pricier, cheaperCost, _ := orderedResponseBillingModels(t, svc.billingService, tokens, anthropicCheapFixtureModel, anthropicPriceyFixtureModel)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:             "gateway_response_model_forged_upgrade",
			Usage:                 ClaudeUsage{InputTokens: 100, OutputTokens: 50},
			Model:                 cheaper,
			UpstreamResponseModel: pricier, // forged/upgraded declaration must not raise cost
			Duration:              time.Second,
		},
		APIKey:  &APIKey{ID: 501, Quota: 100},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      cheaper,
			ChannelMappedModel: cheaper,
			BillingModelSource: BillingModelSourceResponse,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, cheaperCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, cheaperCost.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestGatewayServiceRecordUsage_ResponseModelSafeFallbacks(t *testing.T) {
	tests := []struct {
		name          string
		responseModel func(cheaper string) string
		conflict      bool
		source        string
	}{
		{
			name:          "in_stream_conflict_falls_back_to_baseline",
			responseModel: func(cheaper string) string { return cheaper },
			conflict:      true,
			source:        BillingModelSourceResponse,
		},
		{
			name:          "empty_response_model_falls_back_to_baseline",
			responseModel: func(string) string { return "" },
			source:        BillingModelSourceResponse,
		},
		{
			name:          "unpriced_response_model_falls_back_to_baseline",
			responseModel: func(string) string { return "zz-unpriced-response-model" },
			source:        BillingModelSourceResponse,
		},
		{
			name:          "default_channel_mapped_mode_ignores_response_model",
			responseModel: func(cheaper string) string { return cheaper },
			source:        BillingModelSourceChannelMapped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
			tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
			cheaper, pricier, _, pricierCost := orderedResponseBillingModels(t, svc.billingService, tokens, anthropicCheapFixtureModel, anthropicPriceyFixtureModel)

			err := svc.RecordUsage(context.Background(), &RecordUsageInput{
				Result: &ForwardResult{
					RequestID:                     "gateway_response_model_fallback_" + tt.name,
					Usage:                         ClaudeUsage{InputTokens: 100, OutputTokens: 50},
					Model:                         pricier,
					UpstreamResponseModel:         tt.responseModel(cheaper),
					UpstreamResponseModelConflict: tt.conflict,
					Duration:                      time.Second,
				},
				APIKey:  &APIKey{ID: 501, Quota: 100},
				User:    &User{ID: 601},
				Account: &Account{ID: 701},
				ChannelUsageFields: ChannelUsageFields{
					ChannelID:          9,
					OriginalModel:      pricier,
					ChannelMappedModel: pricier,
					BillingModelSource: tt.source,
				},
			})

			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.InDelta(t, pricierCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, pricierCost.ActualCost, userRepo.lastAmount, 1e-12)
		})
	}
}

// --- OpenAI gateway (OpenAIGatewayService.RecordUsage) ---

func TestOpenAIGatewayServiceRecordUsage_ResponseModelBillsCheaperResponseModel(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	tokens := UsageTokens{InputTokens: 20, OutputTokens: 10}
	cheaper, pricier, cheaperCost, _ := orderedResponseBillingModels(t, svc.billingService, tokens, openAICheapFixtureModel, openAIPriceyFixtureModel)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:             "openai_response_model_downgrade",
			Model:                 pricier,
			UpstreamModel:         pricier,
			UpstreamResponseModel: cheaper,
			Usage:                 OpenAIUsage{InputTokens: 20, OutputTokens: 10},
			Duration:              time.Second,
		},
		APIKey:  &APIKey{ID: 10},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      pricier,
			ChannelMappedModel: pricier,
			BillingModelSource: BillingModelSourceResponse,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, cheaperCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, cheaperCost.ActualCost, userRepo.lastAmount, 1e-12)
	require.True(t, usageRepo.lastLog.ActualCost > 0, "cost must not be zero")
	// 审计链完整保留。
	require.Equal(t, pricier, usageRepo.lastLog.Model)
	require.NotNil(t, usageRepo.lastLog.UpstreamResponseModel)
	require.Equal(t, cheaper, *usageRepo.lastLog.UpstreamResponseModel)
	require.NotNil(t, usageRepo.lastLog.UpstreamModelMismatch)
	require.True(t, *usageRepo.lastLog.UpstreamModelMismatch)
}

func TestOpenAIGatewayServiceRecordUsage_ResponseModelRejectsPricierResponseModel(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	tokens := UsageTokens{InputTokens: 20, OutputTokens: 10}
	cheaper, pricier, cheaperCost, _ := orderedResponseBillingModels(t, svc.billingService, tokens, openAICheapFixtureModel, openAIPriceyFixtureModel)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:             "openai_response_model_forged_upgrade",
			Model:                 cheaper,
			UpstreamModel:         cheaper,
			UpstreamResponseModel: pricier,
			Usage:                 OpenAIUsage{InputTokens: 20, OutputTokens: 10},
			Duration:              time.Second,
		},
		APIKey:  &APIKey{ID: 10},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      cheaper,
			ChannelMappedModel: cheaper,
			BillingModelSource: BillingModelSourceResponse,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, cheaperCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, cheaperCost.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_ResponseModelSafeFallbacks(t *testing.T) {
	tests := []struct {
		name          string
		responseModel func(cheaper string) string
		conflict      bool
		source        string
	}{
		{
			name:          "in_stream_conflict_falls_back_to_baseline",
			responseModel: func(cheaper string) string { return cheaper },
			conflict:      true,
			source:        BillingModelSourceResponse,
		},
		{
			name:          "empty_response_model_falls_back_to_baseline",
			responseModel: func(string) string { return "" },
			source:        BillingModelSourceResponse,
		},
		{
			name:          "unpriced_response_model_falls_back_to_baseline",
			responseModel: func(string) string { return "zz-unpriced-response-model" },
			source:        BillingModelSourceResponse,
		},
		{
			name:          "default_channel_mapped_mode_ignores_response_model",
			responseModel: func(cheaper string) string { return cheaper },
			source:        BillingModelSourceChannelMapped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
			tokens := UsageTokens{InputTokens: 20, OutputTokens: 10}
			cheaper, pricier, _, pricierCost := orderedResponseBillingModels(t, svc.billingService, tokens, openAICheapFixtureModel, openAIPriceyFixtureModel)

			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID:                     "openai_response_model_fallback_" + tt.name,
					Model:                         pricier,
					UpstreamModel:                 pricier,
					UpstreamResponseModel:         tt.responseModel(cheaper),
					UpstreamResponseModelConflict: tt.conflict,
					Usage:                         OpenAIUsage{InputTokens: 20, OutputTokens: 10},
					Duration:                      time.Second,
				},
				APIKey:  &APIKey{ID: 10},
				User:    &User{ID: 20},
				Account: &Account{ID: 30},
				ChannelUsageFields: ChannelUsageFields{
					ChannelID:          9,
					OriginalModel:      pricier,
					ChannelMappedModel: pricier,
					BillingModelSource: tt.source,
				},
			})

			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.InDelta(t, pricierCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, pricierCost.ActualCost, userRepo.lastAmount, 1e-12)
		})
	}
}

// --- 准入规则本身 ---

func TestResponseModelBillingDeclaration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		source      string
		model       string
		conflict    bool
		mediaBilled bool
		want        string
	}{
		{name: "opted_in_and_clean", source: BillingModelSourceResponse, model: " claude-sonnet-4 ", want: "claude-sonnet-4"},
		{name: "other_source_never_looks_at_response", source: BillingModelSourceChannelMapped, model: "claude-sonnet-4"},
		{name: "empty_source_never_looks_at_response", source: "", model: "claude-sonnet-4"},
		{name: "upstream_source_never_looks_at_response", source: BillingModelSourceUpstream, model: "claude-sonnet-4"},
		{name: "in_stream_conflict_rejected", source: BillingModelSourceResponse, model: "claude-sonnet-4", conflict: true},
		{name: "media_billed_request_rejected", source: BillingModelSourceResponse, model: "claude-sonnet-4", mediaBilled: true},
		{name: "blank_declaration_rejected", source: BillingModelSourceResponse, model: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, responseModelBillingDeclaration(tt.source, tt.model, tt.conflict, tt.mediaBilled))
		})
	}
}

// 上游自报的模型名是外部输入。GetModelPricing 的系列兜底会给任意含 "haiku" 的名字
// 返回最便宜的系列价，因此计费准入必须走"确定性识别"，否则上游随手编一个名字就能
// 把账单压到地板价。本用例把这个差异钉死。
func TestBillingServiceHasIdentifiedTokenPricing_RejectsFamilyGuesses(t *testing.T) {
	t.Parallel()
	billing := newGatewayRecordUsageServiceForTest(
		&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{},
	).billingService

	require.True(t, billing.HasIdentifiedTokenPricing("claude-sonnet-4"))
	require.True(t, billing.HasIdentifiedTokenPricing("  CLAUDE-SONNET-4  "), "识别应当忽略大小写与空白")
	require.True(t, billing.HasIdentifiedTokenPricing("gpt-5.4-nano"))

	const forged = "totally-made-up-haiku-v9"
	if _, err := billing.GetModelPricing(forged); err == nil {
		// 这正是本函数存在的理由：宽松查价对编造的名字也会成功。
		require.False(t, billing.HasIdentifiedTokenPricing(forged),
			"family-guessed pricing must not qualify a model as a billing basis")
	}
	require.False(t, billing.HasIdentifiedTokenPricing(""))
	require.False(t, billing.HasIdentifiedTokenPricing("zz-unpriced-response-model"))
}

func TestGatewayServiceRecordUsage_ResponseModelRejectsUnidentifiedFamilyName(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	const forged = "totally-made-up-haiku-v9"

	baselineCost, err := svc.billingService.CalculateCost(anthropicPriceyFixtureModel, tokens, 1.1)
	require.NoError(t, err)
	// 前提：这个编造的名字确实能被宽松查价算出更低的费用——正是必须被拒绝的那条路径。
	forgedCost, err := svc.billingService.CalculateCost(forged, tokens, 1.1)
	require.NoError(t, err)
	require.Less(t, forgedCost.TotalCost, baselineCost.TotalCost)

	err = svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:             "gateway_response_model_forged_family_name",
			Usage:                 ClaudeUsage{InputTokens: 100, OutputTokens: 50},
			Model:                 anthropicPriceyFixtureModel,
			UpstreamResponseModel: forged,
			Duration:              time.Second,
		},
		APIKey:  &APIKey{ID: 501, Quota: 100},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      anthropicPriceyFixtureModel,
			ChannelMappedModel: anthropicPriceyFixtureModel,
			BillingModelSource: BillingModelSourceResponse,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, baselineCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, baselineCost.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_ResponseModelRejectsUnidentifiedFamilyName(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	tokens := UsageTokens{InputTokens: 20, OutputTokens: 10}
	const forged = "totally-made-up-haiku-v9"

	baselineCost, err := svc.billingService.CalculateCost(openAIPriceyFixtureModel, tokens, 1.1)
	require.NoError(t, err)
	forgedCost, err := svc.billingService.CalculateCost(forged, tokens, 1.1)
	require.NoError(t, err)
	require.Less(t, forgedCost.TotalCost, baselineCost.TotalCost)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:             "openai_response_model_forged_family_name",
			Model:                 openAIPriceyFixtureModel,
			UpstreamModel:         openAIPriceyFixtureModel,
			UpstreamResponseModel: forged,
			Usage:                 OpenAIUsage{InputTokens: 20, OutputTokens: 10},
			Duration:              time.Second,
		},
		APIKey:  &APIKey{ID: 10},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      openAIPriceyFixtureModel,
			ChannelMappedModel: openAIPriceyFixtureModel,
			BillingModelSource: BillingModelSourceResponse,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, baselineCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, baselineCost.ActualCost, userRepo.lastAmount, 1e-12)
}

// --- 渠道配置透传 ---

func TestToUsageFields_ResponseModelSourcePassesThrough(t *testing.T) {
	r := ChannelMappingResult{
		MappedModel:        "claude-fable-5",
		ChannelID:          4,
		Mapped:             false,
		BillingModelSource: BillingModelSourceResponse,
	}
	fields := r.ToUsageFields("claude-fable-5", "claude-fable-5")
	require.Equal(t, int64(4), fields.ChannelID)
	require.Equal(t, BillingModelSourceResponse, fields.BillingModelSource)
}
