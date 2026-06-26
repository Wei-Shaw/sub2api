package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// antigravityFailingWriter 模拟客户端断开连接的 gin.ResponseWriter
type antigravityFailingWriter struct {
	gin.ResponseWriter
	failAfter int // 允许成功写入的次数，之后所有写入返回错误
	writes    int
REDACTED

func (w *antigravityFailingWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed: client disconnected")
REDACTED
	w.writes++
	return w.ResponseWriter.Write(p)
REDACTED

// newAntigravityTestService 创建用于流式测试的 AntigravityGatewayService
func newAntigravityTestService(cfg *config.Config) *AntigravityGatewayService {
	return &AntigravityGatewayService{
		settingService: &SettingService{cfg: cfgREDACTED,
REDACTED
REDACTED

func TestAntigravityUpstreamErrorBodyReadLimit_RespectsDiagnosticLimit(t *testing.T) {
	svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{
		LogUpstreamErrorBody:         true,
		LogUpstreamErrorBodyMaxBytes: int(gatewayUpstreamErrorBodyReadLimit) + 1024,
REDACTEDREDACTED)

	require.Equal(t, int64(svc.settingService.cfg.Gateway.LogUpstreamErrorBodyMaxBytes), svc.upstreamErrorBodyReadLimit())
REDACTED

func TestStripSignatureSensitiveBlocksFromClaudeRequest(t *testing.T) {
	req := &antigravity.ClaudeRequest{
		Model: "claude-sonnet-4-5",
		Thinking: &antigravity.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: 1024,
	REDACTED,
		Messages: []antigravity.ClaudeMessage{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"secret plan","signature":""REDACTED,
					{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"REDACTEDREDACTED
				]`),
		REDACTED,
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"tool_result","tool_use_id":"t1","content":"ok","is_error":falseREDACTED,
					{"type":"redacted_thinking","data":"..."REDACTED
				]`),
		REDACTED,
	REDACTED,
REDACTED

	changed, err := stripSignatureSensitiveBlocksFromClaudeRequest(req)
REDACTED
	require.True(t, changed)
	require.Nil(t, req.Thinking)

	require.Len(t, req.Messages, 2)

	var blocks0 []map[string]any
	require.NoError(t, json.Unmarshal(req.Messages[0].Content, &blocks0))
	require.Len(t, blocks0, 2)
	require.Equal(t, "text", blocks0[0]["type"])
	require.Equal(t, "secret plan", blocks0[0]["text"])
	require.Equal(t, "text", blocks0[1]["type"])

	var blocks1 []map[string]any
	require.NoError(t, json.Unmarshal(req.Messages[1].Content, &blocks1))
	require.Len(t, blocks1, 1)
	require.Equal(t, "text", blocks1[0]["type"])
	require.NotEmpty(t, blocks1[0]["text"])
REDACTED

func TestStripThinkingFromClaudeRequest_DoesNotDowngradeTools(t *testing.T) {
	req := &antigravity.ClaudeRequest{
		Model: "claude-sonnet-4-5",
		Thinking: &antigravity.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: 1024,
	REDACTED,
		Messages: []antigravity.ClaudeMessage{
			{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"thinking","thinking":"secret plan"REDACTED,{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"REDACTEDREDACTED]`),
		REDACTED,
	REDACTED,
REDACTED

	changed, err := stripThinkingFromClaudeRequest(req)
REDACTED
	require.True(t, changed)
	require.Nil(t, req.Thinking)

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(req.Messages[0].Content, &blocks))
	require.Len(t, blocks, 2)
	require.Equal(t, "text", blocks[0]["type"])
	require.Equal(t, "secret plan", blocks[0]["text"])
	require.Equal(t, "tool_use", blocks[1]["type"])
REDACTED

func TestIsPromptTooLongError(t *testing.T) {
	require.True(t, isPromptTooLongError([]byte(`{"error":{"message":"Prompt is too long"REDACTEDREDACTED`)))
	require.True(t, isPromptTooLongError([]byte(`{"message":"Prompt is too long"REDACTED`)))
	require.False(t, isPromptTooLongError([]byte(`{"error":{"message":"other"REDACTEDREDACTED`)))
REDACTED

type httpUpstreamStub struct {
	resp *http.Response
	err  error
REDACTED

func (s *httpUpstreamStub) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return s.resp, s.err
REDACTED

func (s *httpUpstreamStub) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.resp, s.err
REDACTED

type queuedHTTPUpstreamStub struct {
	responses     []*http.Response
	errors        []error
	requestBodies [][]byte
	callCount     int
	onCall        func(*http.Request, *queuedHTTPUpstreamStub)
REDACTED

func (s *queuedHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if req != nil && req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		s.requestBodies = append(s.requestBodies, body)
		req.Body = io.NopCloser(bytes.NewReader(body))
REDACTED else {
		s.requestBodies = append(s.requestBodies, nil)
REDACTED

	idx := s.callCount
	s.callCount++
	if s.onCall != nil {
		s.onCall(req, s)
REDACTED

	var resp *http.Response
	if idx < len(s.responses) {
		resp = s.responses[idx]
REDACTED
	var err error
	if idx < len(s.errors) {
		err = s.errors[idx]
REDACTED
	if resp == nil && err == nil {
		return nil, errors.New("unexpected upstream call")
REDACTED
	return resp, err
REDACTED

func (s *queuedHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
REDACTED

type recordingInternal500CounterCache struct {
	incrementCalls []int64
	resetCalls     []int64
REDACTED

func (c *recordingInternal500CounterCache) IncrementInternal500Count(_ context.Context, accountID int64) (int64, error) {
	c.incrementCalls = append(c.incrementCalls, accountID)
	return int64(len(c.incrementCalls)), nil
REDACTED

func (c *recordingInternal500CounterCache) ResetInternal500Count(_ context.Context, accountID int64) error {
	c.resetCalls = append(c.resetCalls, accountID)
	return nil
REDACTED

type antigravitySettingRepoStub struct{REDACTED

func (s *antigravitySettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *antigravitySettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	return "", ErrSettingNotFound
REDACTED

func (s *antigravitySettingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *antigravitySettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *antigravitySettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *antigravitySettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *antigravitySettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

func TestResolveAntigravityProjectID(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
		wantErr bool
REDACTED{
		{
			name: "uses onboard project_id first",
			account: &Account{Credentials: map[string]any{
				"project_id": " onboard-project ",
				antigravityProjectIDFallbackCredentialKey: " configured-project ",
	REDACTED
			want: "onboard-project",
	REDACTED,
		{
			name: "uses configured credentials fallback",
			account: &Account{Credentials: map[string]any{
				antigravityProjectIDFallbackCredentialKey: " configured-project ",
	REDACTED
			want: "configured-project",
	REDACTED,
		{
			name: "uses configured extra fallback",
			account: &Account{Extra: map[string]any{
				antigravityProjectIDFallbackCredentialKey: " extra-project ",
	REDACTED
			want: "extra-project",
	REDACTED,
		{
			name:    "missing project",
			account: &Account{Credentials: map[string]any{REDACTEDREDACTED,
			wantErr: true,
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAntigravityProjectID(tc.account)
			if tc.wantErr {
				require.ErrorIs(t, err, errAntigravityProjectIDRequired)
				require.Empty(t, got)
				return
		REDACTED
		REDACTED
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED

func TestAntigravityGatewayService_ForwardGemini_UsesConfiguredProjectFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)
REDACTED
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-2.5-flash:streamGenerateContent", bytes.NewReader(body))

	upstreamBody := []byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"REDACTED]REDACTED,\"finishReason\":\"STOP\"REDACTED],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1REDACTEDREDACTEDREDACTED\n\n")
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
				Body:       io.NopCloser(bytes.NewReader(upstreamBody)),
		REDACTED,
	REDACTED,
REDACTED
	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{REDACTED, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED),
		tokenProvider:  &AntigravityTokenProvider{REDACTED,
		httpUpstream:   upstream,
REDACTED

	account := &Account{
		ID:          101,
		Name:        "acc-configured-project",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
			antigravityProjectIDFallbackCredentialKey: "configured-project",
			"model_mapping": map[string]any{
				"gemini-2.5-flash": "gemini-2.5-flash",
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "streamGenerateContent", true, body, false)
REDACTED
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)

	var wrapped map[string]any
	require.NoError(t, json.Unmarshal(upstream.requestBodies[0], &wrapped))
	require.Equal(t, "configured-project", wrapped["project"])
REDACTED

func TestAntigravityGatewayService_ForwardGemini_MissingProjectReturnsLocalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)
REDACTED
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-2.5-flash:streamGenerateContent", bytes.NewReader(body))

	upstream := &queuedHTTPUpstreamStub{REDACTED
	internal500Cache := &recordingInternal500CounterCache{REDACTED
	svc := &AntigravityGatewayService{
		tokenProvider:    &AntigravityTokenProvider{REDACTED,
		httpUpstream:     upstream,
		internal500Cache: internal500Cache,
REDACTED

	account := &Account{
		ID:          102,
		Name:        "acc-missing-project",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
			"model_mapping": map[string]any{
				"gemini-2.5-flash": "gemini-2.5-flash",
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "streamGenerateContent", true, body, false)
	require.Nil(t, result)
	require.ErrorIs(t, err, errAntigravityProjectIDRequired)
	require.Equal(t, http.StatusBadRequest, writer.Code)
	require.Empty(t, upstream.requestBodies)
	require.Empty(t, internal500Cache.incrementCalls)
	require.Contains(t, writer.Body.String(), "project_id")
	require.NotContains(t, writer.Body.String(), `"project":""`)
REDACTED

func TestAntigravityGatewayService_Forward_PromptTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"model": "claude-opus-4-6",
		"messages": []map[string]any{
			{"role": "user", "content": "hi"REDACTED,
	REDACTED,
		"max_tokens": 1,
		"stream":     false,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request = req

	respBody := []byte(`{"error":{"message":"Prompt is too long"REDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"X-Request-Id": []string{"req-1"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{REDACTED, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED),
		tokenProvider:  &AntigravityTokenProvider{REDACTED,
		httpUpstream:   &httpUpstreamStub{resp: respREDACTED,
REDACTED

	account := &Account{
		ID:          1,
		Name:        "acc-1",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
	REDACTED,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body, false)
	require.Nil(t, result)

	var promptErr *PromptTooLongError
	require.ErrorAs(t, err, &promptErr)
	require.Equal(t, http.StatusBadRequest, promptErr.StatusCode)
	require.Equal(t, "req-1", promptErr.RequestID)
	require.NotEmpty(t, promptErr.Body)

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "prompt_too_long", events[0].Kind)
REDACTED

// TestAntigravityGatewayService_Forward_ModelRateLimitTriggersFailover
// 验证：当账号存在模型限流且剩余时间 >= antigravityRateLimitThreshold 时，
// Forward 方法应返回 UpstreamFailoverError，触发 Handler 切换账号
func TestAntigravityGatewayService_Forward_ModelRateLimitTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"model": "claude-opus-4-6",
		"messages": []map[string]any{
			{"role": "user", "content": "hi"REDACTED,
	REDACTED,
		"max_tokens": 1,
		"stream":     false,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request = req

	// 不需要真正调用上游，因为预检查会直接返回切换信号
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  &httpUpstreamStub{resp: nil, err: nilREDACTED,
REDACTED

	// 设置模型限流：剩余时间 30 秒（> antigravityRateLimitThreshold 7s）
	futureResetAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
	account := &Account{
		ID:          1,
		Name:        "acc-rate-limited",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
	REDACTED,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-opus-4-6-thinking": map[string]any{
					"rate_limit_reset_at": futureResetAt,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body, false)
	require.Nil(t, result, "Forward should not return result when model rate limited")
	require.NotNil(t, err, "Forward should return error")

	// 核心验证：错误应该是 UpstreamFailoverError，而不是普通 502 错误
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "error should be UpstreamFailoverError to trigger account switch")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	// 非粘性会话请求，ForceCacheBilling 应为 false
	require.False(t, failoverErr.ForceCacheBilling, "ForceCacheBilling should be false for non-sticky session")
REDACTED

// TestAntigravityGatewayService_ForwardGemini_ModelRateLimitTriggersFailover
// 验证：ForwardGemini 方法同样能正确将 AntigravityAccountSwitchError 转换为 UpstreamFailoverError
func TestAntigravityGatewayService_ForwardGemini_ModelRateLimitTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	c.Request = req

	// 不需要真正调用上游，因为预检查会直接返回切换信号
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  &httpUpstreamStub{resp: nil, err: nilREDACTED,
REDACTED

	// 设置模型限流：剩余时间 30 秒（> antigravityRateLimitThreshold 7s）
	futureResetAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
	account := &Account{
		ID:          2,
		Name:        "acc-gemini-rate-limited",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
	REDACTED,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"gemini-2.5-flash": map[string]any{
					"rate_limit_reset_at": futureResetAt,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "generateContent", false, body, false)
	require.Nil(t, result, "ForwardGemini should not return result when model rate limited")
	require.NotNil(t, err, "ForwardGemini should return error")

	// 核心验证：错误应该是 UpstreamFailoverError，而不是普通 502 错误
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "error should be UpstreamFailoverError to trigger account switch")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	// 非粘性会话请求，ForceCacheBilling 应为 false
	require.False(t, failoverErr.ForceCacheBilling, "ForceCacheBilling should be false for non-sticky session")
REDACTED

// TestAntigravityGatewayService_Forward_StickySessionForceCacheBilling
// 验证：粘性会话切换时，UpstreamFailoverError.ForceCacheBilling 应为 true
func TestAntigravityGatewayService_Forward_StickySessionForceCacheBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"model":    "claude-opus-4-6",
		"messages": []map[string]string{{"role": "user", "content": "hello"REDACTEDREDACTED,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request = req

	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  &httpUpstreamStub{resp: nil, err: nilREDACTED,
REDACTED

	// 设置模型限流：剩余时间 30 秒（> antigravityRateLimitThreshold 7s）
	futureResetAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
	account := &Account{
		ID:          3,
		Name:        "acc-sticky-rate-limited",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
	REDACTED,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-opus-4-6-thinking": map[string]any{
					"rate_limit_reset_at": futureResetAt,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	// 传入 isStickySession = true
	result, err := svc.Forward(context.Background(), c, account, body, true)
	require.Nil(t, result, "Forward should not return result when model rate limited")
	require.NotNil(t, err, "Forward should return error")

	// 核心验证：粘性会话切换时，ForceCacheBilling 应为 true
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "error should be UpstreamFailoverError to trigger account switch")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.True(t, failoverErr.ForceCacheBilling, "ForceCacheBilling should be true for sticky session switch")
REDACTED

// TestAntigravityGatewayService_ForwardGemini_StickySessionForceCacheBilling verifies
// that ForwardGemini sets ForceCacheBilling=true for sticky session switch.
func TestAntigravityGatewayService_ForwardGemini_StickySessionForceCacheBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	c.Request = req

	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  &httpUpstreamStub{resp: nil, err: nilREDACTED,
REDACTED

	// 设置模型限流：剩余时间 30 秒（> antigravityRateLimitThreshold 7s）
	futureResetAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
	account := &Account{
		ID:          4,
		Name:        "acc-gemini-sticky-rate-limited",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
	REDACTED,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"gemini-2.5-flash": map[string]any{
					"rate_limit_reset_at": futureResetAt,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	// 传入 isStickySession = true
	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "generateContent", false, body, true)
	require.Nil(t, result, "ForwardGemini should not return result when model rate limited")
	require.NotNil(t, err, "ForwardGemini should return error")

	// 核心验证：粘性会话切换时，ForceCacheBilling 应为 true
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "error should be UpstreamFailoverError to trigger account switch")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.True(t, failoverErr.ForceCacheBilling, "ForceCacheBilling should be true for sticky session switch")
REDACTED

func TestAntigravityGatewayService_ForwardGemini_ClearsStickySessionOnGeminiRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", bytes.NewReader(body))
	c.Request = req

	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"REDACTED
			]
	REDACTED
REDACTED`)
	upstream := &httpUpstreamStub{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTEDREDACTED
	repo := &stubAntigravityAccountRepo{REDACTED
	cache := &stubSmartRetryCache{REDACTED
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  upstream,
		accountRepo:   repo,
		cache:         cache,
REDACTED

	account := &Account{
		ID:          44,
		Name:        "acc-gemini-runtime-rate-limited",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"access_token": "token",
			"expires_at":   time.Now().Add(time.Hour).Format(time.RFC3339),
	REDACTED
	REDACTED,
		Extra: map[string]any{
			"mixed_scheduling": true,
	REDACTED,
REDACTED

	result, err := svc.ForwardGemini(
		context.Background(),
		c,
		account,
		"gemini-3-flash-preview",
		"generateContent",
		false,
		body,
		true,
		WithForwardGeminiSession(77, "gemini:sticky-runtime"),
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.Len(t, repo.modelRateLimitCalls, 2)
	require.Equal(t, "gemini-3-flash", repo.modelRateLimitCalls[0].modelKey)
	require.Equal(t, antigravityGeminiModelRateLimitKey, repo.modelRateLimitCalls[1].modelKey)
	require.Len(t, cache.deleteCalls, 1)
	require.Equal(t, int64(77), cache.deleteCalls[0].groupID)
	require.Equal(t, "gemini:sticky-runtime", cache.deleteCalls[0].sessionHash)
REDACTED

// TestAntigravityGatewayService_Forward_BillsWithMappedModel
// 验证：Antigravity Claude 转发返回的计费模型使用映射后的模型
func TestAntigravityGatewayService_Forward_BillsWithMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"model": "claude-sonnet-4-5",
		"messages": []map[string]any{
			{"role": "user", "content": "hello"REDACTED,
	REDACTED,
		"max_tokens": 16,
		"stream":     true,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request = req

	upstreamBody := []byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"REDACTED]REDACTED,\"finishReason\":\"STOP\"REDACTED],\"usageMetadata\":{\"promptTokenCount\":8,\"candidatesTokenCount\":3REDACTEDREDACTEDREDACTED\n\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"req-bill-1"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader(upstreamBody)),
REDACTED

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{REDACTED, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED),
		tokenProvider:  &AntigravityTokenProvider{REDACTED,
		httpUpstream:   &httpUpstreamStub{resp: respREDACTED,
REDACTED

	const mappedModel = "gemini-3-pro-high"
	account := &Account{
		ID:          5,
		Name:        "acc-forward-billing",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
			"model_mapping": map[string]any{
				"claude-sonnet-4-5": mappedModel,
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body, false)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "claude-sonnet-4-5", result.Model)
	require.Equal(t, mappedModel, result.UpstreamModel)
REDACTED

// TestAntigravityGatewayService_ForwardGemini_BillsWithMappedModel
// 验证：Antigravity Gemini 转发返回的计费模型使用映射后的模型
func TestAntigravityGatewayService_ForwardGemini_BillsWithMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	c.Request = req

	upstreamBody := []byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"REDACTED]REDACTED,\"finishReason\":\"STOP\"REDACTED],\"usageMetadata\":{\"promptTokenCount\":8,\"candidatesTokenCount\":3REDACTEDREDACTEDREDACTED\n\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"req-bill-2"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader(upstreamBody)),
REDACTED

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{REDACTED, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED),
		tokenProvider:  &AntigravityTokenProvider{REDACTED,
		httpUpstream:   &httpUpstreamStub{resp: respREDACTED,
REDACTED

	const mappedModel = "gemini-3-pro-high"
	account := &Account{
		ID:          6,
		Name:        "acc-gemini-billing",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
			"model_mapping": map[string]any{
				"gemini-2.5-flash": mappedModel,
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "generateContent", true, body, false)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "gemini-2.5-flash", result.Model)
	require.Equal(t, mappedModel, result.UpstreamModel)
REDACTED

func TestAntigravityGatewayService_ForwardGemini_RetriesCorruptedThoughtSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"REDACTEDREDACTEDREDACTED,
			{"role": "model", "parts": []map[string]any{{"text": "thinking", "thought": true, "thoughtSignature": "sig_bad_1"REDACTEDREDACTEDREDACTED,
			{"role": "model", "parts": []map[string]any{{"functionCall": map[string]any{"name": "toolA", "args": map[string]any{"x": 1REDACTEDREDACTED, "thoughtSignature": "sig_bad_2"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-3.1-pro-preview:streamGenerateContent", bytes.NewReader(body))
	c.Request = req

	firstRespBody := []byte(`{"response":{"error":{"code":400,"message":"Corrupted thought signature.","status":"INVALID_ARGUMENT"REDACTEDREDACTEDREDACTED`)
	secondRespBody := []byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"REDACTED]REDACTED,\"finishReason\":\"STOP\"REDACTED],\"usageMetadata\":{\"promptTokenCount\":8,\"candidatesTokenCount\":3REDACTEDREDACTEDREDACTED\n\n")

	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header: http.Header{
					"Content-Type": []string{"application/json"REDACTED,
					"X-Request-Id": []string{"req-sig-1"REDACTED,
			REDACTED,
				Body: io.NopCloser(bytes.NewReader(firstRespBody)),
		REDACTED,
			{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"REDACTED,
					"X-Request-Id": []string{"req-sig-2"REDACTED,
			REDACTED,
				Body: io.NopCloser(bytes.NewReader(secondRespBody)),
		REDACTED,
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{REDACTED, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED),
		tokenProvider:  &AntigravityTokenProvider{REDACTED,
		httpUpstream:   upstream,
REDACTED

	const originalModel = "gemini-3.1-pro-preview"
	const mappedModel = "gemini-3.1-pro-high"
	account := &Account{
		ID:          7,
		Name:        "acc-gemini-signature",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
			"model_mapping": map[string]any{
				originalModel: mappedModel,
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.ForwardGemini(context.Background(), c, account, originalModel, "streamGenerateContent", true, body, false)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, originalModel, result.Model)
	require.Equal(t, mappedModel, result.UpstreamModel)
	require.Len(t, upstream.requestBodies, 2, "signature error should trigger exactly one retry")

	firstReq := string(upstream.requestBodies[0])
	secondReq := string(upstream.requestBodies[1])
	require.Contains(t, firstReq, `"thoughtSignature":"sig_bad_1"`)
	require.Contains(t, firstReq, `"thoughtSignature":"sig_bad_2"`)
	require.Contains(t, secondReq, `"thoughtSignature":"skip_thought_signature_validator"`)
	require.NotContains(t, secondReq, `"thoughtSignature":"sig_bad_1"`)
	require.NotContains(t, secondReq, `"thoughtSignature":"sig_bad_2"`)

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.NotEmpty(t, events)
	require.Equal(t, "signature_error", events[0].Kind)
REDACTED

func TestAntigravityGatewayService_ForwardGemini_SignatureRetryPropagatesFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"REDACTEDREDACTEDREDACTED,
			{"role": "model", "parts": []map[string]any{{"text": "thinking", "thought": true, "thoughtSignature": "sig_bad_1"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)
REDACTED

	req := httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-3.1-pro-preview:streamGenerateContent", bytes.NewReader(body))
	c.Request = req

	firstRespBody := []byte(`{"response":{"error":{"code":400,"message":"Corrupted thought signature.","status":"INVALID_ARGUMENT"REDACTEDREDACTEDREDACTED`)

	const originalModel = "gemini-3.1-pro-preview"
	const mappedModel = "gemini-3.1-pro-high"
	account := &Account{
		ID:          8,
		Name:        "acc-gemini-signature-failover",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"access_token": "token",
	REDACTED
			"model_mapping": map[string]any{
				originalModel: mappedModel,
		REDACTED,
	REDACTED,
REDACTED

	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header: http.Header{
					"Content-Type": []string{"application/json"REDACTED,
					"X-Request-Id": []string{"req-sig-failover-1"REDACTED,
			REDACTED,
				Body: io.NopCloser(bytes.NewReader(firstRespBody)),
		REDACTED,
	REDACTED,
		onCall: func(_ *http.Request, stub *queuedHTTPUpstreamStub) {
			if stub.callCount != 1 {
				return
		REDACTED
			futureResetAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
			account.Extra = map[string]any{
				modelRateLimitsKey: map[string]any{
					mappedModel: map[string]any{
						"rate_limit_reset_at": futureResetAt,
				REDACTED,
			REDACTED,
		REDACTED
	REDACTED,
REDACTED

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{REDACTED, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED),
		tokenProvider:  &AntigravityTokenProvider{REDACTED,
		httpUpstream:   upstream,
REDACTED

	result, err := svc.ForwardGemini(context.Background(), c, account, originalModel, "streamGenerateContent", true, body, true)
	require.Nil(t, result)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "signature retry should propagate failover instead of falling back to the original 400")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.True(t, failoverErr.ForceCacheBilling)
	require.Len(t, upstream.requestBodies, 1, "retry should stop at preflight failover and not issue a second upstream request")

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 2)
	require.Equal(t, "signature_error", events[0].Kind)
	require.Equal(t, "failover", events[1].Kind)
REDACTED

// TestStreamUpstreamResponse_UsageAndFirstToken
// 验证：usage 字段可被累积/覆盖更新，并且能记录首 token 时间
func TestStreamUpstreamResponse_UsageAndFirstToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		fmt.Fprintln(pw, `data: {"usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4REDACTEDREDACTED`)
		fmt.Fprintln(pw, `data: {"usage":{"output_tokens":5REDACTEDREDACTED`)
REDACTED()

	start := time.Now().Add(-10 * time.Millisecond)
	result := svc.streamUpstreamResponse(c, resp, start)
	_ = pr.Close()

	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 1, result.usage.InputTokens)
	// 第二次事件覆盖 output_tokens
	require.Equal(t, 5, result.usage.OutputTokens)
	require.Equal(t, 3, result.usage.CacheReadInputTokens)
	require.Equal(t, 4, result.usage.CacheCreationInputTokens)
	require.NotNil(t, result.firstTokenMs)

	// 确保有透传输出
	require.Contains(t, rec.Body.String(), "data:")
REDACTED

// --- 流式 happy path 测试 ---

// TestStreamUpstreamResponse_NormalComplete
// 验证：正常流式转发完成时，数据正确透传、usage 正确收集、clientDisconnect=false
func TestStreamUpstreamResponse_NormalComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		fmt.Fprintln(pw, `event: message_start`)
		fmt.Fprintln(pw, `data: {"type":"message_start","message":{"usage":{"input_tokens":10REDACTEDREDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
		fmt.Fprintln(pw, `event: content_block_delta`)
		fmt.Fprintln(pw, `data: {"type":"content_block_delta","delta":{"text":"hello"REDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
		fmt.Fprintln(pw, `event: message_delta`)
		fmt.Fprintln(pw, `data: {"type":"message_delta","usage":{"output_tokens":5REDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
REDACTED()

	result := svc.streamUpstreamResponse(c, resp, time.Now())
	_ = pr.Close()

	require.NotNil(t, result)
	require.False(t, result.clientDisconnect, "normal completion should not set clientDisconnect")
	require.NotNil(t, result.usage)
	require.Equal(t, 5, result.usage.OutputTokens, "should collect output_tokens from message_delta")
	require.NotNil(t, result.firstTokenMs, "should record first token time")

	// 验证数据被透传到客户端
	body := rec.Body.String()
	require.Contains(t, body, "event: message_start")
	require.Contains(t, body, "content_block_delta")
	require.Contains(t, body, "message_delta")
REDACTED

// TestHandleGeminiStreamingResponse_NormalComplete
// 验证：正常 Gemini 流式转发，数据正确透传、usage 正确收集
func TestHandleGeminiStreamingResponse_NormalComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// 第一个 chunk（部分内容）
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":"Hello"REDACTED]REDACTEDREDACTED],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3REDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
		// 第二个 chunk（最终内容+完整 usage）
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":" world"REDACTED]REDACTED,"finishReason":"STOP"REDACTED],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":8,"cachedContentTokenCount":2REDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
REDACTED()

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())
	_ = pr.Close()

REDACTED
	require.NotNil(t, result)
	require.False(t, result.clientDisconnect, "normal completion should not set clientDisconnect")
	require.NotNil(t, result.usage)
	// Gemini usage: promptTokenCount=10, candidatesTokenCount=8, cachedContentTokenCount=2
	// → InputTokens=10-2=8, OutputTokens=8, CacheReadInputTokens=2
	require.Equal(t, 8, result.usage.InputTokens)
	require.Equal(t, 8, result.usage.OutputTokens)
	require.Equal(t, 2, result.usage.CacheReadInputTokens)
	require.NotNil(t, result.firstTokenMs, "should record first token time")

	// 验证数据被透传到客户端
	body := rec.Body.String()
	require.Contains(t, body, "Hello")
	require.Contains(t, body, "world")
	// 不应包含错误事件
	require.NotContains(t, body, "event: error")
REDACTED

// TestHandleClaudeStreamingResponse_NormalComplete
// 验证：正常 Claude 流式转发（Gemini→Claude 转换），数据正确转换并输出
func TestHandleClaudeStreamingResponse_NormalComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// v1internal 包装格式：Gemini 数据嵌套在 "response" 字段下
		// ProcessLine 先尝试反序列化为 V1InternalResponse，裸格式会导致 Response.UsageMetadata 为空
		fmt.Fprintln(pw, `data: {"response":{"candidates":[{"content":{"parts":[{"text":"Hi there"REDACTED]REDACTED,"finishReason":"STOP"REDACTED],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3REDACTEDREDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
REDACTED()

	result, err := svc.handleClaudeStreamingResponse(c, resp, time.Now(), "claude-sonnet-4-5")
	_ = pr.Close()

REDACTED
	require.NotNil(t, result)
	require.False(t, result.clientDisconnect, "normal completion should not set clientDisconnect")
	require.NotNil(t, result.usage)
	// Gemini→Claude 转换的 usage：promptTokenCount=5→InputTokens=5, candidatesTokenCount=3→OutputTokens=3
	require.Equal(t, 5, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.NotNil(t, result.firstTokenMs, "should record first token time")

	// 验证输出是 Claude SSE 格式（processor 会转换）
	body := rec.Body.String()
	require.Contains(t, body, "event: message_start", "should contain Claude message_start event")
	require.Contains(t, body, "event: message_stop", "should contain Claude message_stop event")
	// 不应包含错误事件
	require.NotContains(t, body, "event: error")
REDACTED

// TestHandleGeminiStreamingResponse_ThoughtsTokenCount
// 验证：Gemini 流式转发时 thoughtsTokenCount 被计入 OutputTokens
func TestHandleGeminiStreamingResponse_ThoughtsTokenCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":"Hello"REDACTED]REDACTEDREDACTED],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":50REDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":" world"REDACTED]REDACTED,"finishReason":"STOP"REDACTED],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":30,"thoughtsTokenCount":80,"cachedContentTokenCount":10REDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
REDACTED()

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())
	_ = pr.Close()

REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	// promptTokenCount=100, cachedContentTokenCount=10 → InputTokens=90
	require.Equal(t, 90, result.usage.InputTokens)
	// candidatesTokenCount=30 + thoughtsTokenCount=80 → OutputTokens=110
	require.Equal(t, 110, result.usage.OutputTokens)
	require.Equal(t, 10, result.usage.CacheReadInputTokens)
REDACTED

// TestHandleClaudeStreamingResponse_ThoughtsTokenCount
// 验证：Gemini→Claude 流式转换时 thoughtsTokenCount 被计入 OutputTokens
func TestHandleClaudeStreamingResponse_ThoughtsTokenCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		fmt.Fprintln(pw, `data: {"response":{"candidates":[{"content":{"parts":[{"text":"Hi"REDACTED]REDACTED,"finishReason":"STOP"REDACTED],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":10,"thoughtsTokenCount":25REDACTEDREDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
REDACTED()

	result, err := svc.handleClaudeStreamingResponse(c, resp, time.Now(), "gemini-2.5-pro")
	_ = pr.Close()

REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	// promptTokenCount=50 → InputTokens=50
	require.Equal(t, 50, result.usage.InputTokens)
	// candidatesTokenCount=10 + thoughtsTokenCount=25 → OutputTokens=35
	require.Equal(t, 35, result.usage.OutputTokens)
REDACTED

// --- 流式客户端断开检测测试 ---

// TestStreamUpstreamResponse_ClientDisconnectDrainsUsage
// 验证：客户端写入失败后，streamUpstreamResponse 继续读取上游以收集 usage
func TestStreamUpstreamResponse_ClientDisconnectDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		fmt.Fprintln(pw, `event: message_start`)
		fmt.Fprintln(pw, `data: {"type":"message_start","message":{"usage":{"input_tokens":10REDACTEDREDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
		fmt.Fprintln(pw, `event: message_delta`)
		fmt.Fprintln(pw, `data: {"type":"message_delta","usage":{"output_tokens":20REDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
REDACTED()

	result := svc.streamUpstreamResponse(c, resp, time.Now())
	_ = pr.Close()

	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.NotNil(t, result.usage)
	require.Equal(t, 20, result.usage.OutputTokens)
REDACTED

// TestStreamUpstreamResponse_ContextCanceled
// 验证：context 取消时返回 usage 且标记 clientDisconnect
func TestStreamUpstreamResponse_ContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)

	resp := &http.Response{StatusCode: http.StatusOK, Body: cancelReadCloser{REDACTED, Header: http.Header{REDACTEDREDACTED

	result := svc.streamUpstreamResponse(c, resp, time.Now())

	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.NotContains(t, rec.Body.String(), "event: error")
REDACTED

// TestStreamUpstreamResponse_Timeout
// 验证：上游超时时返回已收集的 usage
func TestStreamUpstreamResponse_Timeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1, MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	result := svc.streamUpstreamResponse(c, resp, time.Now())
	_ = pw.Close()
	_ = pr.Close()

	require.NotNil(t, result)
	require.False(t, result.clientDisconnect)
REDACTED

// TestStreamUpstreamResponse_TimeoutAfterClientDisconnect
// 验证：客户端断开后上游超时，返回 usage 并标记 clientDisconnect
func TestStreamUpstreamResponse_TimeoutAfterClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1, MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		fmt.Fprintln(pw, `data: {"type":"message_start","message":{"usage":{"input_tokens":5REDACTEDREDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
		// 不关闭 pw → 等待超时
REDACTED()

	result := svc.streamUpstreamResponse(c, resp, time.Now())
	_ = pw.Close()
	_ = pr.Close()

	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
REDACTED

// TestHandleGeminiStreamingResponse_ClientDisconnect
// 验证：Gemini 流式转发中客户端断开后继续 drain 上游
func TestHandleGeminiStreamingResponse_ClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":"hi"REDACTED]REDACTEDREDACTED],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":10REDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
REDACTED()

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())
	_ = pr.Close()

REDACTED
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.NotContains(t, rec.Body.String(), "write_failed")
REDACTED

// TestHandleGeminiStreamingResponse_ContextCanceled
// 验证：context 取消时不注入错误事件
func TestHandleGeminiStreamingResponse_ContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)

	resp := &http.Response{StatusCode: http.StatusOK, Body: cancelReadCloser{REDACTED, Header: http.Header{REDACTEDREDACTED

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())

REDACTED
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.NotContains(t, rec.Body.String(), "event: error")
REDACTED

// TestHandleClaudeStreamingResponse_ClientDisconnect
// 验证：Claude 流式转发中客户端断开后继续 drain 上游
func TestHandleClaudeStreamingResponse_ClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// v1internal 包装格式
		fmt.Fprintln(pw, `data: {"response":{"candidates":[{"content":{"parts":[{"text":"hello"REDACTED]REDACTED,"finishReason":"STOP"REDACTED],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":15REDACTEDREDACTEDREDACTED`)
		fmt.Fprintln(pw, "")
REDACTED()

	result, err := svc.handleClaudeStreamingResponse(c, resp, time.Now(), "claude-sonnet-4-5")
	_ = pr.Close()

REDACTED
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
REDACTED

// TestHandleClaudeStreamingResponse_EmptyStream
// 验证：上游只返回无法解析的 SSE 行时，触发 UpstreamFailoverError 而不是向客户端发出残缺流
func TestHandleClaudeStreamingResponse_EmptyStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{REDACTEDREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// 所有行均为无法 JSON 解析的内容，ProcessLine 全部返回 nil
		fmt.Fprintln(pw, "data: not-valid-json")
		fmt.Fprintln(pw, "")
		fmt.Fprintln(pw, "data: also-invalid")
		fmt.Fprintln(pw, "")
REDACTED()

	_, err := svc.handleClaudeStreamingResponse(c, resp, time.Now(), "claude-sonnet-4-5")
	_ = pr.Close()

	// 应当返回 UpstreamFailoverError 而非 nil，以便上层触发 failover
REDACTED
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)

	// 客户端不应收到任何 SSE 事件（既无 message_start 也无 message_stop）
	body := rec.Body.String()
	require.NotContains(t, body, "event: message_start")
	require.NotContains(t, body, "event: message_stop")
	require.NotContains(t, body, "event: message_delta")
REDACTED

// TestHandleClaudeStreamingResponse_ContextCanceled
// 验证：context 取消时不注入错误事件
func TestHandleClaudeStreamingResponse_ContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTED)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)

	resp := &http.Response{StatusCode: http.StatusOK, Body: cancelReadCloser{REDACTED, Header: http.Header{REDACTEDREDACTED

	result, err := svc.handleClaudeStreamingResponse(c, resp, time.Now(), "claude-sonnet-4-5")

REDACTED
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.NotContains(t, rec.Body.String(), "event: error")
REDACTED

// TestExtractSSEUsage 验证 extractSSEUsage 从 SSE data 行正确提取 usage
func TestExtractSSEUsage(t *testing.T) {
	svc := &AntigravityGatewayService{REDACTED
	tests := []struct {
		name     string
		line     string
		expected ClaudeUsage
REDACTED{
		{
			name:     "message_delta with output_tokens",
			line:     `data: {"type":"message_delta","usage":{"output_tokens":42REDACTEDREDACTED`,
			expected: ClaudeUsage{OutputTokens: 42REDACTED,
	REDACTED,
		{
			name:     "non-data line ignored",
			line:     `event: message_start`,
			expected: ClaudeUsage{REDACTED,
	REDACTED,
		{
			name:     "top-level usage with all fields",
			line:     `data: {"usage":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":5,"cache_creation_input_tokens":3REDACTEDREDACTED`,
			expected: ClaudeUsage{InputTokens: 10, OutputTokens: 20, CacheReadInputTokens: 5, CacheCreationInputTokens: 3REDACTED,
	REDACTED,
		{
			// Anthropic message_start 把 usage 嵌套在 message.usage 下，
			// 必须从这里提取输入侧字段（含 cache_read/cache_creation_input_tokens）。
			name:     "message_start nested usage with input/cache tokens",
			line:     `data: {"type":"message_start","message":{"id":"msg_01","usage":{"input_tokens":35576,"cache_creation_input_tokens":0,"cache_read_input_tokens":12000,"output_tokens":1REDACTEDREDACTEDREDACTED`,
			expected: ClaudeUsage{InputTokens: 35576, OutputTokens: 1, CacheReadInputTokens: 12000REDACTED,
	REDACTED,
		{
			// message_start.message.usage.cache_creation 内的 5m/1h 明细也要解析。
			name:     "message_start nested usage with cache_creation breakdown",
			line:     `data: {"type":"message_start","message":{"usage":{"input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":70REDACTEDREDACTEDREDACTEDREDACTED`,
			expected: ClaudeUsage{InputTokens: 100, CacheCreation5mTokens: 30, CacheCreation1hTokens: 70REDACTED,
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &ClaudeUsage{REDACTED
			svc.extractSSEUsage(tt.line, usage)
			require.Equal(t, tt.expected, *usage)
	REDACTED)
REDACTED
REDACTED

// TestExtractSSEUsage_StreamingSequence 复现 issue #2332：完整的 Anthropic streaming
// 序列（message_start → message_delta）必须把两类事件中的 usage 字段都汇入同一份累计值，
// 否则透传账号产出的 usage_logs 会出现 input_tokens=0、仅有 output_tokens 的"残缺"记录。
func TestExtractSSEUsage_StreamingSequence(t *testing.T) {
	svc := &AntigravityGatewayService{REDACTED
	usage := &ClaudeUsage{REDACTED

	// 1) message_start：携带完整输入侧 usage（input_tokens + cache_read）
	svc.extractSSEUsage(
		`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":35576,"cache_creation_input_tokens":0,"cache_read_input_tokens":12000,"output_tokens":1REDACTEDREDACTEDREDACTED`,
		usage,
	)
	// 2) message_delta：流结束时只带 output_tokens（无 input_tokens 字段）
	svc.extractSSEUsage(
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":nullREDACTED,"usage":{"output_tokens":816REDACTEDREDACTED`,
		usage,
	)

	require.Equal(t, 35576, usage.InputTokens, "message_start 的 input_tokens 必须被记录，否则记账会缺失输入侧 token (#2332)")
	require.Equal(t, 12000, usage.CacheReadInputTokens, "message_start 的 cache_read_input_tokens 必须被记录")
	require.Equal(t, 816, usage.OutputTokens, "message_delta 的最终 output_tokens 必须被记录")
REDACTED

// TestAntigravityClientWriter 验证 antigravityClientWriter 的断开检测
func TestAntigravityClientWriter(t *testing.T) {
	t.Run("normal write succeeds", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(c.Writer, flusher, "test")

		ok := cw.Write([]byte("hello"))
		require.True(t, ok)
		require.False(t, cw.Disconnected())
		require.Contains(t, rec.Body.String(), "hello")
REDACTED)

	t.Run("write failure marks disconnected", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		fw := &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(fw, flusher, "test")

		ok := cw.Write([]byte("hello"))
		require.False(t, ok)
		require.True(t, cw.Disconnected())
REDACTED)

	t.Run("subsequent writes are no-op", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		fw := &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(fw, flusher, "test")

		cw.Write([]byte("first"))
		ok := cw.Fprintf("second %d", 2)
		require.False(t, ok)
		require.True(t, cw.Disconnected())
REDACTED)
REDACTED

// TestUnwrapV1InternalResponse 测试 unwrapV1InternalResponse 的各种输入场景
func TestUnwrapV1InternalResponse(t *testing.T) {
	svc := &AntigravityGatewayService{REDACTED

	// 构造 >50KB 的大型 JSON
	largePadding := strings.Repeat("x", 50*1024)
	largeInput := []byte(fmt.Sprintf(`{"response":{"id":"big","pad":"%s"REDACTEDREDACTED`, largePadding))
	largeExpected := fmt.Sprintf(`{"id":"big","pad":"%s"REDACTED`, largePadding)

	tests := []struct {
		name     string
		input    []byte
		expected string
		wantErr  bool
REDACTED{
		{
			name:     "正常 response 包装",
			input:    []byte(`{"response":{"id":"123","content":"hello"REDACTEDREDACTED`),
			expected: `{"id":"123","content":"hello"REDACTED`,
	REDACTED,
		{
			name:     "无 response 透传",
			input:    []byte(`{"id":"456"REDACTED`),
			expected: `{"id":"456"REDACTED`,
	REDACTED,
		{
			name:     "空 JSON",
			input:    []byte(`{REDACTED`),
			expected: `{REDACTED`,
	REDACTED,
		{
			name:     "response 为 null",
			input:    []byte(`{"response":nullREDACTED`),
			expected: `null`,
	REDACTED,
		{
			name:     "response 为基础类型 string",
			input:    []byte(`{"response":"hello"REDACTED`),
			expected: `"hello"`,
	REDACTED,
		{
			name:     "非法 JSON",
			input:    []byte(`not json`),
			expected: `not json`,
	REDACTED,
		{
			name:     "嵌套 response 只解一层",
			input:    []byte(`{"response":{"response":{"inner":trueREDACTEDREDACTEDREDACTED`),
			expected: `{"response":{"inner":trueREDACTEDREDACTED`,
	REDACTED,
		{
			name:     "大型 JSON >50KB",
			input:    largeInput,
			expected: largeExpected,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.unwrapV1InternalResponse(tt.input)
			if tt.wantErr {
			REDACTED
				return
		REDACTED
		REDACTED
			require.Equal(t, tt.expected, strings.TrimSpace(string(got)))
	REDACTED)
REDACTED
REDACTED

// --- unwrapV1InternalResponse benchmark 对照组 ---

// unwrapV1InternalResponseOld 旧实现：Unmarshal+Marshal 双重开销（仅用于 benchmark 对照）
func unwrapV1InternalResponseOld(body []byte) ([]byte, error) {
	var outer map[string]any
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, err
REDACTED
	if resp, ok := outer["response"]; ok {
		return json.Marshal(resp)
REDACTED
	return body, nil
REDACTED

func BenchmarkUnwrapV1Internal_Old_Small(b *testing.B) {
	body := []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"hello world"REDACTED]REDACTEDREDACTED],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5REDACTEDREDACTEDREDACTED`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = unwrapV1InternalResponseOld(body)
REDACTED
REDACTED

func BenchmarkUnwrapV1Internal_New_Small(b *testing.B) {
	body := []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"hello world"REDACTED]REDACTEDREDACTED],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5REDACTEDREDACTEDREDACTED`)
	svc := &AntigravityGatewayService{REDACTED
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.unwrapV1InternalResponse(body)
REDACTED
REDACTED

func BenchmarkUnwrapV1Internal_Old_Large(b *testing.B) {
	body := generateLargeUnwrapJSON(10 * 1024) // ~10KB
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = unwrapV1InternalResponseOld(body)
REDACTED
REDACTED

func BenchmarkUnwrapV1Internal_New_Large(b *testing.B) {
	body := generateLargeUnwrapJSON(10 * 1024) // ~10KB
	svc := &AntigravityGatewayService{REDACTED
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.unwrapV1InternalResponse(body)
REDACTED
REDACTED

// generateLargeUnwrapJSON 生成指定最小大小的包含 response 包装的 JSON
func generateLargeUnwrapJSON(minSize int) []byte {
	parts := make([]map[string]string, 0)
	current := 0
	for current < minSize {
		text := fmt.Sprintf("这是第 %d 段内容，用于填充 JSON 到目标大小。", len(parts)+1)
		parts = append(parts, map[string]string{"text": textREDACTED)
		current += len(text) + 20 // 估算 JSON 编码开销
REDACTED
	inner := map[string]any{
		"candidates": []map[string]any{
			{"content": map[string]any{"parts": partsREDACTEDREDACTED,
	REDACTED,
		"usageMetadata": map[string]any{
			"promptTokenCount":     100,
			"candidatesTokenCount": 50,
	REDACTED,
REDACTED
	outer := map[string]any{"response": innerREDACTED
	b, _ := json.Marshal(outer)
	return b
REDACTED
