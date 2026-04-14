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
// CalculateCostUnified
// ---------------------------------------------------------------------------

func TestCalculateCostUnified_NilResolver_FallsBackToOldPath(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500REDACTED
	input := CostInput{
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 1.0,
		Resolver:       nil, // no resolver
REDACTED
	cost, err := svc.CalculateCostUnified(input)
REDACTED

	// Should match the old-path result exactly
	expected, err := svc.calculateCostInternal("claude-sonnet-4", tokens, 1.0, "", nil)
REDACTED
	require.InDelta(t, expected.TotalCost, cost.TotalCost, 1e-10)
	require.InDelta(t, expected.ActualCost, cost.ActualCost, 1e-10)
	// BillingMode is NOT set by old path through CalculateCostUnified (resolver == nil)
	require.Empty(t, cost.BillingMode)
REDACTED

func TestCalculateCostUnified_TokenMode(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500REDACTED
	input := CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 1.5,
		Resolver:       resolver,
REDACTED
	cost, err := bs.CalculateCostUnified(input)
REDACTED
	require.NotNil(t, cost)

	// Verify token billing: Input: 1000*3e-6=0.003, Output: 500*15e-6=0.0075
	expectedTotal := 1000*3e-6 + 500*15e-6
	require.InDelta(t, expectedTotal, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedTotal*1.5, cost.ActualCost, 1e-10)
	require.Equal(t, string(BillingModeToken), cost.BillingMode)
REDACTED

func TestCalculateCostUnified_PerRequestMode(t *testing.T) {
	// Set up a ChannelService with a per-request pricing channel
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 1, model: "claude-sonnet-4"REDACTED: {
				BillingMode:     BillingModePerRequest,
				PerRequestPrice: testPtrFloat64(0.05),
		REDACTED,
	REDACTED,
		channelByGroupID: map[int64]*Channel{
			1: {ID: 1, Status: StatusActiveREDACTED,
	REDACTED,
		groupPlatform:           map[int64]string{1: ""REDACTED,
		wildcardByGroupPlatform: map[channelGroupPlatformKey][]*wildcardPricingEntry{REDACTED,
		mappingByGroupModel:     map[channelModelKey]string{REDACTED,
		wildcardMappingByGP:     map[channelGroupPlatformKey][]*wildcardMappingEntry{REDACTED,
		byID:                    map[int64]*Channel{REDACTED,
REDACTED)

	bs := newTestBillingService()
	resolver := NewModelPricingResolver(cs, bs)
	groupID := int64(1)

	input := CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		GroupID:        &groupID,
		Tokens:         UsageTokens{InputTokens: 100, OutputTokens: 50REDACTED,
		RequestCount:   3,
		RateMultiplier: 2.0,
		Resolver:       resolver,
REDACTED
	cost, err := bs.CalculateCostUnified(input)
REDACTED
	require.NotNil(t, cost)

	// 3 requests * $0.05 = $0.15
	require.InDelta(t, 0.15, cost.TotalCost, 1e-10)
	// ActualCost = 0.15 * 2.0 = 0.30
	require.InDelta(t, 0.30, cost.ActualCost, 1e-10)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
REDACTED

func TestCalculateCostUnified_ImageMode(t *testing.T) {
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 2, model: "gemini-image"REDACTED: {
				BillingMode:     BillingModeImage,
				PerRequestPrice: testPtrFloat64(0.10),
		REDACTED,
	REDACTED,
		channelByGroupID: map[int64]*Channel{
			2: {ID: 2, Status: StatusActiveREDACTED,
	REDACTED,
		groupPlatform:           map[int64]string{2: ""REDACTED,
		wildcardByGroupPlatform: map[channelGroupPlatformKey][]*wildcardPricingEntry{REDACTED,
		mappingByGroupModel:     map[channelModelKey]string{REDACTED,
		wildcardMappingByGP:     map[channelGroupPlatformKey][]*wildcardMappingEntry{REDACTED,
		byID:                    map[int64]*Channel{REDACTED,
REDACTED)

	bs := &BillingService{
		cfg:            &config.Config{REDACTED,
		fallbackPrices: map[string]*ModelPricing{REDACTED,
REDACTED
	resolver := NewModelPricingResolver(cs, bs)
	groupID := int64(2)

	input := CostInput{
		Ctx:            context.Background(),
		Model:          "gemini-image",
		GroupID:        &groupID,
		Tokens:         UsageTokens{REDACTED,
		RequestCount:   2,
		RateMultiplier: 1.0,
		Resolver:       resolver,
REDACTED
	cost, err := bs.CalculateCostUnified(input)
REDACTED
	require.NotNil(t, cost)

	// 2 * $0.10 = $0.20
	require.InDelta(t, 0.20, cost.TotalCost, 1e-10)
	require.InDelta(t, 0.20, cost.ActualCost, 1e-10)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
REDACTED

func TestCalculateCostUnified_RateMultiplierZeroDefaultsToOne(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500REDACTED

	costZero, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 0, // should default to 1.0
		Resolver:       resolver,
REDACTED)
REDACTED

	costOne, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 1.0,
		Resolver:       resolver,
REDACTED)
REDACTED

	require.InDelta(t, costOne.ActualCost, costZero.ActualCost, 1e-10)
REDACTED

func TestCalculateCostUnified_NegativeRateMultiplierDefaultsToOne(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	tokens := UsageTokens{InputTokens: 1000REDACTED

	costNeg, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: -5.0,
		Resolver:       resolver,
REDACTED)
REDACTED

	costOne, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 1.0,
		Resolver:       resolver,
REDACTED)
REDACTED

	require.InDelta(t, costOne.ActualCost, costNeg.ActualCost, 1e-10)
REDACTED

func TestCalculateCostUnified_BillingModeFieldFilled(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         UsageTokens{InputTokens: 100REDACTED,
		RateMultiplier: 1.0,
		Resolver:       resolver,
REDACTED)
REDACTED
	require.Equal(t, "token", cost.BillingMode)
REDACTED

func TestCalculateCostUnified_UsesPreResolvedPricing(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	// Pre-resolve with per_request mode to verify it's used instead of re-resolving
	preResolved := &ResolvedPricing{
		Mode:                   BillingModePerRequest,
		DefaultPerRequestPrice: 0.07,
REDACTED

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         UsageTokens{InputTokens: 100REDACTED,
		RequestCount:   2,
		RateMultiplier: 1.0,
		Resolver:       resolver,
		Resolved:       preResolved,
REDACTED)
REDACTED
	require.NotNil(t, cost)

	// 2 * $0.07 = $0.14
	require.InDelta(t, 0.14, cost.TotalCost, 1e-10)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
REDACTED

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestChannelServiceWithCache creates a ChannelService with a pre-populated
// cache snapshot, bypassing the repository layer entirely.
func newTestChannelServiceWithCache(t *testing.T, cache *channelCache) *ChannelService {
REDACTED
	cs := &ChannelService{REDACTED
	cache.loadedAt = time.Now()
	cs.cache.Store(cache)
	return cs
REDACTED
