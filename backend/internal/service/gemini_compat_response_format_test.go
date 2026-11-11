package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const geminiResponseFormatTestSchema = `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"score":{"type":"integer","minimum":1,"maximum":10},"note":{"anyOf":[{"type":"string","maxLength":600},{"type":"null"}]}},"required":["score","note"],"additionalProperties":false}`

func TestApplyGeminiChatResponseFormat(t *testing.T) {
	base := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":32000,"thinkingConfig":{"thinkingLevel":"high"}}}`)
	withTools := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"lookup"}]}],"generationConfig":{"maxOutputTokens":100}}`)
	noConfig := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	tests := []struct {
		name       string
		req        []byte
		format     string
		model      string
		wantMime   bool
		wantSchema string
	}{
		{name: "json_schema writes mime and schema", req: base, format: `{"type":"json_schema","json_schema":{"name":"grade","strict":true,"schema":` + geminiResponseFormatTestSchema + `}}`, wantMime: true, wantSchema: geminiResponseFormatTestSchema},
		{name: "json_schema without schema writes mime only", req: base, format: `{"type":"json_schema","json_schema":{"name":"grade"}}`, wantMime: true},
		{name: "json_object writes mime only", req: base, format: `{"type":"json_object"}`, wantMime: true},
		{name: "json_object creates generationConfig", req: noConfig, format: `{"type":"json_object"}`, wantMime: true},
		{name: "text leaves request unchanged", req: base, format: `{"type":"text"}`},
		{name: "null leaves request unchanged", req: base, format: `null`},
		{name: "empty leaves request unchanged", req: base, format: ``},
		{name: "invalid json leaves request unchanged", req: base, format: `{"type":"json_object"`},
		{name: "non-object leaves request unchanged", req: base, format: `"json_object"`},
		{name: "image model leaves request unchanged", req: base, format: `{"type":"json_object"}`, model: "gemini-2.5-flash-image"},
		{name: "unlisted image model leaves request unchanged", req: base, format: `{"type":"json_object"}`, model: "models/gemini-3.5-flash-image-preview"},
		{name: "text model writes mime", req: base, format: `{"type":"json_object"}`, model: "gemini-4-argon", wantMime: true},
		{name: "tools present leaves request unchanged", req: withTools, format: `{"type":"json_schema","json_schema":{"name":"grade","schema":` + geminiResponseFormatTestSchema + `}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyGeminiChatResponseFormat(tt.req, json.RawMessage(tt.format), tt.model)
			require.True(t, json.Valid(got), string(got))
			if !tt.wantMime {
				require.Equal(t, string(tt.req), string(got))
				return
			}
			require.Equal(t, "application/json", gjson.GetBytes(got, "generationConfig.responseMimeType").String())
			schema := gjson.GetBytes(got, "generationConfig.responseJsonSchema")
			if tt.wantSchema == "" {
				require.False(t, schema.Exists(), string(got))
			} else {
				require.JSONEq(t, tt.wantSchema, schema.Raw)
			}
			require.False(t, gjson.GetBytes(got, "generationConfig.responseSchema").Exists(), string(got))
			require.Equal(t, gjson.GetBytes(tt.req, "contents").Raw, gjson.GetBytes(got, "contents").Raw)
			for _, path := range []string{"generationConfig.maxOutputTokens", "generationConfig.thinkingConfig"} {
				require.Equal(t, gjson.GetBytes(tt.req, path).Raw, gjson.GetBytes(got, path).Raw, path)
			}
		})
	}
}

func TestGeminiForwardAsChatCompletions_ForwardsResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		stream     bool
		format     string
		wantSchema string
	}{
		{name: "non-stream json_schema", format: `{"type":"json_schema","json_schema":{"name":"grade","strict":true,"schema":` + geminiResponseFormatTestSchema + `}}`, wantSchema: geminiResponseFormatTestSchema},
		{name: "stream json_object", stream: true, format: `{"type":"json_object"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: http.StatusOK}
			if tt.stream {
				resp.Header = http.Header{"Content-Type": []string{"text/event-stream"}}
				resp.Body = io.NopCloser(strings.NewReader(`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"{\"score\":7,\"note\":null}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":5}}` + "\n\n"))
			} else {
				resp.Header = http.Header{"Content-Type": []string{"application/json"}}
				resp.Body = io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"{\"score\":7,\"note\":null}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":5}}`))
			}
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{resp}}
			svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
			body := []byte(`{"model":"gemini-4-argon","stream":` + strconv.FormatBool(tt.stream) + `,"reasoning_effort":"high","max_tokens":32000,"response_format":` + tt.format + `,"messages":[{"role":"user","content":"grade this"}]}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

			_, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Len(t, upstream.requestBodies, 1)
			sent := upstream.requestBodies[0]
			require.Equal(t, "application/json", gjson.GetBytes(sent, "generationConfig.responseMimeType").String(), string(sent))
			schema := gjson.GetBytes(sent, "generationConfig.responseJsonSchema")
			if tt.wantSchema == "" {
				require.False(t, schema.Exists(), string(sent))
			} else {
				require.JSONEq(t, tt.wantSchema, schema.Raw)
			}
			require.Equal(t, "high", gjson.GetBytes(sent, "generationConfig.thinkingConfig.thinkingLevel").String())
			require.Equal(t, int64(32000), gjson.GetBytes(sent, "generationConfig.maxOutputTokens").Int())
			if !tt.stream {
				require.Equal(t, `{"score":7,"note":null}`, gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
			}
		})
	}
}

func TestGeminiForwardAsChatCompletions_ResponseFormatSkippedWithTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1}}`)),
	}}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"gemini-4-argon","response_format":{"type":"json_object"},"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"q":{"type":"string"}}}}}],"messages":[{"role":"user","content":"hi"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, upstream.requestBodies, 1)
	sent := upstream.requestBodies[0]
	require.True(t, gjson.GetBytes(sent, "tools").IsArray(), string(sent))
	require.False(t, gjson.GetBytes(sent, "generationConfig.responseMimeType").Exists(), string(sent))
	require.False(t, gjson.GetBytes(sent, "generationConfig.responseJsonSchema").Exists(), string(sent))
}

func TestGeminiForwardAsChatCompletions_CodeAssistKeepsResponseSchemaBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"{\"reasoning\":\"r\",\"answer\":\"a\"}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":5}}}` + "\n\n")),
	}}}
	svc := &GeminiMessagesCompatService{tokenProvider: &GeminiTokenProvider{}, httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID:          612,
		Platform:    PlatformGemini,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "ya29.test-token", "project_id": "project-1"},
		Concurrency: 1,
	}
	schema := `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"string"},"big":{"type":"integer","maximum":9007199254740993}},"required":["reasoning","answer"]}`
	body := []byte(`{"model":"gemini-4-argon","response_format":{"type":"json_schema","json_schema":{"name":"r","schema":` + schema + `}},"messages":[{"role":"user","content":"hi"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, upstream.requestBodies, 1)
	sent := upstream.requestBodies[0]
	require.Equal(t, "gemini-4-argon", gjson.GetBytes(sent, "model").String())
	require.Equal(t, "project-1", gjson.GetBytes(sent, "project").String())
	require.Equal(t, "application/json", gjson.GetBytes(sent, "request.generationConfig.responseMimeType").String(), string(sent))
	require.Equal(t, schema, gjson.GetBytes(sent, "request.generationConfig.responseJsonSchema").Raw)
	require.Equal(t, `{"reasoning":"r","answer":"a"}`, gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
}
