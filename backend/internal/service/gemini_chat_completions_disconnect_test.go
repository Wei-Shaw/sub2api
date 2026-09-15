package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiDisconnectHTTPUpstream struct {
	dispatched chan *http.Request
	response   func(*http.Request) *http.Response
	calls      atomic.Int32
}

func (s *geminiDisconnectHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.calls.Add(1)
	select {
	case s.dispatched <- req:
	default:
	}
	return s.response(req), nil
}

func (s *geminiDisconnectHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

type delayedContextReadCloser struct {
	ctx       context.Context
	release   <-chan []byte
	readStart chan struct{}
	reader    *bytes.Reader
	once      sync.Once
}

func (r *delayedContextReadCloser) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.readStart) })
	if r.reader != nil {
		return r.reader.Read(p)
	}
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	case body := <-r.release:
		r.reader = bytes.NewReader(body)
		return r.reader.Read(p)
	}
}

func (*delayedContextReadCloser) Close() error { return nil }

type contextBlockingReadCloser struct {
	ctx       context.Context
	readStart chan struct{}
	once      sync.Once
}

func (r *contextBlockingReadCloser) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.readStart) })
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (*contextBlockingReadCloser) Close() error { return nil }

type stagedContextReadCloser struct {
	ctx       context.Context
	chunks    <-chan []byte
	readCalls chan struct{}
	reader    *bytes.Reader
}

func (r *stagedContextReadCloser) Read(p []byte) (int, error) {
	for {
		if r.reader != nil && r.reader.Len() > 0 {
			return r.reader.Read(p)
		}
		select {
		case r.readCalls <- struct{}{}:
		default:
		}
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		case chunk, ok := <-r.chunks:
			if !ok {
				return 0, io.EOF
			}
			r.reader = bytes.NewReader(chunk)
		}
	}
}

func (*stagedContextReadCloser) Close() error { return nil }

type failAllGeminiWrites struct {
	gin.ResponseWriter
}

func (*failAllGeminiWrites) Write([]byte) (int, error) {
	return 0, errors.New("downstream connection closed")
}

func (*failAllGeminiWrites) WriteString(string) (int, error) {
	return 0, errors.New("downstream connection closed")
}

func TestGeminiChatCompletionsNonStream_ClientCancelDrainsProviderUsage(t *testing.T) {
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
				ID: 8101, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": "test-key"},
			},
			upstreamBody: `{"candidates":[{"content":{"parts":[{"text":"done"}]} }],"usageMetadata":{"promptTokenCount":11,"cachedContentTokenCount":3,"candidatesTokenCount":5,"thoughtsTokenCount":2}}`,
			wantInput:    8,
			wantOutput:   7,
		},
		{
			name: "oauth buffered streamGenerateContent",
			account: &Account{
				ID: 8102, Platform: PlatformGemini, Type: AccountTypeOAuth, Concurrency: 1,
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
			body := []byte(`{"model":"gemini-2.5-flash","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
			downstreamCtx, cancelDownstream := context.WithCancel(context.Background())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(downstreamCtx)

			type outcome struct {
				result *ForwardResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := svc.ForwardAsChatCompletions(downstreamCtx, c, tt.account, body)
				done <- outcome{result: result, err: err}
			}()

			select {
			case <-upstream.dispatched:
			case <-time.After(time.Second):
				t.Fatal("Gemini request was not dispatched")
			}
			select {
			case <-readStarted:
			case <-time.After(time.Second):
				t.Fatal("Gemini response body was not read")
			}

			cancelDownstream()
			time.Sleep(20 * time.Millisecond)
			releaseBody <- []byte(tt.upstreamBody)

			select {
			case got := <-done:
				require.NoError(t, got.err)
				require.NotNil(t, got.result)
				require.True(t, got.result.ClientDisconnect)
				require.Equal(t, tt.wantInput, got.result.Usage.InputTokens)
				require.Equal(t, tt.wantOutput, got.result.Usage.OutputTokens)
				require.Empty(t, rec.Body.String(), "no response should be written after the client disconnects")
			case <-time.After(time.Second):
				t.Fatal("Gemini forwarding did not finish")
			}
		})
	}
}

func TestGeminiChatCompletionsStream_WriteFailureStillCollectsTerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"parts":[{"text":"hel"}]}}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":1}}` + "\n\n" +
		`data: {"candidates":[{"content":{"parts":[{"text":"hello"}]} ,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":5,"thoughtsTokenCount":2}}` + "\n\n" +
		"data: [DONE]\n\n"
	upstream := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID: 8201, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &failAllGeminiWrites{ResponseWriter: c.Writer}
	body := []byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	result, err := svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
}

func TestGeminiChatCompletionsStream_ClientCancelDrainsTerminalUsageWithoutFurtherWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chunks := make(chan []byte, 2)
	readCalls := make(chan struct{}, 4)
	upstream := &geminiDisconnectHTTPUpstream{
		dispatched: make(chan *http.Request, 1),
		response: func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: &stagedContextReadCloser{
					ctx: req.Context(), chunks: chunks, readCalls: readCalls,
				},
			}
		},
	}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID: 8251, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)

	type outcome struct {
		result *ForwardResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
		done <- outcome{result: result, err: err}
	}()
	chunks <- []byte(`data: {"candidates":[{"content":{"parts":[{"text":"hel"}]}}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":1}}` + "\n\n")
	for range 2 {
		select {
		case <-readCalls:
		case <-time.After(time.Second):
			t.Fatal("Gemini stream did not finish processing the first chunk")
		}
	}
	require.Contains(t, rec.Body.String(), `"content":"hel"`)
	bytesBeforeCancel := rec.Body.Len()
	cancel()
	time.Sleep(20 * time.Millisecond)
	chunks <- []byte(`data: {"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":5,"thoughtsTokenCount":2}}` + "\n\n" + "data: [DONE]\n\n")
	close(chunks)

	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.True(t, got.result.ClientDisconnect)
		require.Equal(t, 4, got.result.Usage.InputTokens)
		require.Equal(t, 7, got.result.Usage.OutputTokens)
		require.Equal(t, bytesBeforeCancel, rec.Body.Len(), "stream writes continued after downstream cancellation")
	case <-time.After(time.Second):
		t.Fatal("Gemini stream did not finish draining")
	}
}

func TestGeminiChatCompletionsStream_ReadErrorReturnsObservedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	partial := `data: {"candidates":[{"content":{"parts":[{"text":"hel"}]}}],"usageMetadata":{"promptTokenCount":7,"cachedContentTokenCount":2,"candidatesTokenCount":3}}` + "\n\n"
	upstream := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(io.MultiReader(strings.NewReader(partial), iotest.ErrReader(errors.New("upstream reset")))),
	}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID: 8301, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	result, err := svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body)

	require.ErrorContains(t, err, "upstream reset")
	require.NotNil(t, result)
	require.False(t, result.ClientDisconnect)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
}

func TestGeminiChatCompletions_PreCanceledRequestIsNotDispatched(t *testing.T) {
	upstream := &geminiDisconnectHTTPUpstream{
		dispatched: make(chan *http.Request, 1),
		response: func(*http.Request) *http.Response {
			return nil
		},
	}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID: 8401, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)

	result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)

	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	require.Zero(t, upstream.calls.Load())
}

func TestGeminiChatCompletions_ClientCancelDoesNotRetryAcceptedAttempt(t *testing.T) {
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
		ID: 8501, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)

	done := make(chan error, 1)
	go func() {
		_, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
		done <- err
	}()
	select {
	case <-upstream.dispatched:
	case <-time.After(time.Second):
		t.Fatal("Gemini request was not dispatched")
	}
	cancel()
	close(releaseResponse)

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled Gemini request entered retry backoff")
	}
	require.Equal(t, int32(1), upstream.calls.Load())
}

func TestGeminiChatCompletions_DisconnectDrainIsBounded(t *testing.T) {
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
		ID: 8601, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)

	type outcome struct {
		result *ForwardResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
		done <- outcome{result: result, err: err}
	}()
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("Gemini response body was not read")
	}
	started := time.Now()
	cancel()

	select {
	case got := <-done:
		require.ErrorIs(t, got.err, context.Canceled)
		require.Nil(t, got.result)
		require.GreaterOrEqual(t, time.Since(started), 900*time.Millisecond)
		require.Less(t, time.Since(started), 2*time.Second)
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("Gemini response drain exceeded its configured bound")
	}
}

func TestGeminiChatCompletionsStream_DrainTimeoutReturnsObservedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chunks := make(chan []byte, 1)
	readCalls := make(chan struct{}, 4)
	upstream := &geminiDisconnectHTTPUpstream{
		dispatched: make(chan *http.Request, 1),
		response: func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: &stagedContextReadCloser{
					ctx: req.Context(), chunks: chunks, readCalls: readCalls,
				},
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
		ID: 8651, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)

	type outcome struct {
		result *ForwardResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
		done <- outcome{result: result, err: err}
	}()
	chunks <- []byte(`data: {"candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":8,"cachedContentTokenCount":3,"candidatesTokenCount":2}}` + "\n\n")
	for range 2 {
		select {
		case <-readCalls:
		case <-time.After(time.Second):
			t.Fatal("Gemini stream did not finish processing partial usage")
		}
	}
	require.Contains(t, rec.Body.String(), `"content":"partial"`)
	cancel()

	select {
	case got := <-done:
		require.ErrorIs(t, got.err, ErrGeminiClientDisconnected)
		require.NotNil(t, got.result)
		require.True(t, got.result.ClientDisconnect)
		require.Equal(t, 5, got.result.Usage.InputTokens)
		require.Equal(t, 2, got.result.Usage.OutputTokens)
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("Gemini partial-usage drain exceeded its configured bound")
	}
}

func TestGeminiChatCompletionsStream_WriteFailureStartsBoundedDrain(t *testing.T) {
	readStarted := make(chan struct{})
	upstream := &geminiDisconnectHTTPUpstream{
		dispatched: make(chan *http.Request, 1),
		response: func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
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
		ID: 8701, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &failAllGeminiWrites{ResponseWriter: c.Writer}
	body := []byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	started := time.Now()
	result, err := svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body)

	require.ErrorIs(t, err, ErrGeminiClientDisconnected)
	require.Nil(t, result)
	require.GreaterOrEqual(t, time.Since(started), 900*time.Millisecond)
	require.Less(t, time.Since(started), 2*time.Second)
}
