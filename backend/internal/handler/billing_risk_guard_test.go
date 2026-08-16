//go:build unit

package handler

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBillingRiskMappedModelAndTextEstimateUseFinalRequestShape(t *testing.T) {
	mapping := service.ChannelMappingResult{Mapped: true, MappedModel: "claude-opus-4.5"}
	mappedModel, mappedUncertain := billingRiskAdmissionModel(mapping, "claude-sonnet-4")
	require.Equal(t, "claude-opus-4.5", mappedModel)
	require.False(t, mappedUncertain)
	requestedModel, requestedUncertain := billingRiskAdmissionModel(service.ChannelMappingResult{}, "claude-sonnet-4")
	require.Equal(t, "claude-sonnet-4", requestedModel)
	require.False(t, requestedUncertain)

	body := []byte(`{"model":"claude-sonnet-4","max_tokens":200,"max_output_tokens":300,"max_completion_tokens":400}`)
	require.Equal(t, 400, billingRiskMaxOutputTokens(body, 0))
	require.Equal(t, 123, billingRiskMaxOutputTokens(body, 123))
	require.Positive(t, billingRiskInputTokens(body))
}

func TestBillingRiskErrorDetailsPreserveAdmissionStatus(t *testing.T) {
	status, code, message := billingRiskErrorDetails(infraerrors.TooManyRequests("BILLING_RISK_BUDGET_EXCEEDED", "请稍后重试"))
	require.Equal(t, 429, status)
	require.Equal(t, "rate_limit_error", code)
	require.Equal(t, "请稍后重试", message)

	status, code, _ = billingRiskErrorDetails(infraerrors.ServiceUnavailable("BILLING_RISK_GUARD_UNAVAILABLE", "暂不可用"))
	require.Equal(t, 503, status)
	require.Equal(t, "billing_service_error", code)
}

func TestBillingRiskLeaseRefreshesAndReleasesBeforeHandoff(t *testing.T) {
	store := &handlerBillingRiskStoreStub{}
	lease := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))
	require.NotNil(t, lease)

	require.Eventually(t, func() bool {
		refresh, _, _ := store.counts()
		return refresh > 0
	}, 250*time.Millisecond, 5*time.Millisecond)

	lease.Close(context.Background())
	_, release, uncertain := store.counts()
	require.Equal(t, 1, release)
	require.Zero(t, uncertain)
}

func TestBillingRiskLeaseHandoffKeepsPermitForUsageBilling(t *testing.T) {
	store := &handlerBillingRiskStoreStub{}
	permit := newHandlerBillingRiskPermit(t, store)
	lease := newBillingRiskLease(permit)

	require.Same(t, permit, lease.Handoff())
	lease.Close(context.Background())
	_, release, uncertain := store.counts()
	require.Zero(t, release)
	require.Zero(t, uncertain)
}

func TestBillingRiskLeaseCyberUsageHandoffOnlyWhenAsyncBillingWillRun(t *testing.T) {
	store := &handlerBillingRiskStoreStub{}
	lease := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))

	require.Nil(t, handoffBillingRiskLeaseForCyberUsage(lease, false))
	lease.Close(context.Background())
	_, release, _ := store.counts()
	require.Equal(t, 1, release)

	lease = newBillingRiskLease(newHandlerBillingRiskPermit(t, store))
	permit := handoffBillingRiskLeaseForCyberUsage(lease, true)
	require.NotNil(t, permit)
	lease.Close(context.Background())
	_, release, _ = store.counts()
	require.Equal(t, 1, release, "已移交给 cyber 异步扣费的许可不应由 handler 提前释放")
}

func TestBillingRiskCyberUsageKeepsRefreshingAfterHandoff(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_gateway_handler.go", "recordCyberPolicyIfMarked"))
	require.Contains(t, source, "cyberUsageTask := wrapBillingRiskUsageRecordTask(")
	require.Contains(t, source, "cyberUsageTask(ctx)")
}

func TestBillingRiskUsageTaskRefreshesPermitWhileQueuedAndStopsAfterBilling(t *testing.T) {
	store := &handlerBillingRiskStoreStub{}
	permit := newHandlerBillingRiskPermit(t, store)
	taskRan := make(chan struct{})
	task := wrapBillingRiskUsageRecordTask(permit, func(context.Context) {
		close(taskRan)
	})

	require.Eventually(t, func() bool {
		refresh, _, _ := store.counts()
		return refresh > 0
	}, 250*time.Millisecond, 5*time.Millisecond, "任务尚在队列中时必须持续续租")
	task(context.Background())
	<-taskRan
	refreshAfterBilling, _, _ := store.counts()
	time.Sleep(20 * time.Millisecond)
	refreshNow, _, _ := store.counts()
	require.Equal(t, refreshAfterBilling, refreshNow, "账务任务完成后必须停止续租")
}

func TestBillingRiskUsageTaskStopsRefreshingAndMarksUncertainWhenLeaseIsLost(t *testing.T) {
	store := &handlerBillingRiskStoreStub{refreshLost: true}
	permit := newHandlerBillingRiskPermit(t, store)
	task := wrapBillingRiskUsageRecordTask(permit, func(context.Context) {})

	require.Eventually(t, func() bool {
		_, _, uncertain := store.counts()
		return uncertain == 1
	}, 250*time.Millisecond, 5*time.Millisecond)
	refreshAfterLoss, _, _ := store.counts()
	time.Sleep(20 * time.Millisecond)
	refreshNow, _, _ := store.counts()
	require.Equal(t, refreshAfterLoss, refreshNow, "租约丢失并进入冷却后必须停止普通续租")

	task(context.Background())
}

func TestBillingRiskLeaseCanMarkUncertain(t *testing.T) {
	store := &handlerBillingRiskStoreStub{}
	lease := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))

	lease.MarkUncertain(context.Background())
	lease.Close(context.Background())
	_, release, uncertain := store.counts()
	require.Zero(t, release)
	require.Equal(t, 1, uncertain)
}

func TestBillingRiskLeaseCannotHandoffAfterReleaseOrUncertain(t *testing.T) {
	store := &handlerBillingRiskStoreStub{}
	released := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))
	released.Close(context.Background())
	require.Nil(t, released.Handoff())

	uncertain := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))
	uncertain.MarkUncertain(context.Background())
	require.Nil(t, uncertain.Handoff())

	_, releaseCalls, uncertainCalls := store.counts()
	require.Equal(t, 1, releaseCalls)
	require.Equal(t, 1, uncertainCalls)
}

func TestBillingRiskTurnLeasesHandoffOneAndReleaseRemainder(t *testing.T) {
	store := &handlerBillingRiskStoreStub{}
	turns := newBillingRiskTurnLeases()
	first := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))
	second := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))
	turns.Store(1, first)
	turns.Store(2, second)

	require.Same(t, first.permit, turns.Take(1).Handoff())
	turns.CloseAll(context.Background())
	_, release, uncertain := store.counts()
	require.Equal(t, 1, release)
	require.Zero(t, uncertain)
}

func TestBillingRiskTurnLeasesClosesLateStoreAfterCloseAll(t *testing.T) {
	store := &handlerBillingRiskStoreStub{}
	turns := newBillingRiskTurnLeases()
	turns.CloseAll(context.Background())

	turns.Store(2, newBillingRiskLease(newHandlerBillingRiskPermit(t, store)))

	require.False(t, turns.Has(2))
	_, release, _ := store.counts()
	require.Equal(t, 1, release)
}

func TestBillingRiskTurnLeasesConcurrentStoreAndCloseAll(t *testing.T) {
	for range 100 {
		store := &handlerBillingRiskStoreStub{}
		turns := newBillingRiskTurnLeases()
		lease := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			turns.Store(2, lease)
		}()
		go func() {
			defer wg.Done()
			<-start
			turns.CloseAll(context.Background())
		}()
		close(start)
		wg.Wait()

		require.False(t, turns.Has(2))
		_, release, _ := store.counts()
		require.Equal(t, 1, release)
	}
}

func TestBillingRiskWebSocketPreviousTurnHoldsSharedBudgetUntilSettlement(t *testing.T) {
	store := &handlerBillingRiskBudgetStore{}
	settings := newEnabledBillingRiskSettingService(t)
	guard := service.NewBillingRiskGuard(store, settings)
	request := service.BillingRiskRequest{
		UserID:          9,
		Balance:         1,
		Kind:            service.BillingRiskRequestWebSocket,
		EstimatedCost:   0.6,
		EstimateCertain: true,
	}

	firstPermit, err := guard.Acquire(context.Background(), request)
	require.NoError(t, err)
	firstLease := newBillingRiskLease(firstPermit)
	require.Same(t, firstPermit, firstLease.Handoff(), "turn 完成后先交给异步账务，不能提前释放")

	secondPermit, err := guard.Acquire(context.Background(), request)
	require.Nil(t, secondPermit)
	require.Equal(t, "BILLING_RISK_BUDGET_EXCEEDED", infraerrors.Reason(err))

	require.NoError(t, guard.Release(context.Background(), firstPermit))
	thirdPermit, err := guard.Acquire(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, thirdPermit)
}

func TestOpenAIResponsesWebSocketAcquiresEveryTurnAndClosesResidualLeases(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_gateway_handler.go", "ResponsesWebSocket"))
	eligibility := strings.Index(source, "CheckBillingEligibility(")
	turnEligibility := strings.LastIndex(source, "CheckBillingEligibility(")
	attemptLoop := strings.Index(source, "for {\n\t\tif ctx.Err()")
	firstAcquire := strings.Index(source, "acquireBillingRiskLease(")
	selection := strings.Index(source, "SelectAccountWithSchedulerForCapability(")
	beforeRequest := strings.Index(source, "BeforeRequest:")
	secondAcquire := strings.LastIndex(source, "acquireBillingRiskLease(")
	afterTurn := strings.Index(source, "AfterTurn:")
	handoff := strings.Index(source, ".Handoff()")
	mandatoryUsage := strings.Index(source, "submitBillingRiskUsageRecordTask(")

	require.Equal(t, 2, strings.Count(source, "acquireBillingRiskLease("))
	require.Equal(t, 2, strings.Count(source, "CheckBillingEligibility("))
	require.GreaterOrEqual(t, attemptLoop, 0)
	require.Less(t, eligibility, firstAcquire)
	require.Greater(t, firstAcquire, attemptLoop)
	require.Less(t, firstAcquire, selection)
	require.Contains(t, source[attemptLoop:firstAcquire], "if !turnRiskLeases.Has(1)")
	require.Less(t, beforeRequest, turnEligibility)
	require.Less(t, turnEligibility, secondAcquire)
	require.Less(t, beforeRequest, secondAcquire)
	require.Less(t, afterTurn, handoff)
	require.Less(t, handoff, mandatoryUsage)
	require.Contains(t, source, "turnRiskLeases.CloseAll(ctx)")
	require.Contains(t, source, "RiskPermit:")
}

func TestOpenAIResponsesWebSocketRiskAdmissionAndBillingShareTurnPricingAt(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_gateway_handler.go", "ResponsesWebSocket"))

	firstPricingContext := strings.Index(source, "wsPricingCtx, firstTurnPricingAt := h.gatewayService.WithOpenAIRequestPricingContext")
	firstAcquire := strings.Index(source, "acquireBillingRiskLease(")
	require.GreaterOrEqual(t, firstPricingContext, 0)
	require.Greater(t, firstAcquire, firstPricingContext)
	require.Contains(t, source, "ctx = wsPricingCtx")
	require.NotContains(t, source, "firstTurnPricingAt := time.Now()")
	require.Contains(t, source, "PricingAt:           firstTurnPricingAt")
	require.Contains(t, source, "turnPricing.freeze(firstTurnPricingAt)")
	require.Contains(t, source, "turnCtx, pricingAt := h.gatewayService.WithOpenAITurnPricingContext")
	require.Contains(t, source, "acquireBillingRiskLease(turnCtx,")
	require.Contains(t, source, "turnProfitCtx = turnCtx")
	require.Contains(t, source, "ProfitControlVetoLatest(turnProfitCtx, account)")
	require.Contains(t, source, "turnPricing.freeze(pricingAt)")
	require.Contains(t, source, "PricingAt:           pricingAt")
	require.Contains(t, source, "turnRecordPricingAt := turnPricing.current()")
	require.NotContains(t, source, "turnCtx, _ := h.gatewayService.WithOpenAITurnPricingContext")
}

func TestGatewayWebSearchRiskAdmissionAndBillingSharePricingAt(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "gateway_web_search.go", "WebSearch"))

	require.Equal(t, 1, strings.Count(source, "pricingAt := time.Now()"))
	require.Equal(t, 2, strings.Count(source, "PricingAt:"))
	require.Contains(t, source, "PricingAt:    pricingAt")
	require.Contains(t, source, "PricingAt:          pricingAt")
	require.NotContains(t, source, "PricingAt:    time.Now()")
}

func TestGatewayResponsesImageRiskUsesConservativeUnknown(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "gateway_handler_responses.go", "Responses"))

	require.Contains(t, source, "billableSearchIntent := billingRiskHasBillableSearchTool(body)")
	require.Contains(t, source, "requestPlatform == service.PlatformGrok && billableSearchIntent")
	require.Contains(t, source, "ConservativeUnknown: imageIntent || billableSearchIntent")
}

func TestGatewayChatCompletionsGeminiImageUsesConservativeSyncImageRisk(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "gateway_handler_chat_completions.go", "ChatCompletions"))

	require.Contains(t, source, "service.IsGeminiImageGenerationRequest(reqModel, channelMapping.MappedModel, body)")
	require.Contains(t, source, "BillingRiskRequestSyncImage")
	require.Contains(t, source, "ConservativeUnknown:")
}

func TestOpenAIResponsesSearchRiskUsesConservativeUnknown(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_gateway_handler.go", "Responses"))

	require.Contains(t, source, "billableSearchIntent := billingRiskHasBillableSearchTool(body)")
	require.Contains(t, source, "requestPlatform == service.PlatformGrok && billableSearchIntent")
	require.Contains(t, source, "ConservativeUnknown: imageIntent || billableSearchIntent")
}

func TestChatAndMessagesSearchRiskUsesConservativeUnknown(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		function string
	}{
		{name: "OpenAI Chat", file: "openai_chat_completions.go", function: "ChatCompletions"},
		{name: "Gateway Messages", file: "gateway_handler.go", function: "Messages"},
		{name: "OpenAI Messages", file: "openai_gateway_handler.go", function: "Messages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			require.Contains(t, source, "billingRiskHasBillableSearchTool(body)")
			require.Contains(t, source, "ConservativeUnknown: billableSearchIntent")
		})
	}
}

func TestResponsesWebSocketCyberBillingTakesOverRiskLease(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_gateway_handler.go", "ResponsesWebSocket"))

	require.Contains(t, source, "recordCyberPolicyIfMarked(c, apiKey, account, subscription, turnRequestedModel, turnErr != nil, cyberBlockKey, turnUsageFields, requestPayloadHash, turnRecordPricingAt, riskLease)")
	require.NotContains(t, source, "riskLease.MarkUncertain(ctx)")
}

func TestOpenAIEmbeddingsImageInputRiskUsesConservativeUnknown(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_embeddings.go", "Embeddings"))

	require.Contains(t, source, "ConservativeUnknown: billingRiskHasImageInput(body)")
}

func TestBillingRiskHasBillableSearchTool(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "native web search", body: `{"tools":[{"type":"web_search"}]}`, want: true},
		{name: "native x search", body: `{"tools":[{"type":"x_search"}]}`, want: true},
		{name: "tool search", body: `{"tools":[{"type":"tool_search"}]}`, want: true},
		{name: "function search", body: `{"tools":[{"type":"function","name":"web_search"}]}`, want: true},
		{name: "ordinary function", body: `{"tools":[{"type":"function","name":"lookup"}]}`},
		{name: "invalid tools", body: `{"tools":"web_search"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, billingRiskHasBillableSearchTool([]byte(tt.body)))
		})
	}
}

func TestBillingRiskHasImageInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "image url object", body: `{"input":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}`, want: true},
		{name: "input image string", body: `{"input":[{"type":"input_image","image_url":"https://example.com/a.png"}]}`, want: true},
		{name: "generic nested image url", body: `{"input":{"items":[{"image_url":"https://example.com/a.png"}]}}`, want: true},
		{name: "empty image url", body: `{"input":[{"type":"input_image","image_url":" "}]}`},
		{name: "text only", body: `{"input":["hello","world"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, billingRiskHasImageInput([]byte(tt.body)))
		})
	}
}

func TestGatewayMessagesRiskAdmissionUsesFinalBillingContext(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "gateway_handler.go", "Messages"))
	attemptMapping := strings.Index(source, "attemptChannelMapping, _ =")
	finalAcquire := strings.LastIndex(source, "acquireBillingRiskLease(")
	forward := strings.Index(source[finalAcquire:], "h.gatewayService.Forward(")

	require.GreaterOrEqual(t, attemptMapping, 0)
	require.Greater(t, finalAcquire, attemptMapping)
	require.Greater(t, forward, 0)
	admissionBlock := source[finalAcquire : finalAcquire+forward]
	compactAdmissionBlock := strings.Join(strings.Fields(admissionBlock), "")
	require.Contains(t, compactAdmissionBlock, "APIKey:currentAPIKey")
	require.Contains(t, compactAdmissionBlock, "Subscription:currentSubscription")
	require.Contains(t, admissionBlock, "billingRiskAdmissionInputForMapping(")
	require.Contains(t, admissionBlock, "attemptChannelMapping, reqModel")
	require.Contains(t, source, "channelUsageFields := clientRequestedUsageFields(c, attemptChannelMapping, reqModel, result.UpstreamModel)")
	require.Contains(t, source, "ChannelUsageFields: channelUsageFields")
	require.Contains(t, source, "riskAdmissionResolved")
	require.Contains(t, source, "var attemptChannelMapping service.ChannelMappingResult")
	fallbackAPIKey := strings.Index(source, "fallbackAPIKey :=")
	fallbackAssignment := strings.Index(source, "currentAPIKey = fallbackAPIKey")
	require.Greater(t, fallbackAssignment, fallbackAPIKey)
	fallbackTransition := source[fallbackAPIKey:fallbackAssignment]
	require.Contains(t, fallbackTransition, "riskLease.Close(c.Request.Context())")
	require.Contains(t, fallbackTransition, "riskLease = nil")
}

func TestBillingRiskPermitUsageCallSitesKeepRefreshingAfterHandoff(t *testing.T) {
	for _, tc := range []struct {
		file     string
		function string
	}{
		{file: "openai_images.go", function: "Images"},
		{file: "gateway_web_search.go", function: "WebSearch"},
		{file: "openai_alpha_search.go", function: "recordAlphaSearchUsage"},
		{file: "grok_audio.go", function: "recordGrokVoiceUsage"},
	} {
		t.Run(tc.file+"/"+tc.function, func(t *testing.T) {
			source := stripGoComments(goFunctionSource(t, tc.file, tc.function))
			require.Contains(t, source, "RiskPermit:")
			require.Contains(t, source, "submitBillingRiskUsageRecordTask(")
			require.NotContains(t, source, "submitMandatoryUsageRecordTask(")
		})
	}
}

func TestBillingRiskAdmissionModelFollowsBillingModelSource(t *testing.T) {
	for _, tc := range []struct {
		name          string
		mapping       service.ChannelMappingResult
		requested     string
		wantModel     string
		wantUncertain bool
	}{
		{
			name: "requested source keeps public model",
			mapping: service.ChannelMappingResult{
				Mapped:             true,
				MappedModel:        "cheap-upstream-model",
				BillingModelSource: service.BillingModelSourceRequested,
			},
			requested: "expensive-public-model",
			wantModel: "expensive-public-model",
		},
		{
			name: "channel mapped source uses mapped model",
			mapping: service.ChannelMappingResult{
				Mapped:             true,
				MappedModel:        "priced-channel-model",
				BillingModelSource: service.BillingModelSourceChannelMapped,
			},
			requested: "public-model",
			wantModel: "priced-channel-model",
		},
		{
			name: "upstream source is unknown before forwarding",
			mapping: service.ChannelMappingResult{
				Mapped:             true,
				MappedModel:        "routing-model",
				BillingModelSource: service.BillingModelSourceUpstream,
			},
			requested:     "public-model",
			wantModel:     "routing-model",
			wantUncertain: true,
		},
		{
			name: "response source is unknown before response",
			mapping: service.ChannelMappingResult{
				Mapped:             true,
				MappedModel:        "routing-model",
				BillingModelSource: service.BillingModelSourceResponse,
			},
			requested:     "public-model",
			wantModel:     "routing-model",
			wantUncertain: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, uncertain := billingRiskAdmissionModel(tc.mapping, tc.requested)
			require.Equal(t, tc.wantModel, model)
			require.Equal(t, tc.wantUncertain, uncertain)
		})
	}
}
