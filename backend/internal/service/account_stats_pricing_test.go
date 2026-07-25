//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// matchAccountStatsRule
// ---------------------------------------------------------------------------

func TestMatchAccountStatsRule_BothEmpty_NoMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{REDACTED
	require.False(t, matchAccountStatsRule(rule, 1, 10))
REDACTED

func TestMatchAccountStatsRule_AccountIDMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{AccountIDs: []int64{1, 2, 3REDACTEDREDACTED
	require.True(t, matchAccountStatsRule(rule, 2, 999))
REDACTED

func TestMatchAccountStatsRule_GroupIDMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{GroupIDs: []int64{10, 20REDACTEDREDACTED
	require.True(t, matchAccountStatsRule(rule, 999, 20))
REDACTED

func TestMatchAccountStatsRule_BothConfigured_AccountMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{
		AccountIDs: []int64{1, 2REDACTED,
		GroupIDs:   []int64{10, 20REDACTED,
REDACTED
	require.True(t, matchAccountStatsRule(rule, 2, 999))
REDACTED

func TestMatchAccountStatsRule_BothConfigured_GroupMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{
		AccountIDs: []int64{1, 2REDACTED,
		GroupIDs:   []int64{10, 20REDACTED,
REDACTED
	require.True(t, matchAccountStatsRule(rule, 999, 10))
REDACTED

func TestMatchAccountStatsRule_BothConfigured_NeitherMatch(t *testing.T) {
	rule := &AccountStatsPricingRule{
		AccountIDs: []int64{1, 2REDACTED,
		GroupIDs:   []int64{10, 20REDACTED,
REDACTED
	require.False(t, matchAccountStatsRule(rule, 999, 999))
REDACTED

// ---------------------------------------------------------------------------
// findPricingForModel
// ---------------------------------------------------------------------------

func TestFindPricingForModel(t *testing.T) {
	exactPricing := ChannelModelPricing{
		ID:     1,
		Models: []string{"claude-opus-4"REDACTED,
REDACTED
	wildcardPricing := ChannelModelPricing{
		ID:     2,
		Models: []string{"claude-*"REDACTED,
REDACTED
	platformPricing := ChannelModelPricing{
		ID:       3,
		Platform: "openai",
		Models:   []string{"gpt-4o"REDACTED,
REDACTED
	emptyPlatformPricing := ChannelModelPricing{
		ID:     4,
		Models: []string{"gemini-2.5-pro"REDACTED,
REDACTED

	tests := []struct {
		name     string
		list     []ChannelModelPricing
		platform string
		model    string
		wantID   int64
		wantNil  bool
REDACTED{
		{
			name:     "exact match",
			list:     []ChannelModelPricing{exactPricingREDACTED,
			platform: "anthropic",
			model:    "claude-opus-4",
			wantID:   1,
	REDACTED,
		{
			name:     "exact match case insensitive",
			list:     []ChannelModelPricing{{ID: 5, Models: []string{"Claude-Opus-4"REDACTEDREDACTEDREDACTED,
			platform: "",
			model:    "claude-opus-4",
			wantID:   5,
	REDACTED,
		{
			name:     "wildcard match",
			list:     []ChannelModelPricing{wildcardPricingREDACTED,
			platform: "anthropic",
			model:    "claude-opus-4",
			wantID:   2,
	REDACTED,
		{
			name:     "exact match takes priority over wildcard",
			list:     []ChannelModelPricing{wildcardPricing, exactPricingREDACTED,
			platform: "anthropic",
			model:    "claude-opus-4",
			wantID:   1,
	REDACTED,
		{
			name:     "platform mismatch skipped",
			list:     []ChannelModelPricing{platformPricingREDACTED,
			platform: "anthropic",
			model:    "gpt-4o",
			wantNil:  true,
	REDACTED,
		{
			name:     "empty platform in pricing matches any",
			list:     []ChannelModelPricing{emptyPlatformPricingREDACTED,
			platform: "gemini",
			model:    "gemini-2.5-pro",
			wantID:   4,
	REDACTED,
		{
			name:     "empty platform in query matches any pricing platform",
			list:     []ChannelModelPricing{platformPricingREDACTED,
			platform: "",
			model:    "gpt-4o",
			wantID:   3,
	REDACTED,
		{
			name:     "no match at all",
			list:     []ChannelModelPricing{exactPricing, wildcardPricingREDACTED,
			platform: "anthropic",
			model:    "gpt-4o",
			wantNil:  true,
	REDACTED,
		{
			name:    "empty list returns nil",
			list:    nil,
			model:   "claude-opus-4",
			wantNil: true,
	REDACTED,
		{
			name: "wildcard matches by config order (first match wins)",
			list: []ChannelModelPricing{
				{ID: 10, Models: []string{"claude-*"REDACTEDREDACTED,
				{ID: 11, Models: []string{"claude-opus-*"REDACTEDREDACTED,
		REDACTED,
			platform: "",
			model:    "claude-opus-4",
			wantID:   10, // config order: "claude-*" is first and matches, so it wins
	REDACTED,
		{
			name: "shorter wildcard used when longer does not match",
			list: []ChannelModelPricing{
				{ID: 10, Models: []string{"claude-*"REDACTEDREDACTED,
				{ID: 11, Models: []string{"claude-opus-*"REDACTEDREDACTED,
		REDACTED,
			platform: "",
			model:    "claude-sonnet-4",
			wantID:   10, // only "claude-*" matches
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findPricingForModel(tt.list, tt.platform, tt.model)
			if tt.wantNil {
				require.Nil(t, result)
				return
		REDACTED
			require.NotNil(t, result)
			require.Equal(t, tt.wantID, result.ID)
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// calculateStatsCost
// ---------------------------------------------------------------------------

func TestCalculateStatsCost_NilPricing(t *testing.T) {
	result := calculateStatsCost(nil, UsageTokens{REDACTED, 1)
	require.Nil(t, result)
REDACTED

func TestCalculateStatsCost_TokenBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(0.001),
		OutputPrice: testPtrFloat64(0.002),
REDACTED
	tokens := UsageTokens{
		InputTokens:  100,
		OutputTokens: 50,
REDACTED
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 = 0.1 + 0.1 = 0.2
	require.InDelta(t, 0.2, *result, 1e-12)
REDACTED

func TestCalculateStatsCost_TokenBilling_WithCache(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModeToken,
		InputPrice:      testPtrFloat64(0.001),
		OutputPrice:     testPtrFloat64(0.002),
		CacheWritePrice: testPtrFloat64(0.003),
		CacheReadPrice:  testPtrFloat64(0.0005),
REDACTED
	tokens := UsageTokens{
		InputTokens:         100,
		OutputTokens:        50,
		CacheCreationTokens: 200,
		CacheReadTokens:     300,
REDACTED
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 + 200*0.003 + 300*0.0005
	// = 0.1 + 0.1 + 0.6 + 0.15 = 0.95
	require.InDelta(t, 0.95, *result, 1e-12)
REDACTED

func TestCalculateStatsCost_TokenBilling_WithImageOutput(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:      BillingModeToken,
		InputPrice:       testPtrFloat64(0.001),
		OutputPrice:      testPtrFloat64(0.002),
		ImageOutputPrice: testPtrFloat64(0.01),
REDACTED
	tokens := UsageTokens{
		InputTokens:       100,
		OutputTokens:      50,
		ImageOutputTokens: 10,
REDACTED
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 + 10*0.01 = 0.1 + 0.1 + 0.1 = 0.3
	require.InDelta(t, 0.3, *result, 1e-12)
REDACTED

func TestCalculateStatsCost_TokenBilling_PartialPricesNil(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(0.001),
		// OutputPrice, CacheWritePrice, etc. are all nil → treated as 0
REDACTED
	tokens := UsageTokens{
		InputTokens:         100,
		OutputTokens:        50,
		CacheCreationTokens: 200,
REDACTED
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	// Only input contributes: 100*0.001 = 0.1
	require.InDelta(t, 0.1, *result, 1e-12)
REDACTED

func TestCalculateStatsCost_TokenBilling_AllTokensZero(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(0.001),
		OutputPrice: testPtrFloat64(0.002),
REDACTED
	tokens := UsageTokens{REDACTED // all zeros
	result := calculateStatsCost(pricing, tokens, 1)
	// totalCost == 0 → returns nil (does not override, falls back to default formula)
	require.Nil(t, result)
REDACTED

func TestCalculateStatsCost_PerRequestBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: testPtrFloat64(0.05),
REDACTED
	tokens := UsageTokens{InputTokens: 999, OutputTokens: 999REDACTED
	result := calculateStatsCost(pricing, tokens, 3)
	require.NotNil(t, result)
	// 0.05 * 3 = 0.15
	require.InDelta(t, 0.15, *result, 1e-12)
REDACTED

func TestCalculateStatsCost_PerRequestBilling_PriceNil(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModePerRequest,
		// PerRequestPrice is nil
REDACTED
	result := calculateStatsCost(pricing, UsageTokens{REDACTED, 1)
	require.Nil(t, result)
REDACTED

func TestCalculateStatsCost_PerRequestBilling_PriceZero(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: testPtrFloat64(0),
REDACTED
	result := calculateStatsCost(pricing, UsageTokens{REDACTED, 1)
	// price == 0 → condition *pricing.PerRequestPrice > 0 is false → returns nil
	require.Nil(t, result)
REDACTED

func TestCalculateStatsCost_ImageBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModeImage,
		PerRequestPrice: testPtrFloat64(0.10),
REDACTED
	result := calculateStatsCost(pricing, UsageTokens{REDACTED, 2)
	require.NotNil(t, result)
	// 0.10 * 2 = 0.20
	require.InDelta(t, 0.20, *result, 1e-12)
REDACTED

func TestCalculateStatsCost_ImageBilling_PriceNil(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeImage,
		// PerRequestPrice is nil
REDACTED
	result := calculateStatsCost(pricing, UsageTokens{REDACTED, 1)
	require.Nil(t, result)
REDACTED

func TestCalculateStatsCost_DefaultBillingMode_FallsToToken(t *testing.T) {
	// BillingMode is empty string (default) → falls into token billing
	pricing := &ChannelModelPricing{
		InputPrice:  testPtrFloat64(0.001),
		OutputPrice: testPtrFloat64(0.002),
REDACTED
	tokens := UsageTokens{
		InputTokens:  100,
		OutputTokens: 50,
REDACTED
	result := calculateStatsCost(pricing, tokens, 1)
	require.NotNil(t, result)
	require.InDelta(t, 0.2, *result, 1e-12)
REDACTED

// ---------------------------------------------------------------------------
// tryCustomRules — 多规则顺序测试
// ---------------------------------------------------------------------------

func TestTryCustomRules_FirstMatchWins(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				GroupIDs: []int64{1REDACTED,
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(0.01), OutputPrice: testPtrFloat64(0.02)REDACTED,
			REDACTED,
		REDACTED,
			{
				GroupIDs: []int64{1REDACTED,
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(0.99), OutputPrice: testPtrFloat64(0.99)REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED
	result := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	require.NotNil(t, result)
	// 应使用第一条规则的价格：100*0.01 + 50*0.02 = 2.0
	require.InDelta(t, 2.0, *result, 1e-12)
REDACTED

func TestTryCustomRules_SkipsNonMatchingRules(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				AccountIDs: []int64{888REDACTED, // 不匹配
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(0.99)REDACTED,
			REDACTED,
		REDACTED,
			{
				GroupIDs: []int64{1REDACTED, // 匹配
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(0.05)REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	tokens := UsageTokens{InputTokens: 100REDACTED
	result := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	require.NotNil(t, result)
	// 跳过规则1（账号不匹配），使用规则2：100*0.05 = 5.0
	require.InDelta(t, 5.0, *result, 1e-12)
REDACTED

func TestTryCustomRules_NoMatch_ReturnsNil(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				AccountIDs: []int64{888REDACTED,
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(0.01)REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	tokens := UsageTokens{InputTokens: 100REDACTED
	result := tryCustomRules(channel, 999, 2, "", "claude-opus-4", tokens, 1)
	require.Nil(t, result) // 账号和分组都不匹配
REDACTED

func TestTryCustomRules_RuleMatchesButModelNot_ContinuesToNext(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				GroupIDs: []int64{1REDACTED,
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"gpt-4o"REDACTED, InputPrice: testPtrFloat64(0.01)REDACTED, // 模型不匹配
			REDACTED,
		REDACTED,
			{
				GroupIDs: []int64{1REDACTED,
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"REDACTED, InputPrice: testPtrFloat64(0.05)REDACTED, // 模型匹配
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	tokens := UsageTokens{InputTokens: 100REDACTED
	result := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	require.NotNil(t, result)
	require.InDelta(t, 5.0, *result, 1e-12) // 使用规则2
REDACTED

// ---------------------------------------------------------------------------
// tryModelFilePricing
// ---------------------------------------------------------------------------

// newTestBillingServiceWithPrices creates a BillingService with pre-populated
// fallback prices for testing. No config or pricing service is needed.
// The key must match what getFallbackPricing resolves to for a given model name.
// E.g., model "claude-sonnet-4" resolves to key "claude-sonnet-4".
func newTestBillingServiceWithPrices(prices map[string]*ModelPricing) *BillingService {
	return &BillingService{
		fallbackPrices: prices,
REDACTED
REDACTED

func TestTryModelFilePricing_Success(t *testing.T) {
	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{
		"claude-sonnet-4": {
			InputPricePerToken:  0.001,
			OutputPricePerToken: 0.002,
	REDACTED,
REDACTED)
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED
	result := tryModelFilePricing(bs, "claude-sonnet-4", tokens)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 = 0.1 + 0.1 = 0.2
	require.InDelta(t, 0.2, *result, 1e-12)
REDACTED

func TestTryModelFilePricing_AppliesLongContextPricing(t *testing.T) {
	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{
		"gpt-5.6-sol": {
			InputPricePerToken:          0.001,
			OutputPricePerToken:         0.002,
			CacheReadPricePerToken:      0.0001,
			LongContextInputThreshold:   100,
			LongContextInputMultiplier:  2,
			LongContextOutputMultiplier: 1.5,
	REDACTED,
REDACTED)
	tokens := UsageTokens{InputTokens: 101, OutputTokens: 10, CacheReadTokens: 5REDACTED

	result := tryModelFilePricing(bs, "gpt-5.6-sol", tokens)

	require.NotNil(t, result)
	// Input and cache-read use the 2x input tier; output uses the 1.5x tier.
	require.InDelta(t, 0.233, *result, 1e-12)
REDACTED

func TestTryModelFilePricing_PricingNotFound(t *testing.T) {
	// "nonexistent-model" does not match any fallback pattern
	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{REDACTED)
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED
	result := tryModelFilePricing(bs, "nonexistent-model", tokens)
	require.Nil(t, result)
REDACTED

func TestTryModelFilePricing_NilFallback(t *testing.T) {
	// getFallbackPricing returns nil when key maps to nil
	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{
		"claude-sonnet-4": nil,
REDACTED)
	tokens := UsageTokens{InputTokens: 100REDACTED
	result := tryModelFilePricing(bs, "claude-sonnet-4", tokens)
	require.Nil(t, result)
REDACTED

func TestTryModelFilePricing_ZeroCost(t *testing.T) {
	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{
		"claude-sonnet-4": {
			InputPricePerToken:  0.001,
			OutputPricePerToken: 0.002,
	REDACTED,
REDACTED)
	tokens := UsageTokens{REDACTED // all zero tokens → cost = 0 → nil
	result := tryModelFilePricing(bs, "claude-sonnet-4", tokens)
	require.Nil(t, result)
REDACTED

func TestTryModelFilePricing_WithImageOutput(t *testing.T) {
	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{
		"claude-sonnet-4": {
			InputPricePerToken:       0.001,
			OutputPricePerToken:      0.002,
			ImageOutputPricePerToken: 0.01,
	REDACTED,
REDACTED)
	tokens := UsageTokens{
		InputTokens:       100,
		OutputTokens:      50,
		ImageOutputTokens: 10,
REDACTED
	result := tryModelFilePricing(bs, "claude-sonnet-4", tokens)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 + 10*0.01 = 0.1 + 0.1 + 0.1 = 0.3
	require.InDelta(t, 0.3, *result, 1e-12)
REDACTED

func TestTryModelFilePricing_WithCacheTokens(t *testing.T) {
	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{
		"claude-sonnet-4": {
			InputPricePerToken:         0.001,
			OutputPricePerToken:        0.002,
			CacheCreationPricePerToken: 0.003,
			CacheReadPricePerToken:     0.0005,
	REDACTED,
REDACTED)
	tokens := UsageTokens{
		InputTokens:         100,
		OutputTokens:        50,
		CacheCreationTokens: 200,
		CacheReadTokens:     300,
REDACTED
	result := tryModelFilePricing(bs, "claude-sonnet-4", tokens)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 + 200*0.003 + 300*0.0005
	// = 0.1 + 0.1 + 0.6 + 0.15 = 0.95
	require.InDelta(t, 0.95, *result, 1e-12)
REDACTED

// ---------------------------------------------------------------------------
// resolveAccountStatsCost — integration tests covering the 4-level priority chain
// ---------------------------------------------------------------------------

func TestResolveAccountStatsCost_NilChannelService(t *testing.T) {
	result := resolveAccountStatsCost(
		context.Background(),
		nil, // channelService is nil
		newTestBillingServiceWithPrices(map[string]*ModelPricing{REDACTED),
		1, 1, "claude-sonnet-4",
		UsageTokens{InputTokens: 100REDACTED, 1, 0.5,
	)
	require.Nil(t, result)
REDACTED

func TestResolveAccountStatsCost_EmptyUpstreamModel(t *testing.T) {
	cs := newTestChannelServiceForStats(t, &Channel{
		ID:     1,
		Status: StatusActive,
REDACTED, 1, "")

	result := resolveAccountStatsCost(
		context.Background(),
		cs,
		newTestBillingServiceWithPrices(map[string]*ModelPricing{REDACTED),
		1, 1, "", // empty upstream model
		UsageTokens{InputTokens: 100REDACTED, 1, 0.5,
	)
	require.Nil(t, result)
REDACTED

func TestResolveAccountStatsCost_GetChannelForGroupReturnsNil(t *testing.T) {
	// Group 99 is NOT in the cache, so GetChannelForGroup returns nil
	cs := newTestChannelServiceForStats(t, &Channel{
		ID:     1,
		Status: StatusActive,
REDACTED, 1, "")

	result := resolveAccountStatsCost(
		context.Background(),
		cs,
		newTestBillingServiceWithPrices(map[string]*ModelPricing{REDACTED),
		1, 99, "claude-sonnet-4", // groupID 99 has no channel
		UsageTokens{InputTokens: 100REDACTED, 1, 0.5,
	)
	require.Nil(t, result)
REDACTED

func TestResolveAccountStatsCost_HitsCustomRule(t *testing.T) {
	channel := &Channel{
		ID:     1,
		Status: StatusActive,
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				GroupIDs: []int64{10REDACTED,
				Pricing: []ChannelModelPricing{
					{
						ID:          100,
						Models:      []string{"claude-sonnet-4"REDACTED,
						InputPrice:  testPtrFloat64(0.01),
						OutputPrice: testPtrFloat64(0.02),
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	cs := newTestChannelServiceForStats(t, channel, 10, "anthropic")

	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED

	result := resolveAccountStatsCost(
		context.Background(),
		cs, nil, // billingService not needed when custom rule hits
		1, 10, "claude-sonnet-4",
		tokens, 1, 999.0, // totalCost ignored because custom rule hits
	)
	require.NotNil(t, result)
	// 100*0.01 + 50*0.02 = 1.0 + 1.0 = 2.0
	require.InDelta(t, 2.0, *result, 1e-12)
REDACTED

func TestResolveAccountStatsCost_ApplyPricingToAccountStats_UsesTotalCost(t *testing.T) {
	channel := &Channel{
		ID:                         1,
		Status:                     StatusActive,
		ApplyPricingToAccountStats: true,
		// No custom rules
REDACTED
	cs := newTestChannelServiceForStats(t, channel, 10, "anthropic")

	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED

	result := resolveAccountStatsCost(
		context.Background(),
		cs, nil,
		1, 10, "claude-sonnet-4",
		tokens, 1, 0.75, // totalCost = 0.75
	)
	require.NotNil(t, result)
	require.InDelta(t, 0.75, *result, 1e-12)
REDACTED

func TestResolveAccountStatsCost_ApplyPricingToAccountStats_ZeroTotalCost_ReturnsNil(t *testing.T) {
	channel := &Channel{
		ID:                         1,
		Status:                     StatusActive,
		ApplyPricingToAccountStats: true,
REDACTED
	cs := newTestChannelServiceForStats(t, channel, 10, "anthropic")

	result := resolveAccountStatsCost(
		context.Background(),
		cs, nil,
		1, 10, "claude-sonnet-4",
		UsageTokens{REDACTED, 1, 0.0, // totalCost = 0
	)
	require.Nil(t, result)
REDACTED

func TestResolveAccountStatsCost_FallsBackToLiteLLM(t *testing.T) {
	channel := &Channel{
		ID:                         1,
		Status:                     StatusActive,
		ApplyPricingToAccountStats: false, // not enabled
		// No custom rules
REDACTED
	cs := newTestChannelServiceForStats(t, channel, 10, "anthropic")

	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{
		"claude-sonnet-4": {
			InputPricePerToken:  0.001,
			OutputPricePerToken: 0.002,
	REDACTED,
REDACTED)

	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED

	result := resolveAccountStatsCost(
		context.Background(),
		cs, bs,
		1, 10, "claude-sonnet-4",
		tokens, 1, 999.0, // totalCost ignored
	)
	require.NotNil(t, result)
	// 100*0.001 + 50*0.002 = 0.1 + 0.1 = 0.2
	require.InDelta(t, 0.2, *result, 1e-12)
REDACTED

func TestResolveAccountStatsCost_Gemini36FlashTierUsesFallbackPricing(t *testing.T) {
	channel := &Channel{
		ID:                         1,
		Status:                     StatusActive,
		ApplyPricingToAccountStats: false,
REDACTED
	cs := newTestChannelServiceForStats(t, channel, 10, "antigravity")
	bs := NewBillingService(&config.Config{REDACTED, nil)

	result := resolveAccountStatsCost(
		context.Background(),
		cs, bs,
		1, 10, "gemini-3.6-flash-low",
		UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000REDACTED, 1, 0,
	)
	require.NotNil(t, result)
	require.InDelta(t, 9.15, *result, 1e-12)
REDACTED

func TestResolveAccountStatsCost_AllMiss_ReturnsNil(t *testing.T) {
	channel := &Channel{
		ID:                         1,
		Status:                     StatusActive,
		ApplyPricingToAccountStats: false,
		// No custom rules
REDACTED
	cs := newTestChannelServiceForStats(t, channel, 10, "anthropic")

	// BillingService with no pricing for the model
	bs := newTestBillingServiceWithPrices(map[string]*ModelPricing{REDACTED)

	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED

	result := resolveAccountStatsCost(
		context.Background(),
		cs, bs,
		1, 10, "totally-unknown-model",
		tokens, 1, 0.0,
	)
	require.Nil(t, result)
REDACTED

func TestResolveAccountStatsCost_NilBillingService_SkipsLiteLLM(t *testing.T) {
	channel := &Channel{
		ID:                         1,
		Status:                     StatusActive,
		ApplyPricingToAccountStats: false,
REDACTED
	cs := newTestChannelServiceForStats(t, channel, 10, "anthropic")

	result := resolveAccountStatsCost(
		context.Background(),
		cs, nil, // billingService is nil
		1, 10, "claude-sonnet-4",
		UsageTokens{InputTokens: 100REDACTED, 1, 0.0,
	)
	require.Nil(t, result)
REDACTED

func TestResolveAccountStatsCost_CustomRulePriorityOverApplyPricing(t *testing.T) {
	// Both custom rule and ApplyPricingToAccountStats are configured;
	// custom rule should take precedence.
	channel := &Channel{
		ID:                         1,
		Status:                     StatusActive,
		ApplyPricingToAccountStats: true,
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				GroupIDs: []int64{10REDACTED,
				Pricing: []ChannelModelPricing{
					{
						ID:         100,
						Models:     []string{"claude-sonnet-4"REDACTED,
						InputPrice: testPtrFloat64(0.05),
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	cs := newTestChannelServiceForStats(t, channel, 10, "anthropic")

	tokens := UsageTokens{InputTokens: 100REDACTED

	result := resolveAccountStatsCost(
		context.Background(),
		cs, nil,
		1, 10, "claude-sonnet-4",
		tokens, 1, 99.0, // totalCost = 99.0 (would be used if ApplyPricing wins)
	)
	require.NotNil(t, result)
	// Custom rule: 100*0.05 = 5.0 (NOT 99.0 from totalCost)
	require.InDelta(t, 5.0, *result, 1e-12)
REDACTED

// ---------------------------------------------------------------------------
// helpers for resolveAccountStatsCost tests
// ---------------------------------------------------------------------------

// newTestChannelServiceForStats creates a ChannelService with a single channel
// mapped to the given groupID, suitable for resolveAccountStatsCost tests.
func newTestChannelServiceForStats(t *testing.T, channel *Channel, groupID int64, platform string) *ChannelService {
REDACTED
	cache := newEmptyChannelCache()
	cache.channelByGroupID[groupID] = channel
	cache.groupPlatform[groupID] = platform
	cs := &ChannelService{REDACTED
	cache.loadedAt = time.Now()
	cs.cache.Store(cache)
	return cs
REDACTED
