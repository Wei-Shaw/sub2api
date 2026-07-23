package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestDetectModelPlatform(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		platform string
		ok       bool
REDACTED{
		{name: "claude", model: "claude-sonnet-4-5", platform: PlatformAnthropic, ok: trueREDACTED,
		{name: "anthropic prefix", model: "anthropic/claude-opus-4-5", platform: PlatformAnthropic, ok: trueREDACTED,
		{name: "gpt", model: "gpt-5.1", platform: PlatformOpenAI, ok: trueREDACTED,
		{name: "o series", model: "o3-mini", platform: PlatformOpenAI, ok: trueREDACTED,
		{name: "embedding", model: "text-embedding-3-large", platform: PlatformOpenAI, ok: trueREDACTED,
		{name: "gemini", model: "gemini-3-pro", platform: PlatformGemini, ok: trueREDACTED,
		{name: "gemini models prefix", model: "models/gemini-2.5-flash", platform: PlatformGemini, ok: trueREDACTED,
		{name: "learnlm", model: "learnlm-2.0-flash-experimental", platform: PlatformGemini, ok: trueREDACTED,
		{name: "grok", model: "grok-4", platform: PlatformGrok, ok: trueREDACTED,
		{name: "xai prefix", model: "xai/grok-4", platform: PlatformGrok, ok: trueREDACTED,
		{name: "unknown", model: "llama-4-maverick", ok: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform, ok := DetectModelPlatform(tt.model)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.platform, platform)
	REDACTED)
REDACTED
REDACTED

func TestQuotaPlatformCompositeUsesResolvedOrForceOnly(t *testing.T) {
	apiKey := &APIKey{Group: &Group{Platform: PlatformCompositeREDACTEDREDACTED

	require.Equal(t, "", QuotaPlatform(context.Background(), apiKey))
	require.Equal(t, PlatformGemini, QuotaPlatform(WithResolvedTargetPlatform(context.Background(), PlatformGemini), apiKey))
	require.Equal(t, PlatformAntigravity, QuotaPlatform(context.WithValue(context.Background(), ctxkey.ForcePlatform, PlatformAntigravity), apiKey))

	ctx := WithResolvedTargetPlatform(context.Background(), PlatformAnthropic)
	ctx = context.WithValue(ctx, ctxkey.ForcePlatform, PlatformAntigravity)
	require.Equal(t, PlatformAntigravity, QuotaPlatform(ctx, apiKey))
REDACTED

func TestCompositeGroupSchedulerHasAllCanonicalPlatformBuckets(t *testing.T) {
	seen := make(map[string]struct{REDACTED)
	for _, bucket := range schedulerCanonicalBuckets(99) {
		seen[bucket.Platform] = struct{REDACTED{REDACTED
REDACTED
	platforms := make([]string, 0, len(seen))
	for platform := range seen {
		platforms = append(platforms, platform)
REDACTED
	require.ElementsMatch(t,
		[]string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrokREDACTED,
		platforms,
	)
REDACTED
