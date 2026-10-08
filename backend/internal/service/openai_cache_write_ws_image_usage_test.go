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
)

func TestOpenAICacheWriteWSImageUsagePreservesPresenceAndSkipsTextInference(t *testing.T) {
	// Exercise the actual passthrough adapter, then bill its per-turn results.
	// The text-only control verifies that inference is enabled and otherwise
	// eligible, so losing image counters cannot silently pass the exclusion check.
	cases := []struct {
		name    string
		payload string
		want    OpenAIUsage
	}{
		{
			name:    "input image without cache write",
			payload: `{"type":"response.completed","response":{"id":"resp_input_missing","usage":{"input_tokens":2000,"output_tokens":500,"input_tokens_details":{"cached_tokens":1000,"image_tokens":40}}}}`,
			want:    OpenAIUsage{InputTokens: 2000, OutputTokens: 500, CacheReadInputTokens: 1000, ImageInputTokens: 40},
		},
		{
			name:    "input image explicit zero",
			payload: `{"type":"response.completed","response":{"id":"resp_input_zero","usage":{"input_tokens":2000,"output_tokens":500,"input_tokens_details":{"cached_tokens":1000,"image_tokens":40,"cache_write_tokens":0}},"tool_usage":{"image_gen":{"input_tokens_details":{"image_tokens":900}}}}}`,
			want:    OpenAIUsage{InputTokens: 2000, OutputTokens: 500, CacheReadInputTokens: 1000, ImageInputTokens: 40, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "prompt image explicit null",
			payload: `{"type":"response.completed","response":{"id":"resp_prompt_null","usage":{"prompt_tokens":2000,"completion_tokens":500,"prompt_tokens_details":{"cached_tokens":1000,"image_tokens":7,"cache_write_tokens":null}}}}`,
			want:    OpenAIUsage{InputTokens: 2000, OutputTokens: 500, CacheReadInputTokens: 1000, ImageInputTokens: 7, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "hosted image without cache write",
			payload: `{"type":"response.completed","response":{"id":"resp_hosted_missing","usage":{"input_tokens":2000,"output_tokens":500,"input_tokens_details":{"cached_tokens":1000}},"tool_usage":{"image_gen":{"input_tokens_details":{"image_tokens":80},"output_tokens_details":{"image_tokens":55}}}}}`,
			want:    OpenAIUsage{InputTokens: 2000, OutputTokens: 500, CacheReadInputTokens: 1000, ImageInputTokens: 80, ImageOutputTokens: 55},
		},
		{
			name:    "text explicit zero",
			payload: `{"type":"response.completed","response":{"id":"resp_text_zero","usage":{"input_tokens":2000,"output_tokens":500,"cache_creation_input_tokens":0,"input_tokens_details":{"cached_tokens":1000}}}}`,
			want:    OpenAIUsage{InputTokens: 2000, OutputTokens: 500, CacheReadInputTokens: 1000, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "text explicit null",
			payload: `{"type":"response.completed","response":{"id":"resp_text_null","usage":{"input_tokens":2000,"output_tokens":500,"cache_write_tokens":null,"input_tokens_details":{"cached_tokens":1000}}}}`,
			want:    OpenAIUsage{InputTokens: 2000, OutputTokens: 500, CacheReadInputTokens: 1000, CacheCreationInputTokensPresent: true},
		},
		{
			name:    "text missing cache write positive control",
			payload: `{"type":"response.completed","response":{"id":"resp_text_missing","usage":{"input_tokens":2000,"output_tokens":500,"input_tokens_details":{"cached_tokens":1000}}}}`,
			want:    OpenAIUsage{InputTokens: 2000, OutputTokens: 500, CacheReadInputTokens: 1000},
		},
	}
	events := make([][]byte, 0, len(cases))
	for _, tc := range cases {
		events = append(events, []byte(tc.payload))
	}
	results := captureCacheWriteWSImageUsageTurns(t, events)
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := results[i]
			require.Equal(t, tc.want, result.Usage, "adapter must preserve this turn's images and presence without inheriting earlier usage")
			require.True(t, result.OpenAIWSMode)
			require.Equal(t, "gpt-6-astra", result.Model)

			fixture := newCacheWriteSimulationFixture(t)
			fixture.input.Result = result
			require.NoError(t, fixture.svc.RecordUsage(context.Background(), fixture.input))
			require.Len(t, fixture.billingRepo.cmds, 1)
			require.NotNil(t, fixture.originalLog())
			require.Equal(t, tc.want.ImageInputTokens, fixture.originalLog().ImageInputTokens)
			require.Equal(t, tc.want.ImageOutputTokens, fixture.originalLog().ImageOutputTokens)

			eligible := tc.want.ImageInputTokens == 0 && tc.want.ImageOutputTokens == 0 && !tc.want.CacheCreationInputTokensPresent
			snapshot := fixture.svc.openaiCacheWriteInferenceTracker.lookupBillingSnapshot(fixture.input.CacheWriteObservationID)
			if eligible {
				require.NotNil(t, snapshot, "text-only missing cache-write control must remain eligible")
			} else {
				require.Nil(t, snapshot, "media and authoritative zero/null must not enter text cache-write reconciliation")
			}
			originalCost := fixture.originalLog().ActualCost
			fixture.reconcile900Tokens()
			if eligible {
				require.Len(t, fixture.billingRepo.cmds, 2)
				require.Len(t, fixture.usageRepo.corrections, 1)
				require.Equal(t, 900, fixture.originalLog().CacheCreationTokens)
			} else {
				require.Len(t, fixture.billingRepo.cmds, 1, "image usage and explicit cache-write fields must not trigger a later text delta bill")
				require.Empty(t, fixture.usageRepo.corrections)
				require.Zero(t, fixture.originalLog().CacheCreationTokens)
				require.Equal(t, originalCost, fixture.originalLog().ActualCost)
			}
		})
	}
}

func captureCacheWriteWSImageUsageTurns(t *testing.T, events [][]byte) []*OpenAIForwardResult {
	t.Helper()
	gin.SetMode(gin.TestMode)
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
		openaiWSPassthroughDialer: &openAIWSCaptureDialer{conn: &openAIWSCaptureConn{events: events}},
	}
	account := &Account{
		ID: 454, Name: "openai-ws-cache-write-image-usage", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough},
	}
	resultCh := make(chan *OpenAIForwardResult, len(events))
	hooks := &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, turnErr error) {
		if turnErr == nil && result != nil {
			resultCh <- result
		}
	}}
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	clientConn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()
	require.NoError(t, clientConn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-6-astra","input":"describe"}`)))
	for range events {
		_, _, err := clientConn.Read(ctx)
		require.NoError(t, err)
	}
	_ = clientConn.Close(coderws.StatusNormalClosure, "done")
	results := make([]*OpenAIForwardResult, 0, len(events))
	for range events {
		select {
		case result := <-resultCh:
			results = append(results, result)
		case <-ctx.Done():
			t.Fatal("passthrough cache-write/image turn result was not reported")
		}
	}
	return results
}
