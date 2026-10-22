package service

import (
	"context"
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

func TestApplyGeminiCompatThinkingConfig(t *testing.T) {
	budget := func(v int64) *int64 { return &v }
	const baseReq = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":8192}}`
	tests := []struct {
		name            string
		model           string
		claudeBody      string
		includeThoughts bool
		wantLevel       string
		wantBudget      *int64
		wantThoughts    bool
		wantNoConfig    bool
	}{
		{name: "effort medium on flash", model: "gemini-3.8-flash", claudeBody: `{"output_config":{"effort":"medium"},"thinking":{"type":"enabled","budget_tokens":4096}}`, wantLevel: "medium"},
		{name: "effort low", model: "gemini-3.7-flash", claudeBody: `{"output_config":{"effort":"low"}}`, wantLevel: "low"},
		{name: "effort none on flash", model: "gemini-3.8-flash", claudeBody: `{"output_config":{"effort":"none"},"thinking":{"type":"enabled","budget_tokens":10240}}`, wantLevel: "minimal"},
		{name: "effort minimal on pro", model: "gemini-3.1-pro-preview", claudeBody: `{"output_config":{"effort":"minimal"}}`, wantLevel: "low"},
		{name: "effort medium on gemini 3 pro", model: "gemini-3-pro-preview", claudeBody: `{"output_config":{"effort":"medium"}}`, wantLevel: "high"},
		{name: "effort medium on gemini 3.1 pro", model: "gemini-3.1-pro-preview", claudeBody: `{"output_config":{"effort":"medium"}}`, wantLevel: "medium"},
		{name: "effort max", model: "gemini-3.7-flash", claudeBody: `{"output_config":{"effort":"max"}}`, wantLevel: "high"},
		{name: "effort xhigh", model: "gemini-3.7-flash", claudeBody: `{"output_config":{"effort":"xhigh"}}`, wantLevel: "high"},
		{name: "thinking disabled wins over effort", model: "gemini-3.8-flash", claudeBody: `{"thinking":{"type":"disabled"},"output_config":{"effort":"high"}}`, wantLevel: "minimal"},
		{name: "budget only maps to high", model: "gemini-3.8-flash", claudeBody: `{"thinking":{"type":"enabled","budget_tokens":31999}}`, wantLevel: "high"},
		{name: "budget only maps to medium", model: "gemini-3.8-flash", claudeBody: `{"thinking":{"type":"enabled","budget_tokens":4096}}`, wantLevel: "medium"},
		{name: "budget only maps to low", model: "gemini-3.8-flash", claudeBody: `{"thinking":{"type":"enabled","budget_tokens":1024}}`, wantLevel: "low"},
		{name: "adaptive keeps default level", model: "gemini-3.7-flash", claudeBody: `{"thinking":{"type":"adaptive"}}`, includeThoughts: true, wantThoughts: true},
		{name: "no thinking params streams thoughts only", model: "gemini-3.7-flash", claudeBody: `{}`, includeThoughts: true, wantThoughts: true},
		{name: "unknown effort ignored", model: "gemini-3.7-flash", claudeBody: `{"output_config":{"effort":"turbo"}}`, wantNoConfig: true},
		{name: "level with thoughts", model: "gemini-3.8-flash", claudeBody: `{"output_config":{"effort":"high"}}`, includeThoughts: true, wantLevel: "high", wantThoughts: true},
		{name: "gemini 4 uses level", model: "gemini-4-argon", claudeBody: `{"output_config":{"effort":"low"}}`, wantLevel: "low"},
		{name: "medium outside verified models becomes high", model: "gemini-4-argon", claudeBody: `{"output_config":{"effort":"medium"}}`, wantLevel: "high"},
		{name: "medium budget outside verified models becomes high", model: "gemini-4-argon", claudeBody: `{"thinking":{"type":"enabled","budget_tokens":4096}}`, wantLevel: "high"},
		{name: "medium on unverified pro becomes high", model: "gemini-3.5-pro", claudeBody: `{"output_config":{"effort":"medium"}}`, wantLevel: "high"},
		{name: "medium on flash lite", model: "gemini-3.1-flash-lite", claudeBody: `{"output_config":{"effort":"medium"}}`, wantLevel: "medium"},
		{name: "tier suffix model keeps name tier", model: "gemini-4-argon-high", claudeBody: `{"output_config":{"effort":"low"}}`, includeThoughts: true, wantThoughts: true},
		{name: "tiered suffix model keeps name tier", model: "gemini-3.8-flash-tiered", claudeBody: `{"output_config":{"effort":"low"}}`, includeThoughts: true, wantThoughts: true},
		{name: "2.5 flash low", model: "gemini-2.5-flash", claudeBody: `{"output_config":{"effort":"low"}}`, wantBudget: budget(1024)},
		{name: "2.5 flash high", model: "gemini-2.5-flash", claudeBody: `{"output_config":{"effort":"high"}}`, includeThoughts: true, wantBudget: budget(24576), wantThoughts: true},
		{name: "2.5 flash none disables thinking and thoughts", model: "gemini-2.5-flash", claudeBody: `{"output_config":{"effort":"none"}}`, includeThoughts: true, wantBudget: budget(0)},
		{name: "2.5 pro cannot disable thinking", model: "gemini-2.5-pro", claudeBody: `{"thinking":{"type":"disabled"}}`, includeThoughts: true, wantBudget: budget(128), wantThoughts: true},
		{name: "2.5 pro high", model: "gemini-2.5-pro", claudeBody: `{"output_config":{"effort":"max"}}`, wantBudget: budget(32768)},
		{name: "2.5 pro budget clamps up", model: "gemini-2.5-pro", claudeBody: `{"thinking":{"type":"enabled","budget_tokens":50}}`, wantBudget: budget(128)},
		{name: "2.5 flash budget clamps down", model: "gemini-2.5-flash", claudeBody: `{"thinking":{"type":"enabled","budget_tokens":31999}}`, wantBudget: budget(24576)},
		{name: "2.5 flash lite budget clamps up", model: "gemini-2.5-flash-lite", claudeBody: `{"thinking":{"type":"enabled","budget_tokens":100}}`, wantBudget: budget(512)},
		{name: "2.5 adaptive keeps dynamic budget", model: "gemini-2.5-flash", claudeBody: `{"thinking":{"type":"adaptive"}}`, wantNoConfig: true},
		{name: "2.0 untouched", model: "gemini-2.0-flash", claudeBody: `{"output_config":{"effort":"high"}}`, includeThoughts: true, wantNoConfig: true},
		{name: "image model untouched", model: "gemini-3-pro-image-preview", claudeBody: `{"output_config":{"effort":"high"}}`, includeThoughts: true, wantNoConfig: true},
		{name: "unparsable model untouched", model: "gemini-flash-latest", claudeBody: `{"output_config":{"effort":"high"}}`, includeThoughts: true, wantNoConfig: true},
		{name: "non gemini model untouched", model: "custom-model", claudeBody: `{"output_config":{"effort":"high"}}`, includeThoughts: true, wantNoConfig: true},
		{name: "models prefix", model: "models/gemini-3.8-flash", claudeBody: `{"output_config":{"effort":"low"}}`, wantLevel: "low"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := applyGeminiCompatThinkingConfig([]byte(baseReq), []byte(tt.claudeBody), tt.model, tt.includeThoughts)
			require.Equal(t, int64(8192), gjson.GetBytes(out, "generationConfig.maxOutputTokens").Int())
			require.Equal(t, "hi", gjson.GetBytes(out, "contents.0.parts.0.text").String())
			cfg := gjson.GetBytes(out, "generationConfig.thinkingConfig")
			if tt.wantNoConfig {
				require.False(t, cfg.Exists(), cfg.Raw)
				return
			}
			require.Equal(t, tt.wantLevel, cfg.Get("thinkingLevel").String())
			if tt.wantBudget != nil {
				require.True(t, cfg.Get("thinkingBudget").Exists(), cfg.Raw)
				require.Equal(t, *tt.wantBudget, cfg.Get("thinkingBudget").Int())
			} else {
				require.False(t, cfg.Get("thinkingBudget").Exists(), cfg.Raw)
			}
			require.Equal(t, tt.wantThoughts, cfg.Get("includeThoughts").Bool(), cfg.Raw)
			require.False(t, cfg.Get("thinkingLevel").Exists() && cfg.Get("thinkingBudget").Exists(), "thinkingLevel and thinkingBudget are mutually exclusive")
		})
	}
}

func geminiCompatThinkingTestAccount() *Account {
	return &Account{
		ID:          611,
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "gemini-api-key"},
		Concurrency: 1,
	}
}

func TestGeminiForwardAsChatCompletions_StreamsThoughtsAsReasoningContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Weighing options","thought":true}]}}]}` + "\n\n" +
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":" carefully.","thought":true}]}}]}` + "\n\n" +
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Final answer","thoughtSignature":"sig"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"thoughtsTokenCount":7}}` + "\n\n"
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"gemini-3.8-flash","stream":true,"reasoning_effort":"medium","messages":[{"role":"user","content":"hi"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, upstream.requestBodies, 1)
	require.Equal(t, "medium", gjson.GetBytes(upstream.requestBodies[0], "generationConfig.thinkingConfig.thinkingLevel").String())
	require.True(t, gjson.GetBytes(upstream.requestBodies[0], "generationConfig.thinkingConfig.includeThoughts").Bool())
	require.Equal(t, "medium", optionalStringValue(result.ReasoningEffort))
	require.Equal(t, 9, result.Usage.OutputTokens)

	var reasoning, content strings.Builder
	firstContentChunk, lastReasoningChunk := -1, -1
	for i, line := range strings.Split(recorder.Body.String(), "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || payload == "[DONE]" {
			continue
		}
		delta := gjson.Get(payload, "choices.0.delta")
		if r := delta.Get("reasoning_content"); r.Exists() {
			_, _ = reasoning.WriteString(r.String())
			lastReasoningChunk = i
		}
		if text := delta.Get("content"); text.Exists() && text.String() != "" {
			_, _ = content.WriteString(text.String())
			if firstContentChunk < 0 {
				firstContentChunk = i
			}
		}
	}
	require.Equal(t, "Weighing options carefully.", reasoning.String())
	require.Equal(t, "Final answer", content.String())
	require.Less(t, lastReasoningChunk, firstContentChunk)
}

func TestGeminiForwardAsChatCompletions_NonStreamForwardsEffortWithoutThoughts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1}}`)),
	}}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"gemini-3.8-flash","reasoning_effort":"none","messages":[{"role":"user","content":"hi"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, upstream.requestBodies, 1)
	cfg := gjson.GetBytes(upstream.requestBodies[0], "generationConfig.thinkingConfig")
	require.Equal(t, "minimal", cfg.Get("thinkingLevel").String())
	require.False(t, cfg.Get("includeThoughts").Exists(), cfg.Raw)
	require.Equal(t, "minimal", optionalStringValue(result.ReasoningEffort))
	require.Equal(t, "ok", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
}

func TestGeminiMessagesForward_IncludeThoughtsFollowsClientThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		body         string
		wantThoughts bool
		wantLevel    string
	}{
		{name: "stream with adaptive thinking", body: `{"model":"gemini-3.7-flash","stream":true,"max_tokens":1024,"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"},"messages":[{"role":"user","content":"hi"}]}`, wantThoughts: true, wantLevel: "medium"},
		{name: "stream without thinking", body: `{"model":"gemini-3.7-flash","stream":true,"max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`},
		{name: "non stream with thinking", body: `{"model":"gemini-3.7-flash","max_tokens":1024,"thinking":{"type":"enabled","budget_tokens":31999},"messages":[{"role":"user","content":"hi"}]}`, wantLevel: "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := gjson.Get(tt.body, "stream").Bool()
			respBody := `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1}}`
			contentType := "application/json"
			if stream {
				respBody = "data: " + respBody + "\n\n"
				contentType = "text/event-stream"
			}
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(respBody)),
			}}}
			svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/messages", []byte(tt.body))

			result, err := svc.Forward(context.Background(), c, geminiCompatThinkingTestAccount(), []byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Len(t, upstream.requestBodies, 1)
			cfg := gjson.GetBytes(upstream.requestBodies[0], "generationConfig.thinkingConfig")
			require.Equal(t, tt.wantThoughts, cfg.Get("includeThoughts").Bool(), cfg.Raw)
			require.Equal(t, tt.wantLevel, cfg.Get("thinkingLevel").String(), cfg.Raw)
			require.Equal(t, tt.wantLevel, optionalStringValue(result.ReasoningEffort))
		})
	}
}

func TestGeminiMessagesHandleStreamingResponse_EmitsThinkingBlockBeforeText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"parts":[{"text":"Plan the reply","thought":true}]}}]}` + "\n\n" +
		`data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"thoughtsTokenCount":4}}` + "\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	svc := &GeminiMessagesCompatService{}

	result, err := svc.handleStreamingResponse(c, resp, time.Now(), "claude-sonnet-4-5")
	require.NoError(t, err)
	require.NotNil(t, result.firstTokenMs)

	events := parseAnthropicContentBlockEvents(t, rec.Body.String())
	open := -1
	var started []string
	for _, ev := range events {
		switch ev.event {
		case "content_block_start":
			require.Equalf(t, -1, open, "block %d opened while block %d was still open", ev.index, open)
			open = ev.index
			started = append(started, ev.blockType)
		case "content_block_stop":
			require.Equal(t, open, ev.index)
			open = -1
		}
	}
	require.Equal(t, -1, open)
	require.Equal(t, []string{"thinking", "text"}, started)

	out := rec.Body.String()
	require.Contains(t, out, `"content_block":{"signature":"","thinking":"","type":"thinking"}`)
	require.Contains(t, out, `"type":"thinking_delta"`)
	require.Contains(t, out, `"thinking":"Plan the reply"`)
	require.NotContains(t, out, `"text":"Plan the reply"`)
}

func TestConvertClaudeMessagesToGemini_DropsHistoryThinkingBlocks(t *testing.T) {
	body := []byte(`{"model":"gemini-3.7-flash","max_tokens":1024,"messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"PRIOR_THOUGHT","signature":""},{"type":"text","text":"Hello"}]},
		{"role":"user","content":"again"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"ONLY_THOUGHT","signature":""},{"type":"redacted_thinking","data":"opaque"}]},
		{"role":"user","content":"last"}
	]}`)
	out, err := convertClaudeMessagesToGeminiGenerateContent(body)
	require.NoError(t, err)
	require.NotContains(t, string(out), "PRIOR_THOUGHT")
	require.NotContains(t, string(out), "ONLY_THOUGHT")
	require.NotContains(t, string(out), "opaque")

	contents := gjson.GetBytes(out, "contents").Array()
	require.Len(t, contents, 4)
	require.Equal(t, "model", contents[1].Get("role").String())
	require.Len(t, contents[1].Get("parts").Array(), 1)
	require.Equal(t, "Hello", contents[1].Get("parts.0.text").String())
	for i, content := range contents {
		require.NotEmpty(t, content.Get("parts").Array(), "contents[%d] has empty parts", i)
	}
}

func TestGeminiForwardAsChatCompletions_DropsHistoryReasoningContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1}}`)),
	}}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"gemini-3.8-flash","messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":"Hello","reasoning_content":"PRIOR_REASONING"},
		{"role":"assistant","content":"Again","reasoning":"ALIAS_REASONING"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"PART_THINKING"},{"type":"reasoning","text":"PART_REASONING"},{"type":"text","text":"Third"}]},
		{"role":"user","content":"next"}
	]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, upstream.requestBodies, 1)
	sent := string(upstream.requestBodies[0])
	require.NotContains(t, sent, "PRIOR_REASONING")
	require.NotContains(t, sent, "ALIAS_REASONING")
	require.NotContains(t, sent, "PART_THINKING")
	require.NotContains(t, sent, "PART_REASONING")
	require.NotContains(t, sent, "<thinking>")
	require.Contains(t, sent, "Third")
	require.Contains(t, sent, "Hello")
	require.Contains(t, sent, "Again")
}

func TestGeminiMessagesForward_SignatureRetryKeepsClientThinkingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	okBody := "data: " + `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1}}` + "\n\n"
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":400,"message":"Function call is missing a thought_signature","status":"INVALID_ARGUMENT"}}`)),
		},
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":400,"message":"Function call is missing a thought_signature","status":"INVALID_ARGUMENT"}}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(okBody)),
		},
	}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"claude-sonnet-4-5","stream":true,"max_tokens":1024,"thinking":{"type":"adaptive"},"output_config":{"effort":"low"},"messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"RETRY_THOUGHT","signature":""},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"q":"x"}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"found"}]},
		{"role":"assistant","content":[{"type":"thinking","thinking":"RETRY_ONLY_THOUGHT","signature":""}]},
		{"role":"user","content":"next"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"RETRY_BLANK_THOUGHT","signature":""},{"type":"text","text":""}]},
		{"role":"user","content":"again"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"RETRY_SPACE_THOUGHT","signature":""},{"type":"text","text":"   "}]},
		{"role":"user","content":"last"}
	]}`)
	account := geminiCompatThinkingTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"claude-sonnet-4-5": "gemini-3.8-flash"}
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/messages", body)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, upstream.requestBodies, 3)
	for i, sent := range upstream.requestBodies {
		require.NotContains(t, string(sent), "RETRY_THOUGHT", "request %d", i)
		require.NotContains(t, string(sent), "RETRY_ONLY_THOUGHT", "request %d", i)
		require.NotContains(t, string(sent), "RETRY_BLANK_THOUGHT", "request %d", i)
		require.NotContains(t, string(sent), "RETRY_SPACE_THOUGHT", "request %d", i)
		for j, content := range gjson.GetBytes(sent, "contents").Array() {
			require.NotEmpty(t, content.Get("parts").Array(), "request %d contents[%d] has empty parts", i, j)
			for k, part := range content.Get("parts").Array() {
				if text := part.Get("text"); text.Exists() {
					require.NotEmpty(t, strings.TrimSpace(text.String()), "request %d contents[%d].parts[%d] is blank text", i, j, k)
				}
			}
		}
		cfg := gjson.GetBytes(sent, "generationConfig.thinkingConfig")
		require.Equal(t, "low", cfg.Get("thinkingLevel").String(), "request %d: %s", i, cfg.Raw)
		require.True(t, cfg.Get("includeThoughts").Bool(), "request %d: %s", i, cfg.Raw)
	}
	require.Equal(t, "low", optionalStringValue(result.ReasoningEffort))
}
