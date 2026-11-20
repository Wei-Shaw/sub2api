//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func nativeSearchResponse(count int) string {
	items := []string{`{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}`}
	for index := 0; index < count; index++ {
		items = append(items, fmt.Sprintf(`{"type":"web_search_call","id":"ws_%d","status":"completed"}`, index))
	}
	return `{"id":"resp_search","object":"response","model":"gpt-5.4","status":"completed","output":[` + strings.Join(items, ",") + `],"usage":{"input_tokens":1000,"output_tokens":500}}`
}

func nativeSearchSSE(count int) string {
	var body strings.Builder
	for index := 0; index < count; index++ {
		itemDone := fmt.Sprintf("event: response.output_item.done\ndata: {\"output_index\":%d,\"item\":{\"type\":\"web_search_call\",\"status\":\"completed\"}}\n\n", index+1)
		completed := fmt.Sprintf("data: {\"type\":\"response.web_search_call.completed\",\"item_id\":\"ws_%d\"}\n\n", index)
		if index%2 == 0 {
			body.WriteString(completed + itemDone)
		} else {
			body.WriteString(itemDone + completed)
		}
	}
	body.WriteString("data: {\"type\":\"response.completed\",\"response\":" + nativeSearchResponse(count) + "}\n\n")
	return body.String()
}

func TestNativeWebSearchCounter(t *testing.T) {
	for _, count := range []int{0, 1, 3} {
		for _, body := range []string{nativeSearchResponse(count), nativeSearchSSE(count)} {
			for _, platform := range []string{PlatformOpenAI, PlatformGrok, PlatformAnthropic} {
				account := &Account{Platform: platform}
				want := 0
				if platform == PlatformOpenAI {
					want = count
				}
				require.Equal(t, want, countOpenAINativeWebSearchCalls(account, []byte(body)))
			}
		}
	}
	counter := newOpenAINativeWebSearchCounter(&Account{Platform: PlatformOpenAI})
	for _, payload := range []string{
		`{"type":"response.output_item.added","item":{"type":"web_search_call","id":"ws_added","status":"completed"}}`,
		`{"type":"response.web_search_call.in_progress","item_id":"ws_progress"}`,
		`{"type":"response.web_search_call.searching","item_id":"ws_searching"}`,
		`{"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_failed","status":"failed"}}`,
		`{"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_progress","status":"in_progress"}}`,
		`{"output":[{"type":"web_search_call","id":"ws_missing_status"},{"type":"x_search_call","id":"xs_1","status":"completed"},{"type":"tool_search_call","id":"ts_1","status":"completed"},{"type":"function_call","name":"web_search","call_id":"call_1","status":"completed"}]}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","name":"web_search","id":"ws_function","status":"completed"}}`,
	} {
		counter.Add([]byte(payload))
	}
	require.Zero(t, counter.Count())
	for repeat := 0; repeat < 3; repeat++ {
		counter.Add([]byte(`{"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_0","status":"completed"}}`))
		counter.Add([]byte(`{"type":"response.web_search_call.completed","item_id":"ws_0"}`))
		counter.Add([]byte(`{"type":"response.completed","response":` + nativeSearchResponse(2) + `}`))
	}
	require.Equal(t, 2, counter.Count())
	require.Zero(t, countOpenAINativeWebSearchCalls(nil, []byte(nativeSearchResponse(2))))
}

func TestNativeWebSearchForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, count := range []int{0, 1, 3} {
				for _, sse := range []bool{false, true} {
					if stream && !sse {
						continue
					}
					t.Run(fmt.Sprintf("passthrough=%v/stream=%v/sse=%v/count=%d", passthrough, stream, sse, count), func(t *testing.T) {
						body, contentType := nativeSearchResponse(count), "application/json"
						if sse {
							body, contentType = nativeSearchSSE(count), "text/event-stream"
						}
						upstream := &httpUpstreamRecorder{resp: nativeSearchHTTPResponse(body, contentType)}
						svc := &OpenAIGatewayService{cfg: passthroughArgsTestConfig(), httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
						account := passthroughArgsFallbackAccount()
						account.Extra = map[string]any{"openai_passthrough": passthrough}
						request := fmt.Sprintf(`{"model":"gpt-5.4","input":"search","stream":%v}`, stream)
						ctx := nativeSearchGinContext()
						result, err := svc.Forward(context.Background(), ctx, account, []byte(request))
						require.NoError(t, err)
						require.Equal(t, count, result.NativeWebSearchCalls)
						require.Zero(t, result.WebSearchCalls)
						require.Zero(t, result.SearchCount)
						require.Equal(t, 1000, result.Usage.InputTokens)
						require.Equal(t, 500, result.Usage.OutputTokens)
					})
				}
			}
		}
	}
}

func nativeSearchHTTPResponse(body, contentType string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}

func nativeSearchGinContext() *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return ctx
}

func TestNativeWebSearchCompatBridges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic} {
		for _, bridge := range []string{"chat", "messages", "http_ws"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%v", platform, bridge, stream), func(t *testing.T) {
					svc := &OpenAIGatewayService{cfg: passthroughArgsTestConfig()}
					resp := nativeSearchHTTPResponse(nativeSearchSSE(2), "text/event-stream")
					ctx := nativeSearchGinContext()
					account := &Account{ID: 1, Platform: platform, Type: AccountTypeAPIKey}
					var result *OpenAIForwardResult
					var err error
					if bridge == "http_ws" {
						svc.httpUpstream = &httpUpstreamRecorder{resp: resp}
						payload := []byte(`{"type":"response.create","model":"gpt-5.4","input":"search"}`)
						result, err = svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), ctx, account, "test", payload, len(payload), "gpt-5.4", "", "", "", "", 1, func([]byte) error { return nil })
					} else if bridge == "chat" && stream {
						result, err = svc.handleChatStreamingResponse(resp, ctx, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now(), 0)
					} else if bridge == "chat" {
						result, err = svc.handleChatBufferedStreamingResponse(resp, ctx, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
					} else if stream {
						result, err = svc.handleAnthropicStreamingResponse(resp, ctx, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
					} else {
						result, err = svc.handleAnthropicBufferedStreamingResponse(resp, ctx, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
					}
					require.NoError(t, err)
					want := 0
					if platform == PlatformOpenAI {
						want = 2
					}
					require.Equal(t, want, result.NativeWebSearchCalls)
					require.Equal(t, 1000, result.Usage.InputTokens)
				})
			}
		}
	}
}

func TestNativeWebSearchCost(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: newTestBillingService()}
	for _, count := range []int{0, 1, 3} {
		for _, price := range []*float64{nil, float64Ptr(0), float64Ptr(.025)} {
			for _, multiplier := range []float64{0, -1, 2} {
				apiKey := &APIKey{Group: &Group{WebSearchPricePerCall: price, SearchPricePer1k: float64Ptr(999)}}
				result := &OpenAIForwardResult{NativeWebSearchCalls: count}
				cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey, []string{"claude-sonnet-4"}, 3, 5, 7, multiplier, UsageTokens{InputTokens: 1000, OutputTokens: 500}, "", boolPtr(false), false, time.Time{})
				require.NoError(t, err)
				unitPrice := .01
				if price != nil {
					unitPrice = *price
				}
				require.InDelta(t, .0105+float64(count)*unitPrice, cost.TotalCost, 1e-9)
				require.InDelta(t, .0315+float64(count)*unitPrice*math.Max(multiplier, 0), cost.ActualCost, 1e-9)
				require.InDelta(t, .003, cost.InputCost, 1e-9)
				require.InDelta(t, .0075, cost.OutputCost, 1e-9)
				_, err = svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey, []string{"unknown-unpriced-model"}, 3, 5, 7, multiplier, UsageTokens{InputTokens: 1000}, "", boolPtr(false), false, time.Time{})
				require.Error(t, err)
			}
		}
	}
}

func TestNativeWebSearchWSPassthroughTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	upstream := newStagedPassthroughConn()
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	results := make(chan *OpenAIForwardResult, 6)
	server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, passthroughLifecycleAccount(), func(*gin.Context) *OpenAIWSIngressHooks {
		return &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) {
			results <- result
		}}
	})
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = client.CloseNow() }()
	for turn, count := range []int{2, 0, 1} {
		if turn > 0 {
			writeCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			err := client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","input":"search"}`))
			stop()
			require.NoError(t, err)
		}
		requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
		payload := strings.ReplaceAll(nativeSearchResponse(count), "resp_search", fmt.Sprintf("resp_turn_%d", turn))
		done := `{"type":"response.completed","response":` + payload + `}`
		upstream.Send(fmt.Sprintf(`{"type":"response.created","response":{"id":"resp_turn_%d"}}`, turn))
		frames := 5
		if count > 0 {
			upstream.Send(`{"type":"response.web_search_call.completed","item_id":"ws_0"}`)
			frames++
		}
		itemDone := fmt.Sprintf(`{"type":"response.output_item.done","output_index":1,"item":{"type":"web_search_call","status":"%s"}}`, map[bool]string{true: "completed", false: "in_progress"}[count > 0])
		upstream.Send(itemDone)
		upstream.Send(done)
		upstream.Send(itemDone)
		upstream.Send(done)
		for frame := 0; frame < frames; frame++ {
			_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
			require.NoError(t, err)
		}
		for _, expectedCount := range []int{count, 0} {
			select {
			case result := <-results:
				require.Equal(t, expectedCount, result.NativeWebSearchCalls)
				require.Equal(t, 1000, result.Usage.InputTokens)
			case <-time.After(3 * time.Second):
				t.Fatal("missing turn usage")
			}
		}
	}
	require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough did not exit")
	}
}

func TestNativeWebSearchCostPreservesMediaAndAlpha(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: newTestBillingService()}
	apiKey := &APIKey{Group: &Group{ImageCountBilling: true, ImagePrice1K: float64Ptr(.05)}}
	for _, result := range []*OpenAIForwardResult{
		{ImageCount: 2, ImageEndpoint: true, ImageSize: "1K"},
		{VideoCount: 1, VideoResolution: "720p", VideoDurationSeconds: 5},
		{WebSearchCalls: 1},
	} {
		calculate := func() *CostBreakdown {
			cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey, []string{"claude-sonnet-4"}, 3, 5, 7, 2, UsageTokens{InputTokens: 1000}, "", boolPtr(false), false, time.Time{})
			require.NoError(t, err)
			return cost
		}
		baseline := calculate()
		result.NativeWebSearchCalls = 2
		cost := calculate()
		surcharge := .02
		if result.WebSearchCalls > 0 {
			surcharge = 0
		}
		require.Equal(t, baseline.BillingMode, cost.BillingMode)
		require.InDelta(t, baseline.TotalCost+surcharge, cost.TotalCost, 1e-9)
		require.InDelta(t, baseline.ActualCost+surcharge*2, cost.ActualCost, 1e-9)
		apiKey.Group.WebSearchPricePerCall = float64Ptr(0)
		freeSearchCost := calculate()
		if result.WebSearchCalls == 0 {
			require.Equal(t, baseline, freeSearchCost)
		} else {
			require.Zero(t, freeSearchCost.TotalCost)
			require.Zero(t, freeSearchCost.ActualCost)
		}
		apiKey.Group.WebSearchPricePerCall = nil
	}
	apiKey.Group.ImagePrice1K = float64Ptr(0)
	apiKey.Group.WebSearchPricePerCall = float64Ptr(0)
	cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{ImageEndpoint: true, ImageCount: 1, ImageSize: "1K", NativeWebSearchCalls: 2}, apiKey, []string{"gpt-image-1"}, 3, 5, 7, 2, UsageTokens{}, "", boolPtr(false), false, time.Time{})
	require.NoError(t, err)
	require.Zero(t, cost.TotalCost)
	require.Zero(t, cost.ActualCost)
}

func TestNativeWebSearchOAuthAndNonOpenAIHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{toolCorrector: NewCodexToolCorrector()}
	for _, platform := range []string{PlatformOpenAI, PlatformGrok, PlatformAnthropic} {
		account := &Account{ID: 1, Platform: platform, Type: AccountTypeOAuth}
		want := 0
		if platform == PlatformOpenAI {
			want = 2
		}
		streamResult, err := svc.handleStreamingResponse(context.Background(), nativeSearchHTTPResponse(nativeSearchSSE(2), "text/event-stream"), nativeSearchGinContext(), account, time.Now(), "gpt-5.4", "gpt-5.4")
		require.NoError(t, err)
		require.Equal(t, want, streamResult.nativeSearches)
		jsonResult, err := svc.handleNonStreamingResponse(context.Background(), nativeSearchHTTPResponse(nativeSearchResponse(2), "application/json"), nativeSearchGinContext(), account, "gpt-5.4", "gpt-5.4")
		require.NoError(t, err)
		require.Equal(t, want, jsonResult.nativeSearches)
	}
}

func TestNativeWebSearchFreeTokenPricing(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: newTestBillingService()}
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	groupID := int64(77)
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{
		ID: groupID, Platform: PlatformOpenAI, Hydrated: true,
		ModelPricing: []ChannelModelPricing{{Models: []string{"gpt-5.4"}, BillingMode: BillingModeToken, InputPrice: float64Ptr(0), OutputPrice: float64Ptr(0)}},
	}}
	for _, count := range []int{0, 2} {
		cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{NativeWebSearchCalls: count}, apiKey, []string{"gpt-5.4"}, 3, 5, 7, 2, UsageTokens{InputTokens: 1000, OutputTokens: 500}, "", boolPtr(false), false, time.Time{})
		require.NoError(t, err)
		require.InDelta(t, .01*float64(count), cost.TotalCost, 1e-9)
		require.InDelta(t, .02*float64(count), cost.ActualCost, 1e-9)
		require.Zero(t, cost.InputCost)
		require.Zero(t, cost.OutputCost)
	}
}
