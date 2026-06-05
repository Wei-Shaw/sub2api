package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestIsModelRateLimited(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * time.Minute).Format(time.RFC3339)
	past := now.Add(-10 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		expected       bool
REDACTED{
		{
			name: "official model ID hit - claude-sonnet-4-5",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       true,
	REDACTED,
		{
			name: "official model ID hit via mapping - request claude-3-5-sonnet, mapped to claude-sonnet-4-5",
			account: &Account{
		REDACTED
					"model_mapping": map[string]any{
						"claude-3-5-sonnet": "claude-sonnet-4-5",
				REDACTED,
			REDACTED,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-3-5-sonnet",
			expected:       true,
	REDACTED,
		{
			name: "no rate limit - expired",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": past,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       false,
	REDACTED,
		{
			name: "no rate limit - no matching key",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-flash": map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       false,
	REDACTED,
		{
			name:           "no rate limit - unsupported model",
			account:        &Account{REDACTED,
			requestedModel: "gpt-4",
			expected:       false,
	REDACTED,
		{
			name:           "no rate limit - empty model",
			account:        &Account{REDACTED,
			requestedModel: "",
			expected:       false,
	REDACTED,
		{
			name: "gemini model hit",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-pro-high": map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3-pro-high",
			expected:       true,
	REDACTED,
		{
			name: "antigravity platform - gemini-3-pro-preview mapped to gemini-3-pro-high",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-pro-high": map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3-pro-preview",
			expected:       true,
	REDACTED,
		{
			name: "antigravity platform - gemini family rate limit blocks mapped preview",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						antigravityGeminiModelRateLimitKey: map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3-pro-preview",
			expected:       true,
	REDACTED,
		{
			name: "antigravity platform - gemini family rate limit does not block claude",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						antigravityGeminiModelRateLimitKey: map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			expected:       false,
	REDACTED,
		{
			name: "non-antigravity platform - gemini-3-pro-preview NOT mapped",
			account: &Account{
		REDACTED
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-pro-high": map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3-pro-preview",
			expected:       false, // gemini 平台不走 antigravity 映射
	REDACTED,
		{
			name: "antigravity platform - claude-opus-4-5-thinking mapped to opus-4-6-thinking",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-opus-4-6-thinking": map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-opus-4-5-thinking",
			expected:       true,
	REDACTED,
		{
			name: "no scope fallback - claude_sonnet should not match",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude_sonnet": map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-3-5-sonnet-20241022",
			expected:       false,
	REDACTED,
		{
			name: "openai image generation family key blocks image model",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						openAIImageGenerationRateLimitKey: map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "gpt-image-2",
			expected:       true,
	REDACTED,
		{
			name: "openai image generation family key does not block text model",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						openAIImageGenerationRateLimitKey: map[string]any{
							"rate_limit_reset_at": future,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "gpt-5.4",
			expected:       false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.isModelRateLimitedWithContext(context.Background(), tt.requestedModel)
			if result != tt.expected {
				t.Errorf("isModelRateLimited(%q) = %v, want %v", tt.requestedModel, result, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestIsModelRateLimited_OpenAIImageGenerationIntentBlocksTextModelImageTool(t *testing.T) {
	future := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	account := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				openAIImageGenerationRateLimitKey: map[string]any{
					"rate_limit_reset_at": future,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	require.False(t, account.isModelRateLimitedWithContext(context.Background(), "gpt-5.4"))
	require.True(t, account.isModelRateLimitedWithContext(WithOpenAIImageGenerationIntent(context.Background()), "gpt-5.4"))
REDACTED

func TestIsModelRateLimited_Antigravity_ThinkingAffectsModelKey(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * time.Minute).Format(time.RFC3339)

	account := &Account{
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5-thinking": map[string]any{
					"rate_limit_reset_at": future,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	ctx := context.WithValue(context.Background(), ctxkey.ThinkingEnabled, true)
	if !account.isModelRateLimitedWithContext(ctx, "claude-sonnet-4-5") {
		t.Errorf("expected model to be rate limited")
REDACTED
REDACTED

func TestGetModelRateLimitRemainingTime(t *testing.T) {
	now := time.Now()
	future10m := now.Add(10 * time.Minute).Format(time.RFC3339)
	future5m := now.Add(5 * time.Minute).Format(time.RFC3339)
	past := now.Add(-10 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		minExpected    time.Duration
		maxExpected    time.Duration
REDACTED{
		{
			name:           "nil account",
			account:        nil,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
	REDACTED,
		{
			name: "model rate limited - direct hit",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future10m,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    9 * time.Minute,
			maxExpected:    11 * time.Minute,
	REDACTED,
		{
			name: "model rate limited - via mapping",
			account: &Account{
		REDACTED
					"model_mapping": map[string]any{
						"claude-3-5-sonnet": "claude-sonnet-4-5",
				REDACTED,
			REDACTED,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future5m,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-3-5-sonnet",
			minExpected:    4 * time.Minute,
			maxExpected:    6 * time.Minute,
	REDACTED,
		{
			name: "expired rate limit",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": past,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
	REDACTED,
		{
			name:           "no rate limit data",
			account:        &Account{REDACTED,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
	REDACTED,
		{
			name: "no scope fallback",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude_sonnet": map[string]any{
							"rate_limit_reset_at": future5m,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-3-5-sonnet-20241022",
			minExpected:    0,
			maxExpected:    0,
	REDACTED,
		{
			name: "antigravity platform - claude-opus-4-5-thinking mapped to opus-4-6-thinking",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-opus-4-6-thinking": map[string]any{
							"rate_limit_reset_at": future5m,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-opus-4-5-thinking",
			minExpected:    4 * time.Minute,
			maxExpected:    6 * time.Minute,
	REDACTED,
		{
			name: "antigravity platform - gemini family rate limit remaining",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						antigravityGeminiModelRateLimitKey: map[string]any{
							"rate_limit_reset_at": future10m,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "gemini-3-pro-preview",
			minExpected:    9 * time.Minute,
			maxExpected:    11 * time.Minute,
	REDACTED,
		{
			name: "antigravity platform - gemini family remaining ignored for claude",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						antigravityGeminiModelRateLimitKey: map[string]any{
							"rate_limit_reset_at": future10m,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetModelRateLimitRemainingTimeWithContext(context.Background(), tt.requestedModel)
			if result < tt.minExpected || result > tt.maxExpected {
				t.Errorf("GetModelRateLimitRemainingTime() = %v, want between %v and %v", result, tt.minExpected, tt.maxExpected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGetRateLimitRemainingTime(t *testing.T) {
	now := time.Now()
	future15m := now.Add(15 * time.Minute).Format(time.RFC3339)
	future5m := now.Add(5 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		minExpected    time.Duration
		maxExpected    time.Duration
REDACTED{
		{
			name:           "nil account",
			account:        nil,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
	REDACTED,
		{
			name: "model rate limited - 15 minutes",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future15m,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    14 * time.Minute,
			maxExpected:    16 * time.Minute,
	REDACTED,
		{
			name: "only model rate limited",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future5m,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    4 * time.Minute,
			maxExpected:    6 * time.Minute,
	REDACTED,
		{
			name: "neither rate limited",
			account: &Account{
				Platform: PlatformAntigravity,
		REDACTED,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetRateLimitRemainingTimeWithContext(context.Background(), tt.requestedModel)
			if result < tt.minExpected || result > tt.maxExpected {
				t.Errorf("GetRateLimitRemainingTime() = %v, want between %v and %v", result, tt.minExpected, tt.maxExpected)
		REDACTED
	REDACTED)
REDACTED
REDACTED
