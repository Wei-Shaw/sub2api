//go:build unit

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBufferChatCompletionsAsResponsesEmitsV2CompactionSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{UserID: 42})
	markOpenAICompactionSummaryExecution(c, openAICompactionKindV2)
	MarkOpenAICompactClientStream(c)

	key := testOpenAICompactionSummaryKey()
	svc := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), key...)}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_summary",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"durable summary"}}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`)),
	}
	result, err := svc.bufferChatCompletionsAsResponses(
		c, resp, "deepseek-chat", nil, nil, false, nil, "deepseek-chat", "deepseek-chat", nil, nil, time.Now(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	body := recorder.Body.String()
	require.Equal(t, 1, strings.Count(body, "event: response.output_item.done"))
	require.Contains(t, body, "event: response.completed")
	require.Contains(t, body, `"type":"compaction"`)
	require.NotContains(t, body, "durable summary")

	sealed := gjson.Get(body, `data.#(type=="response.output_item.done").item.encrypted_content`).String()
	if sealed == "" {
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "data: ") && gjson.Get(strings.TrimPrefix(line, "data: "), "type").String() == "response.output_item.done" {
				sealed = gjson.Get(strings.TrimPrefix(line, "data: "), "item.encrypted_content").String()
				break
			}
		}
	}
	scope, err := NewOpenAICompactionOwnerScope(42, nil)
	require.NoError(t, err)
	summary, err := OpenOpenAICompactionSummary(key, scope, sealed)
	require.NoError(t, err)
	require.Equal(t, "durable summary", summary)
}

func TestBufferChatCompletionsAsResponsesKeepsLegacyCompactionJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{UserID: 42})
	markOpenAICompactionSummaryExecution(c, openAICompactionKindLegacy)

	svc := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...)}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_summary",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"legacy summary"}}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`)),
	}
	_, err := svc.bufferChatCompletionsAsResponses(
		c, resp, "deepseek-chat", nil, nil, false, nil, "deepseek-chat", "deepseek-chat", nil, nil, time.Now(),
	)
	require.NoError(t, err)
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
	require.Equal(t, "compaction", gjson.GetBytes(recorder.Body.Bytes(), "output.0.type").String())
	require.NotContains(t, recorder.Body.String(), "legacy summary")
}

func TestBufferedSSEWithoutCompletedFailsSummaryAndKeepsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{UserID: 42})
	markOpenAICompactionSummaryExecution(c, openAICompactionKindV2)
	MarkOpenAICompactClientStream(c)

	svc := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...)}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"partial\"}\n\n",
		)),
	}
	result, err := svc.handleSSEToJSON(resp, c, &Account{}, []byte(`data: {"type":"response.output_text.delta","output_index":0,"delta":"partial"}`+"\n\n"), "deepseek-chat", "deepseek-chat")
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryGeneration)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Contains(t, recorder.Body.String(), "response.failed")
}

func TestFinalizeSummaryBodyUsesRequestContextService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{UserID: 42})
	markOpenAICompactionSummaryExecution(c, openAICompactionKindV2)
	svc := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...)}
	c.Request = c.Request.WithContext(WithOpenAICompactionSummaryService(c.Request.Context(), svc))
	finalized, err := finalizeOpenAICompactionSummaryBody(c, []byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"context summary"}]}]}`))
	require.NoError(t, err)
	require.Equal(t, "compaction", gjson.GetBytes(finalized, "output.0.type").String())
}

func TestForwardNativeCompactionUsesSummaryOnChatOnlyAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"deepseek-chat","stream":true,"input":[{"type":"message","role":"user","content":"finish the task"},{"type":"compaction_trigger"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	c.Set("api_key", &APIKey{UserID: 42})
	MarkOpenAINativeCompactionV2(c)
	MarkOpenAICompactClientStream(c)

	account := &Account{
		ID:          902,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-deepseek",
			"account_mode": AccountModePayG,
			"api_protocol": APIProtocolChatCompletions,
			"base_url":     "https://api.deepseek.com",
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"deepseek-chat": {ID: "deepseek-chat", ContextWindow: 128_000, MaxOutputTokens: 16_384},
	}})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_summary_forward"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_summary_forward",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"summary from forward"}}],
			"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:                  rawChatCompletionsTestConfig(),
		httpUpstream:         upstream,
		compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}

	result, err := svc.Forward(c.Request.Context(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="compaction_trigger")`).Exists())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "messages.1.content").String(), "produce a faithful, concise summary", string(upstream.lastBody))
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.output_item.done"))
	require.Contains(t, recorder.Body.String(), "event: response.completed")
	require.NotContains(t, recorder.Body.String(), "summary from forward")
}

func TestForwardLegacyCompactWithoutTriggerUsesSummaryAndOrdinaryModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"deepseek-chat","input":[{"type":"message","role":"user","content":"summarize this history"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(string(body)))
	c.Set("api_key", &APIKey{UserID: 42})

	account := &Account{
		ID:          903,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-deepseek",
			"account_mode": AccountModePayG,
			"api_protocol": APIProtocolChatCompletions,
			"base_url":     "https://api.deepseek.com",
			"model_mapping": map[string]any{
				"deepseek-chat": "deepseek-chat",
			},
			"compact_model_mapping": map[string]any{
				"deepseek-chat": "gpt-5.5",
			},
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"deepseek-chat": {ID: "deepseek-chat", ContextWindow: 128_000, MaxOutputTokens: 16_384},
	}})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_legacy_summary",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"legacy summary"}}],
			"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:                  rawChatCompletionsTestConfig(),
		httpUpstream:         upstream,
		compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}

	result, err := svc.Forward(c.Request.Context(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "deepseek-chat", gjson.GetBytes(upstream.lastBody, "model").String(), string(upstream.lastBody))
	require.NotContains(t, gjson.GetBytes(upstream.lastBody, "messages.1.content").String(), "gpt-5.5")
	require.Equal(t, int64(OpenAICompactionSummaryOutputTokenBudget), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
	require.Equal(t, "compaction", gjson.GetBytes(recorder.Body.Bytes(), "output.0.type").String())
	require.NotContains(t, recorder.Body.String(), "legacy summary")
}

func TestForwardNativeSummaryUsesOrdinaryResponsesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"deepseek-chat","stream":true,"input":[{"type":"message","role":"user","content":"continue"},{"type":"compaction_trigger"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	c.Set("api_key", &APIKey{UserID: 42})
	MarkOpenAINativeCompactionV2(c)
	MarkOpenAICompactClientStream(c)

	account := &Account{
		ID:          905,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-deepseek",
			"account_mode": AccountModePayG,
			"api_protocol": APIProtocolResponses,
			"base_url":     "https://responses.example.test/v1",
			"model_mapping": map[string]any{
				"deepseek-chat": "deepseek-chat",
			},
			"compact_model_mapping": map[string]any{
				"deepseek-chat": "gpt-5.5",
			},
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"deepseek-chat": {ID: "deepseek-chat", ContextWindow: 128_000, MaxOutputTokens: 16_384},
	}})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_summary",
			"object":"response",
			"status":"completed",
			"model":"deepseek-chat",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"responses summary"}]}],
			"usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:                  &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream:         upstream,
		compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}

	result, err := svc.Forward(c.Request.Context(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://responses.example.test/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "deepseek-chat", gjson.GetBytes(upstream.lastBody, "model").String(), string(upstream.lastBody))
	require.False(t, gjson.GetBytes(upstream.lastBody, `input.#(type=="compaction_trigger")`).Exists())
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.output_item.done"))
	require.NotContains(t, recorder.Body.String(), "responses summary")
}

func TestForwardSummaryFinalizeFailureReturnsPartialUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"model":"deepseek-chat","stream":true,"input":[{"type":"message","role":"user","content":"finish"},{"type":"compaction_trigger"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	c.Set("api_key", &APIKey{UserID: 42})
	MarkOpenAINativeCompactionV2(c)
	MarkOpenAICompactClientStream(c)
	account := &Account{
		ID: 906, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-deepseek", "account_mode": AccountModePayG,
			"api_protocol": APIProtocolChatCompletions, "base_url": "https://api.deepseek.com",
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"deepseek-chat": {ID: "deepseek-chat", ContextWindow: 128_000},
	}})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_incomplete",
			"choices":[{"index":0,"finish_reason":"length","message":{"role":"assistant","content":"truncated"}}],
			"usage":{"prompt_tokens":20,"completion_tokens":8,"total_tokens":28}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream,
		compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}
	result, err := svc.Forward(c.Request.Context(), c, account, body)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryGeneration)
	require.NotNil(t, result)
	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 8, result.Usage.OutputTokens)
}

func TestForwardSummaryCompactionContinuationAndRepeatedCompaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := testOpenAICompactionSummaryKey()
	account := &Account{
		ID:          904,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-deepseek",
			"account_mode": AccountModePayG,
			"api_protocol": APIProtocolChatCompletions,
			"base_url":     "https://api.deepseek.com",
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"deepseek-chat": {ID: "deepseek-chat", ContextWindow: 128_000, MaxOutputTokens: 16_384},
	}})
	upstream := &httpUpstreamRecorder{resp: chatSummaryTestResponse("first summary")}
	svc := &OpenAIGatewayService{
		cfg:                  rawChatCompletionsTestConfig(),
		httpUpstream:         upstream,
		compactionSummaryKey: append([]byte(nil), key...),
	}

	firstRecorder := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRecorder)
	firstBody := []byte(`{"model":"deepseek-chat","stream":true,"input":[{"type":"message","role":"user","content":"original task"},{"type":"compaction_trigger"}]}`)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(firstBody)))
	firstCtx.Set("api_key", &APIKey{UserID: 42})
	MarkOpenAINativeCompactionV2(firstCtx)
	MarkOpenAICompactClientStream(firstCtx)
	_, err := svc.Forward(firstCtx.Request.Context(), firstCtx, account, firstBody)
	require.NoError(t, err)
	sealed := compactionCiphertextFromSSE(t, firstRecorder.Body.String())

	continuationBody := []byte(`{"model":"deepseek-chat","input":[{"type":"compaction","encrypted_content":"` + sealed + `"},{"type":"message","role":"user","content":"continue normally"}]}`)
	continuationCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	continuationCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(continuationBody)))
	continuationCtx.Set("api_key", &APIKey{UserID: 42})
	restored, changed, err := svc.RestoreOpenAICompactionSummaryInput(continuationCtx.Request.Context(), continuationCtx, continuationBody)
	require.NoError(t, err)
	require.True(t, changed)
	upstream.resp = chatSummaryTestResponse("normal continuation")
	_, err = svc.Forward(continuationCtx.Request.Context(), continuationCtx, account, restored)
	require.NoError(t, err)
	require.NotContains(t, string(upstream.lastBody), sealed)
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "messages.0.content").String(), "first summary")
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "messages.1.content").String(), "continue normally")

	repeatedBody := []byte(`{"model":"deepseek-chat","stream":true,"input":[{"type":"compaction","encrypted_content":"` + sealed + `"},{"type":"message","role":"user","content":"new task"},{"type":"compaction_trigger"}]}`)
	repeatedCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	repeatedCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(repeatedBody)))
	repeatedCtx.Set("api_key", &APIKey{UserID: 42})
	MarkOpenAINativeCompactionV2(repeatedCtx)
	MarkOpenAICompactClientStream(repeatedCtx)
	restoredRepeated, changed, err := svc.RestoreOpenAICompactionSummaryInput(repeatedCtx.Request.Context(), repeatedCtx, repeatedBody)
	require.NoError(t, err)
	require.True(t, changed)
	upstream.resp = chatSummaryTestResponse("second summary")
	_, err = svc.Forward(repeatedCtx.Request.Context(), repeatedCtx, account, restoredRepeated)
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "messages.0.content").String(), "first summary")
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "messages.2.content").String(), "produce a faithful, concise summary")
	require.NotContains(t, string(upstream.lastBody), sealed)
}

func TestForwardCompactionAttemptStateResetsAfterSummaryAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"model":"deepseek-chat","stream":true,"input":[{"type":"message","role":"user","content":"finish"},{"type":"compaction_trigger"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	c.Set("api_key", &APIKey{UserID: 42})
	MarkOpenAINativeCompactionV2(c)
	MarkOpenAICompactClientStream(c)

	summaryAccount := &Account{
		ID: 910, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "summary-key", "account_mode": AccountModePayG,
			"api_protocol": APIProtocolChatCompletions, "base_url": "https://api.deepseek.com",
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	summaryAccount.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"deepseek-chat": {ID: "deepseek-chat", ContextWindow: 128_000},
	}})
	nativeAccount := &Account{
		ID: 911, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "native-key", "account_mode": AccountModePayG,
			"api_protocol": APIProtocolResponses, "base_url": "https://relay.example.test",
			"model_mapping": map[string]any{"deepseek-chat": "native-model"},
		},
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		chatSummaryTestResponse("attempt summary"),
		openAICompatSSECompletedResponse("resp_native_attempt", "native-model"),
	}}
	svc := &OpenAIGatewayService{
		cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream,
		compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}

	_, err := svc.Forward(c.Request.Context(), c, summaryAccount, body)
	require.NoError(t, err)
	require.True(t, IsOpenAICompactionSummaryExecution(c))
	require.False(t, IsOpenAINativeCompactionV2(c))

	_, err = svc.Forward(c.Request.Context(), c, nativeAccount, body)
	require.NoError(t, err)
	require.False(t, IsOpenAICompactionSummaryExecution(c), "the next attempt must not inherit summary execution")
	require.True(t, IsOpenAINativeCompactionV2(c), "the next attempt must restore the original native v2 ingress")
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "native-model", gjson.GetBytes(upstream.bodies[1], "model").String())
	require.Equal(t, "compaction_trigger", gjson.GetBytes(upstream.bodies[1], `input.#(type=="compaction_trigger").type`).String())
	require.NotContains(t, string(upstream.bodies[1]), "produce a faithful, concise summary")
	require.Contains(t, upstream.requests[1].Header.Get("x-codex-beta-features"), "remote_compaction_v2")
}

func TestForwardSummarySSEEOFWithoutCompletedFailsWithUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"model":"deepseek-chat","stream":true,"input":[{"type":"message","role":"user","content":"finish"},{"type":"compaction_trigger"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	c.Set("api_key", &APIKey{UserID: 42})
	MarkOpenAINativeCompactionV2(c)
	MarkOpenAICompactClientStream(c)
	account := &Account{
		ID: 912, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "summary-key", "account_mode": AccountModePayG,
			"api_protocol": APIProtocolResponses, "base_url": "https://responses.example.test/v1",
		},
		Extra: map[string]any{OpenAICompactSummaryEnabledExtraKey: true},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"deepseek-chat": {ID: "deepseek-chat", ContextWindow: 128_000},
	}})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			`data: {"type":"response.output_text.delta","output_index":0,"delta":"partial","usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}` + "\n\n",
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream, compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}

	result, err := svc.Forward(c.Request.Context(), c, account, body)
	require.ErrorIs(t, err, ErrOpenAICompactionSummaryGeneration)
	require.NotNil(t, result)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, int64(OpenAICompactionSummaryOutputTokenBudget), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
}

func chatSummaryTestResponse(content string) *http.Response {
	body, _ := json.Marshal(map[string]any{
		"id": "chatcmpl_" + content,
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": content},
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}
}

func compactionCiphertextFromSSE(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if gjson.Get(data, "type").String() == "response.output_item.done" && gjson.Get(data, "item.type").String() == "compaction" {
			sealed := gjson.Get(data, "item.encrypted_content").String()
			require.NotEmpty(t, sealed)
			return sealed
		}
	}
	t.Fatal("compaction output_item.done not found")
	return ""
}
