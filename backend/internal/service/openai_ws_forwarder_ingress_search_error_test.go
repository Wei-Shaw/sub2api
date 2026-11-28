package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestManagedNativeSearchReadErrorRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, completed := range []bool{false, true} {
		t.Run(fmt.Sprintf("completed=%v", completed), func(t *testing.T) {
			cfg := passthroughLifecycleConfig()
			upstream := &openAIWSCaptureConn{
				events:     [][]byte{[]byte(fmt.Sprintf(`{"type":"response.web_search_call.%s","item_id":"ws_unsettled"}`, map[bool]string{true: "completed", false: "searching"}[completed]))},
				readDelays: []time.Duration{20 * time.Millisecond},
			}
			fresh := &openAIWSCaptureConn{
				events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_retry","usage":{"input_tokens":1,"output_tokens":1}}}`)},
			}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{upstream, fresh}}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			defer pool.Close()
			svc := newPassthroughLifecycleService(cfg, nil)
			svc.openaiWSPool = pool
			account := passthroughLifecycleAccount()
			account.Extra["openai_apikey_responses_websockets_v2_mode"] = OpenAIWSIngressModeCtxPool
			controlCtx, cancelControl := context.WithCancel(context.Background())
			defer cancelControl()
			var result *OpenAIForwardResult
			var turnErr error
			callbackCount := 0
			serverErr := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				conn, err := coderws.Accept(writer, request, nil)
				if err != nil {
					serverErr <- err
					return
				}
				_, firstMessage, err := conn.Read(controlCtx)
				_ = conn.CloseNow()
				if err != nil {
					serverErr <- err
					return
				}
				ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ginCtx.Request = request.Clone(controlCtx)
				hooks := &OpenAIWSIngressHooks{
					AfterTurn: func(turn int, got *OpenAIForwardResult, err error) {
						callbackCount++
						result = got
						turnErr = err
						if !completed {
							cancelControl()
						}
					},
				}
				serverErr <- svc.ProxyResponsesWebSocketFromClient(controlCtx, ginCtx, conn, account, "sk-test", firstMessage, hooks)
			}))
			defer server.Close()
			client := dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"gpt-5.4","stream":true}`)
			defer func() { _ = client.CloseNow() }()
			select {
			case err := <-serverErr:
				if completed {
					require.ErrorContains(t, err, "read upstream websocket event")
					require.Same(t, turnErr, err)
				} else {
					require.NoError(t, err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("managed ingress did not exit")
			}
			require.Equal(t, 1, callbackCount)
			require.NotNil(t, result)
			if completed {
				require.Equal(t, 1, dialer.DialCount())
				require.Equal(t, 1, result.NativeWebSearchCalls)
				require.Equal(t, OpenAIUsage{}, result.Usage)
				require.Empty(t, result.RequestID)
				require.Empty(t, result.UpstreamTerminalEvent)
				require.Equal(t, "gpt-5.4", result.Model)
				require.True(t, result.OpenAIWSMode)
			} else {
				require.Equal(t, 2, dialer.DialCount())
				require.NoError(t, turnErr)
				require.Zero(t, result.NativeWebSearchCalls)
				require.Equal(t, 1, result.Usage.InputTokens)
			}
		})
	}
}
