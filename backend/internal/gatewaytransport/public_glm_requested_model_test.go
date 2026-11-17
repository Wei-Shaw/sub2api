//go:build unit

package gatewaytransport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// A public GLM request may select a model beside its descriptor anchor.
// Admission and settlement must both carry that model (usage is labelled from it).
// Membership of the model in the account's catalogue is native dispatch's decision
// and is covered by the service tests. Dispatch verification denies entry here, so
// no provider is involved.
func TestPublicGLMRequestedModelUsesCatalogueAndExactSettlement(t *testing.T) {
	var original Request
	var admitModels, settleModels []string
	var admits, settlements, entries atomic.Int32
	authority := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/private/native/v1/execution-authority":
			var input publicAuthorizationRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
			_ = json.NewEncoder(w).Encode(publicAuthorizationReply{"fixture-consumer", input.RequestRef, original.Worker, original.Admission, original.Descriptor})
		case "/private/native/v1/public-admit":
			admits.Add(1)
			raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
			require.NoError(t, err)
			var input struct {
				admitRequest
				RequestedModel string `json:"requestedModel"`
			}
			require.NoError(t, json.Unmarshal(raw, &input))
			require.Equal(t, service.GatewayGLMCodingResponsesModel, input.Descriptor.Model, "the descriptor keeps the birth anchor")
			admitModels = append(admitModels, input.RequestedModel)
			_ = json.NewEncoder(w).Encode(ownerReply(input.admitRequest))
		case "/private/native/v1/execution-settle":
			settlements.Add(1)
			raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
			require.NoError(t, err)
			var input struct {
				RequestedModel string `json:"requestedModel"`
			}
			require.NoError(t, json.Unmarshal(raw, &input))
			settleModels = append(settleModels, input.RequestedModel)
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		default:
			t.Error("unexpected callback")
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer authority.Close()
	h, _, _, _ := ownerIngressFixture(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { entries.Add(1) }), func(cfg *Config, _ *ownerFixtureRepo) {
		glm := QualifiedProfile{Profile: service.GatewayGLMCodingResponsesProfile, Model: service.GatewayGLMCodingResponsesModel,
			BaseURL: service.GatewayGLMCodingResponsesBaseURL, QualificationRef: "glm-fixture-qualification",
			RequestBytes: 4096, OutputBytes: 4096, Tokens: 100, ProviderTokenUpperBound: 200}
		cfg.CodingPlans = []QualifiedProfile{glm}
		cfg.CallbackOrigin = authority.URL
		cfg.CallbackTimeout = 5 * time.Second
		cfg.VerifyDispatch = func(context.Context, string, Proof, time.Time) error { return errDenied }
		original = ownerRequest()
		original.Admission.ProfileID = glm.Profile
		original.Descriptor.Profile, original.Descriptor.BaseURL, original.Descriptor.Model = glm.Profile, glm.BaseURL, glm.Model
	})
	public := httptest.NewServer(h.PublicHandler(4))
	defer public.Close()
	client := &http.Client{Timeout: 4 * time.Second}
	call := func(requestID, model string) int {
		payload := []byte(`{"model":"` + model + `","input":"text","stream":true,"store":false}`)
		request, err := http.NewRequest(http.MethodPost, public.URL+"/v1/responses", bytes.NewReader(payload))
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer synthetic-capability-123")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Gateway-Request-Id", requestID)
		response, err := client.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode
	}

	call("glm-catalogue-model", service.GatewayGLMCodingResponsesModelFull)
	require.Equal(t, []string{service.GatewayGLMCodingResponsesModelFull}, admitModels, "admission is attributed to the catalogue model")
	require.Equal(t, []string{service.GatewayGLMCodingResponsesModelFull}, settleModels, "settlement keeps the same model")

	require.EqualValues(t, 1, admits.Load())
	require.EqualValues(t, 1, settlements.Load())
	require.Zero(t, entries.Load(), "usage metadata never grants provider entry")
}
