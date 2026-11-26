//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNativeSearchErrorBoundaries(t *testing.T) {
	for _, path := range []string{"http", "passthrough", "http_buffered", "passthrough_buffered", "chat_buffered", "messages_buffered", "chat_stream", "messages_stream", "http_bridge", "ws_buffered", "ws_stream"} {
		for _, completed := range []bool{false, true} {
			for _, ending := range []string{"eof", "read_error", "failed", "failed_output", "malformed"} {
				t.Run(fmt.Sprintf("%s/completed=%v/%s", path, completed, ending), func(t *testing.T) {
					searchStatus := "searching"
					if completed {
						searchStatus = "completed"
					}
					events := []string{fmt.Sprintf(`{"type":"response.web_search_call.%s","item_id":"ws_1","output_index":0}`, searchStatus)}
					switch ending {
					case "failed":
						events = append(events, `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"rate_limit_exceeded","message":"rate limit exceeded"}}}`)
					case "failed_output":
						status := "in_progress"
						if completed {
							status = "completed"
						}
						events = []string{fmt.Sprintf(`{"type":"response.failed","response":{"id":"resp_failed","status":"failed","output":[{"type":"web_search_call","id":"ws_1","status":"%s"}],"error":{"code":"rate_limit_exceeded","message":"rate limit exceeded"}}}`, status)}
					case "malformed":
						events = append(events, `{"type":`)
					}
					response := nativeSearchHTTPResponse(bufferedItemsSSE(events...), "text/event-stream")
					if ending == "read_error" {
						response.Body = io.NopCloser(&errAfterReader{data: strings.NewReader(bufferedItemsSSE(events...)), err: io.ErrUnexpectedEOF})
					}
					upstream := &httpUpstreamRecorder{resp: response}
					cfg := passthroughArgsTestConfig()
					svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
					account := passthroughArgsFallbackAccount()
					account.Extra = map[string]any{"openai_passthrough": strings.HasPrefix(path, "passthrough")}
					ctx := nativeSearchGinContext()
					var result *OpenAIForwardResult
					var err error
					var dialCount func() int
					switch path {
					case "http", "passthrough", "http_buffered", "passthrough_buffered":
						result, err = svc.Forward(context.Background(), ctx, account, []byte(fmt.Sprintf(`{"model":"gpt-5.4","input":"search","stream":%v}`, !strings.HasSuffix(path, "_buffered"))))
					case "chat_buffered":
						result, err = svc.handleChatBufferedStreamingResponse(response, ctx, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
					case "messages_buffered":
						result, err = svc.handleAnthropicBufferedStreamingResponse(response, ctx, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
					case "chat_stream":
						result, err = svc.handleChatStreamingResponse(response, ctx, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now(), 0)
					case "messages_stream":
						result, err = svc.handleAnthropicStreamingResponse(response, ctx, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
					case "http_bridge":
						payload := []byte(`{"type":"response.create","model":"gpt-5.4","input":"search"}`)
						result, err = svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), ctx, account, "test", payload, len(payload), "gpt-5.4", "", "", "", "", 1, func([]byte) error { return nil })
					default:
						cfg = passthroughLifecycleConfig()
						cfg.Gateway.OpenAIWS.APIKeyEnabled = true
						conn := &openAIWSCaptureConn{}
						for _, event := range events {
							conn.events = append(conn.events, []byte(event))
							conn.readDelays = append(conn.readDelays, time.Millisecond)
						}
						pool := newOpenAIWSConnPool(cfg)
						dialer := &openAIWSCaptureDialer{conn: conn}
						pool.setClientDialerForTest(dialer)
						dialCount = dialer.DialCount
						defer pool.Close()
						svc.cfg, svc.openaiWSPool = cfg, pool
						svc.openaiWSResolver = NewOpenAIWSProtocolResolver(cfg)
						account.Extra = map[string]any{"responses_websockets_v2_enabled": true}
						result, err = svc.Forward(context.Background(), ctx, account, []byte(fmt.Sprintf(`{"model":"gpt-5.4","input":"search","stream":%v}`, path == "ws_stream")))
					}
					if (strings.HasPrefix(path, "ws_") || path == "http_bridge") && strings.HasPrefix(ending, "failed") && err == nil {
						require.NotNil(t, result)
						require.False(t, result.SucceededForScheduling())
					} else if !completed && (path == "http_buffered" || path == "passthrough_buffered") && (ending == "eof" || ending == "malformed") {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
					if completed {
						require.NotNil(t, result)
						require.Equal(t, 1, result.NativeWebSearchCalls)
						require.Equal(t, OpenAIUsage{}, result.Usage)
						require.Equal(t, "gpt-5.4", result.Model)
						if strings.HasPrefix(path, "ws_") {
							require.True(t, result.OpenAIWSMode)
							require.Equal(t, 1, dialCount(), "billable searches must prevent reconnect replay")
							require.Empty(t, upstream.requests, "billable searches must prevent HTTP replay")
						}
					} else if result != nil {
						require.Zero(t, result.NativeWebSearchCalls)
					}
					if !completed && dialCount != nil && (ending == "eof" || ending == "read_error") {
						require.Greater(t, dialCount(), 1, "unbilled transport failures remain retryable")
					}
				})
			}
		}
	}
}

func TestNativeSearchCompactErrorDoesNotReplay(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, completed := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%v/completed=%v", passthrough, completed), func(t *testing.T) {
				ctx := newOpenAICompactFallbackTestContext(t, "/openai/v1/responses")
				MarkOpenAINativeCompactionV2(ctx)
				body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"compaction_trigger"}]}`)
				wire := ""
				if completed {
					wire = bufferedItemsSSE(`{"type":"response.web_search_call.completed","item_id":"ws_1"}`)
				}
				wire += bufferedItemsSSE(`{"type":"response.failed","response":{"status":"failed","error":{"code":"model_not_found","message":"model not found"}}}`)
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					nativeSearchHTTPResponse(wire, "text/event-stream"),
					nativeSearchHTTPResponse(nativeSearchSSE(0), "text/event-stream"),
				}}
				cfg := passthroughArgsTestConfig()
				cfg.Gateway.OpenAICompactModel = "gpt-5.4"
				svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
				account := passthroughArgsFallbackAccount()
				account.Extra = map[string]any{"openai_passthrough": passthrough}
				result, err := svc.Forward(context.Background(), ctx, account, body)
				if completed {
					require.Error(t, err)
					require.Len(t, upstream.bodies, 1)
					require.NotNil(t, result)
					require.Equal(t, 1, result.NativeWebSearchCalls)
					require.Equal(t, OpenAIUsage{}, result.Usage)
				} else {
					require.NoError(t, err)
					require.Len(t, upstream.bodies, 2)
					require.Zero(t, result.NativeWebSearchCalls)
				}
			})
		}
	}
}

func TestNativeSearchFailedJSONRetainsUsage(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%v", passthrough), func(t *testing.T) {
			body := strings.Replace(nativeSearchResponse(1), `"status":"completed"`, `"status":"failed"`, 1)
			upstream := &httpUpstreamRecorder{resp: nativeSearchHTTPResponse(body, "application/json")}
			svc := &OpenAIGatewayService{cfg: passthroughArgsTestConfig(), httpUpstream: upstream}
			account := passthroughArgsFallbackAccount()
			account.Extra = map[string]any{"openai_passthrough": passthrough}
			ctx := nativeSearchGinContext()
			result, err := svc.Forward(context.Background(), ctx, account, []byte(`{"model":"gpt-5.4","input":"search","stream":false}`))
			require.Error(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, result.NativeWebSearchCalls)
			require.Equal(t, 1000, result.Usage.InputTokens)
			require.Equal(t, 500, result.Usage.OutputTokens)
			require.GreaterOrEqual(t, ctx.Writer.Status(), http.StatusBadRequest)
			require.Len(t, upstream.bodies, 1)
		})
	}
}
