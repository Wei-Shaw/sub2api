//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// ---------------------------------------------------------------------------
// normalizeTier
// ---------------------------------------------------------------------------

func TestNormalizeTier(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected string
	}{
		{name: "empty string", raw: "", expected: ""},
		{name: "free-tier", raw: "free-tier", expected: "FREE"},
		{name: "g1-pro-tier", raw: "g1-pro-tier", expected: "PRO"},
		{name: "g1-ultra-tier", raw: "g1-ultra-tier", expected: "ULTRA"},
		{name: "unknown-something", raw: "unknown-something", expected: "UNKNOWN"},
		{name: "Google AI Pro contains pro keyword", raw: "Google AI Pro", expected: "PRO"},
		{name: "case insensitive FREE", raw: "FREE-TIER", expected: "FREE"},
		{name: "case insensitive Ultra", raw: "Ultra Plan", expected: "ULTRA"},
		{name: "arbitrary unrecognized string", raw: "enterprise-custom", expected: "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTier(tt.raw)
			require.Equal(t, tt.expected, got, "normalizeTier(%q)", tt.raw)
		})
	}
}

// ---------------------------------------------------------------------------
// buildUsageInfo
// ---------------------------------------------------------------------------

func aqfBoolPtr(v bool) *bool { return &v }
func aqfIntPtr(v int) *int    { return &v }

func TestBuildUsageInfo_BasicModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.75,
					ResetTime:         "2026-03-08T12:00:00Z",
				},
				DisplayName:      "Claude Sonnet 4",
				SupportsImages:   aqfBoolPtr(true),
				SupportsThinking: aqfBoolPtr(false),
				ThinkingBudget:   aqfIntPtr(0),
				Recommended:      aqfBoolPtr(true),
				MaxTokens:        aqfIntPtr(200000),
				MaxOutputTokens:  aqfIntPtr(16384),
				SupportedMimeTypes: map[string]bool{
					"image/png":  true,
					"image/jpeg": true,
				},
			},
			"gemini-2.5-pro": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.50,
					ResetTime:         "2026-03-08T15:00:00Z",
				},
				DisplayName:     "Gemini 2.5 Pro",
				MaxTokens:       aqfIntPtr(1000000),
				MaxOutputTokens: aqfIntPtr(65536),
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "g1-pro-tier", "PRO", nil)

	// 基本字段
	require.NotNil(t, info.UpdatedAt, "UpdatedAt should be set")
	require.Equal(t, "PRO", info.SubscriptionTier)
	require.Equal(t, "g1-pro-tier", info.SubscriptionTierRaw)

	// AntigravityQuota
	require.Len(t, info.AntigravityQuota, 2)

	sonnetQuota := info.AntigravityQuota["claude-sonnet-4-20250514"]
	require.NotNil(t, sonnetQuota)
	require.Equal(t, 25, sonnetQuota.Utilization) // (1 - 0.75) * 100 = 25
	require.Equal(t, "2026-03-08T12:00:00Z", sonnetQuota.ResetTime)

	geminiQuota := info.AntigravityQuota["gemini-2.5-pro"]
	require.NotNil(t, geminiQuota)
	require.Equal(t, 50, geminiQuota.Utilization) // (1 - 0.50) * 100 = 50
	require.Equal(t, "2026-03-08T15:00:00Z", geminiQuota.ResetTime)

	// AntigravityQuotaDetails
	require.Len(t, info.AntigravityQuotaDetails, 2)

	sonnetDetail := info.AntigravityQuotaDetails["claude-sonnet-4-20250514"]
	require.NotNil(t, sonnetDetail)
	require.Equal(t, "Claude Sonnet 4", sonnetDetail.DisplayName)
	require.Equal(t, aqfBoolPtr(true), sonnetDetail.SupportsImages)
	require.Equal(t, aqfBoolPtr(false), sonnetDetail.SupportsThinking)
	require.Equal(t, aqfIntPtr(0), sonnetDetail.ThinkingBudget)
	require.Equal(t, aqfBoolPtr(true), sonnetDetail.Recommended)
	require.Equal(t, aqfIntPtr(200000), sonnetDetail.MaxTokens)
	require.Equal(t, aqfIntPtr(16384), sonnetDetail.MaxOutputTokens)
	require.Equal(t, map[string]bool{"image/png": true, "image/jpeg": true}, sonnetDetail.SupportedMimeTypes)

	geminiDetail := info.AntigravityQuotaDetails["gemini-2.5-pro"]
	require.NotNil(t, geminiDetail)
	require.Equal(t, "Gemini 2.5 Pro", geminiDetail.DisplayName)
	require.Nil(t, geminiDetail.SupportsImages)
	require.Nil(t, geminiDetail.SupportsThinking)
	require.Equal(t, aqfIntPtr(1000000), geminiDetail.MaxTokens)
	require.Equal(t, aqfIntPtr(65536), geminiDetail.MaxOutputTokens)
}

func TestBuildUsageInfo_DeprecatedModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 1.0,
				},
			},
		},
		DeprecatedModelIDs: map[string]antigravity.DeprecatedModelInfo{
			"claude-3-sonnet-20240229": {NewModelID: "claude-sonnet-4-20250514"},
			"claude-3-haiku-20240307":  {NewModelID: "claude-haiku-3.5-latest"},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.Len(t, info.ModelForwardingRules, 2)
	require.Equal(t, "claude-sonnet-4-20250514", info.ModelForwardingRules["claude-3-sonnet-20240229"])
	require.Equal(t, "claude-haiku-3.5-latest", info.ModelForwardingRules["claude-3-haiku-20240307"])
}

func TestBuildUsageInfo_NoDeprecatedModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"some-model": {
				QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 0.9},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.Nil(t, info.ModelForwardingRules, "ModelForwardingRules should be nil when no deprecated models")
}

func TestBuildUsageInfo_EmptyModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info)
	require.NotNil(t, info.AntigravityQuota)
	require.Empty(t, info.AntigravityQuota)
	require.NotNil(t, info.AntigravityQuotaDetails)
	require.Empty(t, info.AntigravityQuotaDetails)
	require.Nil(t, info.FiveHour, "FiveHour should be nil when no priority model exists")
}

func TestBuildUsageInfo_ModelWithNilQuotaInfo(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"model-without-quota": {
				DisplayName: "No Quota Model",
				// QuotaInfo is nil
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info)
	require.Empty(t, info.AntigravityQuota, "models with nil QuotaInfo should be skipped")
	require.Empty(t, info.AntigravityQuotaDetails, "models with nil QuotaInfo should be skipped from details too")
}

func TestBuildUsageInfo_FiveHourPriorityOrder(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	// priorityModels = ["claude-sonnet-4-20250514", "claude-sonnet-4", "gemini-2.5-pro"]
	// When the first priority model exists, it should be used for FiveHour
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"gemini-2.5-pro": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.40,
					ResetTime:         "2026-03-08T18:00:00Z",
				},
			},
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.80,
					ResetTime:         "2026-03-08T12:00:00Z",
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info.FiveHour, "FiveHour should be set when a priority model exists")
	// claude-sonnet-4-20250514 is first in priority list, so it should be used
	expectedUtilization := (1.0 - 0.80) * 100 // 20
	require.InDelta(t, expectedUtilization, info.FiveHour.Utilization, 0.01)
	require.NotNil(t, info.FiveHour.ResetsAt, "ResetsAt should be parsed from ResetTime")
}

func TestBuildUsageInfo_FiveHourFallbackToClaude4(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	// Only claude-sonnet-4 exists (second in priority list), not claude-sonnet-4-20250514
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.60,
					ResetTime:         "2026-03-08T14:00:00Z",
				},
			},
			"gemini-2.5-pro": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.30,
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info.FiveHour)
	expectedUtilization := (1.0 - 0.60) * 100 // 40
	require.InDelta(t, expectedUtilization, info.FiveHour.Utilization, 0.01)
}

func TestBuildUsageInfo_FiveHourFallbackToGemini(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	// Only gemini-2.5-pro exists (third in priority list)
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"gemini-2.5-pro": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.30,
				},
			},
			"other-model": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.90,
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info.FiveHour)
	expectedUtilization := (1.0 - 0.30) * 100 // 70
	require.InDelta(t, expectedUtilization, info.FiveHour.Utilization, 0.01)
}

func TestBuildUsageInfo_FiveHourNoPriorityModel(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	// None of the priority models exist
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"some-other-model": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.50,
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.Nil(t, info.FiveHour, "FiveHour should be nil when no priority model exists")
}

func TestBuildUsageInfo_FiveHourWithEmptyResetTime(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.50,
					ResetTime:         "", // empty reset time
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info.FiveHour)
	require.Nil(t, info.FiveHour.ResetsAt, "ResetsAt should be nil when ResetTime is empty")
	require.Equal(t, 0, info.FiveHour.RemainingSeconds)
}

func TestBuildUsageInfo_FullUtilization(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.0, // fully used
					ResetTime:         "2026-03-08T12:00:00Z",
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	quota := info.AntigravityQuota["claude-sonnet-4-20250514"]
	require.NotNil(t, quota)
	require.Equal(t, 100, quota.Utilization)
}

func TestBuildUsageInfo_ZeroUtilization(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 1.0, // fully available
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)
	quota := info.AntigravityQuota["claude-sonnet-4-20250514"]
	require.NotNil(t, quota)
	require.Equal(t, 0, quota.Utilization)
}

func TestBuildUsageInfo_AICredits(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{},
	}
	loadResp := &antigravity.LoadCodeAssistResponse{
		PaidTier: &antigravity.PaidTierInfo{
			ID: "g1-pro-tier",
			AvailableCredits: []antigravity.AvailableCredit{
				{
					CreditType:                  "GOOGLE_ONE_AI",
					CreditAmount:                "25",
					MinimumCreditAmountForUsage: "5",
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "g1-pro-tier", "PRO", loadResp)

	require.Len(t, info.AICredits, 1)
	require.Equal(t, "GOOGLE_ONE_AI", info.AICredits[0].CreditType)
	require.Equal(t, 25.0, info.AICredits[0].Amount)
	require.Equal(t, 5.0, info.AICredits[0].MinimumBalance)
}

func TestFetchQuotaUsesConfiguredModelsListBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"model-a":{}}}`))
	}))
	defer server.Close()

	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	t.Cleanup(func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	})
	antigravity.BaseURLs = []string{server.URL}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	cfg := &config.Config{}
	cfg.Gateway.ModelsListReadMaxBytes = 8
	fetcher := NewAntigravityQuotaFetcher(nil, cfg)
	_, err := fetcher.FetchQuota(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "project",
		},
	}, "")
	require.ErrorContains(t, err, "响应超过 8 字节")
}

func TestFetchQuota_ForbiddenReturnsIsForbidden(t *testing.T) {
	// 模拟 FetchQuota 遇到 403 时的行为：
	// FetchAvailableModels 返回 ForbiddenError → FetchQuota 应返回 is_forbidden=true
	forbiddenErr := &antigravity.ForbiddenError{
		StatusCode: 403,
		Body:       "Access denied",
	}

	// 验证 ForbiddenError 满足 errors.As
	var target *antigravity.ForbiddenError
	require.True(t, errors.As(forbiddenErr, &target))
	require.Equal(t, 403, target.StatusCode)
	require.Equal(t, "Access denied", target.Body)
	require.Contains(t, forbiddenErr.Error(), "403")
}

// ---------------------------------------------------------------------------
// classifyForbiddenType
// ---------------------------------------------------------------------------

func TestClassifyForbiddenType(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "VALIDATION_REQUIRED keyword",
			body:     `{"error":{"message":"VALIDATION_REQUIRED"}}`,
			expected: "validation",
		},
		{
			name:     "verify your account",
			body:     `Please verify your account to continue`,
			expected: "validation",
		},
		{
			name:     "contains validation_url field",
			body:     `{"error":{"details":[{"metadata":{"validation_url":"https://..."}}]}}`,
			expected: "validation",
		},
		{
			name:     "terms of service violation",
			body:     `Your account has been suspended for Terms of Service violation`,
			expected: "violation",
		},
		{
			name:     "violation keyword",
			body:     `Account suspended due to policy violation`,
			expected: "violation",
		},
		{
			name:     "generic 403",
			body:     `Access denied`,
			expected: "forbidden",
		},
		{
			name:     "empty body",
			body:     "",
			expected: "forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyForbiddenType(tt.body)
			require.Equal(t, tt.expected, got)
		})
	}
}

// ---------------------------------------------------------------------------
// extractValidationURL
// ---------------------------------------------------------------------------

func TestExtractValidationURL(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "structured validation_url",
			body:     `{"error":{"details":[{"metadata":{"validation_url":"https://accounts.google.com/verify?token=abc"}}]}}`,
			expected: "https://accounts.google.com/verify?token=abc",
		},
		{
			name:     "structured appeal_url",
			body:     `{"error":{"details":[{"metadata":{"appeal_url":"https://support.google.com/appeal/123"}}]}}`,
			expected: "https://support.google.com/appeal/123",
		},
		{
			name:     "validation_url takes priority over appeal_url",
			body:     `{"error":{"details":[{"metadata":{"validation_url":"https://v.com","appeal_url":"https://a.com"}}]}}`,
			expected: "https://v.com",
		},
		{
			name:     "fallback regex with verify keyword",
			body:     `Please verify your account at https://accounts.google.com/verify`,
			expected: "https://accounts.google.com/verify",
		},
		{
			name:     "no URL in generic forbidden",
			body:     `Access denied`,
			expected: "",
		},
		{
			name:     "empty body",
			body:     "",
			expected: "",
		},
		{
			name:     "URL present but no validation keywords",
			body:     `Error at https://example.com/something`,
			expected: "",
		},
		{
			name:     "unicode escaped ampersand",
			body:     `validation required: https://accounts.google.com/verify?a=1\u0026b=2`,
			expected: "https://accounts.google.com/verify?a=1&b=2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractValidationURL(tt.body)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestFetchQuota_MergesRetrieveUserQuotaSummaryAndSkipsDisabledBucket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1internal:fetchAvailableModels":
			_, _ = w.Write([]byte(`{
				"models": {
					"gemini-4-argon-high": {"quotaInfo": {"remainingFraction": 0.1328875, "resetTime": "2026-10-21T00:29:07Z"}},
					"claude-opus-5-5-high": {"quotaInfo": {"resetTime": "2026-10-21T00:38:06Z"}}
				}
			}`))
		case "/v1internal:retrieveUserQuotaSummary":
			_, _ = w.Write([]byte(`{
				"groups": [
					{
						"displayName": "Gemini Models",
						"buckets": [
							{"bucketId": "gemini-weekly", "window": "weekly", "remainingFraction": 0.1328875, "resetTime": "2026-10-21T00:29:07Z"},
							{"bucketId": "gemini-5h", "window": "5h", "remainingFraction": 0.8336412, "resetTime": "2026-10-16T08:59:40Z"}
						]
					},
					{
						"displayName": "Claude and GPT models",
						"buckets": [
							{"bucketId": "3p-weekly", "window": "weekly", "remainingFraction": 0, "resetTime": "2026-10-21T00:38:06Z"},
							{"bucketId": "3p-5h", "window": "5h", "disabled": true, "remainingFraction": 1, "resetTime": "2026-10-16T13:50:47Z"}
						]
					}
				]
			}`))
		case "/v1internal:loadCodeAssist":
			_, _ = w.Write([]byte(`{"paidTier":{"id":"g1-ultra-tier"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	t.Cleanup(func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	})
	antigravity.BaseURLs = []string{server.URL}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	fetcher := NewAntigravityQuotaFetcher(nil, &config.Config{})
	res, err := fetcher.FetchQuota(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "project",
		},
	}, "")
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.UsageInfo)

	// Active summary buckets are recorded with used-percentage utilization.
	require.NotNil(t, res.UsageInfo.AntigravityQuota["gemini-5h"])
	require.Equal(t, 16, res.UsageInfo.AntigravityQuota["gemini-5h"].Utilization)
	require.Equal(t, "2026-10-16T08:59:40Z", res.UsageInfo.AntigravityQuota["gemini-5h"].ResetTime)

	require.NotNil(t, res.UsageInfo.AntigravityQuota["gemini-weekly"])
	require.Equal(t, 86, res.UsageInfo.AntigravityQuota["gemini-weekly"].Utilization)
	require.Equal(t, "2026-10-21T00:29:07Z", res.UsageInfo.AntigravityQuota["gemini-weekly"].ResetTime)

	require.NotNil(t, res.UsageInfo.AntigravityQuota["3p-weekly"])
	require.Equal(t, 100, res.UsageInfo.AntigravityQuota["3p-weekly"].Utilization)
	require.Equal(t, "2026-10-21T00:38:06Z", res.UsageInfo.AntigravityQuota["3p-weekly"].ResetTime)

	// Disabled 3p-5h bucket must not be recorded as a misleading 0% used bar.
	require.Nil(t, res.UsageInfo.AntigravityQuota["3p-5h"])

	// Model-level entries remain available for capability sync and fallback.
	require.NotNil(t, res.UsageInfo.AntigravityQuota["gemini-4-argon-high"])
	require.Equal(t, 86, res.UsageInfo.AntigravityQuota["gemini-4-argon-high"].Utilization)
	require.NotNil(t, res.UsageInfo.AntigravityQuota["claude-opus-5-5-high"])
	require.Equal(t, 100, res.UsageInfo.AntigravityQuota["claude-opus-5-5-high"].Utilization)

	// FiveHour and SevenDay progress are populated from active summary buckets.
	require.NotNil(t, res.UsageInfo.FiveHour)
	require.InDelta(t, 16.63588, res.UsageInfo.FiveHour.Utilization, 0.01)
	require.NotNil(t, res.UsageInfo.SevenDay)
	require.InDelta(t, 100.0, res.UsageInfo.SevenDay.Utilization, 0.01)
}

func TestBuildUsageInfo_FiveHourPriorityIncludesClaude55AndGemini4Argon(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"gemini-4-argon-high": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.25,
					ResetTime:         "2026-10-21T00:29:07Z",
				},
			},
		},
	}

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)
	require.NotNil(t, info.FiveHour)
	require.InDelta(t, 75.0, info.FiveHour.Utilization, 0.01)
}

func TestFetchQuota_UsesForwardEndpointForModelsAndSummary(t *testing.T) {
	for _, tc := range []struct {
		name, plan, override string
		daily                bool
	}{
		{name: "paid", plan: "pro", daily: true},
		{name: "free", plan: "free"},
		{name: "daily override", plan: "free", override: "daily", daily: true},
		{name: "sandbox override", plan: "free", override: "sandbox", daily: true},
		{name: "prod override", plan: "ultra", override: "prod"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(antigravityForwardBaseURLEnv, tc.override)
			requests := map[string][]string{}
			var requestsMu sync.Mutex
			handler := func(endpoint string, remaining string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					requestsMu.Lock()
					requests[endpoint] = append(requests[endpoint], r.URL.Path)
					requestsMu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/v1internal:fetchAvailableModels":
						_, _ = w.Write([]byte(`{"models":{"gemini-4-argon-high":{"quotaInfo":{"remainingFraction":` + remaining + `}}}}`))
					case "/v1internal:retrieveUserQuotaSummary":
						_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-5h","window":"5h","remainingFraction":` + remaining + `}]}]}`))
					case "/v1internal:loadCodeAssist":
						_, _ = w.Write([]byte(`{"currentTier":{"id":"free-tier"}}`))
					}
				}
			}
			prod := httptest.NewServer(handler("prod", "0.93"))
			defer prod.Close()
			daily := httptest.NewServer(handler("daily", "0.62"))
			defer daily.Close()
			old := antigravity.BaseURLs
			t.Cleanup(func() { antigravity.BaseURLs = old })
			antigravity.BaseURLs = []string{prod.URL, daily.URL}
			result, err := NewAntigravityQuotaFetcher(nil, &config.Config{}).FetchQuota(context.Background(), &Account{
				Platform: PlatformAntigravity, Type: AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "token", "project_id": "project", "plan_type": tc.plan},
			}, "")
			require.NoError(t, err)
			selected, other, used := "prod", "daily", 7.0
			if tc.daily {
				selected, other, used = "daily", "prod", 38.0
			}
			requestsMu.Lock()
			defer requestsMu.Unlock()
			require.Contains(t, requests[selected], "/v1internal:fetchAvailableModels")
			require.Contains(t, requests[selected], "/v1internal:retrieveUserQuotaSummary")
			require.NotContains(t, requests[other], "/v1internal:fetchAvailableModels")
			require.NotContains(t, requests[other], "/v1internal:retrieveUserQuotaSummary")
			require.InDelta(t, used, result.UsageInfo.FiveHour.Utilization, 0.001)
		})
	}
}

func TestFetchQuota_SelectedEndpointFailureDoesNotReadOtherQuota(t *testing.T) {
	for _, failedPath := range []string{"/v1internal:fetchAvailableModels", "/v1internal:retrieveUserQuotaSummary"} {
		for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("%s/%d", failedPath, status), func(t *testing.T) {
				t.Setenv(antigravityForwardBaseURLEnv, "")
				var otherQuotaCalls atomic.Int32
				prod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/v1internal:loadCodeAssist" {
						otherQuotaCalls.Add(1)
					}
					_, _ = w.Write([]byte(`{"models":{},"groups":[{"buckets":[{"bucketId":"gemini-5h","window":"5h","remainingFraction":1}]}]}`))
				}))
				defer prod.Close()
				daily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == failedPath {
						w.WriteHeader(status)
						return
					}
					_, _ = w.Write([]byte(`{"models":{"gemini-4-argon-high":{"quotaInfo":{"remainingFraction":0.62}}}}`))
				}))
				defer daily.Close()
				old := antigravity.BaseURLs
				t.Cleanup(func() { antigravity.BaseURLs = old })
				antigravity.BaseURLs = []string{prod.URL, daily.URL}
				result, err := NewAntigravityQuotaFetcher(nil, &config.Config{}).FetchQuota(context.Background(), &Account{
					Credentials: map[string]any{"access_token": "token", "plan_type": "pro"},
				}, "")
				require.Zero(t, otherQuotaCalls.Load())
				if failedPath == "/v1internal:fetchAvailableModels" {
					if status == http.StatusForbidden {
						require.NoError(t, err)
						require.True(t, result.UsageInfo.IsForbidden)
					} else {
						require.ErrorContains(t, err, fmt.Sprint(status))
						require.Nil(t, result)
					}
				} else {
					require.NoError(t, err)
					require.NotNil(t, result.UsageInfo.AntigravityQuota["gemini-4-argon-high"])
					require.Nil(t, result.UsageInfo.AntigravityQuota["gemini-5h"])
				}
			})
		}
	}
}
