package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCaptureOpenAIRoutingSnapshotForAsync_StickySessionClone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	decision := newOpenAIStickyDecisionForTest(t, "load_balance", "hit")
	snapshot := storeOpenAIRoutingSnapshotFromDecision(
		c,
		service.TargetGroupExhausted,
		decision,
		&service.Account{ID: 66, Name: "acc-66"},
		"gpt-5.4-Sys",
		"gpt-5.4",
	)
	require.NotNil(t, snapshot)

	captured := captureOpenAIRoutingSnapshotForAsync(c)
	require.NotNil(t, captured)
	require.NotSame(t, snapshot, captured)

	snapshot.TargetGroup = "active"
	snapshot.ScheduleLayer = "mutated_layer"
	snapshot.EffectiveModel = "mutated-model"
	require.NotNil(t, snapshot.SelectedAccountName)
	*snapshot.SelectedAccountName = "mutated-account"

	originalSticky := reflect.ValueOf(snapshot).Elem().FieldByName("Sticky")
	require.True(t, originalSticky.IsValid())
	require.False(t, originalSticky.IsNil())
	originalSticky.Elem().FieldByName("SessionSource").SetString("mutated_source")
	originalSticky.Elem().FieldByName("EvalResult").SetString("mutated_eval")
	originalSticky.Elem().FieldByName("ParentSessionPresent").SetBool(true)
	originalSticky.Elem().FieldByName("ParentSessionKey").SetString("mutated_parent")

	require.Equal(t, "exhausted", captured.TargetGroup)
	require.Equal(t, "load_balance", captured.ScheduleLayer)
	require.Equal(t, "gpt-5.4", captured.EffectiveModel)
	require.NotNil(t, captured.SelectedAccountName)
	require.Equal(t, "acc-66", *captured.SelectedAccountName)

	capturedSticky := reflect.ValueOf(captured).Elem().FieldByName("Sticky")
	require.True(t, capturedSticky.IsValid())
	require.False(t, capturedSticky.IsNil())
	require.Equal(t, "header_x_session_affinity", capturedSticky.Elem().FieldByName("SessionSource").String())
	require.Equal(t, "hit", capturedSticky.Elem().FieldByName("EvalResult").String())
	require.False(t, capturedSticky.Elem().FieldByName("ParentSessionPresent").Bool())
	require.Empty(t, capturedSticky.Elem().FieldByName("ParentSessionKey").String())
}

func TestCaptureOpenAIRoutingSnapshotForAsync_OpenAIGatewayHandlerUsageTasksUseCapturedSnapshot(t *testing.T) {
	source, err := os.ReadFile("openai_gateway_handler.go")
	require.NoError(t, err)

	text := string(source)
	require.NotContains(t, text, "RoutingSnapshot:    getOpenAIRoutingSnapshot(c)")
	require.GreaterOrEqual(t, strings.Count(text, "RoutingSnapshot:    routingSnapshot"), 3)
}

func TestCaptureOpenAIRoutingSnapshotForAsync_OpenAIGatewayHandlerUsageTasksUseCapturedEndpoints(t *testing.T) {
	source, err := os.ReadFile("openai_gateway_handler.go")
	require.NoError(t, err)

	text := string(source)
	require.NotContains(t, text, "InboundEndpoint:    GetInboundEndpoint(c)")
	require.NotContains(t, text, "UpstreamEndpoint:   GetUpstreamEndpoint(c")
	require.GreaterOrEqual(t, strings.Count(text, "InboundEndpoint:    inboundEndpoint"), 3)
	require.GreaterOrEqual(t, strings.Count(text, "UpstreamEndpoint:   upstreamEndpoint"), 3)
}

func TestCaptureOpenAIUsageEndpointsForAsync_EndpointValuesCloned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(ctxKeyInboundEndpoint, EndpointChatCompletions)

	inboundEndpoint, upstreamEndpoint := captureOpenAIUsageEndpointsForAsync(c, service.PlatformOpenAI)

	c.Set(ctxKeyInboundEndpoint, EndpointResponses)
	c.Request.URL.Path = "/openai/v1/responses/compact"

	require.Equal(t, EndpointChatCompletions, inboundEndpoint)
	require.Equal(t, EndpointResponses, upstreamEndpoint)
}
