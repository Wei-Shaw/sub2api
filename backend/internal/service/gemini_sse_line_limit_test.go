package service

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Upstreams behind a custom base_url can stream a line that never ends. Every
// Gemini stream reader must stop at the configured line limit instead of
// buffering it, and must not forward a truncated prefix.
func TestGeminiStreamsRejectOversizedUpstreamLines(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 256}}
	svc := &GeminiMessagesCompatService{cfg: cfg}
	oversized := `data: {"candidates":[{"content":{"parts":[{"text":"` + strings.Repeat("x", 4096)

	run := map[string]func(c *gin.Context, resp *http.Response) error{
		"native": func(c *gin.Context, resp *http.Response) error {
			_, err := svc.handleNativeStreamingResponse(c, resp, time.Now(), false, &Account{ID: 1}, "")
			return err
		},
		"messages": func(c *gin.Context, resp *http.Response) error {
			_, err := svc.handleStreamingResponse(c, resp, time.Now(), "gemini-2.5-pro")
			return err
		},
		"chat_completions": func(c *gin.Context, resp *http.Response) error {
			_, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-2.5-pro", false, false)
			return err
		},
		"collector": func(_ *gin.Context, resp *http.Response) error {
			_, _, _, err := collectGeminiSSEObserved(cfg, resp.Body, false, nil)
			return err
		},
	}
	for name, fn := range run {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(oversized))}

			err := fn(c, resp)
			require.Error(t, err)
			require.True(t, errors.Is(err, bufio.ErrTooLong), "got %v", err)
			require.NotContains(t, w.Body.String(), "xxxx")
		})
	}
}
