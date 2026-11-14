package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIWSTTFTModeTestEvent struct {
	delay   time.Duration
	payload string
}

// openAIWSTTFTModeTestTurn replays a reasoning turn: the reasoning item is
// announced shortly after the out-of-band codex.* frame, the first delta only
// arrives after the reasoning ends.
func openAIWSTTFTModeTestTurn(responseID string, startDelay, reasoningDelay time.Duration) []openAIWSTTFTModeTestEvent {
	return []openAIWSTTFTModeTestEvent{
		{startDelay, `{"type":"response.created","response":{"id":"` + responseID + `","model":"gpt-5.4"}}`},
		{0, `{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":1}}}`},
		{0, `{"type":"response.in_progress","response":{"id":"` + responseID + `"}}`},
		{300 * time.Millisecond, `{"type":"response.output_item.added","output_index":0,"item":{"id":"rs_` + responseID + `","type":"reasoning","summary":[]}}`},
		{reasoningDelay, `{"type":"response.output_text.delta","output_index":1,"delta":"Hi"}`},
		{0, `{"type":"response.completed","response":{"id":"` + responseID + `","model":"gpt-5.4","usage":{"input_tokens":3,"output_tokens":2}}}`},
	}
}

// openAIWSTTFTModeHTTPExpectedMs is the first-token time the SSE path would
// report for the same events: the delay up to the first event that
// openAIStreamDataStartsTTFT accepts. codex.* frames only exist on WebSocket.
func openAIWSTTFTModeHTTPExpectedMs(t *testing.T, events []openAIWSTTFTModeTestEvent, mode string) time.Duration {
	t.Helper()
	var elapsed time.Duration
	for _, event := range events {
		elapsed += event.delay
		eventType := gjson.Get(event.payload, "type").String()
		if strings.HasPrefix(eventType, "codex.") {
			continue
		}
		if openAIStreamDataStartsTTFT(event.payload, eventType, false, mode) {
			return elapsed
		}
	}
	t.Fatalf("no first-token event for mode %s", mode)
	return 0
}

func TestOpenAIWSPassthroughFirstTokenFollowsTTFTMode(t *testing.T) {
	for _, mode := range []string{OpenAITTFTModeSemantic, OpenAITTFTModeVisible} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIWSPassthroughTTFTModeCase(t, mode)
		})
	}
}

func runOpenAIWSPassthroughTTFTModeCase(t *testing.T, mode string) {
	gin.SetMode(gin.TestMode)
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{openAITTFTMode: mode, expiresAt: time.Now().Add(time.Minute).UnixNano()})
	t.Cleanup(func() {
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{openAITTFTMode: OpenAITTFTModeSemantic, expiresAt: time.Now().Add(time.Minute).UnixNano()})
	})

	const reasoningDelay = 600 * time.Millisecond
	const tolerance = 250 * time.Millisecond
	// The second turn's events wait so its response.create is accepted first.
	turns := [][]openAIWSTTFTModeTestEvent{
		openAIWSTTFTModeTestTurn("resp_ttft_1", 0, reasoningDelay),
		openAIWSTTFTModeTestTurn("resp_ttft_2", 200*time.Millisecond, reasoningDelay),
	}

	upstreamConn := &openAIWSCaptureConn{}
	for _, turn := range turns {
		for _, event := range turn {
			upstreamConn.events = append(upstreamConn.events, []byte(event.payload))
			upstreamConn.readDelays = append(upstreamConn.readDelays, event.delay)
		}
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	svc := &OpenAIGatewayService{
		cfg:                       cfg,
		httpUpstream:              &httpUpstreamRecorder{},
		cache:                     &stubGatewayCache{},
		openaiWSResolver:          NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:             NewCodexToolCorrector(),
		openaiWSPassthroughDialer: &openAIWSCaptureDialer{conn: upstreamConn},
	}
	account := &Account{
		ID: 454, Name: "openai-ws-ttft-mode", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough},
	}

	resultCh := make(chan *OpenAIForwardResult, len(turns))
	hooks := &OpenAIWSIngressHooks{
		AfterTurn: func(_ int, result *OpenAIForwardResult, turnErr error) {
			if turnErr == nil && result != nil {
				resultCh <- result
			}
		},
	}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginCtx.Request = r.Clone(r.Context())
		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			return
		}
		_ = svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, hooks)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	request := []byte(`{"type":"response.create","model":"gpt-5.4","input":"hi"}`)
	for i, turn := range turns {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, request))
		cancelWrite()
		for {
			readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
			_, message, readErr := clientConn.Read(readCtx)
			cancelRead()
			require.NoError(t, readErr)
			if gjson.GetBytes(message, "type").String() == "response.completed" {
				break
			}
		}

		var result *OpenAIForwardResult
		select {
		case result = <-resultCh:
		case <-time.After(5 * time.Second):
			t.Fatalf("turn %d result was not reported", i+1)
		}
		expected := openAIWSTTFTModeHTTPExpectedMs(t, turn, mode)
		require.NotNil(t, result.FirstTokenMs, "turn %d must record first token", i+1)
		got := time.Duration(*result.FirstTokenMs) * time.Millisecond
		require.GreaterOrEqual(t, got, expected, "turn %d first token must not precede the SSE first-token event", i+1)
		require.Less(t, got, expected+tolerance, "turn %d first token must match the SSE rule for mode %s", i+1, mode)
	}
	_ = clientConn.Close(coderws.StatusNormalClosure, "done")
}
