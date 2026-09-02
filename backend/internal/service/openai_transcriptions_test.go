package service

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenAITranscriptionsURL(t *testing.T) {
	require.Equal(t, "https://asr.example/v1/audio/transcriptions", buildOpenAITranscriptionsURL("https://asr.example"))
	require.Equal(t, "https://asr.example/v1/audio/transcriptions", buildOpenAITranscriptionsURL("https://asr.example/v1"))
}

func multipartBoundary(t *testing.T, contentType string) string {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	return params["boundary"]
}

func TestForwardTranscriptions_APIKeyRewritesModelAndPreservesLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "fun-asr-nano"))
	require.NoError(t, writer.WriteField("language", "zh"))
	part, err := writer.CreateFormFile("file", "sample.wav")
	require.NoError(t, err)
	_, err = part.Write([]byte("RIFFaudio"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"text":"hello"}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "sk-test", "base_url": "https://asr.example", "model_mapping": map[string]any{"fun-asr-nano": "FunAudioLLM/Fun-ASR-Nano-2512"},
	}}

	result, err := svc.ForwardTranscriptions(context.Background(), c, account, body.Bytes(), writer.FormDataContentType(), "fun-asr-nano")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://asr.example/v1/audio/transcriptions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	parsed, err := multipart.NewReader(bytes.NewReader(upstream.lastBody), multipartBoundary(t, upstream.lastReq.Header.Get("Content-Type"))).ReadForm(1024)
	require.NoError(t, err)
	require.Equal(t, "FunAudioLLM/Fun-ASR-Nano-2512", parsed.Value["model"][0])
	require.Equal(t, "zh", parsed.Value["language"][0])
	require.Equal(t, "sample.wav", parsed.File["file"][0].Filename)
}
