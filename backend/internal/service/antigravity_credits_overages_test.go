//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestClassifyAntigravity429(t *testing.T) {
	t.Run("明确配额耗尽", func(t *testing.T) {
		body := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"QUOTA_EXHAUSTED"REDACTEDREDACTED`)
		require.Equal(t, antigravity429QuotaExhausted, classifyAntigravity429(body))
REDACTED)

	t.Run("结构化限流", func(t *testing.T) {
		body := []byte(`{
			"error": {
				"status": "RESOURCE_EXHAUSTED",
				"details": [
					{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
					{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"REDACTED
				]
		REDACTED
	REDACTED`)
		require.Equal(t, antigravity429RateLimited, classifyAntigravity429(body))
REDACTED)

	t.Run("未知429", func(t *testing.T) {
		body := []byte(`{"error":{"message":"too many requests"REDACTEDREDACTED`)
		require.Equal(t, antigravity429Unknown, classifyAntigravity429(body))
REDACTED)
REDACTED

func TestIsCreditsExhausted_UsesAICreditsKey(t *testing.T) {
	t.Run("无 AICredits key 则积分可用", func(t *testing.T) {
		account := &Account{
			ID:       1,
			Platform: PlatformAntigravity,
			Extra: map[string]any{
				"allow_overages": true,
		REDACTED,
	REDACTED
		require.False(t, account.isCreditsExhausted())
REDACTED)

	t.Run("AICredits key 生效则积分耗尽", func(t *testing.T) {
		account := &Account{
			ID:       2,
			Platform: PlatformAntigravity,
			Extra: map[string]any{
				"allow_overages": true,
				modelRateLimitsKey: map[string]any{
					creditsExhaustedKey: map[string]any{
						"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
						"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		require.True(t, account.isCreditsExhausted())
REDACTED)

	t.Run("AICredits key 过期则积分可用", func(t *testing.T) {
		account := &Account{
			ID:       3,
			Platform: PlatformAntigravity,
			Extra: map[string]any{
				"allow_overages": true,
				modelRateLimitsKey: map[string]any{
					creditsExhaustedKey: map[string]any{
						"rate_limited_at":     time.Now().Add(-6 * time.Hour).UTC().Format(time.RFC3339),
						"rate_limit_reset_at": time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		require.False(t, account.isCreditsExhausted())
REDACTED)
REDACTED

func TestHandleSmartRetry_QuotaExhausted_UsesCreditsAndStoresIndependentState(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"ok":trueREDACTED`)),
REDACTED
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successRespREDACTED,
		errors:    []error{nilREDACTED,
REDACTED
	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       101,
		Name:     "acc-101",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"allow_overages": true,
	REDACTED,
REDACTED
			"model_mapping": map[string]any{
				"claude-opus-4-6": "claude-sonnet-4-5",
		REDACTED,
	REDACTED,
REDACTED

	respBody := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"QUOTA_EXHAUSTED"REDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED
	params := antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-opus-4-6","request":{REDACTEDREDACTED`),
		httpUpstream:   upstream,
		accountRepo:    repo,
		requestedModel: "claude-opus-4-6",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{REDACTED
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, []string{"https://ag-1.test"REDACTED)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp)
	require.Nil(t, result.switchError)
	require.Len(t, upstream.requestBodies, 1)
	require.Contains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
	require.Empty(t, repo.modelRateLimitCalls, "overages 成功后不应写入普通 model_rate_limits")
REDACTED

func TestHandleSmartRetry_RateLimited_DoesNotUseCredits(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"ok":trueREDACTED`)),
REDACTED
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successRespREDACTED,
		errors:    []error{nilREDACTED,
REDACTED
	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       102,
		Name:     "acc-102",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"allow_overages": true,
	REDACTED,
REDACTED

	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"REDACTED
			]
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED
	params := antigravityRetryLoopParams{
		ctx:          context.Background(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"model":"claude-sonnet-4-5","request":{REDACTEDREDACTED`),
		httpUpstream: upstream,
		accountRepo:  repo,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{REDACTED
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, []string{"https://ag-1.test"REDACTED)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp)
	require.Len(t, upstream.requestBodies, 1)
	require.NotContains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
	require.Empty(t, repo.extraUpdateCalls)
	require.Empty(t, repo.modelRateLimitCalls)
REDACTED

func TestAntigravityRetryLoop_ModelRateLimited_InjectsCredits(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
REDACTED()

	antigravity.BaseURLs = []string{"https://ag-1.test"REDACTED
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{REDACTED,
				Body:       io.NopCloser(strings.NewReader(`{"ok":trueREDACTED`)),
		REDACTED,
	REDACTED,
		errors: []error{nilREDACTED,
REDACTED
	// 模型已限流 + overages 启用 + 无 AICredits key → 应直接注入积分
	account := &Account{
		ID:          103,
		Name:        "acc-103",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"allow_overages": true,
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
					"rate_limit_reset_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{REDACTED
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-sonnet-4-5","request":{REDACTEDREDACTED`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	require.Contains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
REDACTED

func TestAntigravityRetryLoop_CreditsExhausted_DoesNotInject(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
REDACTED()

	antigravity.BaseURLs = []string{"https://ag-1.test"REDACTED
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	// 模型限流 + overages 启用 + AICredits key 生效 → 不应注入积分，应切号
	account := &Account{
		ID:          104,
		Name:        "acc-104",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"allow_overages": true,
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
					"rate_limit_reset_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
			REDACTED,
				creditsExhaustedKey: map[string]any{
					"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
					"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{REDACTED
	_, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-sonnet-4-5","request":{REDACTEDREDACTED`),
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED)

	// 模型限流 + 积分耗尽 → 应触发切号错误
REDACTED
	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr)
REDACTED

func TestAntigravityRetryLoop_CreditErrorMarksExhausted(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
REDACTED()

	antigravity.BaseURLs = []string{"https://ag-1.test"REDACTED
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	repo := &stubAntigravityAccountRepo{REDACTED
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{REDACTED,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Insufficient GOOGLE_ONE_AI credits"REDACTEDREDACTED`)),
		REDACTED,
	REDACTED,
		errors: []error{nilREDACTED,
REDACTED
	// 模型限流 + overages 启用 + 积分可用 → 注入积分但上游返回积分不足
	account := &Account{
		ID:          105,
		Name:        "acc-105",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"allow_overages": true,
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
					"rate_limit_reset_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{accountRepo: repoREDACTED
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-sonnet-4-5","request":{REDACTEDREDACTED`),
		httpUpstream:   upstream,
		accountRepo:    repo,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, result)
	// 验证 AICredits key 已通过 SetModelRateLimit 写入数据库
	require.Len(t, repo.modelRateLimitCalls, 1, "应通过 SetModelRateLimit 写入 AICredits key")
	require.Equal(t, creditsExhaustedKey, repo.modelRateLimitCalls[0].modelKey)
REDACTED

func TestShouldMarkCreditsExhausted(t *testing.T) {
	t.Run("reqErr 不为 nil 时不标记", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusForbiddenREDACTED
		require.False(t, shouldMarkCreditsExhausted(resp, []byte(`{"error":"Insufficient credits"REDACTED`), io.ErrUnexpectedEOF))
REDACTED)

	t.Run("resp 为 nil 时不标记", func(t *testing.T) {
		require.False(t, shouldMarkCreditsExhausted(nil, []byte(`{"error":"Insufficient credits"REDACTED`), nil))
REDACTED)

	t.Run("5xx 响应不标记", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusInternalServerErrorREDACTED
		require.False(t, shouldMarkCreditsExhausted(resp, []byte(`{"error":"Insufficient credits"REDACTED`), nil))
REDACTED)

	t.Run("408 RequestTimeout 不标记", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusRequestTimeoutREDACTED
		require.False(t, shouldMarkCreditsExhausted(resp, []byte(`{"error":"Insufficient credits"REDACTED`), nil))
REDACTED)

	t.Run("Resource has been exhausted 应标记为积分耗尽", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusTooManyRequestsREDACTED
		body := []byte(`{"error":{"message":"Resource has been exhausted"REDACTEDREDACTED`)
		require.True(t, shouldMarkCreditsExhausted(resp, body, nil))
REDACTED)

	t.Run("Resource has been exhausted (check quota) 完整格式应标记", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusTooManyRequestsREDACTED
		body := []byte(`{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"REDACTEDREDACTED`)
		require.True(t, shouldMarkCreditsExhausted(resp, body, nil))
REDACTED)

	t.Run("结构化限流不标记", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusTooManyRequestsREDACTED
		body := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED"REDACTED,{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"0.5s"REDACTED]REDACTEDREDACTED`)
		require.False(t, shouldMarkCreditsExhausted(resp, body, nil))
REDACTED)

	t.Run("含 credits 关键词时标记", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusForbiddenREDACTED
		for _, keyword := range []string{
			"Insufficient GOOGLE_ONE_AI credits",
			"insufficient credit balance",
			"not enough credits for this request",
			"Credits exhausted",
			"minimumCreditAmountForUsage requirement not met",
	REDACTED {
			body := []byte(`{"error":{"message":"` + keyword + `"REDACTEDREDACTED`)
			require.True(t, shouldMarkCreditsExhausted(resp, body, nil), "should mark for keyword: %s", keyword)
	REDACTED
REDACTED)

	t.Run("无 credits 关键词时不标记", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusForbiddenREDACTED
		body := []byte(`{"error":{"message":"permission denied"REDACTEDREDACTED`)
		require.False(t, shouldMarkCreditsExhausted(resp, body, nil))
REDACTED)
REDACTED

func TestInjectEnabledCreditTypes(t *testing.T) {
	t.Run("正常 JSON 注入成功", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4-5","request":{REDACTEDREDACTED`)
		result := injectEnabledCreditTypes(body)
		require.NotNil(t, result)
		require.Contains(t, string(result), `"enabledCreditTypes"`)
		require.Contains(t, string(result), `GOOGLE_ONE_AI`)
REDACTED)

	t.Run("非法 JSON 返回 nil", func(t *testing.T) {
		require.Nil(t, injectEnabledCreditTypes([]byte(`not json`)))
REDACTED)

	t.Run("空 body 返回 nil", func(t *testing.T) {
		require.Nil(t, injectEnabledCreditTypes([]byte{REDACTED))
REDACTED)

	t.Run("已有 enabledCreditTypes 会被覆盖", func(t *testing.T) {
		body := []byte(`{"enabledCreditTypes":["OLD"],"model":"test"REDACTED`)
		result := injectEnabledCreditTypes(body)
		require.NotNil(t, result)
		require.Contains(t, string(result), `GOOGLE_ONE_AI`)
		require.NotContains(t, string(result), `OLD`)
REDACTED)
REDACTED

func TestClearCreditsExhausted(t *testing.T) {
	t.Run("account 为 nil 不操作", func(t *testing.T) {
		repo := &stubAntigravityAccountRepo{REDACTED
		svc := &AntigravityGatewayService{accountRepo: repoREDACTED
		svc.clearCreditsExhausted(context.Background(), nil)
		require.Empty(t, repo.extraUpdateCalls)
REDACTED)

	t.Run("Extra 为 nil 不操作", func(t *testing.T) {
		repo := &stubAntigravityAccountRepo{REDACTED
		svc := &AntigravityGatewayService{accountRepo: repoREDACTED
		svc.clearCreditsExhausted(context.Background(), &Account{ID: 1REDACTED)
		require.Empty(t, repo.extraUpdateCalls)
REDACTED)

	t.Run("无 modelRateLimitsKey 不操作", func(t *testing.T) {
		repo := &stubAntigravityAccountRepo{REDACTED
		svc := &AntigravityGatewayService{accountRepo: repoREDACTED
		svc.clearCreditsExhausted(context.Background(), &Account{
			ID:    1,
			Extra: map[string]any{"some_key": "value"REDACTED,
	REDACTED)
		require.Empty(t, repo.extraUpdateCalls)
REDACTED)

	t.Run("无 AICredits key 不操作", func(t *testing.T) {
		repo := &stubAntigravityAccountRepo{REDACTED
		svc := &AntigravityGatewayService{accountRepo: repoREDACTED
		svc.clearCreditsExhausted(context.Background(), &Account{
			ID: 1,
			Extra: map[string]any{
				modelRateLimitsKey: map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": "2099-03-15T00:00:00Z",
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED)
		require.Empty(t, repo.extraUpdateCalls)
REDACTED)

	t.Run("有 AICredits key 时删除并调用 UpdateExtra", func(t *testing.T) {
		repo := &stubAntigravityAccountRepo{REDACTED
		svc := &AntigravityGatewayService{accountRepo: repoREDACTED
		account := &Account{
			ID: 1,
			Extra: map[string]any{
				modelRateLimitsKey: map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": "2099-03-15T00:00:00Z",
				REDACTED,
					creditsExhaustedKey: map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		svc.clearCreditsExhausted(context.Background(), account)
		require.Len(t, repo.extraUpdateCalls, 1)
		// AICredits key 应被删除
		rawLimits := account.Extra[modelRateLimitsKey].(map[string]any)
		_, exists := rawLimits[creditsExhaustedKey]
		require.False(t, exists, "AICredits key 应被删除")
		// 普通模型限流应保留
		_, exists = rawLimits["claude-sonnet-4-5"]
		require.True(t, exists, "普通模型限流应保留")
REDACTED)
REDACTED
