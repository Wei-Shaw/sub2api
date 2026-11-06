package service

import (
	"bytes"
	"context"
	"encoding/json"

	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAntigravityGatewayService_ForwardGemini_NonStreamPreservesFunctionCallAndThoughtSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"Call get_weather for Tokyo."}]}],"tools":[{"functionDeclarations":[{"name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}]}`)

	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-4-argon:generateContent", bytes.NewReader(body))

	sseStream := strings.Join([]string{
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig_fc_1","functionCall":{"name":"get_weather","args":{"city":"Tokyo"},"id":"call_1"}}]}}],"usageMetadata":{"promptTokenCount":300,"candidatesTokenCount":16,"totalTokenCount":328,"thoughtsTokenCount":12},"modelVersion":"gemini-4-argon","responseId":"resp_1"}}`,
		"",
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":300,"candidatesTokenCount":22,"totalTokenCount":334,"thoughtsTokenCount":12},"modelVersion":"gemini-4-argon","responseId":"resp_1"}}`,
		"",
	}, "\n")

	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sseStream)),
		}},
	}
	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{}, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}),
		tokenProvider:  &AntigravityTokenProvider{},
		httpUpstream:   upstream,
	}
	account := &Account{
		ID:          109,
		Name:        "antigravity-fc-nonstream",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "test-access-token",
			"project_id":   "test-project",
			"model_mapping": map[string]any{
				"gemini-4-argon": "gemini-4-argon",
			},
		},
	}

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-4-argon", "generateContent", false, body, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, writer.Code)

	var respPayload map[string]any
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &respPayload))
	candidates, ok := respPayload["candidates"].([]any)
	require.True(t, ok)
	require.Len(t, candidates, 1)
	candidate, ok := candidates[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "STOP", candidate["finishReason"])

	content, ok := candidate["content"].(map[string]any)
	require.True(t, ok)
	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 1, "trailing empty text part should not overwrite or pollute functionCall part")

	part0, ok := parts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "sig_fc_1", part0["thoughtSignature"])
	fc, ok := part0["functionCall"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "get_weather", fc["name"])
	require.Equal(t, "call_1", fc["id"])
	require.Equal(t, map[string]any{"city": "Tokyo"}, fc["args"])
}

func TestAntigravityGatewayService_ForwardGemini_StreamAndFunctionResponseFollowUp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1) Stream request with functionDeclarations returns unwrapped functionCall and thoughtSignature
	streamReqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"Call get_weather for Tokyo."}]}],"tools":[{"functionDeclarations":[{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}]}]}`)
	streamWriter := httptest.NewRecorder()
	streamCtx, _ := gin.CreateTestContext(streamWriter)
	streamCtx.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-4-argon:streamGenerateContent?alt=sse", bytes.NewReader(streamReqBody))

	streamSSE := strings.Join([]string{
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig_stream_1","functionCall":{"name":"get_weather","args":{"city":"Tokyo"},"id":"call_s1"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}}`,
		"",
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":6}}}`,
		"",
	}, "\n")

	// 2) Non-stream follow-up with functionCall (missing thoughtSignature) + functionResponse (with inlineData image)
	followUpReqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"Call get_weather for Tokyo."}]},{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Tokyo"},"id":"call_s1"}}]},{"role":"user","parts":[{"functionResponse":{"name":"get_weather","id":"call_s1","response":{"weather":"Sunny"},"parts":[{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="}}]}}]}],"tools":[{"functionDeclarations":[{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}]}]}`)
	followUpWriter := httptest.NewRecorder()
	followUpCtx, _ := gin.CreateTestContext(followUpWriter)
	followUpCtx.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-4-argon:generateContent", bytes.NewReader(followUpReqBody))

	followUpSSE := strings.Join([]string{
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"It is sunny in "}]}}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":4}}}`,
		"",
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Tokyo."}]}}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":6}}}`,
		"",
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig_final","text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":8}}}`,
		"",
	}, "\n")

	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(streamSSE)),
			},
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(followUpSSE)),
			},
		},
	}

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{}, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}),
		tokenProvider:  &AntigravityTokenProvider{},
		httpUpstream:   upstream,
	}
	account := &Account{
		ID:          110,
		Name:        "antigravity-fc-stream",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "test-access-token",
			"project_id":   "test-project",
			"model_mapping": map[string]any{
				"gemini-4-argon": "gemini-4-argon",
			},
		},
	}

	res1, err := svc.ForwardGemini(context.Background(), streamCtx, account, "gemini-4-argon", "streamGenerateContent", true, streamReqBody, false)
	require.NoError(t, err)
	require.NotNil(t, res1)
	require.Contains(t, streamWriter.Body.String(), `"functionCall":{"name":"get_weather","args":{"city":"Tokyo"},"id":"call_s1"}`)
	require.Contains(t, streamWriter.Body.String(), `"thoughtSignature":"sig_stream_1"`)

	res2, err := svc.ForwardGemini(context.Background(), followUpCtx, account, "gemini-4-argon", "generateContent", false, followUpReqBody, false)
	require.NoError(t, err)
	require.NotNil(t, res2)
	require.Len(t, upstream.requestBodies, 2)

	// Verify follow-up request had dummy thoughtSignature injected on functionCall and preserved functionResponse image part
	secondSent := string(upstream.requestBodies[1])
	require.Contains(t, secondSent, `"thoughtSignature":"skip_thought_signature_validator"`)
	require.Contains(t, secondSent, `"mimeType":"image/png"`)

	var followUpResp map[string]any
	require.NoError(t, json.Unmarshal(followUpWriter.Body.Bytes(), &followUpResp))
	cands, ok := followUpResp["candidates"].([]any)
	require.True(t, ok)
	cand0, ok := cands[0].(map[string]any)
	require.True(t, ok)
	content, ok := cand0["content"].(map[string]any)
	require.True(t, ok)
	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, parts)
	firstPart, ok := parts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "It is sunny in Tokyo.", firstPart["text"])
}
