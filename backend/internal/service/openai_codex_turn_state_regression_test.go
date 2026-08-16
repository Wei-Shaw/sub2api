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

func TestGuardOpenAICodexTurnStateEcho_BindsExactOpaqueValue(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 17, "sess-value-bound")

	svc.relayOpenAICodexTurnState(c, newTurnStateOAuthAccount(101), http.Header{
		"X-Codex-Turn-State": []string{"state-A"},
	})
	svc.relayOpenAICodexTurnState(c, newTurnStateOAuthAccount(202), http.Header{
		"X-Codex-Turn-State": []string{"state-B"},
	})

	oldA := http.Header{"X-Codex-Turn-State": []string{"state-A"}}
	svc.guardOpenAICodexTurnStateEcho(c, newTurnStateOAuthAccount(202), oldA)
	require.Empty(t, oldA.Get(openAICodexTurnStateHeader), "a later state must not overwrite an older state's source account")

	ownB := http.Header{"X-Codex-Turn-State": []string{"state-B"}}
	svc.guardOpenAICodexTurnStateEcho(c, newTurnStateOAuthAccount(202), ownB)
	require.Equal(t, "state-B", ownB.Get(openAICodexTurnStateHeader))
}

func TestOpenAICodexTurnState_SameOpaqueValueTracksLatestCommittedAccount(t *testing.T) {
	const state = "same-opaque-state"

	for _, tt := range []struct {
		name   string
		commit func(*OpenAIGatewayService, *gin.Context, *Account)
	}{
		{
			name: "direct response commit",
			commit: func(svc *OpenAIGatewayService, c *gin.Context, account *Account) {
				svc.relayOpenAICodexTurnState(c, account, http.Header{
					"X-Codex-Turn-State": []string{state},
				})
			},
		},
		{
			name: "staged response commit",
			commit: func(svc *OpenAIGatewayService, c *gin.Context, account *Account) {
				staged := http.Header{"X-Codex-Turn-State": []string{state}}
				svc.noteStagedOpenAICodexTurnStateCommitted(c, account, staged)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{}
			c, _ := newTurnStateTestContext(t, 117, "sess-same-state")
			accountA := newTurnStateOAuthAccount(1001)
			accountB := newTurnStateOAuthAccount(1002)

			tt.commit(svc, c, accountA)
			tt.commit(svc, c, accountB)

			fromA := http.Header{"X-Codex-Turn-State": []string{state}}
			svc.guardOpenAICodexTurnStateEcho(c, accountA, fromA)
			require.Empty(t, fromA.Get(openAICodexTurnStateHeader))

			fromB := http.Header{"X-Codex-Turn-State": []string{state}}
			svc.guardOpenAICodexTurnStateEcho(c, accountB, fromB)
			require.Equal(t, state, fromB.Get(openAICodexTurnStateHeader))
		})
	}
}

func TestOpenAICodexTurnState_PreservesOpaqueValueExactly(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 23, "sess-opaque")
	const state = " opaque-state "

	svc.relayOpenAICodexTurnState(c, newTurnStateOAuthAccount(701), http.Header{
		"X-Codex-Turn-State": []string{state},
	})

	require.Equal(t, state, c.Writer.Header().Get(openAICodexTurnStateHeader))
	require.NotEqual(t,
		openAICodexTurnStateBindingKey(c, state),
		openAICodexTurnStateBindingKey(c, "opaque-state"),
	)
	h := http.Header{"X-Codex-Turn-State": []string{state}}
	svc.guardOpenAICodexTurnStateEcho(c, newTurnStateOAuthAccount(702), h)
	require.Empty(t, h.Get(openAICodexTurnStateHeader))
}

func TestGuardOpenAICodexTurnStateEcho_APIKeyIsTransparent(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 18, "sess-api-key")
	apiAccount := &Account{ID: 301, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	svc.relayOpenAICodexTurnState(c, apiAccount, http.Header{
		"X-Codex-Turn-State": []string{"api-state"},
	})

	h := http.Header{"X-Codex-Turn-State": []string{"api-state"}}
	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 302, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, h)
	require.Equal(t, "api-state", h.Get(openAICodexTurnStateHeader))
	_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "api-state"))
	require.False(t, tracked)
}

func TestGuardOpenAICodexTurnStateEcho_LoadsCrossInstanceBinding(t *testing.T) {
	sharedCache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	writer := &OpenAIGatewayService{cache: sharedCache}
	reader := &OpenAIGatewayService{cache: sharedCache}
	c, _ := newTurnStateTestContext(t, 19, "sess-cross-instance")
	writer.relayOpenAICodexTurnState(c, newTurnStateOAuthAccount(401), http.Header{
		"X-Codex-Turn-State": []string{"shared-state"},
	})

	h := http.Header{"X-Codex-Turn-State": []string{"shared-state"}}
	reader.guardOpenAICodexTurnStateEcho(c, newTurnStateOAuthAccount(402), h)
	require.Empty(t, h.Get(openAICodexTurnStateHeader))
}

func TestGuardOpenAICodexTurnStateEcho_IsolatesAPIKeys(t *testing.T) {
	svc := &OpenAIGatewayService{}
	writerContext, _ := newTurnStateTestContext(t, 71, "shared-session")
	svc.relayOpenAICodexTurnState(writerContext, newTurnStateOAuthAccount(411), http.Header{
		"X-Codex-Turn-State": []string{"shared-state"},
	})

	otherAPIKeyContext, _ := newTurnStateTestContext(t, 72, "shared-session")
	h := http.Header{"X-Codex-Turn-State": []string{"shared-state"}}
	svc.guardOpenAICodexTurnStateEcho(otherAPIKeyContext, newTurnStateOAuthAccount(412), h)
	require.Equal(t, "shared-state", h.Get(openAICodexTurnStateHeader),
		"a different downstream API key must not inherit another key's provenance")
}

func TestOpenAICodexTurnState_MissingAPIKeyDisablesTracking(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 0, "unscoped-session")
	svc.relayOpenAICodexTurnState(c, newTurnStateOAuthAccount(421), http.Header{
		"X-Codex-Turn-State": []string{"unscoped-state"},
	})

	require.Empty(t, openAICodexTurnStateBindingKey(c, "unscoped-state"))
	tracked := false
	svc.openaiCodexTurnStateOrigins.Range(func(_, _ any) bool {
		tracked = true
		return false
	})
	require.False(t, tracked)
}

func TestRelayOpenAICodexTurnState_PersistsAfterRequestCancellation(t *testing.T) {
	sharedCache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc := &OpenAIGatewayService{cache: sharedCache}
	c, _ := newTurnStateTestContext(t, 20, "sess-cancelled")
	requestCtx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestCtx)
	cancel()

	svc.relayOpenAICodexTurnState(c, newTurnStateOAuthAccount(501), http.Header{
		"X-Codex-Turn-State": []string{"committed-state"},
	})

	require.Equal(t, int64(501), sharedCache.sessionBindings[openAICodexTurnStateBindingKey(c, "committed-state")])
}

func TestHandleStreamingResponsePassthrough_RecordsTurnStateOnlyAfterSuccessfulWrite(t *testing.T) {
	newResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":       []string{"text/event-stream"},
				"X-Codex-Turn-State": []string{"stream-state"},
			},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{}}}\n\n",
			)),
		}
	}
	account := newTurnStateOAuthAccount(601)

	t.Run("successful downstream write records provenance", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 21, "sess-stream-success")
		_, err := svc.handleStreamingResponsePassthrough(context.Background(), newResponse(), c, account, time.Now(), "gpt-5.4", "gpt-5.4")
		require.NoError(t, err)
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "stream-state"))
		require.True(t, tracked)
	})

	t.Run("failed first downstream write leaves provenance unknown", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 22, "sess-stream-failed")
		c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}
		_, _ = svc.handleStreamingResponsePassthrough(context.Background(), newResponse(), c, account, time.Now(), "gpt-5.4", "gpt-5.4")
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "stream-state"))
		require.False(t, tracked)
	})
}

func TestNonStreamingPassthrough_RecordsTurnStateAcrossResponseShapes(t *testing.T) {
	account := newTurnStateOAuthAccount(801)
	for _, tt := range []struct {
		name          string
		contentType   string
		body          string
		compactStream bool
	}{
		{"json", "application/json", `{"id":"resp_json","status":"completed","output":[],"usage":{}}`, false},
		{"sse converted to json", "text/event-stream", "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_sse\",\"status\":\"completed\",\"output\":[],\"usage\":{}}}\n\ndata: [DONE]\n\n", false},
		{"compact sse bridge", "application/json", `{"id":"resp_compact","status":"completed","output":[{"type":"compaction","id":"cmp_1"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`, true},
	} {
		t.Run(tt.name+" succeeds", func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			c, _ := newTurnStateTestContext(t, 24, "sess-non-stream-"+tt.name)
			if tt.compactStream {
				MarkOpenAICompactClientStream(c)
			}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
				"Content-Type": []string{tt.contentType}, "X-Codex-Turn-State": []string{"non-stream-state"},
			}, Body: io.NopCloser(strings.NewReader(tt.body))}
			_, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4")
			require.NoError(t, err)
			_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "non-stream-state"))
			require.True(t, tracked)
		})

		t.Run(tt.name+" write failure leaves provenance unknown", func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			c, _ := newTurnStateTestContext(t, 124, "sess-non-stream-failed-"+tt.name)
			if tt.compactStream {
				MarkOpenAICompactClientStream(c)
			}
			c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
				"Content-Type": []string{tt.contentType}, "X-Codex-Turn-State": []string{"non-stream-failed-state"},
			}, Body: io.NopCloser(strings.NewReader(tt.body))}
			_, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4")
			require.Error(t, err)
			_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "non-stream-failed-state"))
			require.False(t, tracked)
		})
	}
}

func TestHandleStreamingResponse_RecordsTurnStateOnlyAfterSuccessfulWrite(t *testing.T) {
	newResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"}, "X-Codex-Turn-State": []string{"native-stream-state"},
			},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_native_stream\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n",
			)),
		}
	}
	account := newTurnStateOAuthAccount(901)

	t.Run("successful first write records provenance", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 25, "sess-native-stream-success")
		_, err := svc.handleStreamingResponse(context.Background(), newResponse(), c, account, time.Now(), "gpt-5.4", "gpt-5.4")
		require.NoError(t, err)
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "native-stream-state"))
		require.True(t, tracked)
	})

	t.Run("failed first write leaves provenance unknown", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 26, "sess-native-stream-failed")
		c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}
		_, _ = svc.handleStreamingResponse(context.Background(), newResponse(), c, account, time.Now(), "gpt-5.4", "gpt-5.4")
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "native-stream-state"))
		require.False(t, tracked)
	})
}

func TestHandleNonStreamingResponse_RecordsTurnStateAfterCommittedResponse(t *testing.T) {
	account := newTurnStateOAuthAccount(902)
	for _, tt := range []struct {
		name          string
		contentType   string
		body          string
		compactStream bool
	}{
		{"json", "application/json", `{"id":"resp_native_json","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`, false},
		{"sse converted to json", "text/event-stream", "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_native_sse\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\ndata: [DONE]\n\n", false},
		{"compact sse bridge", "application/json", `{"id":"resp_native_compact","status":"completed","output":[{"type":"compaction","id":"cmp_native"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`, true},
	} {
		t.Run(tt.name+" succeeds", func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			c, _ := newTurnStateTestContext(t, 27, "sess-native-non-stream-"+tt.name)
			if tt.compactStream {
				MarkOpenAICompactClientStream(c)
			}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
				"Content-Type": []string{tt.contentType}, "X-Codex-Turn-State": []string{"native-non-stream-state"},
			}, Body: io.NopCloser(strings.NewReader(tt.body))}
			_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4")
			require.NoError(t, err)
			_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "native-non-stream-state"))
			require.True(t, tracked)
		})

		t.Run(tt.name+" write failure leaves provenance unknown", func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			c, _ := newTurnStateTestContext(t, 127, "sess-native-non-stream-failed-"+tt.name)
			if tt.compactStream {
				MarkOpenAICompactClientStream(c)
			}
			c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
				"Content-Type": []string{tt.contentType}, "X-Codex-Turn-State": []string{"native-non-stream-failed-state"},
			}, Body: io.NopCloser(strings.NewReader(tt.body))}
			_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4")
			require.Error(t, err)
			_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "native-non-stream-failed-state"))
			require.False(t, tracked)
		})
	}

	t.Run("parse failure before commit leaves provenance unknown", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{}}
		c, _ := newTurnStateTestContext(t, 28, "sess-native-non-stream-invalid")
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
			"Content-Type": []string{"application/json"}, "X-Codex-Turn-State": []string{"invalid-response-state"},
		}, Body: io.NopCloser(strings.NewReader("not-json"))}
		_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4")
		require.Error(t, err)
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "invalid-response-state"))
		require.False(t, tracked)
	})
}

func TestCommittedCompactKeepalive_DoesNotRecordUnrelayableTurnState(t *testing.T) {
	account := newTurnStateOAuthAccount(903)
	newResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":       []string{"application/json"},
				"X-Codex-Turn-State": []string{"late-turn-state"},
			},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"resp_compact","status":"completed","output":[{"type":"compaction","id":"cmp_1"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
			)),
		}
	}
	newStreamingResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":       []string{"text/event-stream"},
				"X-Codex-Turn-State": []string{"late-stream-turn-state"},
			},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\",\"encrypted_content\":\"x\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream\",\"usage\":{}}}\n\n",
			)),
		}
	}

	t.Run("native response", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{}}
		c, recorder := newTurnStateTestContext(t, 29, "sess-keepalive-native")
		MarkOpenAICompactClientStream(c)
		stop := StartOpenAICompactSSEKeepalive(c, time.Millisecond)
		t.Cleanup(stop)
		require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)

		_, err := svc.handleNonStreamingResponse(context.Background(), newResponse(), c, account, "gpt-5.4", "gpt-5.4")
		require.NoError(t, err)
		require.Empty(t, recorder.Result().Header.Get(openAICodexTurnStateHeader))
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "late-turn-state"))
		require.False(t, tracked)
	})

	t.Run("passthrough response", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{}}
		c, recorder := newTurnStateTestContext(t, 30, "sess-keepalive-passthrough")
		MarkOpenAICompactClientStream(c)
		stop := StartOpenAICompactSSEKeepalive(c, time.Millisecond)
		t.Cleanup(stop)
		require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)

		_, err := svc.handleNonStreamingResponsePassthrough(context.Background(), newResponse(), c, account, "gpt-5.4", "gpt-5.4")
		require.NoError(t, err)
		require.Empty(t, recorder.Result().Header.Get(openAICodexTurnStateHeader))
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "late-turn-state"))
		require.False(t, tracked)
	})

	t.Run("native streaming response", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{}}
		c, recorder := newTurnStateTestContext(t, 31, "sess-keepalive-native-stream")
		MarkOpenAICompactClientStream(c)
		stop := StartOpenAICompactSSEKeepalive(c, time.Millisecond)
		t.Cleanup(stop)
		require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)

		_, err := svc.handleStreamingResponse(context.Background(), newStreamingResponse(), c, account, time.Now(), "gpt-5.4", "gpt-5.4")
		require.NoError(t, err)
		require.Empty(t, recorder.Result().Header.Get(openAICodexTurnStateHeader))
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "late-stream-turn-state"))
		require.False(t, tracked)
	})

	t.Run("passthrough streaming response", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{}}
		c, recorder := newTurnStateTestContext(t, 32, "sess-keepalive-passthrough-stream")
		MarkOpenAICompactClientStream(c)
		stop := StartOpenAICompactSSEKeepalive(c, time.Millisecond)
		t.Cleanup(stop)
		require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)

		_, err := svc.handleStreamingResponsePassthrough(context.Background(), newStreamingResponse(), c, account, time.Now(), "gpt-5.4", "gpt-5.4")
		require.NoError(t, err)
		require.Empty(t, recorder.Result().Header.Get(openAICodexTurnStateHeader))
		_, tracked := svc.openaiCodexTurnStateOrigins.Load(openAICodexTurnStateBindingKey(c, "late-stream-turn-state"))
		require.False(t, tracked)
	})
}

func TestAPIKeyResponsesPreserveTurnStateWithoutCreatingProvenance(t *testing.T) {
	const state = "api-key-opaque-state"
	account := &Account{ID: 9901, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, tt := range []struct {
		name   string
		invoke func(*OpenAIGatewayService, *gin.Context) error
	}{
		{
			name: "native streaming",
			invoke: func(svc *OpenAIGatewayService, c *gin.Context) error {
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
					"Content-Type": []string{"text/event-stream"}, "X-Codex-Turn-State": []string{state},
				}, Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
						"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_api_native_stream\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n",
				))}
				_, err := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5.4", "gpt-5.4")
				return err
			},
		},
		{
			name: "passthrough streaming",
			invoke: func(svc *OpenAIGatewayService, c *gin.Context) error {
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
					"Content-Type": []string{"text/event-stream"}, "X-Codex-Turn-State": []string{state},
				}, Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
						"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_api_passthrough_stream\",\"usage\":{}}}\n\n",
				))}
				_, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "gpt-5.4", "gpt-5.4")
				return err
			},
		},
		{
			name: "native non-streaming",
			invoke: func(svc *OpenAIGatewayService, c *gin.Context) error {
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
					"Content-Type": []string{"application/json"}, "X-Codex-Turn-State": []string{state},
				}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_api_native","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))}
				_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4")
				return err
			},
		},
		{
			name: "passthrough non-streaming",
			invoke: func(svc *OpenAIGatewayService, c *gin.Context) error {
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
					"Content-Type": []string{"application/json"}, "X-Codex-Turn-State": []string{state},
				}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_api_passthrough","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))}
				_, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4")
				return err
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			c, recorder := newTurnStateTestContext(t, 991, "sess-api-key-e2e")
			require.NoError(t, tt.invoke(svc, c))
			require.Equal(t, state, recorder.Result().Header.Get(openAICodexTurnStateHeader))

			tracked := false
			svc.openaiCodexTurnStateOrigins.Range(func(_, _ any) bool {
				tracked = true
				return false
			})
			require.False(t, tracked, "API-key traffic must remain transparent and must not create OAuth provenance")
		})
	}
}
