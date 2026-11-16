//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The catalog is exact membership. Only GLM has a model beside its anchor, and
// the enrollment anchor itself is never widened.
func TestGatewayCodingPlanModelAdmission(t *testing.T) {
	for _, tuple := range []struct {
		profile, model string
		admitted       bool
	}{
		{GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesModel, true},
		{GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesModelFull, true},
		{GatewayGLMCodingResponsesProfile, "glm-5.2", false},
		{GatewayGLMCodingResponsesProfile, "GLM-5.3", false},
		{GatewayGLMCodingResponsesProfile, "glm-5.3 ", false},
		{GatewayGLMCodingResponsesProfile, "", false},
		{GatewayKimiCodeResponsesProfile, GatewayKimiCodeResponsesModel, true},
		{GatewayKimiCodeResponsesProfile, GatewayGLMCodingResponsesModelFull, false},
		{GatewayMiniMaxMPlanResponsesProfile, GatewayMiniMaxMPlanResponsesModel, true},
		{GatewayMiniMaxMPlanResponsesProfile, "MiniMax-M3", false},
		{GatewayNVIDIABuildChatProfile, GatewayNVIDIABuildAnchorModel, false},
		{"unknown-profile", GatewayGLMCodingResponsesModelFull, false},
	} {
		require.Equal(t, tuple.admitted, GatewayCodingPlanModelAdmitted(tuple.profile, tuple.model), "%s %q", tuple.profile, tuple.model)
	}
	require.True(t, GatewayCodingPlanCatalogProfile(GatewayGLMCodingResponsesProfile))
	require.False(t, GatewayCodingPlanCatalogProfile(GatewayKimiCodeResponsesProfile))
	require.False(t, GatewayCodingPlanCatalogProfile(GatewayMiniMaxMPlanResponsesProfile))
	require.True(t, GatewayNativeCodingPlanProfileValid(GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel))
	require.False(t, GatewayNativeCodingPlanProfileValid(GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModelFull),
		"the enrollment anchor stays the single birth model")
}

// A request for the second GLM model must pass the bounded input check; any
// model outside the catalog, or another plan's model, must not.
func TestGatewayCodingPlanPrepareResponsesModels(t *testing.T) {
	payload := func(model string) []byte {
		return []byte(fmt.Sprintf(`{"model":%q,"input":"text","stream":true,"store":false,"max_output_tokens":100}`, model))
	}
	for _, tuple := range []struct {
		profile, model string
		ok             bool
	}{
		{GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesModel, true},
		{GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesModelFull, true},
		{GatewayGLMCodingResponsesProfile, "glm-5.2", false},
		{GatewayKimiCodeResponsesProfile, GatewayGLMCodingResponsesModelFull, false},
		{GatewayMiniMaxMPlanResponsesProfile, GatewayGLMCodingResponsesModelFull, false},
	} {
		_, _, err := gatewayNativePrepareProviderResponses(tuple.profile, payload(tuple.model), 4096)
		if tuple.ok {
			require.NoError(t, err, "%s %s", tuple.profile, tuple.model)
		} else {
			require.ErrorIs(t, err, ErrGatewayNativeIdentity, "%s %s", tuple.profile, tuple.model)
		}
	}
}

// Real ForwardGatewayRoute against a local peer: no provider network is used.
// The admitted model reaches the exact provider path unchanged; every other
// model is rejected before custody entry.
func TestGatewayNativeGLMCatalogDispatch(t *testing.T) {
	for _, tuple := range []struct {
		model string
		ok    bool
	}{
		{GatewayGLMCodingResponsesModel, true},
		{GatewayGLMCodingResponsesModelFull, true},
		{"glm-5.2", false},
		{"GLM-5.3", false},
		{"glm-5.3-extra", false},
	} {
		t.Run(tuple.model, func(t *testing.T) {
			payload := []byte(fmt.Sprintf(`{"model":%q,"input":"text","stream":true,"store":false,"max_output_tokens":100}`, tuple.model))
			var calls atomic.Int32
			var seenModel atomic.Value
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "/api/v1/responses", r.URL.Path)
				wire, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				seenModel.Store(gjson.GetBytes(wire, "model").String())
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n")
			}))
			defer peer.Close()
			target, err := url.Parse(peer.URL)
			require.NoError(t, err)
			a := nativeReviewStreamAccount(t, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesProfile)
			a.Extra[GatewayModelExtraKey] = GatewayGLMCodingResponsesModel
			route, err := GatewayNativeDescriptor(a)
			require.NoError(t, err)
			require.Equal(t, GatewayGLMCodingResponsesModel, route.Model, "the route keeps the birth anchor")
			svc := &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: a}, httpUpstream: &gatewayIdentityRealHTTP{client: &http.Client{Transport: kimiTierPeerTransport{peer: target}}}, cfg: rawChatCompletionsTestConfig()}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/private/native/v1/responses", strings.NewReader(string(payload)))
			_, entered, err := svc.ForwardGatewayRoute(nativeFixtureContext(t, context.Background()), c, route, payload)
			if !tuple.ok {
				require.ErrorIs(t, err, ErrGatewayNativeIdentity)
				require.False(t, entered)
				require.Zero(t, calls.Load())
				return
			}
			require.NoError(t, err)
			require.True(t, entered)
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, tuple.model, seenModel.Load())
		})
	}
}

// Kimi keeps its single anchor even though GLM now has a catalog.
func TestGatewayNativeKimiRejectsGLMCatalogModel(t *testing.T) {
	payload := []byte(fmt.Sprintf(`{"model":%q,"input":"text","stream":true,"store":false,"max_output_tokens":100}`, GatewayGLMCodingResponsesModelFull))
	a := nativeReviewStreamAccount(t, GatewayKimiCodeResponsesBaseURL, GatewayKimiCodeResponsesProfile)
	a.Extra[GatewayModelExtraKey] = GatewayKimiCodeResponsesModel
	route, err := GatewayNativeDescriptor(a)
	require.NoError(t, err)
	svc := &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: a}, cfg: rawChatCompletionsTestConfig()}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/private/native/v1/responses", strings.NewReader(string(payload)))
	_, entered, err := svc.ForwardGatewayRoute(nativeFixtureContext(t, context.Background()), c, route, payload)
	require.ErrorIs(t, err, ErrGatewayNativeIdentity)
	require.False(t, entered)
}

// The GLM catalogue is read through the same fences as the NVIDIA one: logical
// authority before and after, the exact physical route and the encrypted row.
func TestGatewayGLMAccountModelsCatalog(t *testing.T) {
	for _, change := range []string{"none", "profile", "origin", "owner", "epoch", "route_other_profile", "generation_after_read", "credentials_after_read"} {
		t.Run(change, func(t *testing.T) {
			account := nativeReviewStreamAccount(t, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesProfile)
			account.CreatedAt = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
			account.Extra[GatewayModelExtraKey] = GatewayGLMCodingResponsesModel
			route, err := GatewayNativeDescriptor(account)
			require.NoError(t, err)
			scope, err := GatewayNativeCredentialScopeForAccount(account)
			require.NoError(t, err)
			input := GatewayAccountModelsInput{GatewayAccountLimitsInput: GatewayAccountLimitsInput{scope.Owner, scope.Account, GatewayGLMCodingResponsesProfile, 7}}
			switch change {
			case "profile":
				input.ProfileID = GatewayKimiCodeResponsesProfile
			case "origin":
				route.BaseURL = "https://example.invalid/v1"
			case "owner":
				input.OwnerRef = "another-owner"
			case "epoch":
				input.ExpectedAuthorizationEpoch++
			case "route_other_profile":
				// A GLM request must never read another profile's route, e.g. NVIDIA.
				route.Profile = GatewayNVIDIABuildChatProfile
			}
			repository := &gatewayIdentityRepoFixture{row: account}
			gateway := &OpenAIGatewayService{accountRepo: repository}
			authorizations := 0
			reader, err := NewGatewayAccountModelsReader(nil, gateway, func(_ context.Context, in GatewayAccountLimitsInput) (GatewayNativeRoute, error) {
				authorizations++
				if in.OwnerRef != scope.Owner || in.AccountRef != scope.Account || in.ExpectedAuthorizationEpoch != 7 {
					return GatewayNativeRoute{}, ErrGatewayNativeIdentity
				}
				if authorizations == 2 {
					if change == "generation_after_read" {
						changed := route
						changed.Generation = "22345678-1234-4234-8234-123456789abc"
						return changed, nil
					}
					if change == "credentials_after_read" {
						fresh := *account
						fresh.Credentials = map[string]any{"api_key": "changed-credential", "base_url": GatewayGLMCodingResponsesBaseURL}
						repository.row = &fresh
					}
				}
				return route, nil
			})
			require.NoError(t, err)
			// The catalogue never decodes the key, so it must be readable with a
			// context that carries the consumer but no credential custody at all.
			ctx, err := WithGatewayNativeConsumer(context.Background(), custodyTestScope().Consumer)
			require.NoError(t, err)
			result := reader.Read(ctx, input)
			switch change {
			case "none":
			case "profile":
				// Kimi has no catalogue; its stored route is a GLM route, so no snapshot.
				require.NotEqual(t, "snapshot", result.Status)
				return
			default:
				require.Equal(t, GatewayAccountModelsResult{Status: "unavailable"}, result)
				return
			}
			require.Equal(t, 2, authorizations, "catalogue reads recheck logical authority")
			require.Equal(t, "snapshot", result.Status)
			require.Equal(t, "native_catalog", result.Source)
			require.Equal(t, route.Generation, result.PhysicalGeneration)
			require.Equal(t, input.AccountRef, result.PrincipalVersion)
			tools := GatewayAccountModelChoices{Allowed: []string{"function"}, Default: "function"}
			require.Equal(t, []GatewayAccountModel{
				{ModelID: GatewayGLMCodingResponsesModel, Tools: tools},
				{ModelID: GatewayGLMCodingResponsesModelFull, Tools: tools},
			}, *result.Models, "anchor first, no reasoning or fast controls")
			first, err := time.Parse(time.RFC3339Nano, result.FetchedAt)
			require.NoError(t, err)
			last, err := time.Parse(time.RFC3339Nano, result.ExpiresAt)
			require.NoError(t, err)
			require.Equal(t, time.Minute, last.Sub(first))
			wire, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(wire), "sandbox-fixture")
			require.NotContains(t, string(wire), "gcn1.")
		})
	}
}

// The locked entry boundary rechecks the outgoing body against the catalogue.
// Every negative case below passes the earlier admission and must still stop
// here, before the credential is decoded; the control shows nothing else refuses.
func TestGatewayNativeGLMCatalogFinalEntryBoundary(t *testing.T) {
	for _, tuple := range []struct {
		name, requested, body string
		ok                    bool
	}{
		{"catalogue model", GatewayGLMCodingResponsesModelFull, GatewayGLMCodingResponsesModelFull, true},
		{"requested outside the catalogue", "glm-5.2", "glm-5.2", false},
		{"body differs from the admitted model", GatewayGLMCodingResponsesModelFull, GatewayGLMCodingResponsesModel, false},
		{"body outside the catalogue", GatewayGLMCodingResponsesModelFull, "glm-5.2", false},
	} {
		t.Run(tuple.name, func(t *testing.T) {
			account := nativeReviewStreamAccount(t, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesProfile)
			account.Extra[GatewayModelExtraKey] = GatewayGLMCodingResponsesModel
			route, err := GatewayNativeDescriptor(account)
			require.NoError(t, err)
			gateway := &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: account}}
			state := &gatewayNativeDispatch{route: route, account: account, scope: nativeFixtureScope(account), custody: nativeFixtureCustody(t), requestedModel: tuple.requested}
			ctx := context.WithValue(nativeFixtureContext(t, context.Background()), gatewayNativeContextKey{}, state)
			target, err := gateway.gatewayNativeTargetURL(account)
			require.NoError(t, err)
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(fmt.Sprintf(`{"model":%q,"input":"text"}`, tuple.body)))
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer "+account.GetCredential("api_key"))
			release, err := gateway.checkGatewayNativeDispatch(request, account)
			release()
			if tuple.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrGatewayNativeIdentity)
		})
	}
}
