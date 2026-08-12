//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	cheaper, pricier, cheaperCost, _ := orderedResponseBillingModels(t, svc.billingService, tokens, "claude-sonnet-4", "claude-opus-4")

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
	cheaper, pricier, cheaperCost, _ := orderedResponseBillingModels(t, svc.billingService, tokens, "claude-sonnet-4", "claude-opus-4")

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
			cheaper, pricier, _, pricierCost := orderedResponseBillingModels(t, svc.billingService, tokens, "claude-sonnet-4", "claude-opus-4")

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
	cheaper, pricier, cheaperCost, _ := orderedResponseBillingModels(t, svc.billingService, tokens, "gpt-5.1", "gpt-5.5")

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
	cheaper, pricier, cheaperCost, _ := orderedResponseBillingModels(t, svc.billingService, tokens, "gpt-5.1", "gpt-5.5")

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
			cheaper, pricier, _, pricierCost := orderedResponseBillingModels(t, svc.billingService, tokens, "gpt-5.1", "gpt-5.5")

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
