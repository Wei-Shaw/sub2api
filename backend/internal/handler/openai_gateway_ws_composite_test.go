package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type compositeWSRouteRepo struct {
	service.CompositeModelRouteRepository
	routes []service.CompositeModelRoute
	err    error
}

func (r *compositeWSRouteRepo) ListByGroup(context.Context, int64, bool) ([]service.CompositeModelRoute, error) {
	return r.routes, r.err
}

type compositeWSHTTPUpstream struct{ service.HTTPUpstream }

func (*compositeWSHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func compositeWSResolver(platform, endpoint, upstream string) *service.CompositeRouteResolver {
	return service.NewCompositeRouteResolver(&compositeWSRouteRepo{routes: []service.CompositeModelRoute{{
		GroupID: 4201, PublicModel: "public-alias", MatchType: service.CompositeRouteMatchExact,
		TargetPlatform: platform, Endpoint: endpoint, UpstreamModel: upstream, Enabled: true,
	}}})
}
func compositeWSGroup(models ...string) *service.Group {
	g := wsAllowlistGroup(len(models) > 0, models...)
	g.Platform = service.PlatformComposite
	return g
}

func TestOpenAIResponsesWebSocket_CompositeAlias(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformGrok} {
		for _, endpoint := range []string{service.CompositeRouteEndpointResponses, service.CompositeRouteEndpointAny} {
			for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
				t.Run(platform+"/"+endpoint+"/"+mode, func(t *testing.T) {
					upstream := "gpt-5.4"
					if platform == service.PlatformGrok {
						upstream = "grok-4.3"
					}
					got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
						firstPayload:  `{"type":"response.create","model":"public-alias","input":"hi"}`,
						secondPayload: `{"type":"response.create","input":"again"}`,
						group:         compositeWSGroup("public-alias"), accountPlatform: platform, ingressMode: mode,
						compositeResolver: compositeWSResolver(platform, endpoint, upstream),
					})
					require.Len(t, got.upstreamPayloads, 2)
					for i, payload := range got.upstreamPayloads {
						require.Equal(t, upstream, gjson.GetBytes(payload, "model").String())
						require.Equal(t, "public-alias", gjson.GetBytes(got.clientEvents[i], "response.model").String())
						require.Equal(t, "public-alias", got.logs[i].RequestedModel)
						require.NotNil(t, got.logs[i].UpstreamModel)
						require.Equal(t, upstream, *got.logs[i].UpstreamModel)
					}
				})
			}
		}
	}
}

func TestOpenAIResponsesWebSocket_CompositeRouteRejections(t *testing.T) {
	for _, tc := range []struct {
		name, platform, endpoint, model, reason string
		repoErr                                 error
		status                                  coderws.StatusCode
	}{
		{name: "disallowed platform", platform: service.PlatformAnthropic, endpoint: "responses", model: "public-alias", reason: "only supports OpenAI-compatible"},
		{name: "wrong endpoint", platform: service.PlatformOpenAI, endpoint: "messages", model: "public-alias", reason: "only supports OpenAI-compatible"},
		{name: "unknown alias", platform: service.PlatformOpenAI, endpoint: "responses", model: "unknown-alias", reason: "only supports OpenAI-compatible"},
		{name: "resolver error", model: "gpt-5.4", repoErr: errors.New("database unavailable"), reason: "Failed to resolve composite model route", status: coderws.StatusInternalError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolver := compositeWSResolver(tc.platform, tc.endpoint, "gpt-5.4")
			if tc.repoErr != nil {
				resolver = service.NewCompositeRouteResolver(&compositeWSRouteRepo{err: tc.repoErr})
			}
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload: `{"type":"response.create","model":"` + tc.model + `"}`, group: compositeWSGroup(), compositeResolver: resolver,
				firstFrameCloseExpected: true, closeReason: tc.reason, closeStatus: tc.status,
			})
		})
	}
}

func TestOpenAIResponsesWebSocket_CompositeAdmissionUsesPublicModel(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload: `{"type":"response.create","model":"public-alias"}`, group: compositeWSGroup("gpt-5.4"),
		compositeResolver: compositeWSResolver(service.PlatformOpenAI, "responses", "gpt-5.4"), firstFrameCloseExpected: true,
	})
}

func TestOpenAIResponsesWebSocket_CompositeModelSwitchRequiresReconnect(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		for _, model := range []string{"gpt-5.4", "grok-4.3", "unknown-alias"} {
			t.Run(mode+"/"+model, func(t *testing.T) {
				runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
					firstPayload:  `{"type":"response.create","model":"public-alias"}`,
					secondPayload: `{"type":"response.create","model":"` + model + `"}`, group: compositeWSGroup(), ingressMode: mode,
					compositeResolver:       compositeWSResolver(service.PlatformOpenAI, "responses", "gpt-5.4"),
					secondTurnCloseExpected: true, closeReason: "model switch requires reconnect",
				})
			})
		}
	}
}

func TestOpenAIResponsesWebSocket_CompositeChannelBilling(t *testing.T) {
	for _, source := range []string{service.BillingModelSourceRequested, service.BillingModelSourceChannelMapped, service.BillingModelSourceUpstream} {
		t.Run(source, func(t *testing.T) {
			got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:      `{"type":"response.create","model":"gpt-5.6-sol"}`,
				secondPayload:     `{"type":"response.create","model":"gpt-5.6-sol"}`,
				group:             compositeWSGroup("gpt-5.6-sol"),
				compositeResolver: service.NewCompositeRouteResolver(&compositeWSRouteRepo{routes: []service.CompositeModelRoute{{PublicModel: "gpt-5.6-sol", MatchType: service.CompositeRouteMatchExact, TargetPlatform: service.PlatformOpenAI, Endpoint: service.CompositeRouteEndpointResponses, UpstreamModel: "route-target"}}}),
				channelMapping:    map[string]string{"route-target": "gpt-5.4"}, billingModelSource: source,
				accountModelMapping: map[string]any{"gpt-5.4": "gpt-5.4"},
			})
			for i, log := range got.logs {
				require.Equal(t, "gpt-5.4", gjson.GetBytes(got.upstreamPayloads[i], "model").String())
				require.Equal(t, "gpt-5.6-sol", log.RequestedModel)
				require.Equal(t, "gpt-5.6-sol", log.Model)
				if source == service.BillingModelSourceRequested {
					require.InDelta(t, 40e-6, log.TotalCost, 1e-12)
				} else {
					require.InDelta(t, 20e-6, log.TotalCost, 1e-12)
				}
			}
		})
	}
}

func TestOpenAIResponsesWebSocket_CompositeDetectorFallback(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload: `{"type":"response.create","model":"gpt-5.4"}`, group: compositeWSGroup(),
		compositeResolver: service.NewCompositeRouteResolver(&compositeWSRouteRepo{}),
	})
	require.Equal(t, "gpt-5.4", gjson.GetBytes(got.upstreamFirstPayload, "model").String())
}

func TestOpenAIResponsesWebSocket_CompositeSessionModelSwitchRequiresReconnect(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"public-alias"}`,
		midPayload:    `{"type":"session.update","session":{"model":"grok-4.3"}}`,
		secondPayload: `{"type":"response.create"}`, group: compositeWSGroup(),
		compositeResolver:       compositeWSResolver(service.PlatformOpenAI, "responses", "gpt-5.4"),
		secondTurnCloseExpected: true, closeReason: "model switch requires reconnect",
	})
}
