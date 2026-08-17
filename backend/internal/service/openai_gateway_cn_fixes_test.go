//go:build unit

package service

// 国产供应商功能修复回归测试：
//  1. CN 分组不适用 /v1/messages 调度级模型映射（openai 的 gpt-5.x 默认值发给
//     CN 上游必错）；
//  2. 计费候选链对 CN 账号过滤 claude-* 兜底候选（防按 Claude 原价误计 CN 流量）；
//  3. 空候选按 ErrModelPricingUnavailable 处理（零成本落账而非丢弃 usage 记录）；
//  4. Responses×anthropic 流式转换器客户端断开后继续排水、usage 汇总完整。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveMessagesDispatchModel_CNProvidersNoDispatchMapping(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseekREDACTED {
		g := &Group{Platform: platformREDACTED
		require.Empty(t, g.ResolveMessagesDispatchModel("claude-sonnet-4-5"),
			"CN 分组(%s)不得返回调度级映射模型（openai 默认值会发给 CN 上游）", platform)
		require.Empty(t, g.ResolveMessagesDispatchModel("claude-opus-4-1"), platform)
REDACTED
	// 非回归：openai 分组保持原有默认映射行为。
	openaiGroup := &Group{Platform: PlatformOpenAIREDACTED
	require.NotEmpty(t, openaiGroup.ResolveMessagesDispatchModel("claude-sonnet-4-5"),
		"openai 分组的调度默认映射不应受 CN 修复影响")
REDACTED

func TestFilterCNProviderBillingModelCandidates(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED // resolver 为 nil → 无显式分组/渠道定价
	apiKey := &APIKey{Group: &Group{ID: 1, Platform: PlatformKimiREDACTEDREDACTED

	cnAccount := &Account{ID: 1, Platform: PlatformKimiREDACTED
	filtered := svc.filterCNProviderBillingModelCandidates(context.Background(), cnAccount, apiKey,
		[]string{"kimi-k2-0905-preview", "claude-sonnet-4-5", "moonshot-v1-8k"REDACTED)
	require.Equal(t, []string{"kimi-k2-0905-preview", "moonshot-v1-8k"REDACTED, filtered,
		"无显式定价时 claude-* 候选必须被过滤")

	allClaude := svc.filterCNProviderBillingModelCandidates(context.Background(), cnAccount, apiKey,
		[]string{"claude-sonnet-4-5", "claude-sonnet-4-5"REDACTED)
	require.Empty(t, allClaude, "全 claude 候选应被清空（上层走零成本+告警落账）")

	// 非 CN 账号完全不受影响。
	openaiAccount := &Account{ID: 2, Platform: PlatformOpenAIREDACTED
	passthrough := svc.filterCNProviderBillingModelCandidates(context.Background(), openaiAccount, apiKey,
		[]string{"claude-sonnet-4-5", "gpt-5.4"REDACTED)
	require.Equal(t, []string{"claude-sonnet-4-5", "gpt-5.4"REDACTED, passthrough)

	require.Nil(t, svc.filterCNProviderBillingModelCandidates(context.Background(), nil, apiKey, nil))
REDACTED

func TestCalculateOpenAIRecordUsageCost_EmptyCandidatesIsPricingUnavailable(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	apiKey := &APIKey{Group: &Group{ID: 1, Platform: PlatformKimiREDACTEDREDACTED

	_, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(), nil, apiKey, nil,
		1.0, 1.0, 1.0, 1.0, UsageTokens{InputTokens: 100REDACTED, "", nil, time.Time{REDACTED,
	)
REDACTED
	require.True(t, isUsagePricingUnavailableError(err),
		"空候选必须按无价可循处理（上层零成本落账），而不是丢弃整条 usage 记录: %v", err)
REDACTED

func TestResponsesStreamingFromNativeAnthropic_ClientDisconnectDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newNativeAnthropicHangTestService(5)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	// failAfter=0：首次写出即失败，模拟客户端断开（复用测试包既有 failingGinWriter）。
	failWriter := &failingGinWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
	c.Writer = failWriter

	resp, pr, pw := newHangingUpstreamResponse()
	go func() {
		// 首事件触发客户端写失败后，末尾 message_delta 才携带最终 output_tokens：
		// 断开即弃会把整段生成记成 1 token。
		_, _ = pw.Write([]byte(miniAnthropicSSEStream()))
		_ = pw.Close()
REDACTED()
	defer func() { _ = pr.Close() REDACTED()

	res, err := svc.handleResponsesStreamingFromNativeAnthropic(
		resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now(), apicompat.ResponsesClientToolMapping{REDACTED)

	require.NoError(t, err, "断开排水至上游自然结束应返回 nil error（usage 走成功路径落账）")
	require.NotNil(t, res)
	require.True(t, res.ClientDisconnect)
	require.Equal(t, 10, res.Usage.InputTokens, "input_tokens 应来自 message_start")
	require.Equal(t, 5, res.Usage.OutputTokens,
		"output_tokens 必须来自排水读到的末尾 message_delta（断开即弃时会是 1）")
REDACTED

func TestHandle403_CNProviderHTMLBodySkipsAccountPenalty(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseekREDACTED {
		repo := &rateLimitAccountRepoStub{REDACTED
		service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		account := &Account{ID: 401, Platform: platform, Type: AccountTypeAPIKeyREDACTED

		shouldDisable := service.HandleUpstreamError(
			context.Background(),
			account,
			http.StatusForbidden,
			http.Header{REDACTED,
			[]byte("<html><body>Access denied by CDN</body></html>"),
		)

		require.False(t, shouldDisable, "%s: HTML 403（CDN/代理拦截页）不得作为账号失效证据", platform)
		require.Equal(t, 0, repo.setErrorCalls, "%s: 不得永久禁用账号", platform)
		require.Equal(t, 0, repo.tempCalls, "%s: 不得临时停调账号", platform)
REDACTED
REDACTED

func TestHandle403_CNProviderStructured403TempUnschedulableFirstHit(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	counter := &openAI403CounterCacheStub{counts: []int64{1REDACTEDREDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{ID: 402, Platform: PlatformKimi, Type: AccountTypeAPIKeyREDACTED

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{REDACTED,
		[]byte(`{"error":{"message":"forbidden"REDACTEDREDACTED`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls, "首次结构化 403 应临时停调而非永久禁用")
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, "(1/3)")
REDACTED
