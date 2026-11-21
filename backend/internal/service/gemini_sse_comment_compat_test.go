package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiClientRejectsSSEComments(t *testing.T) {
	cases := []struct {
		name string
		hint string
		want bool
	}{
		{"go-genai (Antigravity CLI)", "google-genai-sdk/1.71.0 gl-go/go1.28-20260721-RC03 cl/951519500 +3ebc191975 X:fieldtrack,boringcrypto", true},
		{"python-genai", "google-genai-sdk/1.20.0 gl-python/3.12.4", true},
		{"js-genai tolerates comments", "google-genai-sdk/1.9.0 gl-node/22.3.0", false},
		{"gemini-cli", "GeminiCLI/0.60.0 (darwin; arm64)", false},
		{"curl", "curl/8.7.1", false},
		{"empty", "", false},
		{"case-insensitive", "Google-GenAI-SDK/1.0.0 GL-Go/go1.27", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, geminiClientRejectsSSEComments(tc.hint))
		})
	}
}

func TestDownstreamRejectsSSECommentsReadsBothHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:streamGenerateContent", nil)
	require.False(t, downstreamRejectsSSEComments(c))

	c.Request.Header.Set("X-Goog-Api-Client", "google-genai-sdk/1.71.0 gl-go/go1.28")
	require.True(t, downstreamRejectsSSEComments(c))

	c.Request.Header.Del("X-Goog-Api-Client")
	c.Request.Header.Set("User-Agent", "google-genai-sdk/1.71.0 gl-go/go1.28")
	require.True(t, downstreamRejectsSSEComments(c))

	require.False(t, downstreamRejectsSSEComments(nil))
}

// runAntigravityGeminiStreamWithIdle 起一条上游流：先发一个 data 事件，然后空闲 idle 时长再关闭，
// 返回写给下游的全部字节。用来观察空闲期间网关是否发了 ":\n\n" 心跳。
func runAntigravityGeminiStreamWithIdle(t *testing.T, userAgent string, idle time.Duration) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(
		config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamKeepaliveInterval: 1},
		nil,
	)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:streamGenerateContent", nil)
	if userAgent != "" {
		c.Request.Header.Set("User-Agent", userAgent)
	}
	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}
	done := make(chan error, 1)
	go func() {
		_, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())
		done <- err
	}()
	_, err := io.WriteString(
		writer,
		`data: {"response":{"responseId":"resp_1","candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":1}}}`+"\n\n",
	)
	require.NoError(t, err)
	time.Sleep(idle)
	_, err = io.WriteString(
		writer,
		`data: {"response":{"responseId":"resp_1","candidates":[{"finishReason":"STOP"}]}}`+"\n\n",
	)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, <-done)
	require.NoError(t, reader.Close())
	return recorder.Body.String()
}

func TestAntigravityGeminiStreamKeepsCommentKeepaliveForOrdinaryClients(t *testing.T) {
	// The keepalive interval is configured in whole seconds. Leave enough idle
	// time for the first data event to be processed before the ticker fires;
	// otherwise a busy CI runner can consume the first tick too early and close
	// the stream before any heartbeat is written.
	out := runAntigravityGeminiStreamWithIdle(t, "curl/8.7.1", 2200*time.Millisecond)
	require.Contains(t, out, ":\n\n", "ordinary clients should still get the idle keepalive")
	require.Contains(t, out, `"text":"partial"`)
}

func TestAntigravityGeminiStreamSkipsCommentKeepaliveForGoGenai(t *testing.T) {
	out := runAntigravityGeminiStreamWithIdle(t, "google-genai-sdk/1.71.0 gl-go/go1.28-20260721-RC03", 1200*time.Millisecond)
	require.Contains(t, out, `"text":"partial"`)
	for _, event := range strings.Split(out, "\n\n") {
		require.False(t, strings.HasPrefix(event, ":"), "go-genai must never receive an SSE comment event, got %q", event)
	}
}

func TestGeminiCompatStreamsEmitKeepaliveAndRespectGoGenai(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GeminiMessagesCompatService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize:             defaultMaxLineSize,
				StreamKeepaliveInterval: 1,
			},
		},
	}

	testAccount := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	t.Run("native keeps comment keepalive for ordinary clients", func(t *testing.T) {
		c, rec := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent", nil)
		c.Request.Header.Set("User-Agent", "ai-sdk/google")
		pr, pw := io.Pipe()
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
		done := make(chan error, 1)
		go func() {
			_, err := svc.handleNativeStreamingResponse(c, resp, time.Now(), false, testAccount, "req-1")
			done <- err
		}()
		time.Sleep(1200 * time.Millisecond)
		_, _ = io.WriteString(pw, `data: {"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`+"\n\n")
		_ = pw.Close()
		require.NoError(t, <-done)
		_ = pr.Close()
		require.Contains(t, rec.Body.String(), ":\n\n")
		require.Contains(t, rec.Body.String(), `"text":"ok"`)
	})

	t.Run("native skips comment keepalive for go-genai", func(t *testing.T) {
		c, rec := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent", nil)
		c.Request.Header.Set("User-Agent", "google-genai-sdk/1.71.0 gl-go/go1.28")
		pr, pw := io.Pipe()
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
		done := make(chan error, 1)
		go func() {
			_, err := svc.handleNativeStreamingResponse(c, resp, time.Now(), false, testAccount, "req-1")
			done <- err
		}()
		time.Sleep(1200 * time.Millisecond)
		_, _ = io.WriteString(pw, `data: {"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`+"\n\n")
		_ = pw.Close()
		require.NoError(t, <-done)
		_ = pr.Close()
		require.NotContains(t, rec.Body.String(), ":\n\n")
	})

	t.Run("messages and chat compat emit keepalive and failover after keepalive on pre-output timeout", func(t *testing.T) {
		timeoutSvc := &GeminiMessagesCompatService{
			cfg: &config.Config{
				Gateway: config.GatewayConfig{
					MaxLineSize:               defaultMaxLineSize,
					StreamKeepaliveInterval:   1,
					StreamDataIntervalTimeout: 2,
				},
			},
		}
		for _, mode := range []string{"messages", "chat"} {
			t.Run(mode, func(t *testing.T) {
				c, rec := newAntigravityCompatContext(http.MethodPost, "/v1/"+mode, nil)
				pr, pw := io.Pipe()
				defer func() { _ = pw.Close(); _ = pr.Close() }()
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
				var err error
				if mode == "messages" {
					_, err = timeoutSvc.handleStreamingResponse(c, resp, time.Now(), "gemini-2.5-pro")
					require.Contains(t, rec.Body.String(), "event: ping")
				} else {
					_, err = timeoutSvc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-2.5-pro", false, false)
					require.Contains(t, rec.Body.String(), ": ping\n\n")
				}
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.True(t, failoverErr.RetryableOnSameAccount)
				require.True(t, failoverErr.SafeToFailoverAfterWrite)
			})
		}
	})
}

func TestGeminiStreamPreOutputFailoverAndRefusalMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GeminiMessagesCompatService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
	}

	for _, payload := range []string{
		"",
		`data: {"error":{"code":500,"message":"internal"}}` + "\n\n",
		`data: {"candidates":[{"finishReason":"OTHER"}],"usageMetadata":{"promptTokenCount":12}}` + "\n\n",
		`data: {"candidates":[{"content":{"parts":[{"thoughtSignature":"sig"}]},"finishReason":"MALFORMED_FUNCTION_CALL"}]}` + "\n\n",
	} {
		c, rec := newAntigravityCompatContext(http.MethodPost, "/v1/messages", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}
		_, err := svc.handleStreamingResponse(c, resp, time.Now(), "gemini-2.5-pro")
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr, "payload=%q", payload)
		require.Empty(t, rec.Body.String(), "must not commit message_start before valid content")

		agSvc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
		agCtx, agRec := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-3-pro:streamGenerateContent", nil)
		agResp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}
		_, agErr := agSvc.handleGeminiStreamingResponse(agCtx, agResp, time.Now())
		require.ErrorAs(t, agErr, &failoverErr, "antigravity payload=%q", payload)
		require.Empty(t, agRec.Body.String())

		agClaudeCtx, agClaudeRec := newAntigravityCompatContext(http.MethodPost, "/v1/messages", nil)
		agClaudeResp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}
		_, agClaudeErr := agSvc.handleClaudeStreamingResponse(agClaudeCtx, agClaudeResp, time.Now(), "gemini-3-pro")
		require.ErrorAs(t, agClaudeErr, &failoverErr, "antigravity claude payload=%q", payload)
		require.Empty(t, agClaudeRec.Body.String())
	}

	// Malformed data forwarded after keepalive must NOT mark SafeToFailoverAfterWrite.
	agKeepaliveSvc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize, StreamKeepaliveInterval: 1}, nil)
	agDirtyCtx, agDirtyRec := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-3-pro:streamGenerateContent", nil)
	agDirtyCtx.Request.Header.Set("User-Agent", "ai-sdk/google")
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		_, err := agKeepaliveSvc.handleGeminiStreamingResponse(agDirtyCtx, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}, time.Now())
		done <- err
	}()
	time.Sleep(1200 * time.Millisecond)
	_, _ = io.WriteString(pw, "data: not-json\n\n")
	_ = pw.Close()
	dirtyErr := <-done
	_ = pr.Close()
	var dirtyFailover *UpstreamFailoverError
	require.ErrorAs(t, dirtyErr, &dirtyFailover)
	require.False(t, dirtyFailover.SafeToFailoverAfterWrite)
	require.Contains(t, agDirtyRec.Body.String(), "data: not-json")

	// Committed SSE streams format subsequent non-retryable errors as SSE events rather than bare JSON.
	sseErrCtx, sseErrRec := newAntigravityCompatContext(http.MethodPost, "/v1/messages", nil)
	sseErrCtx.Header("Content-Type", "text/event-stream")
	sseErrCtx.Status(http.StatusOK)
	_, _ = io.WriteString(sseErrCtx.Writer, "event: ping\ndata: {\"type\":\"ping\"}\n\n")
	_ = svc.writeGeminiMappedError(sseErrCtx, &Account{ID: 1, Platform: PlatformGemini}, http.StatusBadRequest, "req-2", []byte(`{"error":{"message":"invalid schema"}}`))
	require.Contains(t, sseErrRec.Body.String(), "event: error\ndata: {\"error\":{\"message\":\"invalid schema\"")

	// Content-filter finishReason (including LANGUAGE) maps to Claude "refusal" and OpenAI Chat "content_filter".
	safetySSE := `data: {"candidates":[{"content":{"parts":[{"text":"blocked"}]},"finishReason":"LANGUAGE"}]}` + "\n\n"
	msgCtx, msgRec := newAntigravityCompatContext(http.MethodPost, "/v1/messages", nil)
	_, err := svc.handleStreamingResponse(msgCtx, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(safetySSE))}, time.Now(), "gemini-2.5-pro")
	require.NoError(t, err)
	require.Contains(t, msgRec.Body.String(), `"stop_reason":"refusal"`)

	ccCtx, ccRec := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
	_, err = svc.handleChatCompletionsStreamingResponseFromGemini(ccCtx, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(safetySSE))}, time.Now(), "gemini-2.5-pro", false, false)
	require.NoError(t, err)
	require.Contains(t, ccRec.Body.String(), `"finish_reason":"content_filter"`)
}

type profileCaptureUpstream struct {
	HTTPUpstream
	gotProfile HTTPUpstreamProfile
}

func (u *profileCaptureUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.gotProfile = HTTPUpstreamProfileFromContext(req.Context())
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`)),
	}, nil
}

func TestGeminiForwardUsesLongStreamHTTPUpstreamProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	up := &profileCaptureUpstream{}
	svc := &GeminiMessagesCompatService{
		httpUpstream: up,
		cfg:          &config.Config{},
	}
	testAccount := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", nil)
	_, err := svc.ForwardNative(context.Background(), c, testAccount, "gemini-2.5-flash", "generateContent", false, []byte(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
	require.NoError(t, err)
	require.Equal(t, HTTPUpstreamProfileLongStream, up.gotProfile)
}

func TestGeminiStreamTerminalStateMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	geminiSvc := &GeminiMessagesCompatService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
	}
	agSvc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	testAccount := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

	const textChunk = `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"partial answer"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2}}` + "\n\n"
	const stopChunk = `data: {"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":4}}` + "\n\n"

	type entrypoint struct {
		name               string
		path               string
		run                func(c *gin.Context, body io.ReadCloser) error
		wantCompleteMarker string
		forbidOnIncomplete []string
		wantErrorMarker    string
	}
	entrypoints := []entrypoint{
		{
			name: "gemini_v1beta",
			path: "/v1beta/models/gemini-2.5-pro:streamGenerateContent",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := geminiSvc.handleNativeStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), false, testAccount, "req-native")
				return err
			},
			wantCompleteMarker: `"finishReason":"STOP"`,
			wantErrorMarker:    `"error":{`,
		},
		{
			name: "antigravity_v1beta",
			path: "/v1beta/models/gemini-3-pro:streamGenerateContent",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleGeminiStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now())
				return err
			},
			wantCompleteMarker: `"finishReason":"STOP"`,
			wantErrorMarker:    `"error":{`,
		},
		{
			name: "gemini_messages",
			path: "/v1/messages",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := geminiSvc.handleStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-2.5-pro")
				return err
			},
			wantCompleteMarker: "event: message_stop",
			forbidOnIncomplete: []string{`"stop_reason":"end_turn"`, "event: message_stop"},
			wantErrorMarker:    "event: error",
		},
		{
			name: "antigravity_messages",
			path: "/v1/messages",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleClaudeStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-3-pro")
				return err
			},
			wantCompleteMarker: "event: message_stop",
			forbidOnIncomplete: []string{`"stop_reason":"end_turn"`, "event: message_stop"},
			wantErrorMarker:    "event: error",
		},
		{
			name: "gemini_chat_completions",
			path: "/v1/chat/completions",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := geminiSvc.handleChatCompletionsStreamingResponseFromGemini(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-2.5-pro", false, false)
				return err
			},
			wantCompleteMarker: "data: [DONE]",
			forbidOnIncomplete: []string{`"finish_reason":"stop"`, "data: [DONE]"},
			wantErrorMarker:    "event: error",
		},
		{
			name: "antigravity_chat_completions",
			path: "/v1/chat/completions",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleChatCompletionsStreamingFromAntigravity(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-3-pro", false)
				return err
			},
			wantCompleteMarker: "data: [DONE]",
			forbidOnIncomplete: []string{`"finish_reason":"stop"`, "data: [DONE]"},
			wantErrorMarker:    `"error":{`,
		},
	}

	for _, ep := range entrypoints {
		t.Run(ep.name+"/stop_completes", func(t *testing.T) {
			c, rec := newAntigravityCompatContext(http.MethodPost, ep.path, nil)
			err := ep.run(c, io.NopCloser(strings.NewReader(textChunk+stopChunk)))
			require.NoError(t, err)
			out := rec.Body.String()
			require.Contains(t, out, "partial answer")
			require.Contains(t, out, ep.wantCompleteMarker)
		})

		t.Run(ep.name+"/clean_eof_without_finish_reason_errors", func(t *testing.T) {
			c, rec := newAntigravityCompatContext(http.MethodPost, ep.path, nil)
			err := ep.run(c, io.NopCloser(strings.NewReader(textChunk)))
			require.Error(t, err)
			out := rec.Body.String()
			require.Contains(t, out, "partial answer")
			require.Contains(t, out, ep.wantErrorMarker)
			for _, forbidden := range ep.forbidOnIncomplete {
				require.NotContains(t, out, forbidden)
			}
		})

		t.Run(ep.name+"/unexpected_eof_without_finish_reason_errors", func(t *testing.T) {
			c, rec := newAntigravityCompatContext(http.MethodPost, ep.path, nil)
			err := ep.run(c, &antigravityCompatErrorReader{data: []byte(textChunk), err: io.ErrUnexpectedEOF})
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			out := rec.Body.String()
			require.Contains(t, out, "partial answer")
			for _, forbidden := range ep.forbidOnIncomplete {
				require.NotContains(t, out, forbidden)
			}
		})
	}
}

func TestGeminiPromptFeedbackTerminalMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	geminiSvc := &GeminiMessagesCompatService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
	}
	agSvc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)

	neutralInputs := map[string]string{
		"empty_prompt_feedback":      `data: {"promptFeedback":{},"usageMetadata":{"promptTokenCount":6}}` + "\n\n",
		"block_reason_unspecified":   `data: {"promptFeedback":{"blockReason":"BLOCK_REASON_UNSPECIFIED"},"usageMetadata":{"promptTokenCount":6}}` + "\n\n",
		"blocked_reason_unspecified": `data: {"promptFeedback":{"blockReason":"BLOCKED_REASON_UNSPECIFIED"},"usageMetadata":{"promptTokenCount":6}}` + "\n\n",
	}
	safetyBlockSSE := `data: {"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":9}}` + "\n\n"

	type entrypoint struct {
		name               string
		path               string
		run                func(c *gin.Context, body io.ReadCloser) error
		wantRefusalMarkers []string
	}
	entrypoints := []entrypoint{
		{
			name: "gemini_messages",
			path: "/v1/messages",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := geminiSvc.handleStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-2.5-pro")
				return err
			},
			wantRefusalMarkers: []string{`"stop_reason":"refusal"`, "event: message_stop"},
		},
		{
			name: "gemini_chat_completions",
			path: "/v1/chat/completions",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := geminiSvc.handleChatCompletionsStreamingResponseFromGemini(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-2.5-pro", false, false)
				return err
			},
			wantRefusalMarkers: []string{`"finish_reason":"content_filter"`, "data: [DONE]"},
		},
		{
			name: "antigravity_messages",
			path: "/v1/messages",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleClaudeStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-3-pro")
				return err
			},
			wantRefusalMarkers: []string{`"stop_reason":"refusal"`, "event: message_stop"},
		},
		{
			name: "antigravity_chat_completions",
			path: "/v1/chat/completions",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleChatCompletionsStreamingFromAntigravity(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-3-pro", false)
				return err
			},
			wantRefusalMarkers: []string{`"finish_reason":"content_filter"`, "data: [DONE]"},
		},
		{
			name: "antigravity_responses",
			path: "/v1/responses",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleResponsesStreamingFromAntigravity(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-3-pro")
				return err
			},
			wantRefusalMarkers: []string{"event: response.incomplete", `"reason":"content_filter"`},
		},
		{
			name: "antigravity_v1beta",
			path: "/v1beta/models/gemini-3-pro:streamGenerateContent",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleGeminiStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now())
				return err
			},
			wantRefusalMarkers: []string{`"blockReason":"SAFETY"`},
		},
	}

	for _, ep := range entrypoints {
		for inputName, sse := range neutralInputs {
			t.Run(ep.name+"/"+inputName+"_fails_over_before_first_write", func(t *testing.T) {
				c, rec := newAntigravityCompatContext(http.MethodPost, ep.path, nil)
				err := ep.run(c, io.NopCloser(strings.NewReader(sse)))
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.True(t, failoverErr.RetryableOnSameAccount)
				require.Empty(t, rec.Body.String())
			})
		}

		t.Run(ep.name+"/safety_block_completes_cleanly", func(t *testing.T) {
			c, rec := newAntigravityCompatContext(http.MethodPost, ep.path, nil)
			err := ep.run(c, io.NopCloser(strings.NewReader(safetyBlockSSE)))
			require.NoError(t, err)
			out := rec.Body.String()
			for _, marker := range ep.wantRefusalMarkers {
				require.Contains(t, out, marker)
			}
		})
	}
}
