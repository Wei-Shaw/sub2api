//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNativeWebSearchWSPassthroughSettlementIsolation(t *testing.T) {
	for _, scenario := range []string{"late_merge_zero", "late_merge_one", "missing_created", "late_item_replay", "late_output_item_replay", "scoped_item_reuse"} {
		t.Run(scenario, func(t *testing.T) {
			replayScenario := scenario == "late_item_replay" || scenario == "late_output_item_replay" || scenario == "scoped_item_reuse"
			gin.SetMode(gin.TestMode)
			controlCtx, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			upstream := newStagedPassthroughConn()
			svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
			results := make(chan *OpenAIForwardResult, 16)
			server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, passthroughLifecycleAccount(), func(*gin.Context) *OpenAIWSIngressHooks {
				return &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) {
					results <- result
				}}
			})
			defer server.Close()
			client := dialPassthroughLifecycleClient(t, server)
			defer func() { _ = client.CloseNow() }()
			send := func(frame string) {
				t.Helper()
				upstream.Send(frame)
				_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.NoError(t, err)
			}
			assertUsage := func(responseID string, count int) {
				t.Helper()
				select {
				case result := <-results:
					require.Equal(t, responseID, result.RequestID)
					require.Equal(t, count, result.NativeWebSearchCalls)
				case <-time.After(3 * time.Second):
					t.Fatal("missing forwarded turn usage")
				}
			}
			for turn := 0; turn < 3; turn++ {
				if turn > 0 {
					writeCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
					err := client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","input":"search"}`))
					stop()
					require.NoError(t, err)
				}
				requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
				responseID := fmt.Sprintf("resp_%d", turn)
				if scenario != "missing_created" {
					send(fmt.Sprintf(`{"type":"response.created","response":{"id":%q}}`, responseID))
				}
				count := 1
				if scenario == "late_merge_zero" && turn > 0 {
					count = 0
				}
				if replayScenario && turn > 0 {
					if scenario == "late_output_item_replay" {
						send(`{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"call_0","status":"completed"}}`)
					} else {
						send(`{"type":"response.web_search_call.completed","item_id":"call_0"}`)
					}
					if turn == 1 {
						count = 0
					}
				}
				if count > 0 {
					if scenario == "scoped_item_reuse" && turn == 2 {
						send(fmt.Sprintf(`{"type":"response.web_search_call.completed","response_id":%q,"item_id":"call_0"}`, responseID))
					} else {
						send(fmt.Sprintf(`{"type":"response.web_search_call.completed","item_id":"call_%d"}`, turn))
					}
				}
				if scenario != "missing_created" && !replayScenario && turn == 0 {
					send(`{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","status":"completed"}}`)
					count = 2
				}
				if scenario != "missing_created" && turn > 0 {
					send(`{"type":"response.completed","response":{"id":"resp_0","output":[{"type":"web_search_call","id":"call_0","status":"completed"}]}}`)
					assertUsage("resp_0", 0)
				}
				output := "[]"
				if scenario == "missing_created" {
					output = fmt.Sprintf(`[{"type":"web_search_call","id":"call_%d","status":"completed"},{"type":"web_search_call","id":"unfinished","status":"in_progress"}]`, turn)
				}
				terminal := fmt.Sprintf(`{"type":"response.completed","response":{"id":%q,"output":%s}}`, responseID, output)
				send(terminal)
				assertUsage(responseID, count)
				send(terminal)
				assertUsage(responseID, 0)
			}
			require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
			select {
			case <-serverErr:
			case <-time.After(3 * time.Second):
				t.Fatal("passthrough did not exit")
			}
		})
	}
}

func TestNativeWebSearchCounterIdentityReconciliation(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%v", reverse), func(t *testing.T) {
			counter := newOpenAINativeWebSearchCounter(&Account{Platform: PlatformOpenAI})
			frames := []string{
				`{"type":"response.web_search_call.completed","item_id":"ws_0"}`,
				`{"type":"response.output_item.done","output_index":1,"item":{"type":"web_search_call","status":"completed"}}`,
			}
			if reverse {
				frames[0], frames[1] = frames[1], frames[0]
			}
			for _, frame := range frames {
				counter.Add([]byte(frame))
			}
			terminal := `{"type":"response.completed","response":` + nativeSearchResponse(2) + `}`
			counter.Add([]byte(terminal))
			require.Equal(t, 2, counter.Count(), "terminal output must reconcile aliases and retain distinct calls")
			for _, frame := range append(frames, terminal) {
				counter.Add([]byte(frame))
			}
			require.Equal(t, 2, counter.Count(), "repeated completion frames must be idempotent")
		})
	}
}

func TestNativeWebSearchCounterResponseScopes(t *testing.T) {
	counter := newOpenAINativeWebSearchCounter(&Account{Platform: PlatformOpenAI})
	for turn := 0; turn < 3; turn++ {
		responseID := fmt.Sprintf("resp_%d", turn)
		counter.Add([]byte(fmt.Sprintf(`{"type":"response.created","response":{"id":%q}}`, responseID)))
		counter.Add([]byte(`{"type":"response.web_search_call.completed","item_id":"reused","output_index":0}`))
		counter.Add([]byte(fmt.Sprintf(`{"type":"response.done","response":{"id":%q,"output":[{"type":"web_search_call","id":"reused","status":"completed"}]}}`, responseID)))
		require.Equal(t, turn+1, counter.Count(), "IDs and indexes belong to a response, not the connection")
	}
	counter.Add([]byte(`{"type":"response.web_search_call.completed","response_id":"resp_0","item_id":"reused","output_index":0}`))
	require.Equal(t, 3, counter.Count(), "late frames must resolve to their original response")
	counter.Add([]byte(`{"type":"response.completed","response":{"id":"terminal_only","output":[{"type":"web_search_call","id":"reused","status":"completed"}]}}`))
	require.Equal(t, 4, counter.Count())
}

func TestNativeWebSearchCounterIdentityBeforeCompletion(t *testing.T) {
	counter := newOpenAINativeWebSearchCounter(&Account{Platform: PlatformOpenAI})
	counter.Add([]byte(`{"type":"response.output_item.added","output_index":0,"item":{"type":"web_search_call","id":"output_index:0","status":"in_progress"}}`))
	counter.Add([]byte(`{"type":"response.web_search_call.searching","item_id":"second","output_index":1}`))
	require.Zero(t, counter.Count())
	for _, frame := range []string{
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","status":"completed"}}`,
		`{"type":"response.web_search_call.completed","item_id":"output_index:0"}`,
		`{"type":"response.web_search_call.completed","item_id":"second"}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"web_search_call","status":"completed"}}`,
	} {
		counter.Add([]byte(frame))
	}
	require.Equal(t, 2, counter.Count(), "identity metadata must link ID-only and index-only completions")
}

func TestNativeWebSearchCounterIndexedCompletion(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		counter := newOpenAINativeWebSearchCounter(&Account{Platform: PlatformOpenAI})
		frames := []string{
			`{"type":"response.web_search_call.completed","item_id":"ws_0","output_index":0}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","status":"completed"}}`,
		}
		if reverse {
			frames[0], frames[1] = frames[1], frames[0]
		}
		for _, frame := range frames {
			counter.Add([]byte(frame))
			require.Equal(t, 1, counter.Count(), "indexed completion must reconcile without waiting for terminal output")
		}
	}
}

func TestNativeWebSearchCounterPartialTerminal(t *testing.T) {
	for _, terminal := range []string{"completed", "done", "failed", "incomplete", "cancelled", "canceled"} {
		t.Run(terminal, func(t *testing.T) {
			counter := newOpenAINativeWebSearchCounter(&Account{Platform: PlatformOpenAI})
			counter.Add([]byte(`{"type":"response.web_search_call.completed","item_id":"completed","output_index":0}`))
			counter.Add([]byte(fmt.Sprintf(`{"type":"response.%s","response":{"id":"partial","output":[{"type":"web_search_call","id":"completed","status":"completed"},{"type":"web_search_call","id":"unfinished","status":"in_progress"},{"type":"web_search_call","id":"failed","status":"failed"},{"type":"web_search_call","id":"missing_status"}]}}`, terminal)))
			require.Equal(t, 1, counter.Count(), "only completed calls are billable, including on partial responses")
		})
	}
}

func TestNativeWebSearchManagedWSPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, ingress := range []bool{false, true} {
		t.Run(fmt.Sprintf("ingress=%v", ingress), func(t *testing.T) {
			cfg := newOpenAIWSExecutionScopeTestConfig()
			var frames [][]byte
			forEachOpenAISSEFrame(nativeSearchSSE(2), func(eventType string, data []byte) {
				frames = append(frames, []byte(openAICompatPayloadWithEventType(string(data), eventType)))
			})
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: &openAIWSCaptureConn{events: frames}})
			defer pool.Close()
			svc := &OpenAIGatewayService{
				cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool,
			}
			account := passthroughLifecycleAccount()
			account.Extra = map[string]any{"responses_websockets_v2_enabled": true}
			if !ingress {
				result, err := svc.Forward(context.Background(), nativeSearchGinContext(), account, []byte(`{"model":"gpt-5.4","input":"search","stream":true}`))
				require.NoError(t, err)
				require.True(t, result.OpenAIWSMode)
				require.Equal(t, 2, result.NativeWebSearchCalls)
				return
			}
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			results := make(chan *OpenAIForwardResult, 1)
			server, serverErr := startPassthroughLifecycleServerWithHooks(t, ctx, svc, account, func(*gin.Context) *OpenAIWSIngressHooks {
				return &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) { results <- result }}
			})
			defer server.Close()
			client := dialPassthroughLifecycleClient(t, server)
			defer func() { _ = client.CloseNow() }()
			for range frames {
				_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.NoError(t, err)
			}
			select {
			case result := <-results:
				require.NotNil(t, result)
				require.Equal(t, 2, result.NativeWebSearchCalls)
			case <-time.After(3 * time.Second):
				t.Fatal("missing managed WS turn usage")
			}
			cancel(context.Canceled)
			require.NoError(t, client.CloseNow())
			select {
			case <-serverErr:
			case <-time.After(3 * time.Second):
				t.Fatal("managed WS did not exit")
			}
		})
	}
}
