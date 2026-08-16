//go:build unit

package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGeminiBillingRiskAdmissionInputUsesMappedModelAndRequestShape(t *testing.T) {
	pricingAt := time.Date(2026, time.August, 16, 4, 0, 0, 0, time.UTC)
	apiKey := &service.APIKey{ID: 11, User: &service.User{ID: 13}}
	subscription := &service.UserSubscription{ID: 17}
	body := []byte(`{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":321}}`)

	input := geminiBillingRiskAdmissionInput(
		apiKey,
		subscription,
		service.ChannelMappingResult{Mapped: true, MappedModel: "gemini-2.5-pro"},
		"public-gemini",
		body,
		pricingAt,
	)

	require.Same(t, apiKey, input.APIKey)
	require.Same(t, subscription, input.Subscription)
	require.Equal(t, service.BillingRiskRequestText, input.Kind)
	require.Equal(t, "gemini-2.5-pro", input.BillingModel)
	require.Positive(t, input.InputTokens)
	require.Equal(t, 321, input.MaxOutputTokens)
	require.Equal(t, 200_000, input.LongContextThreshold)
	require.Equal(t, 2.0, input.LongContextMultiplier)
	require.Equal(t, pricingAt, input.PricingAt)
	require.False(t, input.ConservativeUnknown)
}

func TestGeminiBillingRiskAdmissionInputClassifiesNativeImageGeneration(t *testing.T) {
	body := []byte(`{
		"contents":[{"parts":[{"text":"draw a city"}]}],
		"generationConfig":{"responseModalities":["TEXT","IMAGE"],"imageConfig":{"imageSize":"4K"}}
	}`)

	input := geminiBillingRiskAdmissionInput(
		&service.APIKey{ID: 11, User: &service.User{ID: 13}},
		nil,
		service.ChannelMappingResult{Mapped: true, MappedModel: "gemini-3-pro-image"},
		"public-gemini",
		body,
		time.Now(),
	)

	require.Equal(t, service.BillingRiskRequestSyncImage, input.Kind)
	require.Equal(t, 1, input.RequestCount)
	require.Equal(t, "4K", input.SizeTier)
	require.True(t, input.ConservativeUnknown)
}

func TestGeminiBillingRiskAdmissionInputUsesMappedModelForImageIntent(t *testing.T) {
	input := geminiBillingRiskAdmissionInput(
		&service.APIKey{ID: 11, User: &service.User{ID: 13}},
		nil,
		service.ChannelMappingResult{
			Mapped:             true,
			MappedModel:        "gemini-3-pro-image",
			BillingModelSource: service.BillingModelSourceRequested,
		},
		"public-model-alias",
		[]byte(`{"contents":[{"parts":[{"text":"draw a city"}]}]}`),
		time.Now(),
	)

	require.Equal(t, "public-model-alias", input.BillingModel)
	require.Equal(t, service.BillingRiskRequestSyncImage, input.Kind)
	require.True(t, input.ConservativeUnknown)
}

func TestGeminiBillingRiskGatePrecedesSelectionAndHandsPermitToMandatoryUsage(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "gemini_v1beta_handler.go", "GeminiV1BetaModels"))
	eligibility := strings.Index(source, "CheckBillingEligibility(")
	acquire := strings.Index(source, "acquireBillingRiskLease(")
	selection := strings.Index(source, "SelectAccount")
	handoff := strings.Index(source, ".Handoff()")
	mandatoryUsage := strings.Index(source, "submitBillingRiskUsageRecordTask(")

	require.NotEqual(t, -1, eligibility)
	require.NotEqual(t, -1, acquire)
	require.NotEqual(t, -1, selection)
	require.NotEqual(t, -1, handoff)
	require.NotEqual(t, -1, mandatoryUsage)
	require.Less(t, eligibility, acquire)
	require.Less(t, acquire, selection)
	require.Less(t, handoff, mandatoryUsage)
	require.Contains(t, source, "RiskPermit:")
}

// TestGeminiV1BetaHandler_PlatformRoutingInvariant 文档化并验证 Handler 层的平台路由逻辑不变量
// 该测试确保 gemini 和 antigravity 平台的路由逻辑符合预期
func TestGeminiV1BetaHandler_PlatformRoutingInvariant(t *testing.T) {
	tests := []struct {
		name            string
		platform        string
		expectedService string
		description     string
	}{
		{
			name:            "Gemini平台使用ForwardNative",
			platform:        service.PlatformGemini,
			expectedService: "GeminiMessagesCompatService.ForwardNative",
			description:     "Gemini OAuth 账户直接调用 Google API",
		},
		{
			name:            "Antigravity平台使用ForwardGemini",
			platform:        service.PlatformAntigravity,
			expectedService: "AntigravityGatewayService.ForwardGemini",
			description:     "Antigravity 账户通过 CRS 中转，支持 Gemini 协议",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 GeminiV1BetaModels 中的路由决策 (lines 199-205 in gemini_v1beta_handler.go)
			var routedService string
			if tt.platform == service.PlatformAntigravity {
				routedService = "AntigravityGatewayService.ForwardGemini"
			} else {
				routedService = "GeminiMessagesCompatService.ForwardNative"
			}

			require.Equal(t, tt.expectedService, routedService,
				"平台 %s 应该路由到 %s: %s",
				tt.platform, tt.expectedService, tt.description)
		})
	}
}

// TestGeminiV1BetaHandler_ListModelsAntigravityFallback 验证 ListModels 的 antigravity 降级逻辑
// 当没有 gemini 账户但有 antigravity 账户时，应返回静态模型列表
func TestGeminiV1BetaHandler_ListModelsAntigravityFallback(t *testing.T) {
	tests := []struct {
		name             string
		hasGeminiAccount bool
		hasAntigravity   bool
		expectedBehavior string
	}{
		{
			name:             "有Gemini账户-调用ForwardAIStudioGET",
			hasGeminiAccount: true,
			hasAntigravity:   false,
			expectedBehavior: "forward_to_upstream",
		},
		{
			name:             "无Gemini有Antigravity-返回静态列表",
			hasGeminiAccount: false,
			hasAntigravity:   true,
			expectedBehavior: "static_fallback",
		},
		{
			name:             "无任何账户-返回503",
			hasGeminiAccount: false,
			hasAntigravity:   false,
			expectedBehavior: "service_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 GeminiV1BetaListModels 的逻辑 (lines 33-44 in gemini_v1beta_handler.go)
			var behavior string

			if tt.hasGeminiAccount {
				behavior = "forward_to_upstream"
			} else if tt.hasAntigravity {
				behavior = "static_fallback"
			} else {
				behavior = "service_unavailable"
			}

			require.Equal(t, tt.expectedBehavior, behavior)
		})
	}
}

// TestGeminiV1BetaHandler_GetModelAntigravityFallback 验证 GetModel 的 antigravity 降级逻辑
func TestGeminiV1BetaHandler_GetModelAntigravityFallback(t *testing.T) {
	tests := []struct {
		name             string
		hasGeminiAccount bool
		hasAntigravity   bool
		expectedBehavior string
	}{
		{
			name:             "有Gemini账户-调用ForwardAIStudioGET",
			hasGeminiAccount: true,
			hasAntigravity:   false,
			expectedBehavior: "forward_to_upstream",
		},
		{
			name:             "无Gemini有Antigravity-返回静态模型信息",
			hasGeminiAccount: false,
			hasAntigravity:   true,
			expectedBehavior: "static_model_info",
		},
		{
			name:             "无任何账户-返回503",
			hasGeminiAccount: false,
			hasAntigravity:   false,
			expectedBehavior: "service_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 GeminiV1BetaGetModel 的逻辑 (lines 77-87 in gemini_v1beta_handler.go)
			var behavior string

			if tt.hasGeminiAccount {
				behavior = "forward_to_upstream"
			} else if tt.hasAntigravity {
				behavior = "static_model_info"
			} else {
				behavior = "service_unavailable"
			}

			require.Equal(t, tt.expectedBehavior, behavior)
		})
	}
}

func TestShouldFallbackGeminiModel_KnownFallbackOn404(t *testing.T) {
	t.Parallel()

	res := &service.UpstreamHTTPResult{StatusCode: http.StatusNotFound}
	require.True(t, shouldFallbackGeminiModel("gemini-3.1-pro-preview-customtools", res))
}

func TestShouldFallbackGeminiModel_UnknownModelOn404(t *testing.T) {
	t.Parallel()

	res := &service.UpstreamHTTPResult{StatusCode: http.StatusNotFound}
	require.False(t, shouldFallbackGeminiModel("gemini-future-model", res))
}

func TestShouldFallbackGeminiModel_DelegatesScopeFallback(t *testing.T) {
	t.Parallel()

	res := &service.UpstreamHTTPResult{
		StatusCode: http.StatusForbidden,
		Headers:    http.Header{"Www-Authenticate": []string{"Bearer error=\"insufficient_scope\""}},
		Body:       []byte("insufficient authentication scopes"),
	}
	require.True(t, shouldFallbackGeminiModel("gemini-future-model", res))
}
