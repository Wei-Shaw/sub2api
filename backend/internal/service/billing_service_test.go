//go:build unit

package service

import (
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newTestBillingService() *BillingService {
	return NewBillingService(&config.Config{REDACTED, nil)
REDACTED

func TestCalculateCost_BasicComputation(t *testing.T) {
	svc := newTestBillingService()

	// 使用 claude-sonnet-4 的回退价格：Input $3/MTok, Output $15/MTok
	tokens := UsageTokens{
		InputTokens:  1000,
		OutputTokens: 500,
REDACTED
	cost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	// 1000 * 3e-6 = 0.003, 500 * 15e-6 = 0.0075
	expectedInput := 1000 * 3e-6
	expectedOutput := 500 * 15e-6
	require.InDelta(t, expectedInput, cost.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, cost.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.ActualCost, 1e-10)
REDACTED

func TestCalculateCost_WithCacheTokens(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:         1000,
		OutputTokens:        500,
		CacheCreationTokens: 2000,
		CacheReadTokens:     3000,
REDACTED
	cost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	expectedCacheCreation := 2000 * 3.75e-6
	expectedCacheRead := 3000 * 0.3e-6
	require.InDelta(t, expectedCacheCreation, cost.CacheCreationCost, 1e-10)
	require.InDelta(t, expectedCacheRead, cost.CacheReadCost, 1e-10)

	expectedTotal := cost.InputCost + cost.OutputCost + expectedCacheCreation + expectedCacheRead
	require.InDelta(t, expectedTotal, cost.TotalCost, 1e-10)
REDACTED

func TestCalculateCost_RateMultiplier(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500REDACTED

	cost1x, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	cost2x, err := svc.CalculateCost("claude-sonnet-4", tokens, 2.0)
REDACTED

	// TotalCost 不受倍率影响，ActualCost 翻倍
	require.InDelta(t, cost1x.TotalCost, cost2x.TotalCost, 1e-10)
	require.InDelta(t, cost1x.ActualCost*2, cost2x.ActualCost, 1e-10)
REDACTED

func TestCalculateCost_ZeroMultiplierDefaultsToOne(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 1000REDACTED

	costZero, err := svc.CalculateCost("claude-sonnet-4", tokens, 0)
REDACTED

	costOne, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	require.InDelta(t, costOne.ActualCost, costZero.ActualCost, 1e-10)
REDACTED

func TestCalculateCost_NegativeMultiplierDefaultsToOne(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 1000REDACTED

	costNeg, err := svc.CalculateCost("claude-sonnet-4", tokens, -1.0)
REDACTED

	costOne, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	require.InDelta(t, costOne.ActualCost, costNeg.ActualCost, 1e-10)
REDACTED

func TestGetModelPricing_FallbackMatchesByFamily(t *testing.T) {
	svc := newTestBillingService()

	tests := []struct {
		model         string
		expectedInput float64
REDACTED{
		{"claude-opus-4.5-20250101", 5e-6REDACTED,
		{"claude-3-opus-20240229", 15e-6REDACTED,
		{"claude-sonnet-4-20250514", 3e-6REDACTED,
		{"claude-3-5-sonnet-20241022", 3e-6REDACTED,
		{"claude-3-5-haiku-20241022", 1e-6REDACTED,
		{"claude-3-haiku-20240307", 0.25e-6REDACTED,
REDACTED

	for _, tt := range tests {
		pricing, err := svc.GetModelPricing(tt.model)
		require.NoError(t, err, "模型 %s", tt.model)
		require.InDelta(t, tt.expectedInput, pricing.InputPricePerToken, 1e-12, "模型 %s 输入价格", tt.model)
REDACTED
REDACTED

func TestGetModelPricing_CaseInsensitive(t *testing.T) {
	svc := newTestBillingService()

	p1, err := svc.GetModelPricing("Claude-Sonnet-4")
REDACTED

	p2, err := svc.GetModelPricing("claude-sonnet-4")
REDACTED

	require.Equal(t, p1.InputPricePerToken, p2.InputPricePerToken)
REDACTED

func TestGetModelPricing_UnknownModelFallsBackToSonnet(t *testing.T) {
	svc := newTestBillingService()

	// 不包含 opus/sonnet/haiku 关键词的 Claude 模型会走默认 Sonnet 价格
	pricing, err := svc.GetModelPricing("claude-unknown-model")
REDACTED
	require.InDelta(t, 3e-6, pricing.InputPricePerToken, 1e-12)
REDACTED

func TestCalculateCostWithLongContext_BelowThreshold(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:     50000,
		OutputTokens:    1000,
		CacheReadTokens: 100000,
REDACTED
	// 总输入 150k < 200k 阈值，应走正常计费
	cost, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 200000, 2.0)
REDACTED

	normalCost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	require.InDelta(t, normalCost.ActualCost, cost.ActualCost, 1e-10)
REDACTED

func TestCalculateCostWithLongContext_AboveThreshold_CacheExceedsThreshold(t *testing.T) {
	svc := newTestBillingService()

	// 缓存 210k + 输入 10k = 220k > 200k 阈值
	// 缓存已超阈值：范围内 200k 缓存，范围外 10k 缓存 + 10k 输入
	tokens := UsageTokens{
		InputTokens:     10000,
		OutputTokens:    1000,
		CacheReadTokens: 210000,
REDACTED
	cost, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 200000, 2.0)
REDACTED

	// 范围内：200k cache + 0 input + 1k output
	inRange, _ := svc.CalculateCost("claude-sonnet-4", UsageTokens{
		InputTokens:     0,
		OutputTokens:    1000,
		CacheReadTokens: 200000,
REDACTED, 1.0)

	// 范围外：10k cache + 10k input，倍率 2.0
	outRange, _ := svc.CalculateCost("claude-sonnet-4", UsageTokens{
		InputTokens:     10000,
		CacheReadTokens: 10000,
REDACTED, 2.0)

	require.InDelta(t, inRange.ActualCost+outRange.ActualCost, cost.ActualCost, 1e-10)
REDACTED

func TestCalculateCostWithLongContext_AboveThreshold_CacheBelowThreshold(t *testing.T) {
	svc := newTestBillingService()

	// 缓存 100k + 输入 150k = 250k > 200k 阈值
	// 缓存未超阈值：范围内 100k 缓存 + 100k 输入，范围外 50k 输入
	tokens := UsageTokens{
		InputTokens:     150000,
		OutputTokens:    1000,
		CacheReadTokens: 100000,
REDACTED
	cost, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 200000, 2.0)
REDACTED

	require.True(t, cost.ActualCost > 0, "费用应大于 0")

	// 正常费用不含长上下文
	normalCost, _ := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.True(t, cost.ActualCost > normalCost.ActualCost, "长上下文费用应高于正常费用")
REDACTED

func TestCalculateCostWithLongContext_DisabledThreshold(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 300000, CacheReadTokens: 0REDACTED

	// threshold <= 0 应禁用长上下文计费
	cost1, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 0, 2.0)
REDACTED

	cost2, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	require.InDelta(t, cost2.ActualCost, cost1.ActualCost, 1e-10)
REDACTED

func TestCalculateCostWithLongContext_ExtraMultiplierLessEqualOne(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 300000REDACTED

	// extraMultiplier <= 1 应禁用长上下文计费
	cost, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 200000, 1.0)
REDACTED

	normalCost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	require.InDelta(t, normalCost.ActualCost, cost.ActualCost, 1e-10)
REDACTED

func TestCalculateImageCost(t *testing.T) {
	svc := newTestBillingService()

	price := 0.134
	cfg := &ImagePriceConfig{Price1K: &priceREDACTED
	cost := svc.CalculateImageCost("gpt-image-1", "1K", 3, cfg, 1.0)

	require.InDelta(t, 0.134*3, cost.TotalCost, 1e-10)
	require.InDelta(t, 0.134*3, cost.ActualCost, 1e-10)
REDACTED

func TestCalculateSoraVideoCost(t *testing.T) {
	svc := newTestBillingService()

	price := 0.5
	cfg := &SoraPriceConfig{VideoPricePerRequest: &priceREDACTED
	cost := svc.CalculateSoraVideoCost("sora-video", cfg, 1.0)

	require.InDelta(t, 0.5, cost.TotalCost, 1e-10)
REDACTED

func TestCalculateSoraVideoCost_HDModel(t *testing.T) {
	svc := newTestBillingService()

	hdPrice := 1.0
	normalPrice := 0.5
	cfg := &SoraPriceConfig{
		VideoPricePerRequest:   &normalPrice,
		VideoPricePerRequestHD: &hdPrice,
REDACTED
	cost := svc.CalculateSoraVideoCost("sora2pro-hd", cfg, 1.0)
	require.InDelta(t, 1.0, cost.TotalCost, 1e-10)
REDACTED

func TestIsModelSupported(t *testing.T) {
	svc := newTestBillingService()

	require.True(t, svc.IsModelSupported("claude-sonnet-4"))
	require.True(t, svc.IsModelSupported("Claude-Opus-4.5"))
	require.True(t, svc.IsModelSupported("claude-3-haiku"))
	require.False(t, svc.IsModelSupported("gpt-4o"))
	require.False(t, svc.IsModelSupported("gemini-pro"))
REDACTED

func TestCalculateCost_ZeroTokens(t *testing.T) {
	svc := newTestBillingService()

	cost, err := svc.CalculateCost("claude-sonnet-4", UsageTokens{REDACTED, 1.0)
REDACTED
	require.Equal(t, 0.0, cost.TotalCost)
	require.Equal(t, 0.0, cost.ActualCost)
REDACTED

func TestCalculateCost_LargeTokenCount(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
REDACTED
	cost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
REDACTED

	// Input: 1M * 3e-6 = $3, Output: 1M * 15e-6 = $15
	require.InDelta(t, 3.0, cost.InputCost, 1e-6)
	require.InDelta(t, 15.0, cost.OutputCost, 1e-6)
	require.False(t, math.IsNaN(cost.TotalCost))
	require.False(t, math.IsInf(cost.TotalCost, 0))
REDACTED
