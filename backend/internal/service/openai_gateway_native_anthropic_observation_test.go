//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeObservationResponse(t *testing.T, stream bool, model, speed string, conflict, partial, readError bool) (*http.Response, string) {
	t.Helper()
	usage := map[string]any{"input_tokens": 20, "output_tokens": 10}
	if speed != "" {
		usage["speed"] = speed
	}
	message := map[string]any{
		"id": "msg_observed", "type": "message", "role": "assistant",
		"content": []map[string]string{{"type": "text", "text": "observed response"}}, "usage": usage,
	}
	if model != "" {
		message["model"] = model
	}
	encode := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return string(b)
	}
	wire := encode(message)
	contentType := "application/json"
	if stream {
		contentType = "text/event-stream"
		usage["output_tokens"] = 1
		message["content"] = []any{}
		wire = "event: message_start\ndata: " + encode(map[string]any{"type": "message_start", "message": message}) + "\n\n"
		if conflict {
			message["model"] = "conflicting-model"
			usage["speed"] = "standard"
			wire += "event: message_start\ndata: " + encode(map[string]any{"type": "message_start", "message": message}) + "\n\n"
		}
		wire += "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"observed response\"}}\n\n"
		if !partial {
			wire += "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":10}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
		}
	}
	var body io.ReadCloser = io.NopCloser(strings.NewReader(wire))
	if readError {
		body = &streamReadCloser{payload: []byte(wire), err: io.ErrUnexpectedEOF}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{
		"Content-Type": []string{contentType}, "X-Request-Id": []string{"native-observed"},
	}, Body: body}, wire
}

func TestNativeAnthropicResponseObservationReachesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name       string
		stream     bool
		model      string
		speed      string
		conflict   bool
		partial    bool
		readError  bool
		disconnect bool
		wantModel  string
		wantSpeed  string
	}{
		{name: "buffered matching", model: " k3 ", speed: "standard", wantModel: "k3", wantSpeed: "standard"},
		{name: "buffered mismatch", model: "reported-k3", speed: "fast", wantModel: "reported-k3", wantSpeed: "fast"},
		{name: "buffered undeclared", speed: "fast"},
		{name: "stream matching", stream: true, model: "k3", speed: "standard", wantModel: "k3", wantSpeed: "standard"},
		{name: "stream mismatch", stream: true, model: "reported-k3", speed: "fast", wantModel: "reported-k3", wantSpeed: "fast"},
		{name: "stream undeclared", stream: true, speed: "fast"},
		{name: "stream conflicting declarations", stream: true, model: "reported-k3", speed: "fast", conflict: true, wantModel: "reported-k3"},
		{name: "stream truncated", stream: true, model: "reported-k3", speed: "standard", partial: true, wantModel: "reported-k3", wantSpeed: "standard"},
		{name: "stream read error", stream: true, model: "reported-k3", speed: "standard", partial: true, readError: true, wantModel: "reported-k3", wantSpeed: "standard"},
		{name: "client disconnected", stream: true, model: "reported-k3", speed: "fast", disconnect: true, wantModel: "reported-k3", wantSpeed: "fast"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp, wire := nativeObservationResponse(t, tt.stream, tt.model, tt.speed, tt.conflict, tt.partial, tt.readError)
			upstream := &httpUpstreamRecorder{resp: resp}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			body, err := json.Marshal(map[string]any{
				"model": "client-model", "max_tokens": 32, "stream": tt.stream,
				"messages": []map[string]string{{"role": "user", "content": "hi"}},
			})
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
			if tt.disconnect {
				c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}
			}
			// A previous attempt must not leak its declaration into an undeclared response.
			beginUpstreamResponseModelObservation(c).Observe("stale-model", false)
			account := nativeAnthropicTestAccount()
			result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "k3")
			if tt.partial {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NotNil(t, result)
			require.Equal(t, "k3", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "client-model", result.Model)
			require.Equal(t, "k3", result.BillingModel)
			require.Equal(t, "k3", result.UpstreamModel)
			require.Equal(t, 20, result.Usage.InputTokens)
			wantOutput := 10
			if tt.partial {
				wantOutput = 0
			}
			require.Equal(t, wantOutput, result.Usage.OutputTokens)
			require.Equal(t, tt.disconnect, result.ClientDisconnect)
			if !tt.disconnect {
				require.Equal(t, wire, rec.Body.String(), "observation must preserve the upstream response bytes")
			}
			require.Equal(t, tt.wantModel, observedUpstreamResponseModel(c), "the observer already saw the declaration")

			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			usageSvc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			require.NoError(t, usageSvc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: result, APIKey: &APIKey{ID: 10}, User: &User{ID: 20}, Account: account,
			}))
			require.NotNil(t, usageRepo.lastLog)
			log := usageRepo.lastLog
			require.Equal(t, "k3", *log.UpstreamModel)
			if tt.wantModel == "" {
				require.Nil(t, log.UpstreamResponseModel)
				require.Nil(t, log.UpstreamModelMismatch)
			} else {
				require.NotNil(t, log.UpstreamResponseModel)
				require.Equal(t, tt.wantModel, *log.UpstreamResponseModel)
				require.NotNil(t, log.UpstreamModelMismatch)
				require.Equal(t, tt.wantModel != "k3", *log.UpstreamModelMismatch)
			}
			require.Equal(t, tt.wantModel, result.UpstreamResponseModel)
			require.Equal(t, tt.conflict, result.UpstreamResponseModelConflict)
			require.Equal(t, tt.wantSpeed, result.UpstreamResponseServiceTier)
		})
	}
}

func TestNativeAnthropicResponseObservationRespectsBillingSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for _, source := range []string{BillingModelSourceChannelMapped, BillingModelSourceResponse} {
			name := "buffered/" + source
			if stream {
				name = "stream/" + source
			}
			t.Run(name, func(t *testing.T) {
				usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
				userRepo := &openAIRecordUsageUserRepoStub{}
				usageSvc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
				cheaper, pricier, cheapCost, baselineCost := orderedResponseBillingModels(t, usageSvc.billingService,
					UsageTokens{InputTokens: 20, OutputTokens: 10}, openAICheapFixtureModel, openAIPriceyFixtureModel)
				resp, _ := nativeObservationResponse(t, stream, cheaper, "standard", false, false, false)
				upstream := &httpUpstreamRecorder{resp: resp}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				body, err := json.Marshal(map[string]any{
					"model": pricier, "max_tokens": 32, "stream": stream,
					"messages": []map[string]string{{"role": "user", "content": "hi"}},
				})
				require.NoError(t, err)
				account := nativeAnthropicTestAccount()
				result, err := svc.ForwardAsAnthropic(context.Background(), adaptiveProtocolTestContext("/v1/messages", body), account, body, "", "")
				require.NoError(t, err)
				require.NoError(t, usageSvc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
					Result: result, APIKey: &APIKey{ID: 10}, User: &User{ID: 20}, Account: account,
					ChannelUsageFields: ChannelUsageFields{ChannelID: 9, OriginalModel: pricier, ChannelMappedModel: pricier, BillingModelSource: source},
				}))
				wantCost := baselineCost
				if source == BillingModelSourceResponse {
					wantCost = cheapCost
				}
				require.NotNil(t, usageRepo.lastLog)
				require.InDelta(t, wantCost.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
				require.InDelta(t, wantCost.ActualCost, userRepo.lastAmount, 1e-12)
			})
		}
	}
}
