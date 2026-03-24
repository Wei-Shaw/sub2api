package service

import (
	"context"
	"fmt"

	"log"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// APIKeyRateLimitCacheData holds rate limit usage data cached in Redis.
type APIKeyRateLimitCacheData struct {
	Usage5h  float64 `json:"usage_5h"`
	Usage1d  float64 `json:"usage_1d"`
	Usage7d  float64 `json:"usage_7d"`
	Window5h int64   `json:"window_5h"` // unix timestamp, 0 = not started
	Window1d int64   `json:"window_1d"`
	Window7d int64   `json:"window_7d"`
REDACTED

// BillingCache defines cache operations for billing service
type BillingCache interface {
	// Balance operations
	GetUserBalance(ctx context.Context, userID int64) (float64, error)
	SetUserBalance(ctx context.Context, userID int64, balance float64) error
	DeductUserBalance(ctx context.Context, userID int64, amount float64) error
	InvalidateUserBalance(ctx context.Context, userID int64) error

	// Subscription operations
	GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*SubscriptionCacheData, error)
	SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *SubscriptionCacheData) error
	UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error
	InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error

	// API Key rate limit operations
	GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*APIKeyRateLimitCacheData, error)
	SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *APIKeyRateLimitCacheData) error
	UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error
	InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error
REDACTED

// ModelPricing 模型价格配置（per-token价格，与LiteLLM格式一致）
type ModelPricing struct {
	InputPricePerToken             float64 // 每token输入价格 (USD)
	InputPricePerTokenPriority     float64 // priority service tier 下每token输入价格 (USD)
	OutputPricePerToken            float64 // 每token输出价格 (USD)
	OutputPricePerTokenPriority    float64 // priority service tier 下每token输出价格 (USD)
	CacheCreationPricePerToken     float64 // 缓存创建每token价格 (USD)
	CacheReadPricePerToken         float64 // 缓存读取每token价格 (USD)
	CacheReadPricePerTokenPriority float64 // priority service tier 下缓存读取每token价格 (USD)
	CacheCreation5mPrice           float64 // 5分钟缓存创建每token价格 (USD)
	CacheCreation1hPrice           float64 // 1小时缓存创建每token价格 (USD)
	SupportsCacheBreakdown         bool    // 是否支持详细的缓存分类
	LongContextInputThreshold      int     // 超过阈值后按整次会话提升输入价格
	LongContextInputMultiplier     float64 // 长上下文整次会话输入倍率
	LongContextOutputMultiplier    float64 // 长上下文整次会话输出倍率
REDACTED

const (
	openAIGPT54LongContextInputThreshold   = 272000
	openAIGPT54LongContextInputMultiplier  = 2.0
	openAIGPT54LongContextOutputMultiplier = 1.5
)

func normalizeBillingServiceTier(serviceTier string) string {
	return strings.ToLower(strings.TrimSpace(serviceTier))
REDACTED

func usePriorityServiceTierPricing(serviceTier string, pricing *ModelPricing) bool {
	if pricing == nil || normalizeBillingServiceTier(serviceTier) != "priority" {
		return false
REDACTED
	return pricing.InputPricePerTokenPriority > 0 || pricing.OutputPricePerTokenPriority > 0 || pricing.CacheReadPricePerTokenPriority > 0
REDACTED

func serviceTierCostMultiplier(serviceTier string) float64 {
	switch normalizeBillingServiceTier(serviceTier) {
	case "priority":
		return 2.0
	case "flex":
		return 0.5
	default:
		return 1.0
REDACTED
REDACTED

// UsageTokens 使用的token数量
type UsageTokens struct {
	InputTokens           int
	OutputTokens          int
	CacheCreationTokens   int
	CacheReadTokens       int
	CacheCreation5mTokens int
	CacheCreation1hTokens int
REDACTED

// CostBreakdown 费用明细
type CostBreakdown struct {
	InputCost         float64
	OutputCost        float64
	CacheCreationCost float64
	CacheReadCost     float64
	TotalCost         float64
	ActualCost        float64 // 应用倍率后的实际费用
REDACTED

// BillingService 计费服务
type BillingService struct {
	cfg            *config.Config
	pricingService *PricingService
	fallbackPrices map[string]*ModelPricing // 硬编码回退价格
REDACTED

// NewBillingService 创建计费服务实例
func NewBillingService(cfg *config.Config, pricingService *PricingService) *BillingService {
	s := &BillingService{
		cfg:            cfg,
		pricingService: pricingService,
		fallbackPrices: make(map[string]*ModelPricing),
REDACTED

	// 初始化硬编码回退价格（当动态价格不可用时使用）
	s.initFallbackPricing()

	return s
REDACTED

// initFallbackPricing 初始化硬编码回退价格（当动态价格不可用时使用）
// 价格单位：USD per token（与LiteLLM格式一致）
func (s *BillingService) initFallbackPricing() {
	// Claude 4.5 Opus
	s.fallbackPrices["claude-opus-4.5"] = &ModelPricing{
		InputPricePerToken:         5e-6,    // $5 per MTok
		OutputPricePerToken:        25e-6,   // $25 per MTok
		CacheCreationPricePerToken: 6.25e-6, // $6.25 per MTok
		CacheReadPricePerToken:     0.5e-6,  // $0.50 per MTok
		SupportsCacheBreakdown:     false,
REDACTED

	// Claude 4 Sonnet
	s.fallbackPrices["claude-sonnet-4"] = &ModelPricing{
		InputPricePerToken:         3e-6,    // $3 per MTok
		OutputPricePerToken:        15e-6,   // $15 per MTok
		CacheCreationPricePerToken: 3.75e-6, // $3.75 per MTok
		CacheReadPricePerToken:     0.3e-6,  // $0.30 per MTok
		SupportsCacheBreakdown:     false,
REDACTED

	// Claude 3.5 Sonnet
	s.fallbackPrices["claude-3-5-sonnet"] = &ModelPricing{
		InputPricePerToken:         3e-6,    // $3 per MTok
		OutputPricePerToken:        15e-6,   // $15 per MTok
		CacheCreationPricePerToken: 3.75e-6, // $3.75 per MTok
		CacheReadPricePerToken:     0.3e-6,  // $0.30 per MTok
		SupportsCacheBreakdown:     false,
REDACTED

	// Claude 3.5 Haiku
	s.fallbackPrices["claude-3-5-haiku"] = &ModelPricing{
		InputPricePerToken:         1e-6,    // $1 per MTok
		OutputPricePerToken:        5e-6,    // $5 per MTok
		CacheCreationPricePerToken: 1.25e-6, // $1.25 per MTok
		CacheReadPricePerToken:     0.1e-6,  // $0.10 per MTok
		SupportsCacheBreakdown:     false,
REDACTED

	// Claude 3 Opus
	s.fallbackPrices["claude-3-opus"] = &ModelPricing{
		InputPricePerToken:         15e-6,    // $15 per MTok
		OutputPricePerToken:        75e-6,    // $75 per MTok
		CacheCreationPricePerToken: 18.75e-6, // $18.75 per MTok
		CacheReadPricePerToken:     1.5e-6,   // $1.50 per MTok
		SupportsCacheBreakdown:     false,
REDACTED

	// Claude 3 Haiku
	s.fallbackPrices["claude-3-haiku"] = &ModelPricing{
		InputPricePerToken:         0.25e-6, // $0.25 per MTok
		OutputPricePerToken:        1.25e-6, // $1.25 per MTok
		CacheCreationPricePerToken: 0.3e-6,  // $0.30 per MTok
		CacheReadPricePerToken:     0.03e-6, // $0.03 per MTok
		SupportsCacheBreakdown:     false,
REDACTED

	// Claude 4.6 Opus (与4.5同价)
	s.fallbackPrices["claude-opus-4.6"] = s.fallbackPrices["claude-opus-4.5"]

	// Gemini 3.1 Pro
	s.fallbackPrices["gemini-3.1-pro"] = &ModelPricing{
		InputPricePerToken:         2e-6,   // $2 per MTok
		OutputPricePerToken:        12e-6,  // $12 per MTok
		CacheCreationPricePerToken: 2e-6,   // $2 per MTok
		CacheReadPricePerToken:     0.2e-6, // $0.20 per MTok
		SupportsCacheBreakdown:     false,
REDACTED

	// OpenAI GPT-5.1（本地兜底，防止动态定价不可用时拒绝计费）
	s.fallbackPrices["gpt-5.1"] = &ModelPricing{
		InputPricePerToken:             1.25e-6, // $1.25 per MTok
		InputPricePerTokenPriority:     2.5e-6,  // $2.5 per MTok
		OutputPricePerToken:            10e-6,   // $10 per MTok
		OutputPricePerTokenPriority:    20e-6,   // $20 per MTok
		CacheCreationPricePerToken:     1.25e-6, // $1.25 per MTok
		CacheReadPricePerToken:         0.125e-6,
		CacheReadPricePerTokenPriority: 0.25e-6,
		SupportsCacheBreakdown:         false,
REDACTED
	// OpenAI GPT-5.4（业务指定价格）
	s.fallbackPrices["gpt-5.4"] = &ModelPricing{
		InputPricePerToken:             2.5e-6,  // $2.5 per MTok
		InputPricePerTokenPriority:     5e-6,    // $5 per MTok
		OutputPricePerToken:            15e-6,   // $15 per MTok
		OutputPricePerTokenPriority:    30e-6,   // $30 per MTok
		CacheCreationPricePerToken:     2.5e-6,  // $2.5 per MTok
		CacheReadPricePerToken:         0.25e-6, // $0.25 per MTok
		CacheReadPricePerTokenPriority: 0.5e-6,  // $0.5 per MTok
		SupportsCacheBreakdown:         false,
		LongContextInputThreshold:      openAIGPT54LongContextInputThreshold,
		LongContextInputMultiplier:     openAIGPT54LongContextInputMultiplier,
		LongContextOutputMultiplier:    openAIGPT54LongContextOutputMultiplier,
REDACTED
	s.fallbackPrices["gpt-5.4-mini"] = &ModelPricing{
		InputPricePerToken:     7.5e-7,
		OutputPricePerToken:    4.5e-6,
		CacheReadPricePerToken: 7.5e-8,
		SupportsCacheBreakdown: false,
REDACTED
	s.fallbackPrices["gpt-5.4-nano"] = &ModelPricing{
		InputPricePerToken:     2e-7,
		OutputPricePerToken:    1.25e-6,
		CacheReadPricePerToken: 2e-8,
		SupportsCacheBreakdown: false,
REDACTED
	// OpenAI GPT-5.2（本地兜底）
	s.fallbackPrices["gpt-5.2"] = &ModelPricing{
		InputPricePerToken:             1.75e-6,
		InputPricePerTokenPriority:     3.5e-6,
		OutputPricePerToken:            14e-6,
		OutputPricePerTokenPriority:    28e-6,
		CacheCreationPricePerToken:     1.75e-6,
		CacheReadPricePerToken:         0.175e-6,
		CacheReadPricePerTokenPriority: 0.35e-6,
		SupportsCacheBreakdown:         false,
REDACTED
	// Codex 族兜底统一按 GPT-5.1 Codex 价格计费
	s.fallbackPrices["gpt-5.1-codex"] = &ModelPricing{
		InputPricePerToken:             1.5e-6, // $1.5 per MTok
		InputPricePerTokenPriority:     3e-6,   // $3 per MTok
		OutputPricePerToken:            12e-6,  // $12 per MTok
		OutputPricePerTokenPriority:    24e-6,  // $24 per MTok
		CacheCreationPricePerToken:     1.5e-6, // $1.5 per MTok
		CacheReadPricePerToken:         0.15e-6,
		CacheReadPricePerTokenPriority: 0.3e-6,
		SupportsCacheBreakdown:         false,
REDACTED
	s.fallbackPrices["gpt-5.2-codex"] = &ModelPricing{
		InputPricePerToken:             1.75e-6,
		InputPricePerTokenPriority:     3.5e-6,
		OutputPricePerToken:            14e-6,
		OutputPricePerTokenPriority:    28e-6,
		CacheCreationPricePerToken:     1.75e-6,
		CacheReadPricePerToken:         0.175e-6,
		CacheReadPricePerTokenPriority: 0.35e-6,
		SupportsCacheBreakdown:         false,
REDACTED
	s.fallbackPrices["gpt-5.3-codex"] = s.fallbackPrices["gpt-5.1-codex"]
REDACTED

// getFallbackPricing 根据模型系列获取回退价格
func (s *BillingService) getFallbackPricing(model string) *ModelPricing {
	modelLower := strings.ToLower(model)

	// 按模型系列匹配
	if strings.Contains(modelLower, "opus") {
		if strings.Contains(modelLower, "4.6") || strings.Contains(modelLower, "4-6") {
			return s.fallbackPrices["claude-opus-4.6"]
	REDACTED
		if strings.Contains(modelLower, "4.5") || strings.Contains(modelLower, "4-5") {
			return s.fallbackPrices["claude-opus-4.5"]
	REDACTED
		return s.fallbackPrices["claude-3-opus"]
REDACTED
	if strings.Contains(modelLower, "sonnet") {
		if strings.Contains(modelLower, "4") && !strings.Contains(modelLower, "3") {
			return s.fallbackPrices["claude-sonnet-4"]
	REDACTED
		return s.fallbackPrices["claude-3-5-sonnet"]
REDACTED
	if strings.Contains(modelLower, "haiku") {
		if strings.Contains(modelLower, "3-5") || strings.Contains(modelLower, "3.5") {
			return s.fallbackPrices["claude-3-5-haiku"]
	REDACTED
		return s.fallbackPrices["claude-3-haiku"]
REDACTED
	// Claude 未知型号统一回退到 Sonnet，避免计费中断。
	if strings.Contains(modelLower, "claude") {
		return s.fallbackPrices["claude-sonnet-4"]
REDACTED
	if strings.Contains(modelLower, "gemini-3.1-pro") || strings.Contains(modelLower, "gemini-3-1-pro") {
		return s.fallbackPrices["gemini-3.1-pro"]
REDACTED

	// OpenAI 仅匹配已知 GPT-5/Codex 族，避免未知 OpenAI 型号误计价。
	if strings.Contains(modelLower, "gpt-5") || strings.Contains(modelLower, "codex") {
		normalized := normalizeCodexModel(modelLower)
		switch normalized {
		case "gpt-5.4-mini":
			return s.fallbackPrices["gpt-5.4-mini"]
		case "gpt-5.4-nano":
			return s.fallbackPrices["gpt-5.4-nano"]
		case "gpt-5.4":
			return s.fallbackPrices["gpt-5.4"]
		case "gpt-5.2":
			return s.fallbackPrices["gpt-5.2"]
		case "gpt-5.2-codex":
			return s.fallbackPrices["gpt-5.2-codex"]
		case "gpt-5.3-codex":
			return s.fallbackPrices["gpt-5.3-codex"]
		case "gpt-5.1-codex", "gpt-5.1-codex-max", "gpt-5.1-codex-mini", "codex-mini-latest":
			return s.fallbackPrices["gpt-5.1-codex"]
		case "gpt-5.1":
			return s.fallbackPrices["gpt-5.1"]
	REDACTED
REDACTED

	return nil
REDACTED

// GetModelPricing 获取模型价格配置
func (s *BillingService) GetModelPricing(model string) (*ModelPricing, error) {
	// 标准化模型名称（转小写）
	model = strings.ToLower(model)

	// 1. 优先从动态价格服务获取
	if s.pricingService != nil {
		litellmPricing := s.pricingService.GetModelPricing(model)
		if litellmPricing != nil {
			// 启用 5m/1h 分类计费的条件：
			// 1. 存在 1h 价格
			// 2. 1h 价格 > 5m 价格（防止 LiteLLM 数据错误导致少收费）
			price5m := litellmPricing.CacheCreationInputTokenCost
			price1h := litellmPricing.CacheCreationInputTokenCostAbove1hr
			enableBreakdown := price1h > 0 && price1h > price5m
			return s.applyModelSpecificPricingPolicy(model, &ModelPricing{
				InputPricePerToken:             litellmPricing.InputCostPerToken,
				InputPricePerTokenPriority:     litellmPricing.InputCostPerTokenPriority,
				OutputPricePerToken:            litellmPricing.OutputCostPerToken,
				OutputPricePerTokenPriority:    litellmPricing.OutputCostPerTokenPriority,
				CacheCreationPricePerToken:     litellmPricing.CacheCreationInputTokenCost,
				CacheReadPricePerToken:         litellmPricing.CacheReadInputTokenCost,
				CacheReadPricePerTokenPriority: litellmPricing.CacheReadInputTokenCostPriority,
				CacheCreation5mPrice:           price5m,
				CacheCreation1hPrice:           price1h,
				SupportsCacheBreakdown:         enableBreakdown,
				LongContextInputThreshold:      litellmPricing.LongContextInputTokenThreshold,
				LongContextInputMultiplier:     litellmPricing.LongContextInputCostMultiplier,
				LongContextOutputMultiplier:    litellmPricing.LongContextOutputCostMultiplier,
		REDACTED), nil
	REDACTED
REDACTED

	// 2. 使用硬编码回退价格
	fallback := s.getFallbackPricing(model)
	if fallback != nil {
		log.Printf("[Billing] Using fallback pricing for model: %s", model)
		return s.applyModelSpecificPricingPolicy(model, fallback), nil
REDACTED

	return nil, fmt.Errorf("pricing not found for model: %s", model)
REDACTED

// CalculateCost 计算使用费用
func (s *BillingService) CalculateCost(model string, tokens UsageTokens, rateMultiplier float64) (*CostBreakdown, error) {
	return s.CalculateCostWithServiceTier(model, tokens, rateMultiplier, "")
REDACTED

func (s *BillingService) CalculateCostWithServiceTier(model string, tokens UsageTokens, rateMultiplier float64, serviceTier string) (*CostBreakdown, error) {
	pricing, err := s.GetModelPricing(model)
	if err != nil {
		return nil, err
REDACTED

	breakdown := &CostBreakdown{REDACTED
	inputPricePerToken := pricing.InputPricePerToken
	outputPricePerToken := pricing.OutputPricePerToken
	cacheReadPricePerToken := pricing.CacheReadPricePerToken
	tierMultiplier := 1.0
	if usePriorityServiceTierPricing(serviceTier, pricing) {
		if pricing.InputPricePerTokenPriority > 0 {
			inputPricePerToken = pricing.InputPricePerTokenPriority
	REDACTED
		if pricing.OutputPricePerTokenPriority > 0 {
			outputPricePerToken = pricing.OutputPricePerTokenPriority
	REDACTED
		if pricing.CacheReadPricePerTokenPriority > 0 {
			cacheReadPricePerToken = pricing.CacheReadPricePerTokenPriority
	REDACTED
REDACTED else {
		tierMultiplier = serviceTierCostMultiplier(serviceTier)
REDACTED
	if s.shouldApplySessionLongContextPricing(tokens, pricing) {
		inputPricePerToken *= pricing.LongContextInputMultiplier
		outputPricePerToken *= pricing.LongContextOutputMultiplier
REDACTED

	// 计算输入token费用（使用per-token价格）
	breakdown.InputCost = float64(tokens.InputTokens) * inputPricePerToken

	// 计算输出token费用
	breakdown.OutputCost = float64(tokens.OutputTokens) * outputPricePerToken

	// 计算缓存费用
	if pricing.SupportsCacheBreakdown && (pricing.CacheCreation5mPrice > 0 || pricing.CacheCreation1hPrice > 0) {
		// 支持详细缓存分类的模型（5分钟/1小时缓存，价格为 per-token）
		if tokens.CacheCreation5mTokens == 0 && tokens.CacheCreation1hTokens == 0 && tokens.CacheCreationTokens > 0 {
			// API 未返回 ephemeral 明细，回退到全部按 5m 单价计费
			breakdown.CacheCreationCost = float64(tokens.CacheCreationTokens) * pricing.CacheCreation5mPrice
	REDACTED else {
			breakdown.CacheCreationCost = float64(tokens.CacheCreation5mTokens)*pricing.CacheCreation5mPrice +
				float64(tokens.CacheCreation1hTokens)*pricing.CacheCreation1hPrice
	REDACTED
REDACTED else {
		// 标准缓存创建价格（per-token）
		breakdown.CacheCreationCost = float64(tokens.CacheCreationTokens) * pricing.CacheCreationPricePerToken
REDACTED

	breakdown.CacheReadCost = float64(tokens.CacheReadTokens) * cacheReadPricePerToken

	if tierMultiplier != 1.0 {
		breakdown.InputCost *= tierMultiplier
		breakdown.OutputCost *= tierMultiplier
		breakdown.CacheCreationCost *= tierMultiplier
		breakdown.CacheReadCost *= tierMultiplier
REDACTED

	// 计算总费用
	breakdown.TotalCost = breakdown.InputCost + breakdown.OutputCost +
		breakdown.CacheCreationCost + breakdown.CacheReadCost

	// 应用倍率计算实际费用
	if rateMultiplier <= 0 {
		rateMultiplier = 1.0
REDACTED
	breakdown.ActualCost = breakdown.TotalCost * rateMultiplier

	return breakdown, nil
REDACTED

func (s *BillingService) applyModelSpecificPricingPolicy(model string, pricing *ModelPricing) *ModelPricing {
	if pricing == nil {
		return nil
REDACTED
	if !isOpenAIGPT54Model(model) {
		return pricing
REDACTED
	if pricing.LongContextInputThreshold > 0 && pricing.LongContextInputMultiplier > 0 && pricing.LongContextOutputMultiplier > 0 {
		return pricing
REDACTED
	cloned := *pricing
	if cloned.LongContextInputThreshold <= 0 {
		cloned.LongContextInputThreshold = openAIGPT54LongContextInputThreshold
REDACTED
	if cloned.LongContextInputMultiplier <= 0 {
		cloned.LongContextInputMultiplier = openAIGPT54LongContextInputMultiplier
REDACTED
	if cloned.LongContextOutputMultiplier <= 0 {
		cloned.LongContextOutputMultiplier = openAIGPT54LongContextOutputMultiplier
REDACTED
	return &cloned
REDACTED

func (s *BillingService) shouldApplySessionLongContextPricing(tokens UsageTokens, pricing *ModelPricing) bool {
	if pricing == nil || pricing.LongContextInputThreshold <= 0 {
		return false
REDACTED
	if pricing.LongContextInputMultiplier <= 1 && pricing.LongContextOutputMultiplier <= 1 {
		return false
REDACTED
	totalInputTokens := tokens.InputTokens + tokens.CacheReadTokens
	return totalInputTokens > pricing.LongContextInputThreshold
REDACTED

func isOpenAIGPT54Model(model string) bool {
	normalized := normalizeCodexModel(strings.TrimSpace(strings.ToLower(model)))
	return normalized == "gpt-5.4"
REDACTED

// CalculateCostWithConfig 使用配置中的默认倍率计算费用
func (s *BillingService) CalculateCostWithConfig(model string, tokens UsageTokens) (*CostBreakdown, error) {
	multiplier := s.cfg.Default.RateMultiplier
	if multiplier <= 0 {
		multiplier = 1.0
REDACTED
	return s.CalculateCost(model, tokens, multiplier)
REDACTED

// CalculateCostWithLongContext 计算费用，支持长上下文双倍计费
// threshold: 阈值（如 200000），超过此值的部分按 extraMultiplier 倍计费
// extraMultiplier: 超出部分的倍率（如 2.0 表示双倍）
//
// 示例：缓存 210k + 输入 10k = 220k，阈值 200k，倍率 2.0
// 拆分为：范围内 (200k, 0) + 范围外 (10k, 10k)
// 范围内正常计费，范围外 × 2 计费
func (s *BillingService) CalculateCostWithLongContext(model string, tokens UsageTokens, rateMultiplier float64, threshold int, extraMultiplier float64) (*CostBreakdown, error) {
	// 未启用长上下文计费，直接走正常计费
	if threshold <= 0 || extraMultiplier <= 1 {
		return s.CalculateCost(model, tokens, rateMultiplier)
REDACTED

	// 计算总输入 token（缓存读取 + 新输入）
	total := tokens.CacheReadTokens + tokens.InputTokens
	if total <= threshold {
		return s.CalculateCost(model, tokens, rateMultiplier)
REDACTED

	// 拆分成范围内和范围外
	var inRangeCacheTokens, inRangeInputTokens int
	var outRangeCacheTokens, outRangeInputTokens int

	if tokens.CacheReadTokens >= threshold {
		// 缓存已超过阈值：范围内只有缓存，范围外是超出的缓存+全部输入
		inRangeCacheTokens = threshold
		inRangeInputTokens = 0
		outRangeCacheTokens = tokens.CacheReadTokens - threshold
		outRangeInputTokens = tokens.InputTokens
REDACTED else {
		// 缓存未超过阈值：范围内是全部缓存+部分输入，范围外是剩余输入
		inRangeCacheTokens = tokens.CacheReadTokens
		inRangeInputTokens = threshold - tokens.CacheReadTokens
		outRangeCacheTokens = 0
		outRangeInputTokens = tokens.InputTokens - inRangeInputTokens
REDACTED

	// 范围内部分：正常计费
	inRangeTokens := UsageTokens{
		InputTokens:           inRangeInputTokens,
		OutputTokens:          tokens.OutputTokens, // 输出只算一次
		CacheCreationTokens:   tokens.CacheCreationTokens,
		CacheReadTokens:       inRangeCacheTokens,
		CacheCreation5mTokens: tokens.CacheCreation5mTokens,
		CacheCreation1hTokens: tokens.CacheCreation1hTokens,
REDACTED
	inRangeCost, err := s.CalculateCost(model, inRangeTokens, rateMultiplier)
	if err != nil {
		return nil, err
REDACTED

	// 范围外部分：× extraMultiplier 计费
	outRangeTokens := UsageTokens{
		InputTokens:     outRangeInputTokens,
		CacheReadTokens: outRangeCacheTokens,
REDACTED
	outRangeCost, err := s.CalculateCost(model, outRangeTokens, rateMultiplier*extraMultiplier)
	if err != nil {
		return inRangeCost, fmt.Errorf("out-range cost: %w", err)
REDACTED

	// 合并成本
	return &CostBreakdown{
		InputCost:         inRangeCost.InputCost + outRangeCost.InputCost,
		OutputCost:        inRangeCost.OutputCost,
		CacheCreationCost: inRangeCost.CacheCreationCost,
		CacheReadCost:     inRangeCost.CacheReadCost + outRangeCost.CacheReadCost,
		TotalCost:         inRangeCost.TotalCost + outRangeCost.TotalCost,
		ActualCost:        inRangeCost.ActualCost + outRangeCost.ActualCost,
REDACTED, nil
REDACTED

// ListSupportedModels 列出所有支持的模型（现在总是返回true，因为有模糊匹配）
func (s *BillingService) ListSupportedModels() []string {
	models := make([]string, 0)
	// 返回回退价格支持的模型系列
	for model := range s.fallbackPrices {
		models = append(models, model)
REDACTED
	return models
REDACTED

// IsModelSupported 检查模型是否支持（现在总是返回true，因为有模糊匹配回退）
func (s *BillingService) IsModelSupported(model string) bool {
	// 所有Claude模型都有回退价格支持
	modelLower := strings.ToLower(model)
	return strings.Contains(modelLower, "claude") ||
		strings.Contains(modelLower, "opus") ||
		strings.Contains(modelLower, "sonnet") ||
		strings.Contains(modelLower, "haiku")
REDACTED

// GetEstimatedCost 估算费用（用于前端展示）
func (s *BillingService) GetEstimatedCost(model string, estimatedInputTokens, estimatedOutputTokens int) (float64, error) {
	tokens := UsageTokens{
		InputTokens:  estimatedInputTokens,
		OutputTokens: estimatedOutputTokens,
REDACTED

	breakdown, err := s.CalculateCostWithConfig(model, tokens)
	if err != nil {
		return 0, err
REDACTED

	return breakdown.ActualCost, nil
REDACTED

// GetPricingServiceStatus 获取价格服务状态
func (s *BillingService) GetPricingServiceStatus() map[string]any {
	if s.pricingService != nil {
		return s.pricingService.GetStatus()
REDACTED
	return map[string]any{
		"model_count":  len(s.fallbackPrices),
		"last_updated": "using fallback",
		"local_hash":   "N/A",
REDACTED
REDACTED

// ForceUpdatePricing 强制更新价格数据
func (s *BillingService) ForceUpdatePricing() error {
	if s.pricingService != nil {
		return s.pricingService.ForceUpdate()
REDACTED
	return fmt.Errorf("pricing service not initialized")
REDACTED

// ImagePriceConfig 图片计费配置
type ImagePriceConfig struct {
	Price1K *float64 // 1K 尺寸价格（nil 表示使用默认值）
	Price2K *float64 // 2K 尺寸价格（nil 表示使用默认值）
	Price4K *float64 // 4K 尺寸价格（nil 表示使用默认值）
REDACTED

// SoraPriceConfig Sora 按次计费配置
type SoraPriceConfig struct {
	ImagePrice360          *float64
	ImagePrice540          *float64
	VideoPricePerRequest   *float64
	VideoPricePerRequestHD *float64
REDACTED

// CalculateImageCost 计算图片生成费用
// model: 请求的模型名称（用于获取 LiteLLM 默认价格）
// imageSize: 图片尺寸 "1K", "2K", "4K"
// imageCount: 生成的图片数量
// groupConfig: 分组配置的价格（可能为 nil，表示使用默认值）
// rateMultiplier: 费率倍数
func (s *BillingService) CalculateImageCost(model string, imageSize string, imageCount int, groupConfig *ImagePriceConfig, rateMultiplier float64) *CostBreakdown {
	if imageCount <= 0 {
		return &CostBreakdown{REDACTED
REDACTED

	// 获取单价
	unitPrice := s.getImageUnitPrice(model, imageSize, groupConfig)

	// 计算总费用
	totalCost := unitPrice * float64(imageCount)

	// 应用倍率
	if rateMultiplier <= 0 {
		rateMultiplier = 1.0
REDACTED
	actualCost := totalCost * rateMultiplier

	return &CostBreakdown{
		TotalCost:  totalCost,
		ActualCost: actualCost,
REDACTED
REDACTED

// CalculateSoraImageCost 计算 Sora 图片按次费用
func (s *BillingService) CalculateSoraImageCost(imageSize string, imageCount int, groupConfig *SoraPriceConfig, rateMultiplier float64) *CostBreakdown {
	if imageCount <= 0 {
		return &CostBreakdown{REDACTED
REDACTED

	unitPrice := 0.0
	if groupConfig != nil {
		switch imageSize {
		case "540":
			if groupConfig.ImagePrice540 != nil {
				unitPrice = *groupConfig.ImagePrice540
		REDACTED
		default:
			if groupConfig.ImagePrice360 != nil {
				unitPrice = *groupConfig.ImagePrice360
		REDACTED
	REDACTED
REDACTED

	totalCost := unitPrice * float64(imageCount)
	if rateMultiplier <= 0 {
		rateMultiplier = 1.0
REDACTED
	actualCost := totalCost * rateMultiplier

	return &CostBreakdown{
		TotalCost:  totalCost,
		ActualCost: actualCost,
REDACTED
REDACTED

// CalculateSoraVideoCost 计算 Sora 视频按次费用
func (s *BillingService) CalculateSoraVideoCost(model string, groupConfig *SoraPriceConfig, rateMultiplier float64) *CostBreakdown {
	unitPrice := 0.0
	if groupConfig != nil {
		modelLower := strings.ToLower(model)
		if strings.Contains(modelLower, "sora2pro-hd") {
			if groupConfig.VideoPricePerRequestHD != nil {
				unitPrice = *groupConfig.VideoPricePerRequestHD
		REDACTED
	REDACTED
		if unitPrice <= 0 && groupConfig.VideoPricePerRequest != nil {
			unitPrice = *groupConfig.VideoPricePerRequest
	REDACTED
REDACTED

	totalCost := unitPrice
	if rateMultiplier <= 0 {
		rateMultiplier = 1.0
REDACTED
	actualCost := totalCost * rateMultiplier

	return &CostBreakdown{
		TotalCost:  totalCost,
		ActualCost: actualCost,
REDACTED
REDACTED

// getImageUnitPrice 获取图片单价
func (s *BillingService) getImageUnitPrice(model string, imageSize string, groupConfig *ImagePriceConfig) float64 {
	// 优先使用分组配置的价格
	if groupConfig != nil {
		switch imageSize {
		case "1K":
			if groupConfig.Price1K != nil {
				return *groupConfig.Price1K
		REDACTED
		case "2K":
			if groupConfig.Price2K != nil {
				return *groupConfig.Price2K
		REDACTED
		case "4K":
			if groupConfig.Price4K != nil {
				return *groupConfig.Price4K
		REDACTED
	REDACTED
REDACTED

	// 回退到 LiteLLM 默认价格
	return s.getDefaultImagePrice(model, imageSize)
REDACTED

// getDefaultImagePrice 获取 LiteLLM 默认图片价格
func (s *BillingService) getDefaultImagePrice(model string, imageSize string) float64 {
	basePrice := 0.0

	// 从 PricingService 获取 output_cost_per_image
	if s.pricingService != nil {
		pricing := s.pricingService.GetModelPricing(model)
		if pricing != nil && pricing.OutputCostPerImage > 0 {
			basePrice = pricing.OutputCostPerImage
	REDACTED
REDACTED

	// 如果没有找到价格，使用硬编码默认值（$0.134，来自 gemini-3-pro-image-preview）
	if basePrice <= 0 {
		basePrice = 0.134
REDACTED

	// 2K 尺寸 1.5 倍，4K 尺寸翻倍
	if imageSize == "2K" {
		return basePrice * 1.5
REDACTED
	if imageSize == "4K" {
		return basePrice * 2
REDACTED

	return basePrice
REDACTED
