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

func TestGeminiForwardOpenAICompatibleEmbeddings_SingleInputUsesEmbedContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"X-Request-Id": []string{"gemini-req-1"},
			},
			Body: io.NopCloser(strings.NewReader(`{
				"embedding":{"values":[0.1,0.2,0.3]},
				"usageMetadata":{"promptTokenCount":7}
			}`)),
		},
	}
	svc := &GeminiMessagesCompatService{
		httpUpstream: httpStub,
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       201,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "gemini-api-key",
		},
		Concurrency: 1,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-embedding-2-preview","input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/embeddings", bytes.NewReader(body))

	result, err := svc.ForwardOpenAICompatibleEmbeddings(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, "gemini-embedding-2-preview", result.Model)
	require.Equal(t, "gemini-req-1", result.RequestID)

	require.NotNil(t, httpStub.lastReq)
	require.Contains(t, httpStub.lastReq.URL.String(), "/v1beta/models/gemini-embedding-2-preview:embedContent")
	require.Equal(t, "gemini-api-key", httpStub.lastReq.Header.Get("x-goog-api-key"))
	require.Empty(t, httpStub.lastReq.Header.Get("Authorization"))

	sentBody, err := io.ReadAll(httpStub.lastReq.Body)
	require.NoError(t, err)
	var sent map[string]any
	require.NoError(t, json.Unmarshal(sentBody, &sent))
	content, ok := sent["content"].(map[string]any)
	require.True(t, ok)
	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 1)
	part, ok := parts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "hello", part["text"])

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "list", got["object"])
	data, ok := got["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	first, ok := data[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "embedding", first["object"])
	require.Equal(t, float64(0), first["index"])
	require.Equal(t, "gemini-embedding-2-preview", got["model"])
	usage, ok := got["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(7), usage["prompt_tokens"])
	require.Equal(t, float64(7), usage["total_tokens"])
}

func TestGeminiForwardOpenAICompatibleEmbeddings_BatchInputUsesBatchEmbedContents(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"embeddings":[{"values":[1,2]},{"values":[3,4]}],
				"usageMetadata":{"promptTokenCount":11}
			}`)),
		},
	}
	svc := &GeminiMessagesCompatService{
		httpUpstream: httpStub,
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       202,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "gemini-api-key",
		},
		Concurrency: 1,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-embedding-2-preview","input":["hello","world"]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/embeddings", bytes.NewReader(body))

	result, err := svc.ForwardOpenAICompatibleEmbeddings(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 11, result.Usage.InputTokens)

	require.NotNil(t, httpStub.lastReq)
	require.Contains(t, httpStub.lastReq.URL.String(), "/v1beta/models/gemini-embedding-2-preview:batchEmbedContents")

	sentBody, err := io.ReadAll(httpStub.lastReq.Body)
	require.NoError(t, err)
	var sent map[string]any
	require.NoError(t, json.Unmarshal(sentBody, &sent))
	requests, ok := sent["requests"].([]any)
	require.True(t, ok)
	require.Len(t, requests, 2)

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	data, ok := got["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 2)
	second, ok := data[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1), second["index"])
	embedding, ok := second["embedding"].([]any)
	require.True(t, ok)
	require.Equal(t, []any{float64(3), float64(4)}, embedding)
}

func TestGeminiForwardOpenAICompatibleEmbeddings_RejectsTokenArrayInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &GeminiMessagesCompatService{
		httpUpstream: &geminiCompatHTTPUpstreamStub{},
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       203,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "gemini-api-key",
		},
		Concurrency: 1,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-embedding-2-preview","input":[1,2,3]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/embeddings", bytes.NewReader(body))

	result, err := svc.ForwardOpenAICompatibleEmbeddings(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "token array inputs are not supported")
}
