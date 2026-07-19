//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokChatResponsesBridgeEligibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   string
		want   bool
		reason string
REDACTED{
		{
			name: "plain text chat",
			body: `{"model":"grok","messages":[{"role":"system","content":"concise"REDACTED,{"role":"user","content":"hi"REDACTED],"stream":falseREDACTED`,
			want: true,
	REDACTED,
		{
			name: "safe generation options",
			body: `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"stream":true,"stream_options":{"include_usage":trueREDACTED,"max_completion_tokens":256,"temperature":0.2,"top_p":0.9,"prompt_cache_key":"session","tools":[],"functions":null,"tool_choice":"none"REDACTED`,
			want: true,
	REDACTED,
		{
			name:   "stop falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"stop":"done"REDACTED`,
			reason: "unsupported_stop",
	REDACTED,
		{
			name:   "developer role falls back",
			body:   `{"model":"grok","messages":[{"role":"developer","content":"rules"REDACTED,{"role":"user","content":"hi"REDACTED]REDACTED`,
			reason: "unsupported_message_role_developer",
	REDACTED,
		{
			name: "image content is bridgeable",
			body: `{"model":"grok","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="REDACTEDREDACTED]REDACTED]REDACTED`,
			want: true,
	REDACTED,
		{
			name: "text and image parts are bridgeable",
			body: `{"model":"grok","messages":[{"role":"user","content":[{"type":"text","text":"what is this"REDACTED,{"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="REDACTEDREDACTED]REDACTED]REDACTED`,
			want: true,
	REDACTED,
		{
			name: "text only parts are bridgeable",
			body: `{"model":"grok","messages":[{"role":"user","content":[{"type":"text","text":"hello"REDACTED]REDACTED]REDACTED`,
			want: true,
	REDACTED,
		{
			name:   "unknown content part falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"AA=="REDACTEDREDACTED]REDACTED]REDACTED`,
			reason: "unsupported_content_part_input_audio",
	REDACTED,
		{
			name:   "empty content array falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":[]REDACTED]REDACTED`,
			reason: "empty_message_content",
	REDACTED,
		{
			name: "function tools bridge",
			body: `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"REDACTED,"strict":falseREDACTEDREDACTED]REDACTED`,
			want: true,
	REDACTED,
		{
			name:   "legacy functions fall back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"functions":[{"name":"lookup","parameters":{"type":"object"REDACTEDREDACTED]REDACTED`,
			reason: "unsupported_functions",
	REDACTED,
		{
			name: "automatic tool choice bridges",
			body: `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"tools":[],"tool_choice":"auto"REDACTED`,
			want: true,
	REDACTED,
		{
			name:   "required tool choice without tools falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"tools":[],"tool_choice":"required"REDACTED`,
			reason: "required_tool_choice_without_tools",
	REDACTED,
		{
			name: "tool history bridges",
			body: `{"model":"grok","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"REDACTED"REDACTEDREDACTED]REDACTED,{"role":"tool","tool_call_id":"call_lookup","content":"{\"value\":\"ok\"REDACTED"REDACTED,{"role":"user","content":"summarize"REDACTED],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"REDACTEDREDACTEDREDACTED],"tool_choice":"auto","parallel_tool_calls":trueREDACTED`,
			want: true,
	REDACTED,
		{
			name:   "unknown tool type falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"tools":[{"type":"web_search","function":{"name":"lookup","parameters":{"type":"object"REDACTEDREDACTEDREDACTED]REDACTED`,
			reason: "unsupported_tool_type",
	REDACTED,
		{
			name:   "missing tool schema falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"tools":[{"type":"function","function":{"name":"lookup"REDACTEDREDACTED]REDACTED`,
			reason: "invalid_tool_function_parameters",
	REDACTED,
		{
			name:   "named tool choice falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"REDACTEDREDACTEDREDACTED],"tool_choice":{"type":"function","function":{"name":"lookup"REDACTEDREDACTEDREDACTED`,
			reason: "unsupported_tool_choice",
	REDACTED,
		{
			name:   "invalid tool call arguments fall back",
			body:   `{"model":"grok","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{"REDACTEDREDACTED]REDACTED]REDACTED`,
			reason: "invalid_tool_call_arguments",
	REDACTED,
		{
			name:   "tool result without call id falls back",
			body:   `{"model":"grok","messages":[{"role":"tool","content":"ok"REDACTED]REDACTED`,
			reason: "invalid_tool_call_id",
	REDACTED,
		{
			name:   "non boolean parallel tool calls falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"parallel_tool_calls":"true"REDACTED`,
			reason: "invalid_parallel_tool_calls",
	REDACTED,
		{
			name:   "reasoning effort falls back because conversion adds summary",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"reasoning_effort":"high"REDACTED`,
			reason: "unsupported_reasoning_effort",
	REDACTED,
		{
			name:   "both token limits fall back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"max_tokens":256,"max_completion_tokens":256REDACTED`,
			reason: "conflicting_max_tokens",
	REDACTED,
		{
			name:   "empty message falls back",
			body:   `{"model":"grok","messages":[{"role":"assistant","content":""REDACTED,{"role":"user","content":"hi"REDACTED]REDACTED`,
			reason: "empty_message_content",
	REDACTED,
		{
			name:   "empty tool history falls back",
			body:   `{"model":"grok","messages":[{"role":"assistant","content":"","tool_calls":[]REDACTED]REDACTED`,
			reason: "empty_message_content",
	REDACTED,
		{
			name:   "unknown field falls back",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"seed":7REDACTED`,
			reason: "unknown_field_seed",
	REDACTED,
		{
			name:   "small max tokens falls back because conversion clamps it",
			body:   `{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"max_tokens":32REDACTED`,
			reason: "unsafe_max_tokens",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := grokChatResponsesBridgeEligibility([]byte(tt.body))
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.reason, reason)
	REDACTED)
REDACTED
REDACTED

func TestGrokChatResponsesRuntimeEligibility(t *testing.T) {
	t.Parallel()
	require.True(t, grokChatResponsesRuntimeEligible("grok-4.5", "isolated-id"))
	require.False(t, grokChatResponsesRuntimeEligible("grok-4.3", "isolated-id"))
	require.False(t, grokChatResponsesRuntimeEligible("grok-4.5-build-free", "isolated-id"))
	require.False(t, grokChatResponsesRuntimeEligible("grok-4.5", ""))
REDACTED

func TestForwardGrokChatViaResponsesNonStreamingCachesAndReturnsChat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"system","content":"be concise"REDACTED,{"role":"user","content":"hi"REDACTED],"stream":false,"prompt_cache_key":"stable-session","tools":[],"functions":null,"tool_choice":"none"REDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7101REDACTED)

	account := grokChatBridgeTestAccount(71)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: grokChatBridgeCompletedResponse("resp_grok_chat_cache", 9856)REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, "grok-4.5", result.UpstreamModel)
	require.Equal(t, 9908, result.Usage.InputTokens)
	require.Equal(t, 12, result.Usage.OutputTokens)
	require.Equal(t, 9856, result.Usage.CacheReadInputTokens)

	identity := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	require.NotEmpty(t, identity)
	require.NotEqual(t, "stable-session", identity)
	require.Equal(t, identity, upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "x_search", gjson.GetBytes(upstream.lastBody, "tools.1.type").String())
	require.Equal(t, grokFreeCacheDisabledToolChoice, gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "system", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.1.role").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "include").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "store").Exists())

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "cached ok", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
	require.Equal(t, int64(9856), gjson.Get(recorder.Body.String(), "usage.prompt_tokens_details.cached_tokens").Int())
	require.NotNil(t, repo.updates[account.ID][grokQuotaSnapshotExtraKey])
REDACTED

func TestForwardGrokChatViaResponsesTraeToolHistoryKeepsCacheRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	firstTurnBody := []byte(`{"model":"grok","messages":[{"role":"system","content":"Be concise"REDACTED,{"role":"user","content":"Find alpha"REDACTED],"stream":false,"prompt_cache_key":"trae-session","tools":[{"type":"function","function":{"name":"lookup","description":"Lookup a value","parameters":{"type":"object","properties":{"key":{"type":"string"REDACTEDREDACTED,"required":["key"]REDACTED,"strict":falseREDACTEDREDACTED],"tool_choice":"auto","parallel_tool_calls":trueREDACTED`)
	body := []byte(`{"model":"grok","messages":[{"role":"system","content":"Be concise"REDACTED,{"role":"user","content":"Find alpha"REDACTED,{"role":"assistant","content":null,"tool_calls":[{"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"alpha\"REDACTED"REDACTEDREDACTED]REDACTED,{"role":"tool","tool_call_id":"call_lookup","content":"{\"value\":\"ok\"REDACTED"REDACTED,{"role":"user","content":"Summarize"REDACTED],"stream":false,"prompt_cache_key":"trae-session","tools":[{"type":"function","function":{"name":"lookup","description":"Lookup a value","parameters":{"type":"object","properties":{"key":{"type":"string"REDACTEDREDACTED,"required":["key"]REDACTED,"strict":falseREDACTEDREDACTED],"tool_choice":"auto","parallel_tool_calls":trueREDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Request.Header.Set(grokClientToolCacheOptInHeader, "prefer-cache")
	c.Set("api_key", &APIKey{ID: 7151REDACTED)

	account := grokChatBridgeTestAccount(715)
	account.Credentials["subscription_tier"] = "free"
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: grokChatBridgeCompletedResponse("resp_grok_chat_trae", 8192)REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	firstTurnIdentity := resolveGrokCacheIdentity(c, firstTurnBody, "", "grok-4.5")
	extendedTurnIdentity := resolveGrokCacheIdentity(c, body, "", "grok-4.5")
	require.NotEmpty(t, firstTurnIdentity)
	require.Equal(t, firstTurnIdentity, extendedTurnIdentity)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, extendedTurnIdentity, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, extendedTurnIdentity, upstream.lastReq.Header.Get(grokConversationIDHeader))

	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 3)
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "lookup", tools[0].Get("name").String())
	require.Equal(t, "Lookup a value", tools[0].Get("description").String())
	require.Equal(t, "string", tools[0].Get("parameters.properties.key.type").String())
	require.True(t, tools[0].Get("strict").Exists())
	require.False(t, tools[0].Get("strict").Bool())
	require.Equal(t, "web_search", tools[1].Get("type").String())
	require.Equal(t, "x_search", tools[2].Get("type").String())
	require.Equal(t, "auto", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Bool())

	require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.2.type").String())
	require.Equal(t, "call_lookup", gjson.GetBytes(upstream.lastBody, "input.2.call_id").String())
	require.Equal(t, "lookup", gjson.GetBytes(upstream.lastBody, "input.2.name").String())
	require.Equal(t, `{"key":"alpha"REDACTED`, gjson.GetBytes(upstream.lastBody, "input.2.arguments").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.3.type").String())
	require.Equal(t, "call_lookup", gjson.GetBytes(upstream.lastBody, "input.3.call_id").String())
	require.Equal(t, `{"value":"ok"REDACTED`, gjson.GetBytes(upstream.lastBody, "input.3.output").String())
REDACTED

func TestForwardGrokChatViaResponsesStreamingPropagatesCachedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"stream":trueREDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7201REDACTED)

	account := grokChatBridgeTestAccount(72)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: grokChatBridgeCompletedResponse("resp_grok_chat_stream", 4096)REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
REDACTED
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, 4096, result.Usage.CacheReadInputTokens)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, recorder.Body.String(), `"content":"cached ok"`)
	require.Contains(t, recorder.Body.String(), `"cached_tokens":4096`)
	require.Contains(t, recorder.Body.String(), "data: [DONE]")
REDACTED

func TestForwardGrokChatRuntimeGateFallsBackToRaw(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		setAPIKey    bool
		mappedModel  string
		wantUpstream string
REDACTED{
		{name: "missing cache identity", wantUpstream: "grok-4.5"REDACTED,
		{name: "non cache capable mapped model", setAPIKey: true, mappedModel: "grok-4.3", wantUpstream: "grok-4.3"REDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"stream":falseREDACTED`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
			if tt.setAPIKey {
				c.Set("api_key", &APIKey{ID: int64(7301 + index)REDACTED)
		REDACTED

			account := grokChatBridgeTestAccount(int64(73 + index))
			if tt.mappedModel != "" {
				account.Credentials["model_mapping"] = map[string]any{"grok": tt.mappedModelREDACTED
		REDACTED
			repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
				accountsByID: map[int64]*Account{account.ID: accountREDACTED,
		REDACTEDREDACTED
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
				Body: io.NopCloser(strings.NewReader(
					`{"id":"chat_raw","object":"chat.completion","model":"` + tt.wantUpstream + `","choices":[{"index":0,"message":{"role":"assistant","content":"raw ok"REDACTED,"finish_reason":"stop"REDACTED],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3REDACTEDREDACTED`,
				)),
		REDACTEDREDACTED
			svc := &OpenAIGatewayService{
				httpUpstream:      upstream,
				grokTokenProvider: NewGrokTokenProvider(repo, nil),
				accountRepo:       repo,
		REDACTED

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		REDACTED
			require.NotNil(t, result)
			require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, grokChatRawEndpoint, result.UpstreamEndpoint)
			require.Equal(t, tt.wantUpstream, result.UpstreamModel)
			require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
			require.Equal(t, "raw ok", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
	REDACTED)
REDACTED
REDACTED

func TestForwardGrokChatViaResponses429UsesGrokRateLimitPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"stream":falseREDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7501REDACTED)

	account := grokChatBridgeTestAccount(75)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type": []string{"application/json"REDACTED,
			"Retry-After":  []string{"45"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"REDACTEDREDACTED`)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED
	before := time.Now()

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
REDACTED
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "45", failoverErr.ResponseHeaders.Get("Retry-After"))
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, grokChatResponsesEndpoint, GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Zero(t, repo.tempUnschedCalls)
	require.WithinDuration(t, before.Add(45*time.Second), repo.lastRateLimitResetAt, time.Second)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestForwardGrokRawChat429PreservesRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"stream":false,"stop":"done"REDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7551REDACTED)

	account := grokChatBridgeTestAccount(755)
	account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type": []string{"application/json"REDACTED,
			"Retry-After":  []string{"45"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"REDACTEDREDACTED`)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

REDACTED
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "45", failoverErr.ResponseHeaders.Get("Retry-After"))
	require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
REDACTED

func TestForwardGrokRawChatErrorRecordsActualEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED],"stream":false,"stop":"done"REDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7601REDACTED)

	account := grokChatBridgeTestAccount(76)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"bad request"REDACTEDREDACTED`)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
REDACTED

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
REDACTED
	require.Nil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, grokChatRawEndpoint, GetActualOpenAIUpstreamEndpoint(c))
REDACTED

func grokChatBridgeTestAccount(id int64) *Account {
REDACTED
		ID:          id,
		Name:        "grok-cache-bridge",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			"base_url":      xai.DefaultCLIBaseURL,
	REDACTED,
REDACTED
REDACTED

func grokChatBridgeCompletedResponse(responseID string, cachedTokens int) *http.Response {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","sequence_number":0,"delta":"cached ok"REDACTED`,
		"",
		`data: {"type":"response.completed","sequence_number":1,"response":{"id":"` + responseID + `","object":"response","model":"grok-4.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"cached ok"REDACTED]REDACTED],"usage":{"input_tokens":9908,"output_tokens":12,"total_tokens":9920,"input_tokens_details":{"cached_tokens":` + strconv.Itoa(cachedTokens) + `REDACTEDREDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":                   []string{"text/event-stream"REDACTED,
			"Xai-Request-Id":                 []string{responseID + "-request"REDACTED,
			"X-Ratelimit-Limit-Requests":     []string{"10"REDACTED,
			"X-Ratelimit-Remaining-Requests": []string{"9"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(body)),
REDACTED
REDACTED
