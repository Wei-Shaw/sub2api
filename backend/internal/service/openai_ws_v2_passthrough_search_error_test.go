package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPassthroughNativeSearchAbnormalClose(t *testing.T) {
	for _, settledTurn := range []bool{false, true} {
		for _, completedSearch := range []bool{false, true} {
			t.Run(fmt.Sprintf("settled=%v/search=%v", settledTurn, completedSearch), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				controlCtx, cancel := context.WithCancelCause(context.Background())
				defer cancel(context.Canceled)
				upstream := newStagedPassthroughConn()
				svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
				type callback struct {
					turn   int
					result *OpenAIForwardResult
					err    error
				}
				callbacks := make(chan callback, 3)
				server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, passthroughLifecycleAccount(), func(*gin.Context) *OpenAIWSIngressHooks {
					return &OpenAIWSIngressHooks{AfterTurn: func(turn int, result *OpenAIForwardResult, err error) {
						callbacks <- callback{turn, result, err}
					}}
				})
				defer server.Close()
				client := dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"gpt-5.4","stream":true}`)
				defer func() { _ = client.CloseNow() }()
				send := func(payload string) {
					t.Helper()
					upstream.Send(payload)
					_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
					require.NoError(t, err)
				}
				takeCallback := func() callback {
					t.Helper()
					select {
					case got := <-callbacks:
						return got
					case <-time.After(3 * time.Second):
						t.Fatal("missing AfterTurn callback")
						return callback{}
					}
				}
				requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
				wantTurn := 1
				if settledTurn {
					send(`{"type":"response.created","response":{"id":"resp_settled"}}`)
					send(`{"type":"response.web_search_call.completed","item_id":"ws_settled"}`)
					send(`{"type":"response.completed","response":{"id":"resp_settled","usage":{"input_tokens":123,"output_tokens":45}}}`)
					got := takeCallback()
					require.NoError(t, got.err)
					require.Equal(t, 1, got.turn)
					require.Equal(t, 1, got.result.NativeWebSearchCalls)
					require.Equal(t, 123, got.result.Usage.InputTokens)
					writeCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
					err := client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","input":"search again"}`))
					stop()
					require.NoError(t, err)
					requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
					wantTurn++
				}
				send(`{"type":"response.created","response":{"id":"resp_unsettled"}}`)
				if completedSearch {
					send(`{"type":"response.web_search_call.completed","item_id":"ws_unsettled"}`)
				} else {
					send(`{"type":"response.web_search_call.searching","item_id":"ws_unsettled"}`)
				}
				upstreamErr := errors.New("upstream abnormal close without terminal")
				upstream.Fail(upstreamErr)
				_, readErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.Error(t, readErr)
				got := takeCallback()
				require.Equal(t, wantTurn, got.turn)
				require.ErrorIs(t, got.err, upstreamErr)
				var turnErr *openAIWSIngressTurnError
				require.ErrorAs(t, got.err, &turnErr)
				require.Equal(t, "read_upstream", turnErr.stage)
				require.True(t, turnErr.wroteDownstream)
				if completedSearch {
					require.NotNil(t, got.result)
					require.Equal(t, 1, got.result.NativeWebSearchCalls)
					require.Equal(t, OpenAIUsage{}, got.result.Usage)
					require.Empty(t, got.result.RequestID)
					require.Empty(t, got.result.UpstreamTerminalEvent)
					require.Equal(t, "gpt-5.4", got.result.Model)
					require.True(t, got.result.OpenAIWSMode)
				} else {
					require.Nil(t, got.result)
				}
				select {
				case err := <-serverErr:
					require.Same(t, got.err, err)
				case <-time.After(3 * time.Second):
					t.Fatal("passthrough did not exit")
				}
				require.Empty(t, callbacks)
			})
		}
	}
}
