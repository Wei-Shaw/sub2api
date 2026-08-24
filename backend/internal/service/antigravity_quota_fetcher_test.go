//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
REDACTED{
		{name: "empty string", raw: "", expected: ""REDACTED,
		{name: "free-tier", raw: "free-tier", expected: "FREE"REDACTED,
		{name: "g1-pro-tier", raw: "g1-pro-tier", expected: "PRO"REDACTED,
		{name: "g1-ultra-tier", raw: "g1-ultra-tier", expected: "ULTRA"REDACTED,
		{name: "unknown-something", raw: "unknown-something", expected: "UNKNOWN"REDACTED,
		{name: "Google AI Pro contains pro keyword", raw: "Google AI Pro", expected: "PRO"REDACTED,
		{name: "case insensitive FREE", raw: "FREE-TIER", expected: "FREE"REDACTED,
		{name: "case insensitive Ultra", raw: "Ultra Plan", expected: "ULTRA"REDACTED,
		{name: "arbitrary unrecognized string", raw: "enterprise-custom", expected: "UNKNOWN"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTier(tt.raw)
			require.Equal(t, tt.expected, got, "normalizeTier(%q)", tt.raw)
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// buildUsageInfo
// ---------------------------------------------------------------------------

func aqfBoolPtr(v bool) *bool { return &v REDACTED
func aqfIntPtr(v int) *int    { return &v REDACTED

func TestBuildUsageInfo_BasicModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.75,
					ResetTime:         "2026-03-08T12:00:00Z",
			REDACTED,
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
			REDACTED,
		REDACTED,
			"gemini-2.5-pro": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.50,
					ResetTime:         "2026-03-08T15:00:00Z",
			REDACTED,
				DisplayName:     "Gemini 2.5 Pro",
				MaxTokens:       aqfIntPtr(1000000),
				MaxOutputTokens: aqfIntPtr(65536),
		REDACTED,
	REDACTED,
REDACTED

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
	require.Equal(t, map[string]bool{"image/png": true, "image/jpeg": trueREDACTED, sonnetDetail.SupportedMimeTypes)

	geminiDetail := info.AntigravityQuotaDetails["gemini-2.5-pro"]
	require.NotNil(t, geminiDetail)
	require.Equal(t, "Gemini 2.5 Pro", geminiDetail.DisplayName)
	require.Nil(t, geminiDetail.SupportsImages)
	require.Nil(t, geminiDetail.SupportsThinking)
	require.Equal(t, aqfIntPtr(1000000), geminiDetail.MaxTokens)
	require.Equal(t, aqfIntPtr(65536), geminiDetail.MaxOutputTokens)
REDACTED

func TestBuildUsageInfo_DeprecatedModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 1.0,
			REDACTED,
		REDACTED,
	REDACTED,
		DeprecatedModelIDs: map[string]antigravity.DeprecatedModelInfo{
			"claude-3-sonnet-20240229": {NewModelID: "claude-sonnet-4-20250514"REDACTED,
			"claude-3-haiku-20240307":  {NewModelID: "claude-haiku-3.5-latest"REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.Len(t, info.ModelForwardingRules, 2)
	require.Equal(t, "claude-sonnet-4-20250514", info.ModelForwardingRules["claude-3-sonnet-20240229"])
	require.Equal(t, "claude-haiku-3.5-latest", info.ModelForwardingRules["claude-3-haiku-20240307"])
REDACTED

func TestBuildUsageInfo_NoDeprecatedModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"some-model": {
				QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 0.9REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.Nil(t, info.ModelForwardingRules, "ModelForwardingRules should be nil when no deprecated models")
REDACTED

func TestBuildUsageInfo_EmptyModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info)
	require.NotNil(t, info.AntigravityQuota)
	require.Empty(t, info.AntigravityQuota)
	require.NotNil(t, info.AntigravityQuotaDetails)
	require.Empty(t, info.AntigravityQuotaDetails)
	require.Nil(t, info.FiveHour, "FiveHour should be nil when no priority model exists")
REDACTED

func TestBuildUsageInfo_ModelWithNilQuotaInfo(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"model-without-quota": {
				DisplayName: "No Quota Model",
				// QuotaInfo is nil
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info)
	require.Empty(t, info.AntigravityQuota, "models with nil QuotaInfo should be skipped")
	require.Empty(t, info.AntigravityQuotaDetails, "models with nil QuotaInfo should be skipped from details too")
REDACTED

func TestBuildUsageInfo_FiveHourPriorityOrder(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	// priorityModels = ["claude-sonnet-4-20250514", "claude-sonnet-4", "gemini-2.5-pro"]
	// When the first priority model exists, it should be used for FiveHour
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"gemini-2.5-pro": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.40,
					ResetTime:         "2026-03-08T18:00:00Z",
			REDACTED,
		REDACTED,
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.80,
					ResetTime:         "2026-03-08T12:00:00Z",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info.FiveHour, "FiveHour should be set when a priority model exists")
	// claude-sonnet-4-20250514 is first in priority list, so it should be used
	expectedUtilization := (1.0 - 0.80) * 100 // 20
	require.InDelta(t, expectedUtilization, info.FiveHour.Utilization, 0.01)
	require.NotNil(t, info.FiveHour.ResetsAt, "ResetsAt should be parsed from ResetTime")
REDACTED

func TestBuildUsageInfo_FiveHourFallbackToClaude4(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	// Only claude-sonnet-4 exists (second in priority list), not claude-sonnet-4-20250514
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.60,
					ResetTime:         "2026-03-08T14:00:00Z",
			REDACTED,
		REDACTED,
			"gemini-2.5-pro": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.30,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info.FiveHour)
	expectedUtilization := (1.0 - 0.60) * 100 // 40
	require.InDelta(t, expectedUtilization, info.FiveHour.Utilization, 0.01)
REDACTED

func TestBuildUsageInfo_FiveHourFallbackToGemini(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	// Only gemini-2.5-pro exists (third in priority list)
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"gemini-2.5-pro": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.30,
			REDACTED,
		REDACTED,
			"other-model": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.90,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info.FiveHour)
	expectedUtilization := (1.0 - 0.30) * 100 // 70
	require.InDelta(t, expectedUtilization, info.FiveHour.Utilization, 0.01)
REDACTED

func TestBuildUsageInfo_FiveHourNoPriorityModel(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	// None of the priority models exist
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"some-other-model": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.50,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.Nil(t, info.FiveHour, "FiveHour should be nil when no priority model exists")
REDACTED

func TestBuildUsageInfo_FiveHourWithEmptyResetTime(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.50,
					ResetTime:         "", // empty reset time
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	require.NotNil(t, info.FiveHour)
	require.Nil(t, info.FiveHour.ResetsAt, "ResetsAt should be nil when ResetTime is empty")
	require.Equal(t, 0, info.FiveHour.RemainingSeconds)
REDACTED

func TestBuildUsageInfo_FullUtilization(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 0.0, // fully used
					ResetTime:         "2026-03-08T12:00:00Z",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)

	quota := info.AntigravityQuota["claude-sonnet-4-20250514"]
	require.NotNil(t, quota)
	require.Equal(t, 100, quota.Utilization)
REDACTED

func TestBuildUsageInfo_ZeroUtilization(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"claude-sonnet-4-20250514": {
				QuotaInfo: &antigravity.ModelQuotaInfo{
					RemainingFraction: 1.0, // fully available
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "", "", nil)
	quota := info.AntigravityQuota["claude-sonnet-4-20250514"]
	require.NotNil(t, quota)
	require.Equal(t, 0, quota.Utilization)
REDACTED

func TestBuildUsageInfo_AICredits(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{REDACTED
	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{REDACTED,
REDACTED
	loadResp := &antigravity.LoadCodeAssistResponse{
		PaidTier: &antigravity.PaidTierInfo{
			ID: "g1-pro-tier",
			AvailableCredits: []antigravity.AvailableCredit{
				{
					CreditType:                  "GOOGLE_ONE_AI",
					CreditAmount:                "25",
					MinimumCreditAmountForUsage: "5",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	info := fetcher.buildUsageInfo(modelsResp, "g1-pro-tier", "PRO", loadResp)

	require.Len(t, info.AICredits, 1)
	require.Equal(t, "GOOGLE_ONE_AI", info.AICredits[0].CreditType)
	require.Equal(t, 25.0, info.AICredits[0].Amount)
	require.Equal(t, 5.0, info.AICredits[0].MinimumBalance)
REDACTED

func TestFetchQuotaUsesConfiguredModelsListBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"model-a":{REDACTEDREDACTEDREDACTED`))
REDACTED))
	defer server.Close()

	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	t.Cleanup(func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
REDACTED)
	antigravity.BaseURLs = []string{server.URLREDACTED
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	cfg := &config.Config{REDACTED
	cfg.Gateway.ModelsListReadMaxBytes = 8
	fetcher := NewAntigravityQuotaFetcher(nil, cfg)
	_, err := fetcher.FetchQuota(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "token",
			"project_id":   "project",
	REDACTED,
REDACTED, "")
	require.ErrorContains(t, err, "响应超过 8 字节")
REDACTED

func TestFetchQuota_ForbiddenReturnsIsForbidden(t *testing.T) {
	// 模拟 FetchQuota 遇到 403 时的行为：
	// FetchAvailableModels 返回 ForbiddenError → FetchQuota 应返回 is_forbidden=true
	forbiddenErr := &antigravity.ForbiddenError{
		StatusCode: 403,
		Body:       "Access denied",
REDACTED

	// 验证 ForbiddenError 满足 errors.As
	var target *antigravity.ForbiddenError
	require.True(t, errors.As(forbiddenErr, &target))
	require.Equal(t, 403, target.StatusCode)
	require.Equal(t, "Access denied", target.Body)
	require.Contains(t, forbiddenErr.Error(), "403")
REDACTED

// ---------------------------------------------------------------------------
// classifyForbiddenType
// ---------------------------------------------------------------------------

func TestClassifyForbiddenType(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
REDACTED{
		{
			name:     "VALIDATION_REQUIRED keyword",
			body:     `{"error":{"message":"VALIDATION_REQUIRED"REDACTEDREDACTED`,
			expected: "validation",
	REDACTED,
		{
			name:     "verify your account",
			body:     `Please verify your account to continue`,
			expected: "validation",
	REDACTED,
		{
			name:     "contains validation_url field",
			body:     `{"error":{"details":[{"metadata":{"validation_url":"https://..."REDACTEDREDACTED]REDACTEDREDACTED`,
			expected: "validation",
	REDACTED,
		{
			name:     "terms of service violation",
			body:     `Your account has been suspended for Terms of Service violation`,
			expected: "violation",
	REDACTED,
		{
			name:     "violation keyword",
			body:     `Account suspended due to policy violation`,
			expected: "violation",
	REDACTED,
		{
			name:     "generic 403",
			body:     `Access denied`,
			expected: "forbidden",
	REDACTED,
		{
			name:     "empty body",
			body:     "",
			expected: "forbidden",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyForbiddenType(tt.body)
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// extractValidationURL
// ---------------------------------------------------------------------------

func TestExtractValidationURL(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
REDACTED{
		{
			name:     "structured validation_url",
			body:     `{"error":{"details":[{"metadata":{"validation_url":"https://accounts.google.com/verify?token=abc"REDACTEDREDACTED]REDACTEDREDACTED`,
			expected: "https://accounts.google.com/verify?token=abc",
	REDACTED,
		{
			name:     "structured appeal_url",
			body:     `{"error":{"details":[{"metadata":{"appeal_url":"https://support.google.com/appeal/123"REDACTEDREDACTED]REDACTEDREDACTED`,
			expected: "https://support.google.com/appeal/123",
	REDACTED,
		{
			name:     "validation_url takes priority over appeal_url",
			body:     `{"error":{"details":[{"metadata":{"validation_url":"https://v.com","appeal_url":"https://a.com"REDACTEDREDACTED]REDACTEDREDACTED`,
			expected: "https://v.com",
	REDACTED,
		{
			name:     "fallback regex with verify keyword",
			body:     `Please verify your account at https://accounts.google.com/verify`,
			expected: "https://accounts.google.com/verify",
	REDACTED,
		{
			name:     "no URL in generic forbidden",
			body:     `Access denied`,
			expected: "",
	REDACTED,
		{
			name:     "empty body",
			body:     "",
			expected: "",
	REDACTED,
		{
			name:     "URL present but no validation keywords",
			body:     `Error at https://example.com/something`,
			expected: "",
	REDACTED,
		{
			name:     "unicode escaped ampersand",
			body:     `validation required: https://accounts.google.com/verify?a=1\u0026b=2`,
			expected: "https://accounts.google.com/verify?a=1&b=2",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractValidationURL(tt.body)
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED
