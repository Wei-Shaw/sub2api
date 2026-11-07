package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildOpenAIEndpointURL_MultimodalEmbeddings(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		base string
		want string
	}{
		{"https://upstream.example", "https://upstream.example/v1/embeddings/multimodal"},
		{"https://upstream.example/v1/", "https://upstream.example/v1/embeddings/multimodal"},
		{"https://ark.cn-beijing.volces.com/api/v3", "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"},
		{"https://ark.cn-beijing.volces.com/api/v3/", "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"},
		{"https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal", "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"},
		{"https://upstream.example/v1/embeddings/multimodal/", "https://upstream.example/v1/embeddings/multimodal"},
		{"https://upstream.example/api/v3?tenant=example#stale", "https://upstream.example/api/v3/embeddings/multimodal?tenant=example"},
	} {
		t.Run(tt.base, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, buildOpenAIEndpointURL(tt.base, "/v1/embeddings/multimodal"))
		})
	}
}

func TestForwardMultimodalEmbeddings_PassthroughAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, input := range []struct {
		name string
		json string
	}{
		{"image URL", `[{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]`},
		{"inline image", `[{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2U="}}]`},
		{"text and image", `[{"type":"text","text":"A blue sky"},{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]`},
		{"video", `[{"type":"video_url","video_url":{"url":"https://example.com/video.mp4","fps":1,"max_video_tokens":10240}}]`},
	} {
		t.Run(input.name, func(t *testing.T) {
			reqBody := []byte(`{"model":"public-vision-embedding","input":` + input.json + `,"dimensions":1024,"encoding_format":"float","instructions":"Find matching content","multi_embedding":{"type":"enabled","compression":"zstd"}}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings/multimodal", bytes.NewReader(reqBody))
			const responseBody = `{"id":"embedding-id","model":"doubao-embedding-vision-251215","object":"list","data":{"object":"embedding","embedding":[0.1,0.2],"multi_embedding":"compressed-vector"},"usage":{"prompt_tokens":110,"prompt_tokens_details":{"image_tokens":100,"text_tokens":10},"total_tokens":110}}`
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"multimodal-rid"}},
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{
				ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Credentials: map[string]any{
					"api_key": "upstream-test-key", "base_url": "https://ark.cn-beijing.volces.com/api/v3",
					"model_mapping": map[string]any{"public-vision-embedding": "doubao-embedding-vision-251215"},
				},
			}

			result, err := svc.ForwardMultimodalEmbeddings(context.Background(), c, account, reqBody, "")

			require.NoError(t, err)
			require.Equal(t, "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer upstream-test-key", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
			require.JSONEq(t, string(ReplaceModelInBody(reqBody, "doubao-embedding-vision-251215")), string(upstream.lastBody))
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, responseBody, rec.Body.String())
			require.Equal(t, "public-vision-embedding", result.Model)
			require.Equal(t, "doubao-embedding-vision-251215", result.BillingModel)
			require.Equal(t, "doubao-embedding-vision-251215", result.UpstreamModel)
			require.Equal(t, "multimodal-rid", result.RequestID)
			require.Equal(t, 110, result.Usage.InputTokens)
			require.Equal(t, 100, result.Usage.ImageInputTokens)
			require.Zero(t, result.Usage.OutputTokens)
		})
	}
}

func TestForwardMultimodalEmbeddings_UpstreamValidationErrorIsPreserved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reqBody := []byte(`{"model":"doubao-embedding-vision-251215","input":[]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings/multimodal", bytes.NewReader(reqBody))
	const responseBody = `{"error":{"code":"InvalidParameter","message":"input must not be empty","type":"InvalidParameter"}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "upstream-test-key", "base_url": "https://ark.cn-beijing.volces.com/api/v3"}}

	result, err := svc.ForwardMultimodalEmbeddings(context.Background(), c, account, reqBody, "")

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, responseBody, rec.Body.String())
}

func TestForwardEmbeddings_MultimodalModelKeepsOrdinaryEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reqBody := []byte(`{"model":"doubao-embedding-vision-251215","input":"text"}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(reqBody))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"data":[],"usage":{"prompt_tokens":1}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "upstream-test-key", "base_url": "https://ark.cn-beijing.volces.com/api/v3"}}

	_, err := svc.ForwardEmbeddings(context.Background(), c, account, reqBody, "")

	require.NoError(t, err)
	require.Equal(t, "/api/v3/embeddings", upstream.lastReq.URL.Path)
	require.Equal(t, "text", gjson.GetBytes(upstream.lastBody, "input").String())
}
