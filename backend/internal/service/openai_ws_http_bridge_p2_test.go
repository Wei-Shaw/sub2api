package service

import (
	"context"
	"errors"
	"fmt"
	"io"
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

func TestOpenAIWSHTTPBridgeRejectsUnresolvedPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name         string
		priorPayload string
		terminal     string
		responseID   string
	}{
		{name: "fresh_connection"},
		{name: "mismatching_parent", priorPayload: `{"input":[{"role":"user","content":"first"}]}`, terminal: "response.completed", responseID: "resp_other"},
		{name: "empty_history", priorPayload: `{}`, terminal: "response.completed", responseID: "resp_parent"},
		{name: "missing_response_id", priorPayload: `{"input":[{"role":"user","content":"first"}]}`, terminal: "response.completed"},
		{name: "failed_parent", priorPayload: `{"input":[{"role":"user","content":"first"}]}`, terminal: "response.failed", responseID: "resp_parent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{}
			if tc.priorPayload != "" {
				body := fmt.Sprintf("data: {\"type\":%q,\"response\":{\"id\":%q,\"output\":[]}}\n\n", tc.terminal, tc.responseID)
				if tc.terminal == "response.failed" {
					body = `data: {"type":"response.failed","response":{"id":"resp_parent","output":[],"error":{"type":"invalid_request_error","code":"invalid_value","message":"invalid input"}}}` + "\n\n"
				}
				upstream.responses = append(upstream.responses, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))})
			}
			upstream.responses = append(upstream.responses, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_should_not_post\",\"output\":[]}}\n\n"))})
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg)}
			account := &Account{
				ID: 9005, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-test"},
				Extra:       map[string]any{"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeHTTPBridge},
				Status:      StatusActive, Schedulable: true, Concurrency: 1,
			}
			errCh := make(chan error, 1)
			wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					errCh <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()
				readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				_, firstMessage, err := conn.Read(readCtx)
				cancel()
				if err != nil {
					errCh <- err
					return
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r.Clone(r.Context())
				proxyErr := svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "sk-test", firstMessage, nil)
				var closeErr *OpenAIWSClientCloseError
				if errors.As(proxyErr, &closeErr) {
					_ = conn.Close(closeErr.StatusCode(), closeErr.Reason())
				}
				errCh <- proxyErr
			}))
			defer wsServer.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			priorTurns := 0
			if tc.priorPayload != "" {
				priorTurns = 1
				priorFrame := `{"type":"response.create","model":"gpt-6.1-sol",` + strings.TrimPrefix(tc.priorPayload, "{")
				if tc.priorPayload == `{}` {
					priorFrame = `{"type":"response.create","model":"gpt-6.1-sol"}`
				}
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(priorFrame)))
				_, event, err := client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, tc.terminal, gjson.GetBytes(event, "type").String())
			}
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-6.1-sol","previous_response_id":"resp_parent","input":[{"role":"user","content":"delta needing parent"}]}`)))
			_, event, readErr := client.Read(ctx)
			require.Equal(t, coderws.StatusPolicyViolation, coderws.CloseStatus(readErr), "unresolved parent must fail closed, received: %s", event)
			require.Contains(t, readErr.Error(), "previous_response_id")
			require.Contains(t, readErr.Error(), "full input")
			select {
			case proxyErr := <-errCh:
				var closeErr *OpenAIWSClientCloseError
				require.ErrorAs(t, proxyErr, &closeErr)
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for websocket bridge rejection")
			}
			require.Len(t, upstream.bodies, priorTurns, "never POST a delta whose parent cannot be reconstructed")
		})
	}
}
