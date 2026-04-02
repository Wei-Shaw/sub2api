//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetModelPricing(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Models: []string{"claude-sonnet-4"REDACTED, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(3e-6)REDACTED,
			{ID: 3, Models: []string{"gpt-5.1"REDACTED, BillingMode: BillingModePerRequestREDACTED,
	REDACTED,
REDACTED

	tests := []struct {
		name    string
		model   string
		wantID  int64
		wantNil bool
REDACTED{
		{"exact match", "claude-sonnet-4", 1, falseREDACTED,
		{"case insensitive", "Claude-Sonnet-4", 1, falseREDACTED,
		{"not found", "gemini-3.1-pro", 0, trueREDACTED,
		{"wildcard pattern not matched", "claude-opus-4-20250514", 0, trueREDACTED,
		{"per_request model", "gpt-5.1", 3, falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ch.GetModelPricing(tt.model)
			if tt.wantNil {
				require.Nil(t, result)
				return
		REDACTED
			require.NotNil(t, result)
			require.Equal(t, tt.wantID, result.ID)
	REDACTED)
REDACTED
REDACTED

func TestGetModelPricing_ReturnsCopy(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Models: []string{"claude-sonnet-4"REDACTED, InputPrice: testPtrFloat64(3e-6)REDACTED,
	REDACTED,
REDACTED

	result := ch.GetModelPricing("claude-sonnet-4")
	require.NotNil(t, result)

	// Modify the returned copy's slice — original should be unchanged
	result.Models = append(result.Models, "hacked")

	// Original should be unchanged
	require.Equal(t, 1, len(ch.ModelPricing[0].Models))
REDACTED

func TestGetModelPricing_EmptyPricing(t *testing.T) {
	ch := &Channel{ModelPricing: nilREDACTED
	require.Nil(t, ch.GetModelPricing("any-model"))

	ch2 := &Channel{ModelPricing: []ChannelModelPricing{REDACTEDREDACTED
	require.Nil(t, ch2.GetModelPricing("any-model"))
REDACTED

func TestGetIntervalForContext(t *testing.T) {
	p := &ChannelModelPricing{
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)REDACTED,
			{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6)REDACTED,
	REDACTED,
REDACTED

	tests := []struct {
		name      string
		tokens    int
		wantPrice *float64
		wantNil   bool
REDACTED{
		{"first interval", 50000, testPtrFloat64(1e-6), falseREDACTED,
		// (min, max] — 128000 在第一个区间的 max，包含，所以匹配第一个
		{"boundary: max of first (inclusive)", 128000, testPtrFloat64(1e-6), falseREDACTED,
		// 128001 > 128000，匹配第二个区间
		{"boundary: just above first max", 128001, testPtrFloat64(2e-6), falseREDACTED,
		{"unbounded interval", 500000, testPtrFloat64(2e-6), falseREDACTED,
		// (0, max] — 0 不匹配任何区间（左开）
		{"zero tokens: no match", 0, nil, trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.GetIntervalForContext(tt.tokens)
			if tt.wantNil {
				require.Nil(t, result)
				return
		REDACTED
			require.NotNil(t, result)
			require.InDelta(t, *tt.wantPrice, *result.InputPrice, 1e-12)
	REDACTED)
REDACTED
REDACTED

func TestGetIntervalForContext_NoMatch(t *testing.T) {
	p := &ChannelModelPricing{
		Intervals: []PricingInterval{
			{MinTokens: 10000, MaxTokens: testPtrInt(50000)REDACTED,
	REDACTED,
REDACTED
	require.Nil(t, p.GetIntervalForContext(5000))     // 5000 <= 10000, not > min
	require.Nil(t, p.GetIntervalForContext(10000))    // 10000 not > 10000 (left-open)
	require.NotNil(t, p.GetIntervalForContext(50000)) // 50000 <= 50000 (right-closed)
	require.Nil(t, p.GetIntervalForContext(50001))    // 50001 > 50000
REDACTED

func TestGetIntervalForContext_Empty(t *testing.T) {
	p := &ChannelModelPricing{Intervals: nilREDACTED
	require.Nil(t, p.GetIntervalForContext(1000))
REDACTED

func TestGetTierByLabel(t *testing.T) {
	p := &ChannelModelPricing{
		Intervals: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)REDACTED,
			{TierLabel: "2K", PerRequestPrice: testPtrFloat64(0.08)REDACTED,
			{TierLabel: "HD", PerRequestPrice: testPtrFloat64(0.12)REDACTED,
	REDACTED,
REDACTED

	tests := []struct {
		name    string
		label   string
		wantNil bool
		want    float64
REDACTED{
		{"exact match", "1K", false, 0.04REDACTED,
		{"case insensitive", "hd", false, 0.12REDACTED,
		{"not found", "4K", true, 0REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.GetTierByLabel(tt.label)
			if tt.wantNil {
				require.Nil(t, result)
				return
		REDACTED
			require.NotNil(t, result)
			require.InDelta(t, tt.want, *result.PerRequestPrice, 1e-12)
	REDACTED)
REDACTED
REDACTED

func TestGetTierByLabel_Empty(t *testing.T) {
	p := &ChannelModelPricing{Intervals: nilREDACTED
	require.Nil(t, p.GetTierByLabel("1K"))
REDACTED

func TestChannelClone(t *testing.T) {
	original := &Channel{
		ID:       1,
		Name:     "test",
		GroupIDs: []int64{10, 20REDACTED,
		ModelPricing: []ChannelModelPricing{
			{
				ID:         100,
				Models:     []string{"model-a"REDACTED,
				InputPrice: testPtrFloat64(5e-6),
		REDACTED,
	REDACTED,
REDACTED

	cloned := original.Clone()
	require.NotNil(t, cloned)
	require.Equal(t, original.ID, cloned.ID)
	require.Equal(t, original.Name, cloned.Name)

	// Modify clone slices — original should not change
	cloned.GroupIDs[0] = 999
	require.Equal(t, int64(10), original.GroupIDs[0])

	cloned.ModelPricing[0].Models[0] = "hacked"
	require.Equal(t, "model-a", original.ModelPricing[0].Models[0])
REDACTED

func TestChannelClone_Nil(t *testing.T) {
	var ch *Channel
	require.Nil(t, ch.Clone())
REDACTED

func TestChannelModelPricingClone(t *testing.T) {
	original := ChannelModelPricing{
		Models: []string{"a", "b"REDACTED,
		Intervals: []PricingInterval{
			{MinTokens: 0, TierLabel: "tier1"REDACTED,
	REDACTED,
REDACTED

	cloned := original.Clone()

	// Modify clone slices — original unchanged
	cloned.Models[0] = "hacked"
	require.Equal(t, "a", original.Models[0])

	cloned.Intervals[0].TierLabel = "hacked"
	require.Equal(t, "tier1", original.Intervals[0].TierLabel)
REDACTED

// --- BillingMode.IsValid ---

func TestBillingModeIsValid(t *testing.T) {
	tests := []struct {
		name string
		mode BillingMode
		want bool
REDACTED{
		{"token", BillingModeToken, trueREDACTED,
		{"per_request", BillingModePerRequest, trueREDACTED,
		{"image", BillingModeImage, trueREDACTED,
		{"empty", BillingMode(""), trueREDACTED,
		{"unknown", BillingMode("unknown"), falseREDACTED,
		{"random", BillingMode("xyz"), falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.mode.IsValid())
	REDACTED)
REDACTED
REDACTED

// --- Channel.IsActive ---

func TestChannelIsActive(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
REDACTED{
		{"active", StatusActive, trueREDACTED,
		{"disabled", "disabled", falseREDACTED,
		{"empty", "", falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &Channel{Status: tt.statusREDACTED
			require.Equal(t, tt.want, ch.IsActive())
	REDACTED)
REDACTED
REDACTED

// --- ChannelModelPricing.Clone edge cases ---

func TestChannelModelPricingClone_EdgeCases(t *testing.T) {
	t.Run("nil models", func(t *testing.T) {
		original := ChannelModelPricing{Models: nilREDACTED
		cloned := original.Clone()
		require.Nil(t, cloned.Models)
REDACTED)

	t.Run("nil intervals", func(t *testing.T) {
		original := ChannelModelPricing{Intervals: nilREDACTED
		cloned := original.Clone()
		require.Nil(t, cloned.Intervals)
REDACTED)

	t.Run("empty models", func(t *testing.T) {
		original := ChannelModelPricing{Models: []string{REDACTEDREDACTED
		cloned := original.Clone()
		require.NotNil(t, cloned.Models)
		require.Empty(t, cloned.Models)
REDACTED)
REDACTED

// --- Channel.Clone edge cases ---

func TestChannelClone_EdgeCases(t *testing.T) {
	t.Run("nil model mapping", func(t *testing.T) {
		original := &Channel{ID: 1, ModelMapping: nilREDACTED
		cloned := original.Clone()
		require.Nil(t, cloned.ModelMapping)
REDACTED)

	t.Run("nil model pricing", func(t *testing.T) {
		original := &Channel{ID: 1, ModelPricing: nilREDACTED
		cloned := original.Clone()
		require.Nil(t, cloned.ModelPricing)
REDACTED)

	t.Run("deep copy model mapping", func(t *testing.T) {
		original := &Channel{
			ID: 1,
			ModelMapping: map[string]map[string]string{
				"openai": {"gpt-4": "gpt-4-turbo"REDACTED,
		REDACTED,
	REDACTED
		cloned := original.Clone()

		// Modify the cloned nested map
		cloned.ModelMapping["openai"]["gpt-4"] = "hacked"

		// Original must remain unchanged
		require.Equal(t, "gpt-4-turbo", original.ModelMapping["openai"]["gpt-4"])
REDACTED)
REDACTED

// --- ValidateIntervals ---

func TestValidateIntervals_Empty(t *testing.T) {
	require.NoError(t, ValidateIntervals(nil))
	require.NoError(t, ValidateIntervals([]PricingInterval{REDACTED))
REDACTED

func TestValidateIntervals_ValidIntervals(t *testing.T) {
	tests := []struct {
		name      string
		intervals []PricingInterval
REDACTED{
		{
			name: "single bounded interval",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)REDACTED,
		REDACTED,
	REDACTED,
		{
			name: "two intervals with gap",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(100000), InputPrice: testPtrFloat64(1e-6)REDACTED,
				{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6)REDACTED,
		REDACTED,
	REDACTED,
		{
			name: "two contiguous intervals",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)REDACTED,
				{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6)REDACTED,
		REDACTED,
	REDACTED,
		{
			name: "unsorted input (auto-sorted by validator)",
			intervals: []PricingInterval{
				{MinTokens: 128000, MaxTokens: nil, InputPrice: testPtrFloat64(2e-6)REDACTED,
				{MinTokens: 0, MaxTokens: testPtrInt(128000), InputPrice: testPtrFloat64(1e-6)REDACTED,
		REDACTED,
	REDACTED,
		{
			name: "single unbounded interval",
			intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: nil, InputPrice: testPtrFloat64(1e-6)REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, ValidateIntervals(tt.intervals))
	REDACTED)
REDACTED
REDACTED

func TestValidateIntervals_NegativeMinTokens(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: -1, MaxTokens: testPtrInt(100), InputPrice: testPtrFloat64(1e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals)
REDACTED
	require.Contains(t, err.Error(), "min_tokens")
	require.Contains(t, err.Error(), ">= 0")
REDACTED

func TestValidateIntervals_MaxTokensZero(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(0), InputPrice: testPtrFloat64(1e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals)
REDACTED
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> 0")
REDACTED

func TestValidateIntervals_MaxLessThanMin(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 100, MaxTokens: testPtrInt(50), InputPrice: testPtrFloat64(1e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals)
REDACTED
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> min_tokens")
REDACTED

func TestValidateIntervals_MaxEqualsMin(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 100, MaxTokens: testPtrInt(100), InputPrice: testPtrFloat64(1e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals)
REDACTED
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> min_tokens")
REDACTED

func TestValidateIntervals_NegativePrice(t *testing.T) {
	negPrice := -0.01
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(100), InputPrice: &negPriceREDACTED,
REDACTED
	err := ValidateIntervals(intervals)
REDACTED
	require.Contains(t, err.Error(), "input_price")
	require.Contains(t, err.Error(), ">= 0")
REDACTED

func TestValidateIntervals_OverlappingIntervals(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(200), InputPrice: testPtrFloat64(1e-6)REDACTED,
		{MinTokens: 100, MaxTokens: testPtrInt(300), InputPrice: testPtrFloat64(2e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals)
REDACTED
	require.Contains(t, err.Error(), "overlap")
REDACTED

func TestValidateIntervals_UnboundedNotLast(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: nil, InputPrice: testPtrFloat64(1e-6)REDACTED,
		{MinTokens: 128000, MaxTokens: testPtrInt(256000), InputPrice: testPtrFloat64(2e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals)
REDACTED
	require.Contains(t, err.Error(), "unbounded")
	require.Contains(t, err.Error(), "last")
REDACTED
