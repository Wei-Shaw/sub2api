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
		TimePricing: &ChannelTimePricing{
			Timezone: "Asia/Shanghai",
			Periods: []ChannelTimePricingPeriod{{
				StartTime:  "09:00",
				EndTime:    "12:00",
				Multiplier: 2,
	REDACTED
	REDACTED,
REDACTED

	cloned := original.Clone()

	// Modify clone slices — original unchanged
	cloned.Models[0] = "hacked"
	require.Equal(t, "a", original.Models[0])

	cloned.Intervals[0].TierLabel = "hacked"
	require.Equal(t, "tier1", original.Intervals[0].TierLabel)

	cloned.TimePricing.Timezone = "America/New_York"
	cloned.TimePricing.Periods[0].StartTime = "10:00"
	cloned.TimePricing.Periods[0].Multiplier = 3
	require.Equal(t, "Asia/Shanghai", original.TimePricing.Timezone)
	require.Equal(t, "09:00", original.TimePricing.Periods[0].StartTime)
	require.Equal(t, 2.0, original.TimePricing.Periods[0].Multiplier)
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
	require.NoError(t, ValidateIntervals(nil, BillingModeToken))
	require.NoError(t, ValidateIntervals([]PricingInterval{REDACTED, BillingModeToken))
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
			require.NoError(t, ValidateIntervals(tt.intervals, BillingModeToken))
	REDACTED)
REDACTED
REDACTED

func TestValidateIntervals_NegativeMinTokens(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: -1, MaxTokens: testPtrInt(100), InputPrice: testPtrFloat64(1e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeToken)
REDACTED
	require.Contains(t, err.Error(), "min_tokens")
	require.Contains(t, err.Error(), ">= 0")
REDACTED

func TestValidateIntervals_MaxTokensZero(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(0), InputPrice: testPtrFloat64(1e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeToken)
REDACTED
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> 0")
REDACTED

func TestValidateIntervals_MaxLessThanMin(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 100, MaxTokens: testPtrInt(50), InputPrice: testPtrFloat64(1e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeToken)
REDACTED
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> min_tokens")
REDACTED

func TestValidateIntervals_MaxEqualsMin(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 100, MaxTokens: testPtrInt(100), InputPrice: testPtrFloat64(1e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeToken)
REDACTED
	require.Contains(t, err.Error(), "max_tokens")
	require.Contains(t, err.Error(), "> min_tokens")
REDACTED

func TestValidateIntervals_NegativePrice(t *testing.T) {
	negPrice := -0.01
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(100), InputPrice: &negPriceREDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeToken)
REDACTED
	require.Contains(t, err.Error(), "input_price")
	require.Contains(t, err.Error(), ">= 0")
REDACTED

func TestValidateIntervals_OverlappingIntervals(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: testPtrInt(200), InputPrice: testPtrFloat64(1e-6)REDACTED,
		{MinTokens: 100, MaxTokens: testPtrInt(300), InputPrice: testPtrFloat64(2e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeToken)
REDACTED
	require.Contains(t, err.Error(), "overlap")
REDACTED

func TestValidateIntervals_UnboundedNotLast(t *testing.T) {
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: nil, InputPrice: testPtrFloat64(1e-6)REDACTED,
		{MinTokens: 128000, MaxTokens: testPtrInt(256000), InputPrice: testPtrFloat64(2e-6)REDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeToken)
REDACTED
	require.Contains(t, err.Error(), "unbounded")
	require.Contains(t, err.Error(), "last")
REDACTED

func TestValidateIntervals_ImageModeAllowsMultipleUnboundedTiers(t *testing.T) {
	// image / per_request 按 tier_label 匹配，多条 min=0/max=nil 是合法形态。
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: nil, TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)REDACTED,
		{MinTokens: 0, MaxTokens: nil, TierLabel: "2K", PerRequestPrice: testPtrFloat64(0.06)REDACTED,
		{MinTokens: 0, MaxTokens: nil, TierLabel: "4K", PerRequestPrice: testPtrFloat64(0.08)REDACTED,
REDACTED
	require.NoError(t, ValidateIntervals(intervals, BillingModeImage))
	require.NoError(t, ValidateIntervals(intervals, BillingModePerRequest))
REDACTED

func TestValidateIntervals_ImageModeStillRejectsNegativePrice(t *testing.T) {
	// image 模式只跳过区间重叠校验，单条字段自洽（价格非负）仍要校验。
	intervals := []PricingInterval{
		{MinTokens: 0, MaxTokens: nil, TierLabel: "1K", PerRequestPrice: testPtrFloat64(-1)REDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeImage)
REDACTED
	require.Contains(t, err.Error(), "must be >= 0")
REDACTED

func TestValidateIntervals_ImageModeStillRejectsBadMaxTokens(t *testing.T) {
	// image 模式仍校验 max <= min 这种单条不合法。
	intervals := []PricingInterval{
		{MinTokens: 100, MaxTokens: testPtrInt(50), TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)REDACTED,
REDACTED
	err := ValidateIntervals(intervals, BillingModeImage)
REDACTED
	require.Contains(t, err.Error(), "must be > min_tokens")
REDACTED

func TestSupportedModels_ExactKeysAndPricing(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 10, Platform: "anthropic", Models: []string{"claude-sonnet-4-6"REDACTED, InputPrice: testPtrFloat64(3e-6)REDACTED,
			{ID: 11, Platform: "anthropic", Models: []string{"claude-opus-4-6"REDACTED, InputPrice: testPtrFloat64(1.5e-5)REDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-4-6": "claude-sonnet-4-6",
				"claude-opus-4-6":   "claude-opus-4-6",
		REDACTED,
	REDACTED,
REDACTED

	got := ch.SupportedModels()
	require.Len(t, got, 2)
	require.Equal(t, "anthropic", got[0].Platform)
	require.Equal(t, "claude-opus-4-6", got[0].Name)
	require.NotNil(t, got[0].Pricing)
	require.Equal(t, int64(11), got[0].Pricing.ID)
	require.Equal(t, "claude-sonnet-4-6", got[1].Name)
	require.Equal(t, int64(10), got[1].Pricing.ID)
REDACTED

func TestSupportedModels_WildcardExpandedFromPricing(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6", "claude-sonnet-4-5"REDACTEDREDACTED,
			{ID: 2, Platform: "anthropic", Models: []string{"claude-opus-4-6"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-*": "claude-sonnet-4-6",
		REDACTED,
	REDACTED,
REDACTED

	got := ch.SupportedModels()
	names := make([]string, 0, len(got))
	for _, m := range got {
		names = append(names, m.Name)
REDACTED
	require.ElementsMatch(t, []string{"claude-sonnet-4-5", "claude-sonnet-4-6", "claude-opus-4-6"REDACTED, names)
	for _, m := range got {
		require.NotContains(t, m.Name, "*")
REDACTED
REDACTED

func TestSupportedModels_MissingPricingKeepsNilPricing(t *testing.T) {
	ch := &Channel{
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-4-6": "claude-sonnet-4-6"REDACTED,
	REDACTED,
REDACTED

	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "claude-sonnet-4-6", got[0].Name)
	require.Nil(t, got[0].Pricing)
REDACTED

func TestSupportedModels_DedupAndSort(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6", "claude-sonnet-4-5"REDACTEDREDACTED,
			{ID: 2, Platform: "openai", Models: []string{"gpt-4o"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"claude-sonnet-4-6": "upstream-a",
				"claude-sonnet-*":   "upstream-a",
		REDACTED,
			"openai": {"gpt-4o": "gpt-4o"REDACTED,
	REDACTED,
REDACTED

	got := ch.SupportedModels()
	require.Len(t, got, 3)
	require.Equal(t, "anthropic", got[0].Platform)
	require.Equal(t, "claude-sonnet-4-5", got[0].Name)
	require.Equal(t, "anthropic", got[1].Platform)
	require.Equal(t, "claude-sonnet-4-6", got[1].Name)
	require.Equal(t, "openai", got[2].Platform)
	require.Equal(t, "gpt-4o", got[2].Name)
REDACTED

func TestSupportedModels_NilChannelAndEmpty(t *testing.T) {
	var nilCh *Channel
	require.Nil(t, nilCh.SupportedModels())

	empty := &Channel{REDACTED
	require.Nil(t, empty.SupportedModels())
REDACTED

func TestGetModelPricingByPlatform(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6"REDACTED, InputPrice: testPtrFloat64(3e-6)REDACTED,
			{ID: 2, Platform: "openai", Models: []string{"claude-sonnet-4-6"REDACTED, InputPrice: testPtrFloat64(1e-6)REDACTED,
	REDACTED,
REDACTED

	ant := ch.GetModelPricingByPlatform("anthropic", "claude-sonnet-4-6")
	require.NotNil(t, ant)
	require.Equal(t, int64(1), ant.ID)

	oa := ch.GetModelPricingByPlatform("openai", "claude-sonnet-4-6")
	require.NotNil(t, oa)
	require.Equal(t, int64(2), oa.ID)

	require.Nil(t, ch.GetModelPricingByPlatform("gemini", "claude-sonnet-4-6"))
REDACTED

func TestSupportedModels_WildcardOnlyPricingRowsSkipped(t *testing.T) {
	// 定价中含通配符条目（pattern），不应被当作具体模型名展开。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-*", "claude-sonnet-4-6"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-*": "claude-sonnet-4-6"REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "claude-sonnet-4-6", got[0].Name)
	for _, m := range got {
		require.NotContains(t, m.Name, "*")
REDACTED
REDACTED

func TestSupportedModels_WildcardPrefixMatchesNothing(t *testing.T) {
	// 通配符模式无任何对应定价模型时，该平台 mapping 路不产出；
	// 但其他平台的 pricing-only 模型仍会通过 Pass B 出现。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"gpt-4o"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {"gpt-foo-*": "gpt-foo-1"REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "openai", got[0].Platform)
	require.Equal(t, "gpt-4o", got[0].Name)
REDACTED

func TestSupportedModels_CrossPlatformPricingDoesNotBleed(t *testing.T) {
	// anthropic 的通配符不应把 openai 定价行拉到 anthropic 平台下；
	// openai 的 pricing-only 模型则正常通过 Pass B 暴露在 openai 平台下。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"claude-sonnet-4-6"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet-*": "x"REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "openai", got[0].Platform, "不能把 openai 定价标记为 anthropic 模型")
	require.Equal(t, "claude-sonnet-4-6", got[0].Name)
REDACTED

func TestSupportedModels_CaseInsensitiveDedup(t *testing.T) {
	// 两行定价用不同大小写定义了同一模型，结果应去重为 1 条；首次出现的原始大小写保留。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"GPT-4o"REDACTEDREDACTED,
			{ID: 2, Platform: "openai", Models: []string{"gpt-4o"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"openai": {"gpt-*": "x"REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "GPT-4o", got[0].Name)
REDACTED

func TestSupportedModels_EmptyPlatformMapping(t *testing.T) {
	// ModelMapping 平台 key 存在但 value 为空 map：mapping 路跳过该平台，
	// 但 pricing 路仍会把该平台的定价模型补齐（关键修复：azcc 这种"只配定价不配映射"渠道）。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-sonnet-4-6"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "anthropic", got[0].Platform)
	require.Equal(t, "claude-sonnet-4-6", got[0].Name)
	require.NotNil(t, got[0].Pricing)
REDACTED

func TestSupportedModels_ExactKeyUsesPricedCaseWhenAvailable(t *testing.T) {
	// mapping key uses uppercase, pricing uses lowercase — pricing's case should win.
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"gpt-4o"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"openai": {"GPT-4o": "gpt-4o"REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 1)
	require.Equal(t, "gpt-4o", got[0].Name) // pricing's case wins
REDACTED

func TestSupportedModels_AsteriskOnlyMappingExpandsAllPriced(t *testing.T) {
	// 映射 key 为单独的 "*"：前缀为空 → 命中该平台所有定价模型（透传场景）。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "openai", Models: []string{"gpt-4o", "gpt-4o-mini"REDACTEDREDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"openai": {"*": "gpt-4o"REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 2)
	names := []string{got[0].Name, got[1].NameREDACTED
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4o-mini"REDACTED, names)
REDACTED

func TestSupportedModels_PricingOnlyNoMapping(t *testing.T) {
	// 渠道完全没配 mapping，只配了定价 —— 应该把所有定价模型作为支持模型返回。
	// 这是修复前的核心 bug 场景（前端显示"未配置模型"）。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"claude-opus-4-6"REDACTED, InputPrice: testPtrFloat64(1.5e-5)REDACTED,
			{ID: 2, Platform: "anthropic", Models: []string{"claude-haiku-4-5"REDACTED, InputPrice: testPtrFloat64(3e-7)REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 2)
	require.Equal(t, "claude-haiku-4-5", got[0].Name)
	require.NotNil(t, got[0].Pricing)
	require.Equal(t, int64(2), got[0].Pricing.ID)
	require.Equal(t, "claude-opus-4-6", got[1].Name)
	require.Equal(t, int64(1), got[1].Pricing.ID)
REDACTED

func TestSupportedModels_ExactMappingUsesTargetPricing(t *testing.T) {
	// 精确 mapping `src → target`：定价应按 target 查（实际计费的是 target），
	// 而不是按 src 自查。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 100, Platform: "anthropic", Models: []string{"req-model"REDACTED, InputPrice: testPtrFloat64(3e-6)REDACTED,
			{ID: 200, Platform: "anthropic", Models: []string{"served-model"REDACTED, InputPrice: testPtrFloat64(1.5e-5)REDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"req-model": "served-model",
		REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 2)
	require.Equal(t, "req-model", got[0].Name)
	require.NotNil(t, got[0].Pricing)
	require.Equal(t, int64(200), got[0].Pricing.ID, "req-model 显示但定价是 served-model 的（mapping target）")
	require.Equal(t, "served-model", got[1].Name)
	require.Equal(t, int64(200), got[1].Pricing.ID)
REDACTED

func TestSupportedModels_ExactMappingTargetMissingFromPricing(t *testing.T) {
	// `src → target` 但 target 不在渠道定价里 —— 结果中 src 的 Pricing 为 nil
	// （等待 ListAvailable 阶段的全局 LiteLLM 回落填充）。
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Platform: "anthropic", Models: []string{"some-priced-model"REDACTED, InputPrice: testPtrFloat64(1.5e-5)REDACTED,
	REDACTED,
		ModelMapping: map[string]map[string]string{
			"anthropic": {
				"missing-src": "missing-target",
		REDACTED,
	REDACTED,
REDACTED
	got := ch.SupportedModels()
	require.Len(t, got, 2)
	require.Equal(t, "missing-src", got[0].Name)
	require.Nil(t, got[0].Pricing, "target 在渠道定价中缺失时不虚假填充，留给 ListAvailable 走 LiteLLM 回落")
	require.Equal(t, "some-priced-model", got[1].Name)
	require.NotNil(t, got[1].Pricing)
REDACTED
