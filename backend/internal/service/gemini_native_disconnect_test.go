package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiNativeNonStream_ClientCancelDrainsProviderUsage(t *testing.T) {
	tests := []struct {
		name         string
		account      *Account
		upstreamBody string
		wantInput    int
		wantOutput   int
	}{
		{
			name: "api key generateContent",
			account: &Account{
				ID: 9101, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": "test-key"},
			},
			upstreamBody: `{"candidates":[{"content":{"parts":[{"text":"done"}]}}],"usageMetadata":{"promptTokenCount":11,"cachedContentTokenCount":3,"candidatesTokenCount":5,"thoughtsTokenCount":2}}`,
			wantInput:    8,
			wantOutput:   7,
		},
		{
			name: "oauth buffered streamGenerateContent",
			account: &Account{
				ID: 9102, Platform: PlatformGemini, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{"access_token": "test-token", "project_id": "test-project"},
			},
			upstreamBody: "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"done\"}]}}],\"usageMetadata\":{\"promptTokenCount\":13,\"cachedContentTokenCount\":2,\"candidatesTokenCount\":6,\"thoughtsTokenCount\":3}}}\n\n" +
				"data: [DONE]\n\n",
			wantInput:  11,
			wantOutput: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			releaseBody := make(chan []byte, 1)
			readStarted := make(chan struct{})
			upstream := &geminiDisconnectHTTPUpstream{
				dispatched: make(chan *http.Request, 1),
				response: func(req *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body: &delayedContextReadCloser{
							ctx: req.Context(), release: releaseBody, readStart: readStarted,
						},
					}
				},
			}
			svc := &GeminiMessagesCompatService{
				tokenProvider: &GeminiTokenProvider{},
				httpUpstream:  upstream,
				cfg:           &config.Config{},
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
			ctx, cancel := context.WithCancel(context.Background())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body)).WithContext(ctx)

			type outcome struct {
				result *ForwardResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := svc.ForwardNative(ctx, c, tt.account, "gemini-2.5-flash", "generateContent", false, body)
				done <- outcome{result: result, err: err}
			}()
			select {
			case <-readStarted:
			case <-time.After(time.Second):
				t.Fatal("Gemini native response body was not read")
			}
			cancel()
			time.Sleep(20 * time.Millisecond)
			releaseBody <- []byte(tt.upstreamBody)

			select {
			case got := <-done:
				require.NoError(t, got.err)
				require.NotNil(t, got.result)
				require.True(t, got.result.ClientDisconnect)
				require.Equal(t, tt.wantInput, got.result.Usage.InputTokens)
				require.Equal(t, tt.wantOutput, got.result.Usage.OutputTokens)
				require.Empty(t, rec.Body.String())
			case <-time.After(time.Second):
				t.Fatal("Gemini native forwarding did not finish")
			}
		})
	}
}

func TestGeminiNativeStream_WriteFailureStillCollectsTerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"parts":[{"text":"hel"}]}}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":1}}` + "\n\n" +
		`data: {"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":5,"thoughtsTokenCount":2}}` + "\n\n" +
		"data: [DONE]\n\n"
	upstream := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID: 9201, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &failAllGeminiWrites{ResponseWriter: c.Writer}
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse", bytes.NewReader(body))

	result, err := svc.ForwardNative(c.Request.Context(), c, account, "gemini-2.5-flash", "streamGenerateContent", true, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
}

func TestGeminiNativeStream_ReadErrorReturnsObservedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	partial := `data: {"candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":7,"cachedContentTokenCount":2,"candidatesTokenCount":3}}` + "\n\n"
	upstream := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(io.MultiReader(strings.NewReader(partial), iotest.ErrReader(errors.New("upstream reset")))),
	}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID: 9301, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse", bytes.NewReader(body))

	result, err := svc.ForwardNative(c.Request.Context(), c, account, "gemini-2.5-flash", "streamGenerateContent", true, body)

	require.ErrorContains(t, err, "upstream reset")
	require.NotNil(t, result)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
}

func TestGeminiNativeCountTokens_ClientCancelStillCancelsUpstream(t *testing.T) {
	readStarted := make(chan struct{})
	upstream := &geminiDisconnectHTTPUpstream{
		dispatched: make(chan *http.Request, 1),
		response: func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       &contextBlockingReadCloser{ctx: req.Context(), readStart: readStarted},
			}
		},
	}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID: 9401, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:countTokens", bytes.NewReader(body)).WithContext(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := svc.ForwardNative(ctx, c, account, "gemini-2.5-flash", "countTokens", false, body)
		done <- err
	}()
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("countTokens response body was not read")
	}
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("countTokens did not follow downstream cancellation")
	}
}

func TestGeminiNative_ClientCancelDoesNotRetryAcceptedAttempt(t *testing.T) {
	releaseResponse := make(chan struct{})
	upstream := &geminiDisconnectHTTPUpstream{
		dispatched: make(chan *http.Request, 1),
		response: func(*http.Request) *http.Response {
			<-releaseResponse
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"retryable"}}`)),
			}
		},
	}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID: 9501, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body)).WithContext(ctx)

	done := make(chan error, 1)
	go func() {
		_, err := svc.ForwardNative(ctx, c, account, "gemini-2.5-flash", "generateContent", false, body)
		done <- err
	}()
	select {
	case <-upstream.dispatched:
	case <-time.After(time.Second):
		t.Fatal("Gemini native request was not dispatched")
	}
	cancel()
	close(releaseResponse)

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled Gemini native request entered retry backoff")
	}
	require.Equal(t, int32(1), upstream.calls.Load())
}

func TestGeminiNative_DisconnectDrainIsBounded(t *testing.T) {
	readStarted := make(chan struct{})
	upstream := &geminiDisconnectHTTPUpstream{
		dispatched: make(chan *http.Request, 1),
		response: func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       &contextBlockingReadCloser{ctx: req.Context(), readStart: readStarted},
			}
		},
	}
	svc := &GeminiMessagesCompatService{
		httpUpstream: upstream,
		cfg: &config.Config{Gateway: config.GatewayConfig{
			GeminiDisconnectDrainTimeoutSeconds: 1,
		}},
	}
	account := &Account{
		ID: 9601, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body)).WithContext(ctx)

	type outcome struct {
		result *ForwardResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.ForwardNative(ctx, c, account, "gemini-2.5-flash", "generateContent", false, body)
		done <- outcome{result: result, err: err}
	}()
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("Gemini native response body was not read")
	}
	started := time.Now()
	cancel()

	select {
	case got := <-done:
		require.ErrorIs(t, got.err, ErrGeminiClientDisconnected)
		require.ErrorIs(t, got.err, context.Canceled)
		require.Nil(t, got.result)
		require.GreaterOrEqual(t, time.Since(started), 900*time.Millisecond)
		require.Less(t, time.Since(started), 2*time.Second)
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("Gemini native response drain exceeded its configured bound")
	}
}
