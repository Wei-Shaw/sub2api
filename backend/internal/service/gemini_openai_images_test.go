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

func TestGeminiForwardOpenAICompatibleImagesGenerations_ReturnsB64JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"X-Request-Id": []string{"gemini-img-1"},
			},
			Body: io.NopCloser(strings.NewReader(`{
				"candidates":[{"content":{"parts":[
					{"text":"done"},
					{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}
				]}}],
				"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}
			}`)),
		},
	}
	svc := &GeminiMessagesCompatService{
		httpUpstream: httpStub,
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       301,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "gemini-api-key",
		},
		Concurrency: 1,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash-image","prompt":"draw a cat","size":"1024x1024","response_format":"b64_json"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/images/generations", bytes.NewReader(body))

	result, err := svc.ForwardOpenAICompatibleImagesGenerations(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gemini-img-1", result.RequestID)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "1024x1024", result.ImageInputSize)

	require.NotNil(t, httpStub.lastReq)
	require.Contains(t, httpStub.lastReq.URL.String(), "/v1beta/models/gemini-2.5-flash-image:generateContent")
	require.Equal(t, "gemini-api-key", httpStub.lastReq.Header.Get("x-goog-api-key"))
	sentBody, err := io.ReadAll(httpStub.lastReq.Body)
	require.NoError(t, err)
	var sent map[string]any
	require.NoError(t, json.Unmarshal(sentBody, &sent))
	contents, ok := sent["contents"].([]any)
	require.True(t, ok)
	require.Len(t, contents, 1)
	content, ok := contents[0].(map[string]any)
	require.True(t, ok)
	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 1)
	part, ok := parts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "draw a cat", part["text"])
	generationConfig, ok := sent["generationConfig"].(map[string]any)
	require.True(t, ok)
	modalities, ok := generationConfig["responseModalities"].([]any)
	require.True(t, ok)
	require.Contains(t, modalities, "TEXT")
	require.Contains(t, modalities, "IMAGE")

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	data, ok := got["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	first, ok := data[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "iVBORw0KGgo=", first["b64_json"])
	require.NotContains(t, first, "url")
}

func TestGeminiForwardOpenAICompatibleImagesGenerations_RejectsURLResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpStub := &geminiCompatHTTPUpstreamStub{}
	svc := &GeminiMessagesCompatService{
		httpUpstream: httpStub,
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       302,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "gemini-api-key",
		},
		Concurrency: 1,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash-image","prompt":"draw a cat","response_format":"url"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/images/generations", bytes.NewReader(body))

	result, err := svc.ForwardOpenAICompatibleImagesGenerations(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "response_format=url is not supported")
	require.Zero(t, httpStub.calls)
}

func TestGeminiForwardOpenAICompatibleImagesGenerations_RejectsMultipleImages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpStub := &geminiCompatHTTPUpstreamStub{}
	svc := &GeminiMessagesCompatService{
		httpUpstream: httpStub,
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       303,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "gemini-api-key",
		},
		Concurrency: 1,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash-image","prompt":"draw a cat","n":2}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/images/generations", bytes.NewReader(body))

	result, err := svc.ForwardOpenAICompatibleImagesGenerations(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "n greater than 1 is not supported")
	require.Zero(t, httpStub.calls)
}

func TestGeminiForwardOpenAICompatibleImagesGenerations_RequiresImageModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpStub := &geminiCompatHTTPUpstreamStub{}
	svc := &GeminiMessagesCompatService{
		httpUpstream: httpStub,
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       304,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "gemini-api-key",
		},
		Concurrency: 1,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","prompt":"draw a cat"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/images/generations", bytes.NewReader(body))

	result, err := svc.ForwardOpenAICompatibleImagesGenerations(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "requires an image generation model")
	require.Zero(t, httpStub.calls)
}
