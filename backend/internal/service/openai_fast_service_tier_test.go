package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ---------------------------------------------------------------------------
// 请求侧：service_tier 校验（fast/priority 等价、非法值拒绝、省略保持现状）
// ---------------------------------------------------------------------------

func TestValidateOpenAIServiceTierField(t *testing.T) {
	t.Parallel()

	t.Run("fast normalizes to priority", func(t *testing.T) {
		norm, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","service_tier":"fast"REDACTED`))
	REDACTED
		require.Equal(t, "priority", norm)
REDACTED)

	t.Run("priority passes through", func(t *testing.T) {
		norm, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","service_tier":"priority"REDACTED`))
	REDACTED
		require.Equal(t, "priority", norm)
REDACTED)

	t.Run("case and whitespace insensitive", func(t *testing.T) {
		norm, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","service_tier":"  FAST "REDACTED`))
	REDACTED
		require.Equal(t, "priority", norm)
REDACTED)

	t.Run("official tiers pass through", func(t *testing.T) {
		for _, tier := range []string{"flex", "auto", "default", "scale"REDACTED {
			norm, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","service_tier":"` + tier + `"REDACTED`))
			require.NoError(t, err, "tier %q must be accepted", tier)
			require.Equal(t, tier, norm)
	REDACTED
REDACTED)

	t.Run("invalid tier rejected", func(t *testing.T) {
		_, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","service_tier":"turbo"REDACTED`))
	REDACTED
		var invalid *ErrInvalidOpenAIServiceTier
		require.True(t, errors.As(err, &invalid))
		require.Equal(t, "turbo", invalid.Value)
		require.Contains(t, err.Error(), "invalid service_tier")
		require.Contains(t, err.Error(), "fast", "allowed-value hint must mention fast")
REDACTED)

	t.Run("omitted field stays valid", func(t *testing.T) {
		norm, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","input":"hi"REDACTED`))
	REDACTED
		require.Empty(t, norm)
REDACTED)

	t.Run("null value keeps omission semantics", func(t *testing.T) {
		norm, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","service_tier":nullREDACTED`))
	REDACTED
		require.Empty(t, norm)
REDACTED)

	t.Run("explicit empty string rejected as invalid enum value", func(t *testing.T) {
		_, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","service_tier":""REDACTED`))
	REDACTED
		var invalid *ErrInvalidOpenAIServiceTier
		require.True(t, errors.As(err, &invalid))
REDACTED)

	t.Run("non-string service_tier rejected", func(t *testing.T) {
		// service_tier 必须为字符串；数字/布尔/对象/数组等类型同样按非法值拒绝。
		for _, raw := range []string{
			`{"model":"gpt-5.5","service_tier":123REDACTED`,
			`{"model":"gpt-5.5","service_tier":trueREDACTED`,
			`{"model":"gpt-5.5","service_tier":{REDACTEDREDACTED`,
			`{"model":"gpt-5.5","service_tier":["priority"]REDACTED`,
	REDACTED {
			_, err := ValidateOpenAIServiceTierField([]byte(raw))
			require.Error(t, err, "raw=%s must be rejected", raw)
			var invalid *ErrInvalidOpenAIServiceTier
			require.True(t, errors.As(err, &invalid), "raw=%s", raw)
			require.Equal(t, "<non-string>", invalid.Value, "raw=%s", raw)
			require.Contains(t, err.Error(), "invalid service_tier")
	REDACTED
REDACTED)

	t.Run("oversized unknown string is truncated", func(t *testing.T) {
		blob := strings.Repeat("z", 4096)
		_, err := ValidateOpenAIServiceTierField([]byte(`{"model":"gpt-5.5","service_tier":"` + blob + `"REDACTED`))
	REDACTED
		require.Contains(t, err.Error(), "invalid service_tier")
		require.NotContains(t, err.Error(), blob)
		require.Less(t, len(err.Error()), 200)
		var invalid *ErrInvalidOpenAIServiceTier
		require.True(t, errors.As(err, &invalid))
		require.Equal(t, strings.Repeat("z", 64)+"...", invalid.Value)
REDACTED)

	t.Run("non-string large object/array is not echoed", func(t *testing.T) {
		blob := strings.Repeat("x", 4096)
		payloads := []string{
			`{"model":"gpt-5.5","service_tier":{"blob":"` + blob + `"REDACTEDREDACTED`,
			`{"model":"gpt-5.5","service_tier":["` + blob + `"]REDACTED`,
	REDACTED
		for _, raw := range payloads {
			_, err := ValidateOpenAIServiceTierField([]byte(raw))
		REDACTED
			require.Contains(t, err.Error(), "invalid service_tier")
			require.NotContains(t, err.Error(), blob)
			require.Less(t, len(err.Error()), 200)
			var invalid *ErrInvalidOpenAIServiceTier
			require.True(t, errors.As(err, &invalid))
			require.Equal(t, "<non-string>", invalid.Value)
	REDACTED
REDACTED)
REDACTED

// ---------------------------------------------------------------------------
// 计费：gpt-5.6 系列 / gpt-5.4 按标准价 2x，gpt-5.5 按标准价 2.5x
// ---------------------------------------------------------------------------

func TestApplyModelSpecificPricingPolicy_EnforcesOpenAIFastRatios(t *testing.T) {
	t.Parallel()

	svc := &BillingService{REDACTED

	t.Run("gpt-5.5 catalog 2x priority is corrected to 2.5x", func(t *testing.T) {
		// 模拟本地 LiteLLM 目录仍携带官方旧口径（gpt-5.5 priority = 2x）。
		catalog := &ModelPricing{
			InputPricePerToken:             5e-6,
			InputPricePerTokenPriority:     10e-6,
			OutputPricePerToken:            30e-6,
			OutputPricePerTokenPriority:    60e-6,
			CacheReadPricePerToken:         0.5e-6,
			CacheReadPricePerTokenPriority: 1e-6,
	REDACTED
		got := svc.applyModelSpecificPricingPolicy("gpt-5.5", catalog)
		require.InDelta(t, 12.5e-6, got.InputPricePerTokenPriority, 1e-12)
		require.InDelta(t, 75e-6, got.OutputPricePerTokenPriority, 1e-12)
		require.InDelta(t, 1.25e-6, got.CacheReadPricePerTokenPriority, 1e-12)
		// 标准价不被改动。
		require.InDelta(t, 5e-6, got.InputPricePerToken, 1e-12)
		// 原始指针不被污染。
		require.InDelta(t, 10e-6, catalog.InputPricePerTokenPriority, 1e-12)
REDACTED)

	t.Run("gpt-5.4 keeps 2x", func(t *testing.T) {
		got := svc.applyModelSpecificPricingPolicy("gpt-5.4", &ModelPricing{
			InputPricePerToken:          2.5e-6,
			InputPricePerTokenPriority:  5e-6,
			OutputPricePerToken:         15e-6,
			OutputPricePerTokenPriority: 30e-6,
	REDACTED)
		require.InDelta(t, 5e-6, got.InputPricePerTokenPriority, 1e-12)
		require.InDelta(t, 30e-6, got.OutputPricePerTokenPriority, 1e-12)
REDACTED)

	t.Run("gpt-5.6 family keeps 2x", func(t *testing.T) {
		for _, model := range []string{"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.6-max", "gpt-5.6-sol-preview"REDACTED {
			got := svc.applyModelSpecificPricingPolicy(model, &ModelPricing{
				InputPricePerToken:             5e-6,
				InputPricePerTokenPriority:     10e-6,
				OutputPricePerToken:            30e-6,
				OutputPricePerTokenPriority:    60e-6,
				CacheReadPricePerToken:         0.5e-6,
				CacheReadPricePerTokenPriority: 1e-6,
		REDACTED)
			require.InDelta(t, 10e-6, got.InputPricePerTokenPriority, 1e-12, "model %s", model)
			require.InDelta(t, 60e-6, got.OutputPricePerTokenPriority, 1e-12, "model %s", model)
	REDACTED
REDACTED)

	t.Run("missing priority prices are backfilled from standard", func(t *testing.T) {
		got := svc.applyModelSpecificPricingPolicy("gpt-5.5", &ModelPricing{
			InputPricePerToken:         5e-6,
			OutputPricePerToken:        30e-6,
			CacheReadPricePerToken:     0.5e-6,
			CacheCreationPricePerToken: 5e-6,
	REDACTED)
		require.InDelta(t, 12.5e-6, got.InputPricePerTokenPriority, 1e-12)
		require.InDelta(t, 75e-6, got.OutputPricePerTokenPriority, 1e-12)
		require.InDelta(t, 1.25e-6, got.CacheReadPricePerTokenPriority, 1e-12)
		require.InDelta(t, 12.5e-6, got.CacheCreationPricePerTokenPriority, 1e-12)
REDACTED)

	t.Run("gpt-5.5-pro has no mandated fast tier", func(t *testing.T) {
		got := svc.applyModelSpecificPricingPolicy("gpt-5.5-pro", &ModelPricing{
			InputPricePerToken:         30e-6,
			InputPricePerTokenPriority: 60e-6,
			OutputPricePerToken:        180e-6,
	REDACTED)
		require.InDelta(t, 60e-6, got.InputPricePerTokenPriority, 1e-12)
REDACTED)

	t.Run("unrelated models untouched", func(t *testing.T) {
		got := svc.applyModelSpecificPricingPolicy("claude-opus-5", &ModelPricing{InputPricePerToken: 1, OutputPricePerToken: 2REDACTED)
		require.InDelta(t, 1, got.InputPricePerToken, 1e-12)
		require.Zero(t, got.InputPricePerTokenPriority)
REDACTED)
REDACTED

func TestOpenAIFastBillingMultiplier_2xAnd25x(t *testing.T) {
	t.Parallel()

	// 目录数据携带官方旧口径（gpt-5.5 priority=2x）；修正后 fast 必须按 2.5x 计费。
	catalog := map[string]*LiteLLMModelPricing{
		"gpt-5.4": {
			InputCostPerToken:               2.5e-6,
			InputCostPerTokenPriority:       5e-6,
			OutputCostPerToken:              15e-6,
			OutputCostPerTokenPriority:      30e-6,
			CacheReadInputTokenCost:         0.25e-6,
			CacheReadInputTokenCostPriority: 0.5e-6,
	REDACTED,
		"gpt-5.5": {
			InputCostPerToken:               5e-6,
			InputCostPerTokenPriority:       10e-6,
			OutputCostPerToken:              30e-6,
			OutputCostPerTokenPriority:      60e-6,
			CacheReadInputTokenCost:         0.5e-6,
			CacheReadInputTokenCostPriority: 1e-6,
	REDACTED,
		"gpt-5.6-sol": {
			InputCostPerToken:               5e-6,
			InputCostPerTokenPriority:       10e-6,
			OutputCostPerToken:              30e-6,
			OutputCostPerTokenPriority:      60e-6,
			CacheReadInputTokenCost:         0.5e-6,
			CacheReadInputTokenCostPriority: 1e-6,
	REDACTED,
REDACTED
	billing := NewBillingService(&config.Config{REDACTED, &PricingService{pricingData: catalogREDACTED)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000REDACTED

	standard := func(model string) *CostBreakdown {
		cost, err := billing.CalculateCost(model, tokens, 1)
	REDACTED
		return cost
REDACTED
	fast := func(model, tier string) *CostBreakdown {
		cost, err := billing.CalculateCostWithServiceTier(model, tokens, 1, tier)
	REDACTED
		return cost
REDACTED

	tests := []struct {
		model string
		ratio float64
REDACTED{
		{model: "gpt-5.4", ratio: 2.0REDACTED,
		{model: "gpt-5.5", ratio: 2.5REDACTED,
		{model: "gpt-5.6-sol", ratio: 2.0REDACTED,
		{model: "gpt-5.6-terra", ratio: 2.0REDACTED,
		{model: "gpt-5.6-luna", ratio: 2.0REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.model+"/fast", func(t *testing.T) {
			base := standard(tt.model)
			fastCost := fast(tt.model, "fast")
			require.InDelta(t, base.TotalCost*tt.ratio, fastCost.TotalCost, 1e-9,
				"fast total must be %.1fx standard", tt.ratio)
	REDACTED)
		t.Run(tt.model+"/priority_alias", func(t *testing.T) {
			fastCost := fast(tt.model, "fast")
			priorityCost := fast(tt.model, "priority")
			require.InDelta(t, fastCost.TotalCost, priorityCost.TotalCost, 1e-12,
				"client alias fast must bill identically to priority")
			require.InDelta(t, standard(tt.model).TotalCost*tt.ratio, priorityCost.TotalCost, 1e-9)
	REDACTED)
		t.Run(tt.model+"/no_tier_unchanged", func(t *testing.T) {
			base := standard(tt.model)
			noTier, err := billing.CalculateCostWithServiceTier(tt.model, tokens, 1, "")
		REDACTED
			require.InDelta(t, base.TotalCost, noTier.TotalCost, 1e-12)
	REDACTED)
		t.Run(tt.model+"/default_equals_standard", func(t *testing.T) {
			base := standard(tt.model)
			defaultCost, err := billing.CalculateCostWithServiceTier(tt.model, tokens, 1, "default")
		REDACTED
			require.InDelta(t, base.TotalCost, defaultCost.TotalCost, 1e-12)
			require.InDelta(t, base.InputCost, defaultCost.InputCost, 1e-12)
			require.InDelta(t, base.OutputCost, defaultCost.OutputCost, 1e-12)
			require.InDelta(t, base.CacheReadCost, defaultCost.CacheReadCost, 1e-12)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIFastBilling_FastMultiplierOverridesEnforcedRatio(t *testing.T) {
	t.Parallel()

	svc := &BillingService{REDACTED
	catalog := &ModelPricing{
		InputPricePerToken:             5e-6,
		InputPricePerTokenPriority:     10e-6,
		OutputPricePerToken:            30e-6,
		OutputPricePerTokenPriority:    60e-6,
		CacheReadPricePerToken:         0.5e-6,
		CacheReadPricePerTokenPriority: 1e-6,
REDACTED
	pricing := svc.applyModelSpecificPricingPolicy("gpt-5.5", catalog)
	require.InDelta(t, 12.5e-6, pricing.InputPricePerTokenPriority, 1e-12, "enforce must still write 2.5x priority prices")
	require.InDelta(t, 75e-6, pricing.OutputPricePerTokenPriority, 1e-12)

	multiplier := 1.7
	pricing.FastMultiplier = &multiplier

	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000REDACTED
	standard := svc.computeTokenBreakdown(pricing, tokens, 1, "", false)
	fast := svc.computeTokenBreakdown(pricing, tokens, 1, "fast", false)
	priority := svc.computeTokenBreakdown(pricing, tokens, 1, "priority", false)

	require.InDelta(t, standard.TotalCost*1.7, fast.TotalCost, 1e-9)
	require.InDelta(t, fast.TotalCost, priority.TotalCost, 1e-12)

	withoutOverride := *pricing
	withoutOverride.FastMultiplier = nil
	enforced := svc.computeTokenBreakdown(&withoutOverride, tokens, 1, "fast", false)
	require.InDelta(t, standard.TotalCost*2.5, enforced.TotalCost, 1e-9,
		"without FastMultiplier the same enforced prices still bill 2.5x")
REDACTED

// ---------------------------------------------------------------------------
// 上游 payload：fast 归一化为 priority 并确实到达上游
// ---------------------------------------------------------------------------

func TestForwardAsChatCompletions_ServiceTierFastNormalizedToPriorityUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"REDACTED],"service_tier":"fast","stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-chat-st"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop"REDACTEDREDACTED`)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{REDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          21,
		Name:        "openai-compatible",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-compatible"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
REDACTED

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
REDACTED // upstream 400 → 错误返回，但请求体已被 recorder 捕获
	require.NotNil(t, upstream.lastBody)
	require.Equal(t, "priority", gjson.GetBytes(upstream.lastBody, "service_tier").String(),
		"client alias fast must reach upstream as priority")
REDACTED

func TestForwardAsChatCompletions_ServiceTierPriorityPreservedUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"REDACTED],"service_tier":"priority","stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-chat-st2"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop"REDACTEDREDACTED`)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{REDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          2,
		Name:        "openai-compatible",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-compatible"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
REDACTED

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
REDACTED
	require.NotNil(t, upstream.lastBody)
	require.Equal(t, "priority", gjson.GetBytes(upstream.lastBody, "service_tier").String())
REDACTED

func TestForward_ResponsesServiceTierFastNormalizedToPriorityUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","service_tier":"fast","input":"hello","stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-resp-st"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_1","object":"response","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`,
		)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          7,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-test"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastBody)
	require.Equal(t, "priority", gjson.GetBytes(upstream.lastBody, "service_tier").String(),
		"client alias fast must reach the upstream as priority")
	// 计费上下文：result 携带归一化后的 tier。
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "priority", *result.ServiceTier)
REDACTED

func TestForward_ResponsesServiceTierOmittedStaysOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","input":"hello","stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-resp-st2"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_2","object":"response","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`,
		)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          7,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-test"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastBody)
	require.False(t, gjson.GetBytes(upstream.lastBody, "service_tier").Exists(),
		"omitted service_tier must stay omitted")
	require.Nil(t, result.ServiceTier)
REDACTED

// ---------------------------------------------------------------------------
// 流式计费上下文：service_tier 需要从请求体传到 usage 计费
// ---------------------------------------------------------------------------

func TestForwardStreaming_ServiceTierPropagatedToResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","service_tier":"fast","input":"hello","stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	streamPayload := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_s1\",\"object\":\"response\",\"model\":\"gpt-5.5\",\"status\":\"in_progress\"REDACTEDREDACTED\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"it_1\",\"output_index\":0,\"delta\":\"hi\"REDACTED\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_s1\",\"object\":\"response\",\"model\":\"gpt-5.5\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1REDACTEDREDACTEDREDACTED\n\n" +
		"data: [DONE]\n\n"

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid-resp-stream-st"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(streamPayload)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          7,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-test"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "priority", *result.ServiceTier, "streaming billing context must carry the normalized tier")
	// /v1/responses 流是上游 SSE 原样透传：上游没回 service_tier 就不该出现；
	// 网关只在计费结果里携带请求侧 tier，不往下游流里注入。
	require.Contains(t, rec.Body.String(), `"delta":"hi"`, "streamed content must reach the client")
	require.NotContains(t, rec.Body.String(), `"service_tier"`, "upstream did not return service_tier, client stream must stay untouched")
REDACTED

// ---------------------------------------------------------------------------
// 上游回显优先：请求 fast 但上游真实返回 default → 计费按标准价
// ---------------------------------------------------------------------------

func TestForward_ResponsesUpstreamEchoesDefault_OverridesRequestFast(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","service_tier":"fast","input":"hello","stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	// 上游回显 service_tier=default（例如请求实际被降级）。
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-resp-echo"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_1","object":"response","status":"completed","model":"gpt-5.5","service_tier":"default","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`,
		)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          7,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-test"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "default", *result.ServiceTier,
		"upstream-echoed default must override the client-requested fast tier for billing")
	// 非流式响应原样透传：客户端同样看到 default。
	require.Contains(t, rec.Body.String(), `"service_tier":"default"`)
	require.NotContains(t, rec.Body.String(), `"service_tier":"priority"`)
REDACTED

func TestForwardStreaming_UpstreamEchoesDefault_OverridesRequestFast(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","service_tier":"fast","input":"hello","stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	streamPayload := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_s1\",\"object\":\"response\",\"model\":\"gpt-5.5\",\"status\":\"in_progress\"REDACTEDREDACTED\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_s1\",\"object\":\"response\",\"model\":\"gpt-5.5\",\"status\":\"completed\",\"service_tier\":\"default\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1REDACTEDREDACTEDREDACTED\n\n" +
		"data: [DONE]\n\n"

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid-resp-echo-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(streamPayload)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          7,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-test"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "default", *result.ServiceTier,
		"terminal SSE event's upstream-echoed default must win for billing")
	// 流式原样透传：客户端在终止事件里看到 default。
	require.Contains(t, rec.Body.String(), `"service_tier":"default"`)
REDACTED

func TestForwardAsChatCompletions_UpstreamEchoesDefault_BillsStandard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"REDACTED],"service_tier":"fast","stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	streamPayload := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_c1\",\"object\":\"response\",\"model\":\"gpt-5.5\",\"status\":\"in_progress\"REDACTEDREDACTED\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"it_1\",\"output_index\":0,\"delta\":\"hi\"REDACTED\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_c1\",\"object\":\"response\",\"model\":\"gpt-5.5\",\"status\":\"completed\",\"service_tier\":\"default\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1REDACTEDREDACTEDREDACTED\n\n" +
		"data: [DONE]\n\n"

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid-chat-echo"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(streamPayload)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          21,
		Name:        "openai-compatible",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-compatible"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.5")
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "default", *result.ServiceTier,
		"CC bridge must bill on the upstream-echoed default, not the requested fast")
	// 缓冲转回 Chat Completions：客户端响应里如实回显 default。
	require.Contains(t, rec.Body.String(), `"service_tier":"default"`)
	require.NotContains(t, rec.Body.String(), `"service_tier":"priority"`)
REDACTED

// ---------------------------------------------------------------------------
// policy filter：删除 service_tier 后不得再按原请求 Fast 计费
// ---------------------------------------------------------------------------

func TestForward_ServiceTierFilteredByPolicyBillsStandard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","input":"hello","stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	// 管理员配置 priority → filter：字段在出站前被删除。
	settings := &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
		ServiceTier: OpenAIFastTierPriority,
		Action:      BetaPolicyActionFilter,
		Scope:       BetaPolicyScopeAll,
REDACTEDREDACTEDREDACTED
	raw, err := json.Marshal(settings)
REDACTED
	repo := &openAIFastPolicyRepoStub{values: map[string]string{SettingKeyOpenAIFastPolicySettings: string(raw)REDACTEDREDACTED

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-resp-filter"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_1","object":"response","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`,
		)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
		httpUpstream:   upstream,
		settingService: NewSettingService(repo, &config.Config{REDACTED),
REDACTED
	account := &Account{
		ID:          7,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED"api_key": "sk-test"REDACTED,
		Extra:       map[string]any{"openai_responses_supported": trueREDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	// 出站 body 已剥离 service_tier、上游也未回显 → 无 tier → 按标准价计费。
	require.False(t, gjson.GetBytes(upstream.lastBody, "service_tier").Exists(),
		"policy filter must strip service_tier from the outbound body")
	require.Nil(t, result.ServiceTier, "filtered request must not bill as fast")
REDACTED

// ---------------------------------------------------------------------------
// 上游回显观察与解析器单测
// ---------------------------------------------------------------------------

func TestUpstreamResponseModelObserver_ObservesServiceTier(t *testing.T) {
	t.Parallel()

	observer := &upstreamResponseModelObserver{REDACTED
	// 上游约束：非终止且有类型的事件（response.created）回显的是请求档位而非
	// 实际处理档位，忽略。
	observer.ObserveOpenAI([]byte(`{"type":"response.created","response":{"model":"gpt-5.5","service_tier":"flex"REDACTEDREDACTED`), "response.created")
	require.Empty(t, observer.ServiceTier())

	// terminal 声明（带 model 帧）优先。
	observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-5.5","service_tier":"default"REDACTEDREDACTED`), "response.completed")
	require.Equal(t, "default", observer.ServiceTier())

	// Chat Completions 顶层 service_tier 按 untyped payload 观察（无 type 字段）。
	ccObserver := &upstreamResponseModelObserver{REDACTED
	ccObserver.ObserveOpenAI([]byte(`{"id":"chatcmpl-1","model":"gpt-5.5","service_tier":"priority","choices":[]REDACTED`), "")
	require.Equal(t, "priority", ccObserver.ServiceTier())

	// 无 model 的帧不触发 tier 观察（上游约束：tier 声明必带 model）。
	modelFree := &upstreamResponseModelObserver{REDACTED
	modelFree.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"service_tier":"default"REDACTEDREDACTED`), "response.completed")
	require.Empty(t, modelFree.ServiceTier())
REDACTED

func TestResolvedOpenAIUpstreamServiceTier(t *testing.T) {
	t.Parallel()

	priority := func() *string { v := "priority"; return &v REDACTED()

	t.Run("upstream echo wins over outbound tier", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(nil)
		observer := beginUpstreamResponseModelObservation(c)
		observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-5.5","service_tier":"default"REDACTEDREDACTED`), "response.completed")

		got := resolvedOpenAIUpstreamServiceTier(c, priority)
		require.NotNil(t, got)
		require.Equal(t, "default", *got)
REDACTED)

	t.Run("no upstream echo falls back to outbound tier", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(nil)
		beginUpstreamResponseModelObservation(c)

		got := resolvedOpenAIUpstreamServiceTier(c, priority)
		require.NotNil(t, got)
		require.Equal(t, "priority", *got)
REDACTED)

	t.Run("upstream alias fast normalizes to priority", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(nil)
		observer := beginUpstreamResponseModelObservation(c)
		observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-5.5","service_tier":"fast"REDACTEDREDACTED`), "response.completed")

		got := resolvedOpenAIUpstreamServiceTier(c, nil)
		require.NotNil(t, got)
		require.Equal(t, "priority", *got)
REDACTED)

	t.Run("no observer keeps outbound tier", func(t *testing.T) {
		got := resolvedOpenAIUpstreamServiceTier(nil, priority)
		require.NotNil(t, got)
		require.Equal(t, "priority", *got)
REDACTED)

	t.Run("no observer and no outbound tier stays nil", func(t *testing.T) {
		require.Nil(t, resolvedOpenAIUpstreamServiceTier(nil, nil))
REDACTED)

	t.Run("local observer wins without gin context", func(t *testing.T) {
		observer := &upstreamResponseModelObserver{REDACTED
		observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-5.5","service_tier":"default"REDACTEDREDACTED`), "response.completed")

		got := resolvedOpenAIUpstreamServiceTierFromObserver(observer, priority)
		require.NotNil(t, got)
		require.Equal(t, "default", *got)
REDACTED)

	t.Run("nil local observer falls back to outbound tier", func(t *testing.T) {
		got := resolvedOpenAIUpstreamServiceTierFromObserver(nil, priority)
		require.NotNil(t, got)
		require.Equal(t, "priority", *got)
REDACTED)
REDACTED
