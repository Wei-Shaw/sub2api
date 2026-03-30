//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func channelTestPtrFloat64(v float64) *float64 { return &v REDACTED
func channelTestPtrInt(v int) *int             { return &v REDACTED

func TestGetModelPricing(t *testing.T) {
	ch := &Channel{
		ModelPricing: []ChannelModelPricing{
			{ID: 1, Models: []string{"claude-sonnet-4"REDACTED, BillingMode: BillingModeToken, InputPrice: channelTestPtrFloat64(3e-6)REDACTED,
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
			{ID: 1, Models: []string{"claude-sonnet-4"REDACTED, InputPrice: channelTestPtrFloat64(3e-6)REDACTED,
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
			{MinTokens: 0, MaxTokens: channelTestPtrInt(128000), InputPrice: channelTestPtrFloat64(1e-6)REDACTED,
			{MinTokens: 128000, MaxTokens: nil, InputPrice: channelTestPtrFloat64(2e-6)REDACTED,
	REDACTED,
REDACTED

	tests := []struct {
		name       string
		tokens     int
		wantPrice  *float64
		wantNil    bool
REDACTED{
		{"first interval", 50000, channelTestPtrFloat64(1e-6), falseREDACTED,
		// (min, max] — 128000 在第一个区间的 max，包含，所以匹配第一个
		{"boundary: max of first (inclusive)", 128000, channelTestPtrFloat64(1e-6), falseREDACTED,
		// 128001 > 128000，匹配第二个区间
		{"boundary: just above first max", 128001, channelTestPtrFloat64(2e-6), falseREDACTED,
		{"unbounded interval", 500000, channelTestPtrFloat64(2e-6), falseREDACTED,
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
			{MinTokens: 10000, MaxTokens: channelTestPtrInt(50000)REDACTED,
	REDACTED,
REDACTED
	require.Nil(t, p.GetIntervalForContext(5000))  // 5000 <= 10000, not > min
	require.Nil(t, p.GetIntervalForContext(10000)) // 10000 not > 10000 (left-open)
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
			{TierLabel: "1K", PerRequestPrice: channelTestPtrFloat64(0.04)REDACTED,
			{TierLabel: "2K", PerRequestPrice: channelTestPtrFloat64(0.08)REDACTED,
			{TierLabel: "HD", PerRequestPrice: channelTestPtrFloat64(0.12)REDACTED,
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
				InputPrice: channelTestPtrFloat64(5e-6),
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
