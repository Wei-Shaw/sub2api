//go:build unit

package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func a208NewNonStreamTestContext(limit int64) (*GeminiMessagesCompatService, *gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	svc := &GeminiMessagesCompatService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				UpstreamResponseReadMaxBytes: limit,
			},
		},
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return svc, c, w
}

func a208GeminiTextResponse(text string) *http.Response {
	body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"` + text +
		`"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":7}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestGeminiMessagesCompatHandleNonStreamingResponse_A208AllowsBodyAbove8MB(t *testing.T) {
	svc, c, w := a208NewNonStreamTestContext(64 << 20)

	resp := a208GeminiTextResponse(strings.Repeat("a", 9<<20))
	usage, err := svc.handleNonStreamingResponse(c, resp, "gemini-2.5-flash")

	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.OutputTokens)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestGeminiMessagesCompatHandleNonStreamingResponse_A208ReportsTooLarge(t *testing.T) {
	svc, c, w := a208NewNonStreamTestContext(1024)

	resp := a208GeminiTextResponse(strings.Repeat("a", 2048))
	usage, err := svc.handleNonStreamingResponse(c, resp, "gemini-2.5-flash")

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUpstreamResponseBodyTooLarge), "err=%v", err)
	require.Nil(t, usage)
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Contains(t, w.Body.String(), "Upstream response too large")
	require.True(t, IsResponseCommitted(c))
}
