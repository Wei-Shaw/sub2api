//go:build unit

package service

import (
	"bufio"
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

// A2-02：Gemini / OpenAI Images 的上游 SSE 读取必须受 gateway.max_line_size 约束，
// 上游发来一条不带换行的超长行时应尽早报错，而不是把整行无上限地缓冲进内存。

const (
	a202MaxLineSize = 1 << 20
	a202TotalBytes  = 16 << 20
	// 允许的额外读取量：bufio 缓冲区 + 余量。
	a202ReadSlack = 64 << 10
)

type a202RepeatReader struct{ b byte }

func (r a202RepeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}

type a202CountingReader struct {
	r io.Reader
	n int64
}

func (c *a202CountingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// a202OverlongLineBody 返回 "data: " 后跟 16MB 'a' 且无换行的上游 body。
func a202OverlongLineBody() *a202CountingReader {
	return &a202CountingReader{r: io.MultiReader(
		strings.NewReader("data: "),
		io.LimitReader(a202RepeatReader{b: 'a'}, a202TotalBytes),
	)}
}

func a202StreamResponse(body io.Reader) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(body),
	}
}

func a202TestContext(path string) *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
	return c
}

func a202Config() *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{MaxLineSize: a202MaxLineSize}}
}

func a202RequireBounded(t *testing.T, err error, body *a202CountingReader) {
	t.Helper()
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.Less(t, body.n, int64(a202MaxLineSize+a202ReadSlack), "超长行应在超过 max_line_size 后立即停止读取")
}

func a202GeminiAccount() *Account {
	return &Account{
		ID:          2202,
		Name:        "gemini-line-limit-test",
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}
}

func TestA202GeminiNativeStreaming_LineLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GeminiMessagesCompatService{cfg: a202Config()}
	body := a202OverlongLineBody()
	c := a202TestContext("/v1beta/models/gemini-2.5-flash:streamGenerateContent")

	_, err := svc.handleNativeStreamingResponse(c, a202StreamResponse(body), time.Now(), false, a202GeminiAccount(), "req-a202")
	a202RequireBounded(t, err, body)
}

func TestA202GeminiMessagesCompatStreaming_LineLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GeminiMessagesCompatService{cfg: a202Config()}
	body := a202OverlongLineBody()
	c := a202TestContext("/v1/messages")

	_, err := svc.handleStreamingResponse(c, a202StreamResponse(body), time.Now(), "gemini-2.5-flash")
	a202RequireBounded(t, err, body)
}

func TestA202GeminiChatCompletionsStreaming_LineLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GeminiMessagesCompatService{cfg: a202Config()}
	body := a202OverlongLineBody()
	c := a202TestContext("/v1/chat/completions")

	_, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, a202StreamResponse(body), time.Now(), "gemini-2.5-flash", false, false)
	a202RequireBounded(t, err, body)
}

func TestA202CollectGeminiSSE_LineLimit(t *testing.T) {
	body := a202OverlongLineBody()
	_, _, _, err := collectGeminiSSEObserved(body, false, a202MaxLineSize, nil)
	a202RequireBounded(t, err, body)
}

// 正常行（含超过 bufio 默认 4KB 缓冲的长行、末尾无换行的最后一行）行为不变。
func TestA202CollectGeminiSSE_LineLimitKeepsNormalLines(t *testing.T) {
	longText := strings.Repeat("x", 10<<10)
	sse := `data: {"candidates":[{"content":{"parts":[{"text":"` + longText + `"}],"role":"model"}}]}` + "\n\n" +
		`data: {"candidates":[{"content":{"parts":[{"text":"tail"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"candidatesTokenCount":4,"promptTokenCount":10,"totalTokenCount":14}}`

	var events []string
	_, usage, stats, err := collectGeminiSSEObserved(strings.NewReader(sse), false, a202MaxLineSize, func(raw []byte) {
		events = append(events, string(raw))
	})
	require.NoError(t, err)
	require.Equal(t, 2, stats.dataEvents)
	require.Len(t, events, 2)
	require.Contains(t, events[0], longText)
	require.Contains(t, events[1], `"tail"`)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.InputTokens)
}

func TestA202OpenAIImagesStreaming_LineLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name     string
		interval int
	}{
		{name: "sync", interval: 0},
		{name: "pump", interval: 600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := a202Config()
			cfg.Gateway.ImageStreamDataIntervalTimeout = tc.interval
			svc := &OpenAIGatewayService{cfg: cfg}
			body := a202OverlongLineBody()
			c := a202TestContext("/v1/images/generations")

			_, _, _, _, err := svc.handleOpenAIImagesStreamingResponse(a202StreamResponse(body), c, time.Now(), nil)
			a202RequireBounded(t, err, body)
		})
	}
}

func TestA202OpenAIImagesOAuthStreaming_LineLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name     string
		interval int
	}{
		{name: "sync", interval: 0},
		{name: "pump", interval: 600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := a202Config()
			cfg.Gateway.ImageStreamDataIntervalTimeout = tc.interval
			svc := &OpenAIGatewayService{cfg: cfg}
			body := a202OverlongLineBody()
			c := a202TestContext("/v1/images/generations")

			_, _, _, _, err := svc.handleOpenAIImagesOAuthStreamingResponse(a202StreamResponse(body), c, time.Now(), "b64_json", "image_generation", "gpt-image-2")
			a202RequireBounded(t, err, body)
		})
	}
}
