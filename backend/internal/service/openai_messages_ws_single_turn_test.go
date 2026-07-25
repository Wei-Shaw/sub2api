package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIMessagesWSConn struct {
	mu               sync.Mutex
	events           [][]byte
	read             int
	writes           []map[string]any
	writeErr         error
	readErr          error
	onWrite          func()
	onRead           func(read int)
	blockAfterEvents bool
	closed           bool
}

func (c *openAIMessagesWSConn) WriteJSON(_ context.Context, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	c.mu.Lock()
	c.writes = append(c.writes, payload)
	writeErr := c.writeErr
	onWrite := c.onWrite
	c.mu.Unlock()
	if onWrite != nil {
		onWrite()
	}
	return writeErr
}

func (c *openAIMessagesWSConn) ReadMessage(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	if c.read < len(c.events) {
		message := append([]byte(nil), c.events[c.read]...)
		c.read++
		read := c.read
		onRead := c.onRead
		c.mu.Unlock()
		if onRead != nil {
			onRead(read)
		}
		return message, nil
	}
	readErr := c.readErr
	blockAfterEvents := c.blockAfterEvents
	c.mu.Unlock()
	if readErr != nil {
		return nil, readErr
	}
	if blockAfterEvents {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, errors.New("unexpected end of websocket events")
}

func (c *openAIMessagesWSConn) Ping(context.Context) error { return nil }

func (c *openAIMessagesWSConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

type openAIMessagesWSDialer struct {
	mu         sync.Mutex
	conn       openAIWSClientConn
	err        error
	statusCode int
	headers    http.Header
	calls      int
	url        string
	dialHeader http.Header
}

func (d *openAIMessagesWSDialer) Dial(_ context.Context, wsURL string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	d.url = wsURL
	d.dialHeader = cloneHeader(headers)
	return d.conn, d.statusCode, cloneHeader(d.headers), d.err
}

func newOpenAIMessagesWSTestContext(t *testing.T, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "claude-code/test")
	return c, rec
}

func newOpenAIMessagesWSAccount(enabled bool) *Account {
	return &Account{
		ID:          901,
		Name:        "messages-ws-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
		Extra: map[string]any{OpenAIMessagesWebSocketV2ExtraKey: enabled},
	}
}

func newOpenAIMessagesWSService(dialer openAIWSClientDialer) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 2
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 2
	cfg.Gateway.OpenAIWS.MessagesDrainTimeoutSeconds = 1
	return &OpenAIGatewayService{
		cfg:                       cfg,
		openaiWSPassthroughDialer: dialer,
	}
}

func openAIMessagesWSEvents() [][]byte {
	return [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp_ws","model":"gpt-5.4","status":"in_progress"}}`),
		[]byte(`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_ws","type":"message","role":"assistant","content":[]}}`),
		[]byte(`{"type":"response.content_part.added","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`),
		[]byte(`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"ok"}`),
		[]byte(`{"type":"response.output_text.done","output_index":0,"content_index":0,"text":"ok"}`),
		[]byte(`{"type":"response.completed","response":{"id":"resp_ws","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_ws","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`),
	}
}

func TestAccountIsOpenAIMessagesWebSocketV2Enabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil", account: nil},
		{name: "missing", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}},
		{name: "false", account: newOpenAIMessagesWSAccount(false)},
		{name: "wrong type", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAIMessagesWebSocketV2ExtraKey: "true"}}},
		{name: "api key rejected", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{OpenAIMessagesWebSocketV2ExtraKey: true}}},
		{name: "other platform rejected", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{OpenAIMessagesWebSocketV2ExtraKey: true}}},
		{name: "oauth enabled", account: newOpenAIMessagesWSAccount(true), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.account.IsOpenAIMessagesWebSocketV2Enabled())
		})
	}
}

func TestMessagesWSFlagDoesNotChangeResponsesOrNativeIngressProtocolSelection(t *testing.T) {
	account := newOpenAIMessagesWSAccount(true)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	decision := NewOpenAIWSProtocolResolver(cfg).Resolve(account)
	require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
	require.Equal(t, "account_disabled", decision.Reason)
	require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeOff))
}

func TestForwardAsAnthropicMessagesWSDisabledUsesExistingHTTPPath(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	dialer := &openAIMessagesWSDialer{err: errors.New("must not dial")}
	upstreamBody := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_http\",\"model\":\"gpt-5.4\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"http\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	httpUpstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := newOpenAIMessagesWSService(dialer)
	svc.httpUpstream = httpUpstream

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(false), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 0, dialer.calls)
	require.Len(t, httpUpstream.requests, 1)
	require.False(t, result.OpenAIWSMode)
}

func TestOpenAIMessagesWSDrainTimeoutDefaultOverrideAndReadCap(t *testing.T) {
	t.Parallel()

	defaultSvc := &OpenAIGatewayService{}
	require.Equal(t, 30*time.Second, defaultSvc.openAIMessagesWSDrainTimeout())

	overrideSvc := &OpenAIGatewayService{cfg: &config.Config{}}
	overrideSvc.cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 90
	overrideSvc.cfg.Gateway.OpenAIWS.MessagesDrainTimeoutSeconds = 12
	require.Equal(t, 12*time.Second, overrideSvc.openAIMessagesWSDrainTimeout())

	overrideSvc.cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	require.Equal(t, 5*time.Second, overrideSvc.openAIMessagesWSDrainTimeout())
}

func TestForwardAsAnthropicMessagesWSBufferedSingleTurn(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, rec := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{events: openAIMessagesWSEvents()}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols, headers: http.Header{"x-request-id": []string{"rid-ws"}}}
	svc := newOpenAIMessagesWSService(dialer)
	svc.cfg.Gateway.OpenAIWS.AllowStoreRecovery = true
	account := newOpenAIMessagesWSAccount(true)
	account.Extra["openai_ws_allow_store_recovery"] = true

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "cache-key", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, dialer.calls)
	require.Equal(t, "wss://chatgpt.com/backend-api/codex/responses", dialer.url)
	require.Equal(t, openAIWSBetaV2Value, dialer.dialHeader.Get("OpenAI-Beta"))
	require.Len(t, conn.writes, 1)
	require.Equal(t, "response.create", conn.writes[0]["type"])
	require.Equal(t, false, conn.writes[0]["store"])
	require.NotContains(t, conn.writes[0], "previous_response_id")
	require.True(t, conn.closed)
	require.False(t, result.Stream)
	require.Equal(t, RequestTypeSync, result.RequestType)
	require.True(t, result.OpenAIWSMode)
	require.Equal(t, openAIMessagesResponsesUpstreamEndpoint, result.UpstreamEndpoint)
	require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
	require.Equal(t, "resp_ws", result.ResponseID)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.NotEmpty(t, bytes.TrimSpace(rec.Body.Bytes()))
	require.Equal(t, "ok", jsonResponseText(t, rec.Body.Bytes()))
}

func TestForwardAsAnthropicMessagesWSCancelledTerminalConversion(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c, rec := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{events: [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp_cancelled","model":"gpt-5.4","status":"in_progress"}}`),
		[]byte(`{"type":"response.cancelled","response":{"id":"resp_cancelled","model":"gpt-5.4","status":"cancelled","usage":{"input_tokens":2,"output_tokens":0}}}`),
	}}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "response.cancelled", result.UpstreamTerminalEvent)
	require.Nil(t, result.FirstTokenMs)
	require.Contains(t, rec.Body.String(), "event: message_stop")
}

func TestForwardAsAnthropicMessagesWSStreamingConversion(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c, rec := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{events: openAIMessagesWSEvents()}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.True(t, result.Stream)
	require.Equal(t, RequestTypeStream, result.RequestType)
	require.True(t, result.OpenAIWSMode)
	require.Contains(t, rec.Body.String(), "event: message_start")
	require.Contains(t, rec.Body.String(), "event: content_block_delta")
	require.Contains(t, rec.Body.String(), "event: message_stop")
}

func TestForwardAsAnthropicMessagesWSClientDisconnectDrainsTerminalUsage(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	ctx, cancel := context.WithCancel(context.Background())
	conn := &openAIMessagesWSConn{
		events: openAIMessagesWSEvents(),
		onRead: func(read int) {
			if read == 1 {
				cancel()
			}
		},
	}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(ctx, c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, len(openAIMessagesWSEvents()), conn.read)
	require.True(t, conn.closed)
}

func TestForwardAsAnthropicMessagesWSConcatenatedDeltaAndCompleted(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c, rec := newOpenAIMessagesWSTestContext(t, body)
	events := openAIMessagesWSEvents()
	concatenated := append(append([]byte(nil), events[3]...), events[5]...)
	conn := &openAIMessagesWSConn{events: [][]byte{events[0], events[1], events[2], concatenated}}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"text":"ok"`)
	require.Contains(t, rec.Body.String(), "event: message_stop")
}

func TestForwardAsAnthropicMessagesWSLateDisconnectGetsFreshDrainDeadline(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := openAIMessagesWSEvents()
	conn := &openAIMessagesWSConn{events: events}
	conn.onRead = func(read int) {
		if read == len(events)-1 {
			cancel()
			time.Sleep(1100 * time.Millisecond)
		}
	}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)
	svc.cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 1
	svc.cfg.Gateway.OpenAIWS.MessagesDrainTimeoutSeconds = 1

	result, err := svc.ForwardAsAnthropic(ctx, c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
	require.Equal(t, len(events), conn.read)
}

func TestForwardAsAnthropicMessagesWSReadDrainIsBounded(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	ctx, cancel := context.WithCancel(context.Background())
	conn := &openAIMessagesWSConn{
		events: [][]byte{
			[]byte(`{"type":"response.created","response":{"id":"resp_bounded","model":"gpt-5.4","status":"in_progress"}}`),
		},
		onWrite:          cancel,
		blockAfterEvents: true,
	}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)
	svc.cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 900
	svc.cfg.Gateway.OpenAIWS.MessagesDrainTimeoutSeconds = 1

	started := time.Now()
	result, err := svc.ForwardAsAnthropic(ctx, c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.Error(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Contains(t, err.Error(), context.DeadlineExceeded.Error())
	require.Less(t, time.Since(started), 3*time.Second)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.True(t, conn.closed)
}

func TestForwardAsAnthropicMessagesWSConverterTimeoutTriggersBoundedDrain(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{
		events: [][]byte{
			[]byte(`{"type":"response.created","response":{"id":"resp_timeout","model":"gpt-5.4","status":"in_progress"}}`),
		},
		blockAfterEvents: true,
	}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)
	svc.cfg.Gateway.StreamDataIntervalTimeout = 1
	svc.cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 900
	svc.cfg.Gateway.OpenAIWS.MessagesDrainTimeoutSeconds = 1

	started := time.Now()
	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "stream data interval timeout")
	require.Less(t, time.Since(started), 4*time.Second)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.True(t, conn.closed)
}

func TestForwardAsAnthropicMessagesWSPrewriteCancellationWritesNothing(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	ctx, cancel := context.WithCancel(context.Background())
	conn := &openAIMessagesWSConn{}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	dialer.conn = conn
	svc := newOpenAIMessagesWSService(dialer)
	originalDialer := svc.openaiWSPassthroughDialer
	svc.openaiWSPassthroughDialer = openAIMessagesWSDialerFunc(func(ctx context.Context, wsURL string, headers http.Header, proxyURL string) (openAIWSClientConn, int, http.Header, error) {
		cancel()
		return originalDialer.Dial(ctx, wsURL, headers, proxyURL)
	})

	result, err := svc.ForwardAsAnthropic(ctx, c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, conn.writes)
	require.True(t, conn.closed)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

type openAIMessagesWSDialerFunc func(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error)

func (f openAIMessagesWSDialerFunc) Dial(ctx context.Context, wsURL string, headers http.Header, proxyURL string) (openAIWSClientConn, int, http.Header, error) {
	return f(ctx, wsURL, headers, proxyURL)
}

func TestForwardAsAnthropicMessagesWSWriteErrorNeverFailover(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{writeErr: errors.New("partial write")}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Len(t, conn.writes, 1)
	require.True(t, conn.closed)
}

func TestForwardAsAnthropicMessagesWSReadErrorNeverFailover(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{
		events:  openAIMessagesWSEvents()[:1],
		readErr: errors.New("read failed"),
	}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.NotNil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Len(t, conn.writes, 1)
	require.True(t, conn.closed)
}

func TestForwardAsAnthropicMessagesWSFailedTerminalNeverFailover(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{events: [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp_failed","model":"gpt-5.4","status":"in_progress"}}`),
		[]byte(`{"type":"response.failed","response":{"id":"resp_failed","model":"gpt-5.4","status":"failed","error":{"code":"server_error","message":"failed"},"usage":{"input_tokens":3,"output_tokens":0}}}`),
	}}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.NotNil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, "response.failed", result.UpstreamTerminalEvent)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Nil(t, result.FirstTokenMs)
}

func TestOpenAIMessagesWSCredentialErrorCanFailover(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	dialer := &openAIMessagesWSDialer{err: errors.New("must not dial")}
	svc := newOpenAIMessagesWSService(dialer)
	account := newOpenAIMessagesWSAccount(true)
	delete(account.Credentials, "access_token")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, 0, dialer.calls)
}

func TestForwardAsAnthropicMessagesWSNilDialConnectionCanFailover(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	dialer := &openAIMessagesWSDialer{statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, 1, dialer.calls)
}

func TestForwardAsAnthropicMessagesWSDialErrorCanFailover(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	dialer := &openAIMessagesWSDialer{err: errors.New("unauthorized"), statusCode: http.StatusUnauthorized}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusUnauthorized, failoverErr.StatusCode)
}

func TestForwardAsAnthropicMessagesWSOAuthKeepsBearerAuthorization(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{events: openAIMessagesWSEvents()}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, newOpenAIMessagesWSAccount(true), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Bearer oauth-token", dialer.dialHeader.Get("Authorization"))
	require.Equal(t, "chatgpt-account", dialer.dialHeader.Get("ChatGPT-Account-Id"))
}

func TestForwardAsAnthropicMessagesWSAgentIdentityRefreshesAuthorization(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{events: openAIMessagesWSEvents()}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	key, privateKey := newTestAgentIdentityKey(t)
	account := newOpenAIMessagesWSAccount(true)
	account.ID = 903
	account.Credentials = map[string]any{
		"auth_mode":          OpenAIAuthModeAgentIdentity,
		"agent_runtime_id":   key.runtimeID,
		"agent_private_key":  privateKey,
		"task_id":            key.taskID,
		"chatgpt_account_id": "agent-account",
	}
	svc := newOpenAIMessagesWSService(dialer)

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	authorization := dialer.dialHeader.Get("Authorization")
	require.True(t, strings.HasPrefix(authorization, "AgentAssertion "))
	require.NotContains(t, authorization, "Bearer ")
	require.Equal(t, "agent-account", dialer.dialHeader.Get("ChatGPT-Account-Id"))
}

func TestForwardAsAnthropicMessagesWSShadowAgentIdentityUsesParentHeaders(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := newOpenAIMessagesWSTestContext(t, body)
	conn := &openAIMessagesWSConn{events: openAIMessagesWSEvents()}
	dialer := &openAIMessagesWSDialer{conn: conn, statusCode: http.StatusSwitchingProtocols}
	key, privateKey := newTestAgentIdentityKey(t)
	parent := newOpenAIMessagesWSAccount(true)
	parent.ID = 904
	parent.Credentials = map[string]any{
		"auth_mode":          OpenAIAuthModeAgentIdentity,
		"agent_runtime_id":   key.runtimeID,
		"agent_private_key":  privateKey,
		"task_id":            key.taskID,
		"chatgpt_account_id": "parent-agent-account",
	}
	parentID := parent.ID
	shadow := newOpenAIMessagesWSAccount(true)
	shadow.ID = 905
	shadow.ParentAccountID = &parentID
	shadow.Credentials = map[string]any{}
	svc := newOpenAIMessagesWSService(dialer)
	svc.accountRepo = &agentIdentityCredentialsRepo{account: parent}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, shadow, body, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	authorization := dialer.dialHeader.Get("Authorization")
	require.True(t, strings.HasPrefix(authorization, "AgentAssertion "))
	require.NotContains(t, authorization, "Bearer ")
	require.Equal(t, "parent-agent-account", dialer.dialHeader.Get("ChatGPT-Account-Id"))
}

func jsonResponseText(t *testing.T, body []byte) string {
	t.Helper()
	var response struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.NotEmpty(t, response.Content)
	return response.Content[0].Text
}
