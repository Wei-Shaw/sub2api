//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestBillingServiceForResolver() *BillingService {
	bs := &BillingService{
		fallbackPrices: make(map[string]*ModelPricing),
REDACTED
	bs.fallbackPrices["claude-sonnet-4"] = &ModelPricing{
		InputPricePerToken:         3e-6,
		OutputPricePerToken:        15e-6,
		CacheCreationPricePerToken: 3.75e-6,
		CacheReadPricePerToken:     0.3e-6,
		SupportsCacheBreakdown:     false,
REDACTED
	return bs
REDACTED

func TestResolve_NoGroupID(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: nil,
REDACTED)

	require.NotNil(t, resolved)
	require.Equal(t, BillingModeToken, resolved.Mode)
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 3e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
	// BillingService.GetModelPricing uses fallback internally, but resolveBasePricing
	// reports "litellm" when GetModelPricing succeeds (regardless of internal source)
	require.Equal(t, "litellm", resolved.Source)
REDACTED

func TestResolve_UnknownModel(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "unknown-model-xyz",
		GroupID: nil,
REDACTED)

	require.NotNil(t, resolved)
	require.Nil(t, resolved.BasePricing)
	// Unknown model: GetModelPricing returns error, source is "fallback"
	require.Equal(t, "fallback", resolved.Source)
REDACTED

func TestGetIntervalPricing_NoIntervals(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	basePricing := &ModelPricing{InputPricePerToken: 5e-6REDACTED
	resolved := &ResolvedPricing{
		Mode:        BillingModeToken,
		BasePricing: basePricing,
		Intervals:   nil,
REDACTED

	result := r.GetIntervalPricing(resolved, 50000)
	require.Equal(t, basePricing, result)
REDACTED

func TestGetIntervalPricing_MatchesInterval(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	resolved := &ResolvedPricing{
		Mode:                   BillingModeToken,
		BasePricing:            &ModelPricing{InputPricePerToken: 5e-6REDACTED,
		SupportsCacheBreakdown: true,
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(2e-6)REDACTED,
			{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(3e-6), OutputPrice: testPtrFloat64(6e-6)REDACTED,
	REDACTED,
REDACTED

	result := r.GetIntervalPricing(resolved, 50000)
	require.NotNil(t, result)
	require.InDelta(t, 1e-6, result.InputPricePerToken, 1e-12)
	require.InDelta(t, 2e-6, result.OutputPricePerToken, 1e-12)
	require.True(t, result.SupportsCacheBreakdown)

	result2 := r.GetIntervalPricing(resolved, 200000)
	require.NotNil(t, result2)
	require.InDelta(t, 3e-6, result2.InputPricePerToken, 1e-12)
REDACTED

func TestGetIntervalPricing_NoMatch_FallsBackToBase(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	basePricing := &ModelPricing{InputPricePerToken: 99e-6REDACTED
	resolved := &ResolvedPricing{
		Mode:        BillingModeToken,
		BasePricing: basePricing,
		Intervals: []PricingInterval{
			{MinTokens: 10000, MaxTokens: testPtrInt(50000), InputPrice: testPtrFloat64(1e-6)REDACTED,
	REDACTED,
REDACTED

	result := r.GetIntervalPricing(resolved, 5000)
	require.Equal(t, basePricing, result)
REDACTED

func TestGetRequestTierPrice(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	resolved := &ResolvedPricing{
		Mode: BillingModePerRequest,
		RequestTiers: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)REDACTED,
			{TierLabel: "2K", PerRequestPrice: testPtrFloat64(0.08)REDACTED,
	REDACTED,
REDACTED

	require.InDelta(t, 0.04, r.GetRequestTierPrice(resolved, "1K"), 1e-12)
	require.InDelta(t, 0.08, r.GetRequestTierPrice(resolved, "2K"), 1e-12)
	require.InDelta(t, 0.0, r.GetRequestTierPrice(resolved, "4K"), 1e-12)
REDACTED

func TestGetRequestTierPriceByContext(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	resolved := &ResolvedPricing{
		Mode: BillingModePerRequest,
		RequestTiers: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(128000), PerRequestPrice: testPtrFloat64(0.05)REDACTED,
			{MinTokens: 128000, MaxTokens: nil, PerRequestPrice: testPtrFloat64(0.10)REDACTED,
	REDACTED,
REDACTED

	require.InDelta(t, 0.05, r.GetRequestTierPriceByContext(resolved, 50000), 1e-12)
	require.InDelta(t, 0.10, r.GetRequestTierPriceByContext(resolved, 200000), 1e-12)
REDACTED

func TestGetRequestTierPrice_NilPerRequestPrice(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	resolved := &ResolvedPricing{
		Mode: BillingModePerRequest,
		RequestTiers: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: nilREDACTED,
	REDACTED,
REDACTED

	require.InDelta(t, 0.0, r.GetRequestTierPrice(resolved, "1K"), 1e-12)
REDACTED

// ===========================================================================
// Channel override tests — exercises applyChannelOverrides via Resolve
// ===========================================================================

// helper: creates a resolver wired to a ChannelService that returns the given
// channel (active, groupID=100, platform=anthropic) with the specified pricing.
func newResolverWithChannel(t *testing.T, pricing []ChannelModelPricing) *ModelPricingResolver {
REDACTED
	const groupID = 100
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{{
				ID:           1,
				Name:         "test-channel",
				Status:       StatusActive,
				GroupIDs:     []int64{groupIDREDACTED,
				ModelPricing: pricing,
	REDACTED nil
	REDACTED,
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{groupID: "anthropic"REDACTED, nil
	REDACTED,
REDACTED
	cs := NewChannelService(repo, nil, nil, nil)
	bs := newTestBillingServiceForResolver()
	return NewModelPricingResolver(cs, bs)
REDACTED

// groupIDPtr returns a pointer to groupID 100 (the test constant).
func groupIDPtr() *int64 { v := int64(100); return &v REDACTED

// ---------------------------------------------------------------------------
// 1. Token mode overrides
// ---------------------------------------------------------------------------

func TestResolve_WithChannelOverride_TokenFlat(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(10e-6),
		OutputPrice: testPtrFloat64(50e-6),
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.NotNil(t, resolved)
	require.Equal(t, BillingModeToken, resolved.Mode)
	require.Equal(t, "channel", resolved.Source)
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 10e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 10e-6, resolved.BasePricing.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 50e-6, resolved.BasePricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 50e-6, resolved.BasePricing.OutputPricePerTokenPriority, 1e-12)
REDACTED

func TestResolve_WithChannelOverride_TokenPartialOverride(t *testing.T) {
	// Channel only sets InputPrice; OutputPrice should remain from the base (LiteLLM/fallback).
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(20e-6),
		// OutputPrice intentionally nil
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.NotNil(t, resolved)
	require.Equal(t, "channel", resolved.Source)
	require.NotNil(t, resolved.BasePricing)
	// InputPrice overridden by channel
	require.InDelta(t, 20e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
	// OutputPrice kept from base (fallback: 15e-6)
	require.InDelta(t, 15e-6, resolved.BasePricing.OutputPricePerToken, 1e-12)
REDACTED

func TestResolve_WithChannelOverride_TokenWithIntervals(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(2e-6), OutputPrice: testPtrFloat64(8e-6)REDACTED,
			{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(4e-6), OutputPrice: testPtrFloat64(16e-6)REDACTED,
	REDACTED,
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.NotNil(t, resolved)
	require.Equal(t, "channel", resolved.Source)
	require.Len(t, resolved.Intervals, 2)

	// GetIntervalPricing should use channel intervals
	iv := r.GetIntervalPricing(resolved, 50000)
	require.NotNil(t, iv)
	require.InDelta(t, 2e-6, iv.InputPricePerToken, 1e-12)
	require.InDelta(t, 8e-6, iv.OutputPricePerToken, 1e-12)

	iv2 := r.GetIntervalPricing(resolved, 200000)
	require.NotNil(t, iv2)
	require.InDelta(t, 4e-6, iv2.InputPricePerToken, 1e-12)
	require.InDelta(t, 16e-6, iv2.OutputPricePerToken, 1e-12)
REDACTED

func TestResolve_WithChannelOverride_TokenNilBasePricing(t *testing.T) {
	// Base pricing is nil (unknown model), channel has flat prices → creates new BasePricing.
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"unknown-model-xyz"REDACTED,
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(7e-6),
		OutputPrice: testPtrFloat64(21e-6),
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "unknown-model-xyz",
		GroupID: groupIDPtr(),
REDACTED)

	require.NotNil(t, resolved)
	require.Equal(t, "channel", resolved.Source)
	// BasePricing was nil from resolveBasePricing but applyTokenOverrides creates a new one
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 7e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 21e-6, resolved.BasePricing.OutputPricePerToken, 1e-12)
REDACTED

// ---------------------------------------------------------------------------
// 2. Per-request mode overrides
// ---------------------------------------------------------------------------

func TestResolve_WithChannelOverride_PerRequest(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:        "anthropic",
		Models:          []string{"claude-sonnet-4"REDACTED,
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: testPtrFloat64(0.05),
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(128000), PerRequestPrice: testPtrFloat64(0.03)REDACTED,
			{MinTokens: 128000, MaxTokens: nil, PerRequestPrice: testPtrFloat64(0.10)REDACTED,
	REDACTED,
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.NotNil(t, resolved)
	require.Equal(t, BillingModePerRequest, resolved.Mode)
	require.Equal(t, "channel", resolved.Source)
	require.InDelta(t, 0.05, resolved.DefaultPerRequestPrice, 1e-12)
	require.Len(t, resolved.RequestTiers, 2)

	// Verify tier lookups
	require.InDelta(t, 0.03, r.GetRequestTierPriceByContext(resolved, 50000), 1e-12)
	require.InDelta(t, 0.10, r.GetRequestTierPriceByContext(resolved, 200000), 1e-12)
REDACTED

func TestResolve_WithChannelOverride_PerRequestNilPrice(t *testing.T) {
	// PerRequestPrice nil → DefaultPerRequestPrice stays 0.
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModePerRequest,
		// PerRequestPrice intentionally nil
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(128000), PerRequestPrice: testPtrFloat64(0.02)REDACTED,
	REDACTED,
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.NotNil(t, resolved)
	require.Equal(t, BillingModePerRequest, resolved.Mode)
	require.InDelta(t, 0.0, resolved.DefaultPerRequestPrice, 1e-12)
	require.Len(t, resolved.RequestTiers, 1)
REDACTED

// ---------------------------------------------------------------------------
// 3. Image mode overrides
// ---------------------------------------------------------------------------

func TestResolve_WithChannelOverride_Image(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:        "anthropic",
		Models:          []string{"claude-sonnet-4"REDACTED,
		BillingMode:     BillingModeImage,
		PerRequestPrice: testPtrFloat64(0.08),
		Intervals: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)REDACTED,
			{TierLabel: "2K", PerRequestPrice: testPtrFloat64(0.08)REDACTED,
			{TierLabel: "4K", PerRequestPrice: testPtrFloat64(0.16)REDACTED,
	REDACTED,
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.NotNil(t, resolved)
	require.Equal(t, BillingModeImage, resolved.Mode)
	require.Equal(t, "channel", resolved.Source)
	require.InDelta(t, 0.08, resolved.DefaultPerRequestPrice, 1e-12)
	require.Len(t, resolved.RequestTiers, 3)
REDACTED

func TestResolve_WithChannelOverride_ImageTierLabels(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeImage,
		Intervals: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)REDACTED,
			{TierLabel: "2K", PerRequestPrice: testPtrFloat64(0.08)REDACTED,
			{TierLabel: "4K", PerRequestPrice: testPtrFloat64(0.16)REDACTED,
	REDACTED,
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.InDelta(t, 0.04, r.GetRequestTierPrice(resolved, "1K"), 1e-12)
	require.InDelta(t, 0.08, r.GetRequestTierPrice(resolved, "2K"), 1e-12)
	require.InDelta(t, 0.16, r.GetRequestTierPrice(resolved, "4K"), 1e-12)
	require.InDelta(t, 0.0, r.GetRequestTierPrice(resolved, "8K"), 1e-12) // not found
REDACTED

// ---------------------------------------------------------------------------
// 4. Source tracking & default mode
// ---------------------------------------------------------------------------

func TestResolve_WithChannelOverride_SourceIsChannel(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(1e-6),
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.Equal(t, "channel", resolved.Source)
REDACTED

func TestResolve_WithChannelOverride_DefaultMode(t *testing.T) {
	// Channel pricing with empty BillingMode → defaults to BillingModeToken.
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: "", // intentionally empty
		InputPrice:  testPtrFloat64(5e-6),
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.Equal(t, "channel", resolved.Source)
	require.Equal(t, BillingModeToken, resolved.Mode)
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 5e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
REDACTED

// ---------------------------------------------------------------------------
// 5. GetIntervalPricing integration after channel override
// ---------------------------------------------------------------------------

func TestGetIntervalPricing_WithChannelIntervals(t *testing.T) {
	// Channel provides intervals that override the base pricing path.
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(100000), InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(5e-6)REDACTED,
			{MinTokens: 100000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6), OutputPrice: testPtrFloat64(10e-6)REDACTED,
	REDACTED,
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	// Token count 50000 matches first interval
	pricing := r.GetIntervalPricing(resolved, 50000)
	require.NotNil(t, pricing)
	require.InDelta(t, 1e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 5e-6, pricing.OutputPricePerToken, 1e-12)

	// Token count 150000 matches second interval
	pricing2 := r.GetIntervalPricing(resolved, 150000)
	require.NotNil(t, pricing2)
	require.InDelta(t, 2e-6, pricing2.InputPricePerToken, 1e-12)
	require.InDelta(t, 10e-6, pricing2.OutputPricePerToken, 1e-12)
REDACTED

func TestGetIntervalPricing_ChannelIntervalsNoMatch(t *testing.T) {
	// Channel intervals don't match token count → falls back to BasePricing.
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			// Only covers tokens > 50000
			{MinTokens: 50000, MaxTokens: testPtrInt(200000), InputPrice: testPtrFloat64(9e-6)REDACTED,
	REDACTED,
REDACTEDREDACTED)

	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	// Token count 1000 doesn't match any interval (1000 <= 50000 minTokens)
	pricing := r.GetIntervalPricing(resolved, 1000)
	// Should fall back to BasePricing (from the billing service fallback)
	require.NotNil(t, pricing)
	require.Equal(t, resolved.BasePricing, pricing)
	require.InDelta(t, 3e-6, pricing.InputPricePerToken, 1e-12) // original base price
REDACTED

// ===========================================================================
// 6. Error path tests
// ===========================================================================

func TestResolve_WithChannelOverride_CacheError(t *testing.T) {
	// When ListAll returns an error, the ChannelService cache build fails.
	// Resolve should gracefully fall back to base pricing without panicking.
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return nil, errors.New("database unavailable")
	REDACTED,
REDACTED
	cs := NewChannelService(repo, nil, nil, nil)
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(cs, bs)

	gid := int64(100)
	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: &gid,
REDACTED)

	require.NotNil(t, resolved)
	// Should NOT panic, should NOT have source "channel"
	require.NotEqual(t, "channel", resolved.Source)
	// Base pricing should still be present (from BillingService fallback)
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 3e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
REDACTED

// ===========================================================================
// 7. GetRequestTierPriceByContext boundary tests
// ===========================================================================

func TestGetRequestTierPriceByContext_EmptyTiers(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	resolved := &ResolvedPricing{
		Mode:         BillingModePerRequest,
		RequestTiers: nil, // empty
REDACTED

	price := r.GetRequestTierPriceByContext(resolved, 50000)
	require.InDelta(t, 0.0, price, 1e-12)

	// Also test with explicit empty slice
	resolved2 := &ResolvedPricing{
		Mode:         BillingModePerRequest,
		RequestTiers: []PricingInterval{REDACTED,
REDACTED

	price2 := r.GetRequestTierPriceByContext(resolved2, 50000)
	require.InDelta(t, 0.0, price2, 1e-12)
REDACTED

func TestGetRequestTierPriceByContext_ExactBoundary(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	r := NewModelPricingResolver(&ChannelService{REDACTED, bs)

	resolved := &ResolvedPricing{
		Mode: BillingModePerRequest,
		RequestTiers: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(128000), PerRequestPrice: testPtrFloat64(0.05)REDACTED,
			{MinTokens: 128000, MaxTokens: nil, PerRequestPrice: testPtrFloat64(0.10)REDACTED,
	REDACTED,
REDACTED

	// totalContextTokens = 128000 exactly:
	// FindMatchingInterval checks: totalTokens > MinTokens && totalTokens <= MaxTokens
	// For first interval: 128000 > 0 (true) && 128000 <= 128000 (true) → matches first interval
	price := r.GetRequestTierPriceByContext(resolved, 128000)
	require.InDelta(t, 0.05, price, 1e-12)

	// totalContextTokens = 128001 should match second interval
	// For first interval: 128001 > 0 (true) && 128001 <= 128000 (false) → no match
	// For second interval: 128001 > 128000 (true) && MaxTokens == nil → matches
	price2 := r.GetRequestTierPriceByContext(resolved, 128001)
	require.InDelta(t, 0.10, price2, 1e-12)
REDACTED

// ===========================================================================
// 8. filterValidIntervals
// ===========================================================================

func TestFilterValidIntervals(t *testing.T) {
	tests := []struct {
		name      string
		intervals []PricingInterval
		wantLen   int
REDACTED{
		{
			name:      "empty list",
			intervals: nil,
			wantLen:   0,
	REDACTED,
		{
			name: "all-nil interval filtered out",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(128000)REDACTED,
		REDACTED,
			wantLen: 0,
	REDACTED,
		{
			name: "interval with only InputPrice kept",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)REDACTED,
		REDACTED,
			wantLen: 1,
	REDACTED,
		{
			name: "interval with only OutputPrice kept",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(128000), OutputPrice: testPtrFloat64(2e-6)REDACTED,
		REDACTED,
			wantLen: 1,
	REDACTED,
		{
			name: "interval with only CacheWritePrice kept",
			intervals: []PricingInterval{
				{MinTokens: 0, CacheWritePrice: testPtrFloat64(3e-6)REDACTED,
		REDACTED,
			wantLen: 1,
	REDACTED,
		{
			name: "interval with only CacheReadPrice kept",
			intervals: []PricingInterval{
				{MinTokens: 0, CacheReadPrice: testPtrFloat64(0.5e-6)REDACTED,
		REDACTED,
			wantLen: 1,
	REDACTED,
		{
			name: "interval with only PerRequestPrice kept",
			intervals: []PricingInterval{
				{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)REDACTED,
		REDACTED,
			wantLen: 1,
	REDACTED,
		{
			name: "mixed valid and invalid",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)REDACTED,
				{MinTokens: 128000, MaxTokens: nilREDACTED, // all-nil → filtered out
				{MinTokens: 256000, OutputPrice: testPtrFloat64(5e-6)REDACTED,
		REDACTED,
			wantLen: 2,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterValidIntervals(tt.intervals)
			require.Len(t, result, tt.wantLen)
	REDACTED)
REDACTED
REDACTED

// ===========================================================================
// 9. ImageOutputPriceExplicit tests
// ===========================================================================

func TestApplyTokenOverrides_FlatSetsImageOutputPriceExplicit(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(3e-6),
		OutputPrice: testPtrFloat64(15e-6),
		// ImageOutputPrice intentionally nil
REDACTEDREDACTED)
	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.Equal(t, PricingSourceChannel, resolved.Source)
	require.True(t, resolved.BasePricing.ImageOutputPriceExplicit)
	require.Equal(t, 0.0, resolved.BasePricing.ImageOutputPricePerToken)
REDACTED

func TestApplyTokenOverrides_FlatWithImageOutputPriceSetsExplicit(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:         "anthropic",
		Models:           []string{"claude-sonnet-4"REDACTED,
		BillingMode:      BillingModeToken,
		InputPrice:       testPtrFloat64(3e-6),
		OutputPrice:      testPtrFloat64(15e-6),
		ImageOutputPrice: testPtrFloat64(50e-6),
REDACTEDREDACTED)
	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	require.True(t, resolved.BasePricing.ImageOutputPriceExplicit)
	require.InDelta(t, 50e-6, resolved.BasePricing.ImageOutputPricePerToken, 1e-12)
REDACTED

func TestApplyTokenOverrides_IntervalSetsImageOutputPriceExplicit(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"REDACTED,
		BillingMode: BillingModeToken,
		// No ImageOutputPrice
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(100000), InputPrice: testPtrFloat64(3e-6), OutputPrice: testPtrFloat64(15e-6)REDACTED,
	REDACTED,
REDACTEDREDACTED)
	resolved := r.Resolve(context.Background(), PricingInput{
		Model:   "claude-sonnet-4",
		GroupID: groupIDPtr(),
REDACTED)

	// BasePricing should have explicit mark (for interval fallback)
	require.True(t, resolved.BasePricing.ImageOutputPriceExplicit)
	require.Equal(t, 0.0, resolved.BasePricing.ImageOutputPricePerToken)

	// intervalToModelPricing should also have explicit mark
	pricing := r.GetIntervalPricing(resolved, 50000)
	require.True(t, pricing.ImageOutputPriceExplicit)
	require.Equal(t, 0.0, pricing.ImageOutputPricePerToken)
REDACTED
