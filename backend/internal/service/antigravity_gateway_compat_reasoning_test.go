package service

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const antigravityCompatGeminiThinkingModel = "gemini-3.8-flash-tiered"

// antigravityCompatGeminiSSEResponse is a deterministic v1internal fixture.
// These tests validate the local protocol bridge and never contact Antigravity.
func antigravityCompatGeminiSSEResponse(thought bool) *http.Response {
	var body string
	if thought {
		body = strings.Join([]string{
			`data: {"response":{"responseId":"resp_reasoning","candidates":[{"content":{"role":"model","parts":[{"text":"provider thought","thought":true,"thoughtSignature":"sig_reasoning"}]}}]}}`,
			`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"final answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":13,"candidatesTokenCount":7}}}`,
		}, "\n\n") + "\n\n"
	} else {
		body = `data: {"response":{"responseId":"resp_plain","candidates":[{"content":{"role":"model","parts":[{"text":"plain answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":4}}}` + "\n\n"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"request-reasoning-test"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func antigravityCompatReasoningAccount() *Account {
	account := newAntigravityCompatAccount(AccountTypeOAuth)
	account.Credentials["model_mapping"].(map[string]any)[antigravityCompatGeminiThinkingModel] = antigravityCompatGeminiThinkingModel
	return account
}

func decodeChatCompletionChunks(t *testing.T, body string) []apicompat.ChatCompletionsChunk {
	t.Helper()

	scanner := bufio.NewScanner(strings.NewReader(body))
	chunks := make([]apicompat.ChatCompletionsChunk, 0)
	done := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "data: [DONE]" {
			require.False(t, done, "stream emitted [DONE] more than once")
			done = true
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		var chunk apicompat.ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk))
		chunks = append(chunks, chunk)
	}
	require.NoError(t, scanner.Err())
	require.True(t, done, "stream did not emit [DONE]")
	return chunks
}

func collectChatChunkText(chunks []apicompat.ChatCompletionsChunk) (reasoning, content string, finishReasons []string) {
	var reasoningBuilder, contentBuilder strings.Builder
	for _, chunk := range chunks {
		for _, choice := range chunk.Choices {
			if choice.Delta.ReasoningContent != nil {
				reasoningBuilder.WriteString(*choice.Delta.ReasoningContent)
			}
			if choice.Delta.Content != nil {
				contentBuilder.WriteString(*choice.Delta.Content)
			}
			if choice.FinishReason != nil {
				finishReasons = append(finishReasons, *choice.FinishReason)
			}
		}
	}
	return reasoningBuilder.String(), contentBuilder.String(), finishReasons
}

func TestAntigravityCompatChatCompletionsStreamExposesGeminiThoughts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{antigravityCompatGeminiSSEResponse(true)},
	}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{"model":"gemini-3.8-flash-tiered","messages":[{"role":"user","content":"think"}],"reasoning_effort":"high","max_tokens":4096,"stream":true,"stream_options":{"include_usage":true}}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	result, err := svc.ForwardAsChatCompletions(
		context.Background(),
		c,
		antigravityCompatReasoningAccount(),
		body,
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Len(t, upstream.requestBodies, 1)
	require.Equal(t, "gemini-3.8-flash-tiered", gjson.GetBytes(upstream.requestBodies[0], "model").String())
	require.Equal(t, "HIGH", gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.thinkingLevel").String())
	require.True(t, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.includeThoughts").Bool())

	chunks := decodeChatCompletionChunks(t, recorder.Body.String())
	reasoning, content, finishReasons := collectChatChunkText(chunks)
	require.Equal(t, "provider thought", reasoning)
	require.Equal(t, "final answer", content)
	require.Equal(t, []string{"stop"}, finishReasons)
	require.Equal(t, 13, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
}

func TestAntigravityCompatChatCompletionsMapsReasoningEffortToGeminiLevel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		effort string
		level  string
	}{
		{effort: "low", level: "LOW"},
		{effort: "medium", level: "MEDIUM"},
		{effort: "high", level: "HIGH"},
		{effort: "xhigh", level: "HIGH"},
	}

	for _, tt := range tests {
		t.Run(tt.effort, func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{
				responses: []*http.Response{antigravityCompatGeminiSSEResponse(false)},
			}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			body := []byte(`{"model":"gemini-3.8-flash-tiered","messages":[{"role":"user","content":"answer"}],"reasoning_effort":"` + tt.effort + `","stream":true}`)
			c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

			_, err := svc.ForwardAsChatCompletions(
				context.Background(),
				c,
				antigravityCompatReasoningAccount(),
				body,
				nil,
			)

			require.NoError(t, err)
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, tt.level, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.thinkingLevel").String())
			require.True(t, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.includeThoughts").Bool())
		})
	}
}

func TestAntigravityCompatChatCompletionsDefaultsOmittedEffortToLowestGeminiLevel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{antigravityCompatGeminiSSEResponse(false)},
	}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{"model":"gemini-3.8-flash-low","messages":[{"role":"user","content":"answer"}],"stream":true}`)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	_, err := svc.ForwardAsChatCompletions(
		context.Background(),
		c,
		antigravityCompatReasoningAccount(),
		body,
		nil,
	)

	require.NoError(t, err)
	require.Len(t, upstream.requestBodies, 1)
	require.Equal(t, "LOW", gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.thinkingLevel").String())
	require.True(t, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.includeThoughts").Bool())
}

func TestAntigravityCompatChatCompletionsStreamDoesNotInventReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{antigravityCompatGeminiSSEResponse(false)},
	}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{"model":"gemini-3.8-flash-tiered","messages":[{"role":"user","content":"answer"}],"reasoning_effort":"high","stream":true}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	_, err := svc.ForwardAsChatCompletions(
		context.Background(),
		c,
		antigravityCompatReasoningAccount(),
		body,
		nil,
	)

	require.NoError(t, err)
	chunks := decodeChatCompletionChunks(t, recorder.Body.String())
	reasoning, content, finishReasons := collectChatChunkText(chunks)
	require.Empty(t, reasoning)
	require.Equal(t, "plain answer", content)
	require.Equal(t, []string{"stop"}, finishReasons)
	for _, chunk := range chunks {
		for _, choice := range chunk.Choices {
			require.Nil(t, choice.Delta.ReasoningContent)
		}
	}
}

func TestAntigravityCompatChatCompletionsNonStreamExposesGeminiThoughts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{antigravityCompatGeminiSSEResponse(true)},
	}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
	body := []byte(`{"model":"gemini-3.8-flash-tiered","messages":[{"role":"user","content":"think"}],"reasoning_effort":"high","max_tokens":4096}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

	result, err := svc.ForwardAsChatCompletions(
		context.Background(),
		c,
		antigravityCompatReasoningAccount(),
		body,
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	var response apicompat.ChatCompletionsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Choices, 1)
	require.Equal(t, "provider thought", response.Choices[0].Message.ReasoningContent)
	var content string
	require.NoError(t, json.Unmarshal(response.Choices[0].Message.Content, &content))
	require.Equal(t, "final answer", content)
	require.Equal(t, "stop", response.Choices[0].FinishReason)
}
