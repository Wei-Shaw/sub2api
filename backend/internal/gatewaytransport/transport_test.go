//go:build unit

package gatewaytransport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var fixtureProviderKey = gatewayIngressGuardFixtureOpaque()
var fixtureExecutionToken = gatewayIngressGuardFixtureOpaque()
var fixtureCallbackToken = gatewayIngressGuardFixtureOpaque()
var fixtureCleanupToken = gatewayIngressGuardFixtureOpaque()
var fixtureForeignToken = gatewayIngressGuardFixtureOpaque()

// Assert the specified source bounds over HTTP, independently of the
// implementation's channel capacities (changing those must change the result).
const fixtureExecutionAdmissionCap = 32
const fixtureControlAdmissionCap = 8

const (
	ownerExecution   = "11111111-1111-4111-8111-111111111111"
	ownerIssuer      = "22222222-2222-4222-8222-222222222222"
	ownerGeneration  = "33333333-3333-4333-8333-333333333333"
	ownerWorker      = "44444444-4444-4444-8444-444444444444"
	ownerIncarnation = "55555555-5555-4555-8555-555555555555"
	ownerAccount     = "66666666-6666-4666-8666-666666666666"
)

// Concrete disposable fixtures use the accepted registry wire, without
// asserting durable SQL authority or depending on unfinished helper names.
func ownerRequest() Request {
	return Request{RequestRef: "fixture-request", Worker: ownerWorker,
		Admission: Admission{ExecutionRef: ownerExecution, IssuerEpoch: ownerIssuer,
			InvocationRef: "fixture-invocation", AttemptRef: "fixture-attempt", AccountRef: ownerAccount,
			AuthorizationEpoch: 3, SubjectRef: "fixture-subject", PolicyRevision: 4, BindingRevision: 5,
			ProfileID: service.GatewayMiMoResponsesProfile,
			Limits:    Limits{Requests: 2, Concurrency: 1, RequestBytes: 4096, OutputBytes: 4096, Tokens: 100},
			ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)},
		Descriptor: service.GatewayNativeRoute{AccountID: 7, Generation: ownerGeneration,
			CreatedAt: time.Date(2026, 10, 3, 1, 2, 3, 123456789, time.UTC),
			Profile:   service.GatewayMiMoResponsesProfile, BaseURL: "https://fixture.invalid", Model: "fixture-model"},
		Payload: json.RawMessage(`{"model":"fixture-model","input":"fixture","store":false,"stream":true,"service_tier":"default"}`)}
}

func ownerReply(req admitRequest) admitReply {
	p := Proof{ExecutionRef: req.Admission.ExecutionRef, RequestRef: req.RequestRef,
		Worker: req.Worker, Token: "fixture-proof", Native: req.Native}
	d := req.Descriptor
	return admitReply{ConsumerID: req.ConsumerID, Admission: req.Admission,
		Status: requestStatus{RequestRef: req.RequestRef, Effect: "dispatch_started"},
		Dispatch: &dispatch{Descriptor: kernelDescriptor{d.Generation, d.Generation, d.Profile},
			ExecutionRef: p.ExecutionRef, RequestRef: p.RequestRef, Limits: req.Admission.Limits,
			Deadline: req.Admission.ExpiresAt, Closure: p},
		NativeDescriptor: &d, LeaseExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}
}

// Sealed lost-ACK state fixture never admits or enters a provider. Other tests
// below run the real constructor and actual HTTP/TLS forwarding boundaries.
func ownerRegistry() *Handler {
	return &Handler{cfg: Config{MaxEntries: 1, CleanupTimeout: time.Second,
		Enrollment: Enrollment{"fixture-origin", ownerIncarnation, "fixture-qualification"}},
		entries: make(map[transportKey]*reservation), requests: make(map[requestKey]transportKey)}
}

func ownerReserve(t *testing.T, h *Handler, r Request) *reservation {
	t.Helper()
	deadline, err := parseInstant(r.Admission.ExpiresAt)
	if err != nil {
		t.Fatal("invalid fixture deadline", err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	e, first, err := h.reserve("fixture-consumer", r, ctx, cancel, func() {})
	if err != nil || !first {
		cancel()
		t.Fatal("original reservation failed", err)
	}
	t.Cleanup(func() { h.seal(e); e.life.NoForward() })
	return e
}

type ownerHTTPResult struct {
	code     int
	receipt  Receipt
	err      error
	body     []byte
	duration time.Duration
}

// No testing.Fatal in the forwarding goroutine; cancellation can interrupt
// the stream, but control responses must arrive and decode without ambiguity.
func ownerPOST(client *http.Client, origin, path, token string, input any) ownerHTTPResult {
	start := time.Now()
	raw, err := json.Marshal(input)
	if err != nil {
		return ownerHTTPResult{err: err}
	}
	req, err := http.NewRequest("POST", origin+transportsPath+path, bytes.NewReader(raw))
	if err != nil {
		return ownerHTTPResult{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return ownerHTTPResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	result := ownerHTTPResult{code: resp.StatusCode, err: err, body: data, duration: time.Since(start)}
	if len(data) > 65536 {
		result.err = errors.New("fixture response exceeded cap")
	} else if err == nil && resp.StatusCode == http.StatusAccepted {
		result.err = json.Unmarshal(data, &result.receipt)
	}
	return result
}

// Controlled storage is not SQL qualification. The actual service, AEAD
// Authorization boundary, TLS provider, registry and private HTTP paths run.
type ownerFixtureRepo struct {
	service.AccountRepository
	row              *service.Account
	rows             map[int64]*service.Account
	mu               sync.Mutex
	beforeNativeLock func() // controlled test repository gate, never production wiring
}

func (r *ownerFixtureRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rows != nil {
		return r.rows[id], nil
	}
	return r.row, nil
}
func (r *ownerFixtureRepo) LockGatewayNativeAccount(_ context.Context, id int64) (*service.Account, func(), error) {
	if r.beforeNativeLock != nil {
		r.beforeNativeLock()
	}
	r.mu.Lock()
	if r.rows != nil {
		return r.rows[id], r.mu.Unlock, nil
	}
	return r.row, r.mu.Unlock, nil
}

// One simultaneous catalog, real admit HTTP and TLS Responses entry for both
// presets. Cross-tuples and each profile's own caps deny before callback/entry.
func TestFiniteAPIKeyProfilesHTTPAdmissionAndForward(t *testing.T) {
	var entries, admits, glmCatalogue atomic.Int32
	const dynamicOpenRouterModel = "unseen-vendor/new-model:free"
	var driftAfterAdmit atomic.Bool
	var quotaResponse atomic.Bool
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/models" {
			require.Empty(t, r.Header.Get("Authorization"))
			_, _ = io.WriteString(w, `{"data":[{"id":"`+dynamicOpenRouterModel+`","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["tools"]}]}`)
			return
		}
		entries.Add(1)
		if quotaResponse.Load() {
			time.Sleep(150 * time.Millisecond)
			w.Header().Set("X-RateLimit-Limit", "1000")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1793750400000")
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `{"error":{"message":"PRIVATE_QUOTA_SENTINEL"}}`)
			return
		}
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		if r.Host == "api.kimi.ai" || r.Host == "api.z.ai" || r.Host == "api.minimax.io" {
			paths := map[string]string{"api.kimi.ai": "/coding/v1/responses", "api.z.ai": "/api/v1/responses", "api.minimax.io": "/v1/responses"}
			models := map[string]string{"api.kimi.ai": "k3-256k", "api.z.ai": "glm-5.3-flash", "api.minimax.io": "MiniMax-M3.1-Flash"}
			require.Equal(t, paths[r.Host], r.URL.Path)
			if r.Host == "api.z.ai" {
				// The GLM catalog carries the model through unchanged; count the
				// catalogue model separately so a swap in either direction is caught.
				require.Contains(t, []any{models[r.Host], "glm-5.3"}, payload["model"])
				if payload["model"] == "glm-5.3" {
					glmCatalogue.Add(1)
				}
			} else {
				require.Equal(t, models[r.Host], payload["model"])
			}
			require.Equal(t, float64(40), payload["max_output_tokens"])
			if _, firstTurn := payload["input"].(string); firstTurn {
				require.NotContains(t, payload, "service_tier")
				require.NotContains(t, payload, "reasoning")
				encodedText, err := json.Marshal(payload["text"])
				require.NoError(t, err)
				require.JSONEq(t, `{"format":{"type":"json_schema","name":"codex_output_schema","strict":true,"schema":{"type":"object","properties":{"findings":{"type":"array"},"revalidations":{"type":"array"}},"required":["findings","revalidations"],"additionalProperties":false}}}`, string(encodedText))
				require.Equal(t, "33333333-3333-4333-8333-333333333333", payload["prompt_cache_key"])
				require.Equal(t, map[string]any{"x-codex-installation-id": "44444444-4444-4444-8444-444444444444"}, payload["client_metadata"])
			} else if r.Host == "api.kimi.ai" || r.Host == "api.minimax.io" {
				require.NotContains(t, payload, "service_tier")
			} else {
				require.Equal(t, "default", payload["service_tier"])
			}
			tools, ok := payload["tools"].([]any)
			require.True(t, ok)
			require.NotEmpty(t, tools)
			require.Equal(t, "function", tools[0].(map[string]any)["type"], "custom tools must use the qualified function upstream codec")
			if input, ok := payload["input"].([]any); ok {
				require.Equal(t, "function_call", input[0].(map[string]any)["type"])
				require.Equal(t, "function_call_output", input[1].(map[string]any)["type"])
				require.Equal(t, "preserved result", input[1].(map[string]any)["output"])
			}

		} else if r.Host == "openrouter.ai" {
			require.Equal(t, "/api/v1/responses", r.URL.Path)
			require.Equal(t, dynamicOpenRouterModel, payload["model"])
			require.Equal(t, float64(40), payload["max_output_tokens"])
		} else {
			require.Equal(t, "mimo.fixture.invalid", r.Host)
			require.Equal(t, "/v1/responses", r.URL.Path)
			require.Equal(t, "fixture-model", payload["model"])
			require.Equal(t, float64(100), payload["max_output_tokens"])
		}
		require.Equal(t, "Bearer "+fixtureProviderKey, r.Header.Get("Authorization"))
		require.Equal(t, false, payload["store"])
		require.Equal(t, true, payload["stream"])
		require.Contains(t, payload, "tools")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"custom_tool_call\",\"call_id\":\"call_1\",\"name\":\"exec\",\"input\":\"echo preserved\"},{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"controlled final\"}]}]}}\n\n")
	}))
	defer provider.Close()
	local := provider.Client().Transport.(*http.Transport).Clone()
	local.Proxy = nil
	local.TLSClientConfig.ServerName = "example.com"
	local.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "mimo.fixture.invalid:443" && address != "openrouter.ai:443" && address != "api.kimi.ai:443" && address != "api.z.ai:443" && address != "api.minimax.io:443" {
			return nil, errDenied
		}
		return (&net.Dialer{}).DialContext(ctx, network, provider.Listener.Addr().String())
	}
	defer local.CloseIdleConnections()
	custody, err := service.NewGatewayNativeCredentialCustody("fixture", map[string][]byte{"fixture": []byte(strings.Repeat("K", 32))})
	require.NoError(t, err)
	primary := QualifiedProfile{Profile: service.GatewayMiMoResponsesProfile, BaseURL: "https://mimo.fixture.invalid/v1", Model: "fixture-model", QualificationRef: "mimo-fixture-qualified", RequestBytes: 4096, OutputBytes: 4096, Tokens: 100, ProviderTokenUpperBound: 200}
	secondary := QualifiedProfile{Profile: service.GatewayOpenRouterResponsesProfile, BaseURL: service.GatewayOpenRouterBaseURL, Model: service.GatewayOpenRouterModel, QualificationRef: "openrouter-fixture-qualified", RequestBytes: 2048, OutputBytes: 2048, Tokens: 40, ProviderTokenUpperBound: 80}
	plans := []QualifiedProfile{
		{Profile: service.GatewayKimiCodeResponsesProfile, BaseURL: service.GatewayKimiCodeResponsesBaseURL, Model: service.GatewayKimiCodeResponsesModel},
		{Profile: service.GatewayGLMCodingResponsesProfile, BaseURL: service.GatewayGLMCodingResponsesBaseURL, Model: service.GatewayGLMCodingResponsesModel},
		{Profile: service.GatewayMiniMaxMPlanResponsesProfile, BaseURL: service.GatewayMiniMaxMPlanResponsesBaseURL, Model: service.GatewayMiniMaxMPlanResponsesModel},
	}
	for i := range plans {
		plans[i].QualificationRef = "controlled-source-v1"
		plans[i].RequestBytes = 4096
		plans[i].OutputBytes = 4096
		plans[i].Tokens = 40
		plans[i].ProviderTokenUpperBound = 80
	}
	repo := &ownerFixtureRepo{rows: map[int64]*service.Account{}}
	inputs := make([]Request, 0, 2)
	for i, profile := range append([]QualifiedProfile{primary, secondary}, plans...) {
		input := ownerRequest()
		input.RequestRef = fmt.Sprintf("profile-request-%d", i)
		input.Admission.ExecutionRef = fmt.Sprintf("profile-execution-%d", i)
		input.Admission.InvocationRef = fmt.Sprintf("profile-invocation-%d", i)
		input.Admission.ProfileID = profile.Profile
		input.Admission.Limits.RequestBytes = profile.RequestBytes
		input.Admission.Limits.OutputBytes = profile.OutputBytes
		input.Admission.Limits.Tokens = profile.Tokens
		input.Descriptor.AccountID += int64(i)
		input.Descriptor.Profile, input.Descriptor.BaseURL, input.Descriptor.Model = profile.Profile, profile.BaseURL, profile.Model
		if i > 0 {
			input.Descriptor.Generation = fmt.Sprintf("77777777-7777-4777-8777-%012d", i)
			input.Admission.AccountRef = fmt.Sprintf("profile-account-%d", i)
		}
		input.Payload = json.RawMessage(fmt.Sprintf(`{"model":%q,"input":"preserved","store":false,"stream":true,"service_tier":"default","tools":[{"type":"custom","name":"exec","format":{"type":"text"}}]}`, profile.Model))
		if i == 1 {
			input.Payload = json.RawMessage(strings.Replace(string(input.Payload), profile.Model, dynamicOpenRouterModel, 1))
		}
		if i >= 2 {
			input.Payload = json.RawMessage(strings.Replace(string(input.Payload), `,"service_tier":"default"`, `,"reasoning":null,"text":{"format":{"type":"json_schema","name":"codex_output_schema","strict":true,"schema":{"type":"object","properties":{"findings":{"type":"array"},"revalidations":{"type":"array"}},"required":["findings","revalidations"],"additionalProperties":false}}},"prompt_cache_key":"33333333-3333-4333-8333-333333333333","client_metadata":{"x-codex-installation-id":"44444444-4444-4444-8444-444444444444"}`, 1))
		}
		scope := service.GatewayNativeCredentialScope{Consumer: "fixture-consumer", Owner: "fixture-owner", Account: input.Admission.AccountRef, Generation: input.Descriptor.Generation, Purpose: service.GatewayCredentialPurpose}
		envelope, err := custody.Seal(scope, fixtureProviderKey)
		require.NoError(t, err)
		repo.rows[input.Descriptor.AccountID] = &service.Account{ID: input.Descriptor.AccountID, CreatedAt: input.Descriptor.CreatedAt, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive,
			Credentials: map[string]any{"api_key": envelope, "base_url": profile.BaseURL}, Extra: map[string]any{service.GatewayGenerationExtraKey: scope.Generation, service.GatewayProfileExtraKey: profile.Profile, service.GatewayModelExtraKey: profile.Model, service.GatewayCredentialScopeExtraKey: scope.Metadata(), "openai_responses_mode": "force_responses", "openai_passthrough": true, "native_api_key_cancel_on_disconnect": true, "openai_preserve_compatible_reasoning": true}}
		inputs = append(inputs, input)
	}
	u, err := service.NewGatewayNativeLifetimeUpstream(&ownerFixtureHTTP{client: &http.Client{Transport: local}})
	require.NoError(t, err)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, &config.Config{}, nil, nil, nil, nil, nil, u, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, gateway.SetGatewayOpenRouterModels(service.NewGatewayOpenRouterModels(local)))
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admits.Add(1)
		var input admitRequest
		require.Equal(t, "/private/native/v1/admit", r.URL.Path)
		require.Equal(t, "Bearer "+fixtureCallbackToken, r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		if driftAfterAdmit.Swap(false) {
			repo.mu.Lock()
			repo.rows[input.Descriptor.AccountID].Credentials["base_url"] = primary.BaseURL
			repo.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ownerReply(input))
	}))
	defer callback.Close()
	cfg := Config{Gateway: gateway, Custody: custody, Enrollment: Enrollment{"fixture-origin", ownerIncarnation, primary.QualificationRef}, Profile: primary, OpenRouter: &secondary, CodingPlans: plans,
		MaxEntries: 16, EnvelopeBytes: 8192, CallbackBytes: 65536, CallbackOrigin: callback.URL, CallbackCredential: fixtureCallbackToken,
		CallbackTimeout: time.Second, IOTimeout: 5 * time.Second, ProviderReadIdle: time.Second, CleanupTimeout: time.Second,
		Authorize: func(r *http.Request) (Peer, error) {
			if r.Header.Get("Authorization") == "Bearer "+fixtureExecutionToken {
				return Peer{"fixture-consumer", "execution"}, nil
			}
			return Peer{}, errDenied
		}, VerifyEnrollment: func(context.Context, Enrollment) error { return nil }, VerifyDispatch: func(context.Context, string, Proof, time.Time) error { return nil },
		AuthorizeCleanup: func(context.Context, string, Proof, CleanupLease) error { return errDenied }, AcknowledgeClosure: func(context.Context, string, Proof, CleanupLease, Receipt) error { return errDenied }}
	for _, idle := range []time.Duration{300 * time.Second, 300*time.Second + time.Nanosecond} {
		bounded := cfg
		bounded.ProviderReadIdle = idle
		candidate, err := New(context.Background(), bounded)
		if (err == nil) != (idle == 300*time.Second) {
			t.Fatal("private provider read-idle ceiling is not 300s", idle, err)
		}
		if candidate != nil {
			_ = candidate.Stop(context.Background())
		}
	}
	h, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer h.Stop(context.Background())
	server := httptest.NewServer(h)
	defer server.Close()
	// Mutating the composition input cannot turn an enrolled tuple into another.
	secondary.Model = "caller-model"
	plans[0].BaseURL = "https://caller.invalid"
	projected := h.CodingPlanProfiles()
	projected[0].Model = "caller-model"
	for _, mutate := range []func(*Request){
		func(r *Request) { r.Descriptor = inputs[0].Descriptor },
		func(r *Request) { r.Admission.ProfileID = service.GatewayMiMoResponsesProfile },
		func(r *Request) { r.Descriptor.BaseURL = primary.BaseURL },
		func(r *Request) { r.Descriptor.Model = primary.Model },
		func(r *Request) {
			r.Payload = json.RawMessage(strings.Replace(string(r.Payload), dynamicOpenRouterModel, primary.Model, 1))
		},
		func(r *Request) {
			r.Payload = json.RawMessage(strings.TrimSuffix(string(r.Payload), "}") + `,"max_output_tokens":81}`)
		},
		func(r *Request) { r.Admission.Limits.Tokens = 41 },
		func(r *Request) { r.Admission.Limits.RequestBytes = 2049 },
		func(r *Request) { r.Admission.Limits.OutputBytes = 2049 },
		func(r *Request) { r.Admission.ProfileID = "arbitrary-profile" },
	} {
		bad := inputs[1]
		mutate(&bad)
		result := ownerPOST(server.Client(), server.URL, "", fixtureExecutionToken, bad)
		require.NoError(t, result.err)
		require.Equal(t, http.StatusBadRequest, result.code)
	}
	require.Zero(t, admits.Load())
	require.Zero(t, entries.Load())
	for _, input := range inputs {
		result := ownerPOST(server.Client(), server.URL, "", fixtureExecutionToken, input)
		require.NoError(t, result.err)
		require.Equal(t, http.StatusOK, result.code)
		require.Contains(t, string(result.body), "custom_tool_call")
		require.Contains(t, string(result.body), "controlled final")
	}
	// Tool continuation is a second explicit admission, with native custom and
	// function result bytes preserved through the same finite Responses transport.
	for _, input := range inputs[2:] {
		input.RequestRef += "-tool-turn"
		input.Payload = json.RawMessage(fmt.Sprintf(`{"model":%q,"input":[{"type":"custom_tool_call","call_id":"c1","name":"exec","input":"echo preserved"},{"type":"custom_tool_call_output","call_id":"c1","output":"preserved result"},{"type":"function_call","call_id":"c2","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"c2","output":"function result"}],"store":false,"stream":true,"service_tier":"default","tools":[{"type":"custom","name":"exec","format":{"type":"text"}},{"type":"function","name":"read","parameters":{"type":"object"}}]}`, input.Descriptor.Model))
		result := ownerPOST(server.Client(), server.URL, "", fixtureExecutionToken, input)
		require.NoError(t, result.err)
		require.Equal(t, http.StatusOK, result.code)
		require.Contains(t, string(result.body), "controlled final")
	}
	// The GLM catalog model beside the enrollment anchor passes the same
	// transport; a model outside the catalogue is claimed and refused in dispatch.
	glm := inputs[3]
	require.Equal(t, service.GatewayGLMCodingResponsesProfile, glm.Descriptor.Profile)
	glm.RequestRef += "-catalog-model"
	glm.Payload = json.RawMessage(strings.Replace(string(glm.Payload), service.GatewayGLMCodingResponsesModel, service.GatewayGLMCodingResponsesModelFull, 1))
	glmResult := ownerPOST(server.Client(), server.URL, "", fixtureExecutionToken, glm)
	require.NoError(t, glmResult.err)
	require.Equal(t, http.StatusOK, glmResult.code)
	require.Contains(t, string(glmResult.body), "controlled final")
	require.EqualValues(t, 1, glmCatalogue.Load(), "exactly the catalogue request reaches the provider as glm-5.3")
	require.EqualValues(t, 9, admits.Load())
	require.EqualValues(t, 9, entries.Load())
	for _, model := range []string{"glm-5.2", "GLM-5.3", "glm-5.3-extra"} {
		outside := glm
		outside.RequestRef += "-outside-" + model
		outside.Payload = json.RawMessage(strings.Replace(string(glm.Payload), service.GatewayGLMCodingResponsesModelFull, model, 1))
		denied := ownerPOST(server.Client(), server.URL, "", fixtureExecutionToken, outside)
		require.NoError(t, denied.err)
		// Structure is checked before the claim; membership is native dispatch's
		// decision, so the request is claimed but never reaches the provider.
		require.NotContains(t, string(denied.body), "controlled final", model)
	}
	require.EqualValues(t, 12, admits.Load(), "outside models are claimed and refused in dispatch")
	require.EqualValues(t, 9, entries.Load(), "no provider entry for a model outside the catalogue")
	quotaResponse.Store(true)
	h.cfg.IOTimeout = 50 * time.Millisecond
	quotaInput := inputs[1]
	quotaInput.RequestRef = "openrouter-confirmed-quota"
	encodedQuota, err := json.Marshal(quotaInput)
	require.NoError(t, err)
	quotaRequest, err := http.NewRequest(http.MethodPost, server.URL+transportsPath, bytes.NewReader(encodedQuota))
	require.NoError(t, err)
	quotaRequest.Header.Set("Content-Type", "application/json")
	quotaRequest.Header.Set("Authorization", "Bearer "+fixtureExecutionToken)
	quotaReply, err := server.Client().Do(quotaRequest)
	require.NoError(t, err)
	require.Equal(t, 502, quotaReply.StatusCode)
	require.Equal(t, quotaInput.RequestRef, quotaReply.Header.Get("X-Gateway-Request-Ref"))
	require.Equal(t, "1000", quotaReply.Header.Get("X-Gateway-Provider-Quota-Limit"))
	require.Equal(t, "0", quotaReply.Header.Get("X-Gateway-Provider-Quota-Remaining"))
	require.Equal(t, "2026-11-04T00:00:00.000Z", quotaReply.Header.Get("X-Gateway-Provider-Quota-Reset-At"))
	quotaBody, err := io.ReadAll(quotaReply.Body)
	require.NoError(t, err)
	require.NoError(t, quotaReply.Body.Close())
	require.NotContains(t, string(quotaBody), "PRIVATE_QUOTA_SENTINEL")
	quotaResponse.Store(false)
	h.cfg.IOTimeout = 5 * time.Second
	// A valid ingress tuple cannot survive a stored cross-profile endpoint
	// change while the sole admit callback is in flight. Forward rechecks it.
	driftAfterAdmit.Store(true)
	stale := inputs[1]
	stale.RequestRef = "openrouter-stale-tuple"
	result := ownerPOST(server.Client(), server.URL, "", fixtureExecutionToken, stale)
	require.NoError(t, result.err)
	require.EqualValues(t, 14, admits.Load())
	require.EqualValues(t, 10, entries.Load())
	require.NotContains(t, string(result.body), "controlled final")
}

type ownerFixtureHTTP struct {
	client       *http.Client
	closeFails   bool
	closeGate    <-chan struct{}
	closeStarted chan<- struct{}
}

func (u *ownerFixtureHTTP) Do(r *http.Request, _ string, _ int64, concurrency int) (*http.Response, error) {
	if concurrency != 0 {
		return nil, errors.New("ordinary limiter entered")
	}
	resp, err := u.client.Do(r)
	if err == nil && u.closeGate != nil {
		resp.Body = &ownerBlockedClose{ReadCloser: resp.Body, gate: u.closeGate, started: u.closeStarted}
	}
	if err == nil && u.closeFails {
		resp.Body = &ownerCloseFailure{ReadCloser: resp.Body}
	}
	return resp, err
}
func (u *ownerFixtureHTTP) DoWithTLS(r *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, proxy, id, concurrency)
}

// Regression: cancellation does not certify closure while the physical Close
// is delayed, even when the HTTP request itself has already been interrupted.
type ownerBlockedClose struct {
	io.ReadCloser
	gate    <-chan struct{}
	started chan<- struct{}
}

func (b *ownerBlockedClose) Close() error {
	b.started <- struct{}{}
	<-b.gate
	return b.ReadCloser.Close()
}

type ownerCloseFailure struct{ io.ReadCloser }

func (b *ownerCloseFailure) Close() error {
	_ = b.ReadCloser.Close()
	return errors.New("fixture physical close failure")
}

func TestGatewayNativeOwnerHTTPClosureOnActualCompletedTransport(t *testing.T) {
	// Regression: partial provider output stalls forever despite server IdleTimeout;
	// timeout is mistaken for completion or releases unknown occupied evidence.
	for _, mode := range []string{"completed", "paused", "steadyBeyondIO", "held", "closeFailed", "idle", "idleCloseFailed", "idleBlockedClose"} {
		t.Run(mode, func(t *testing.T) {
			var entries, admits, acks atomic.Int32
			closeGate := make(chan struct{})
			closeStarted := make(chan struct{}, 1)
			var releaseClose sync.Once
			defer releaseClose.Do(func() { close(closeGate) })
			providerEntered := make(chan struct{}, 1)
			providerClosed := make(chan struct{}, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				entries.Add(1)
				if r.Header.Get("Authorization") != "Bearer "+fixtureProviderKey || r.URL.Path != "/v1/responses" {
					t.Error("native provider authority changed")
				}
				select {
				case providerEntered <- struct{}{}:
				default:
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if mode == "held" || strings.HasPrefix(mode, "idle") || mode == "paused" || mode == "steadyBeyondIO" {
					_, _ = io.WriteString(w, "data: {\"type\":\"response.created\"}\n\n")
				} else {
					_, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n")
				}
				flusher, ok := w.(http.Flusher)
				if !ok {
					t.Error("fixture response cannot flush")
					return
				}
				flusher.Flush()
				if mode == "steadyBeyondIO" {
					// Active Responses output lasts longer than one downstream I/O
					// timeout, without exceeding the trusted expiry or read-idle.
					for i := 0; i < 6; i++ {
						select {
						case <-time.After(250 * time.Millisecond):
						case <-r.Context().Done():
							return
						}
						if _, err := io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"x\"}\n\n"); err != nil {
							return
						}
						flusher.Flush()
					}
					_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
					flusher.Flush()
				}
				if mode == "paused" {
					// A legitimate reasoning pause longer than the old hardcoded
					// second must survive the trusted two-second fixture policy.
					select {
					case <-time.After(1200 * time.Millisecond):
					case <-r.Context().Done():
						return
					}
					_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
					flusher.Flush()
				}
				// Completion must close the live body without waiting for EOF.
				<-r.Context().Done()
				select {
				case providerClosed <- struct{}{}:
				default:
				}
			}))
			defer upstream.Close()
			custodyKey := make([]byte, 32)
			if _, err := rand.Read(custodyKey); err != nil {
				t.Fatal(err)
			}
			custody, err := service.NewGatewayNativeCredentialCustody("fixture", map[string][]byte{"fixture": custodyKey})
			if err != nil {
				t.Fatal(err)
			}
			scope := service.GatewayNativeCredentialScope{Consumer: "fixture-consumer", Owner: "fixture-owner", Account: ownerAccount, Generation: ownerGeneration, Purpose: service.GatewayCredentialPurpose}
			envelope, err := custody.Seal(scope, fixtureProviderKey)
			if err != nil {
				t.Fatal(err)
			}
			row := &service.Account{ID: 7, Name: "fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, CreatedAt: time.Now().UTC(),
				Credentials: map[string]any{"api_key": envelope, "base_url": upstream.URL}, Extra: map[string]any{service.GatewayGenerationExtraKey: ownerGeneration, service.GatewayProfileExtraKey: service.GatewayMiMoResponsesProfile,
					service.GatewayModelExtraKey: "fixture-model", service.GatewayCredentialScopeExtraKey: scope.Metadata(), "openai_responses_mode": "force_responses", "openai_passthrough": true,
					"native_api_key_cancel_on_disconnect": true, "openai_preserve_compatible_reasoning": true}}
			repo := &ownerFixtureRepo{row: row}
			providerHTTP := &ownerFixtureHTTP{client: upstream.Client(), closeFails: mode == "closeFailed" || mode == "idleCloseFailed"}
			if mode == "idleBlockedClose" {
				providerHTTP.closeGate, providerHTTP.closeStarted = closeGate, closeStarted
			}
			u, err := service.NewGatewayNativeLifetimeUpstream(providerHTTP)
			if err != nil {
				t.Fatal(err)
			}
			svcCfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}
			gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, svcCfg, nil, nil, nil, nil, nil, u, nil, nil, nil, nil, nil, nil, nil, nil)
			proofCh := make(chan Proof, 1)
			callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				admits.Add(1)
				var input admitRequest
				if r.URL.Path != "/private/native/v1/admit" || r.Header.Get("Authorization") != "Bearer "+fixtureCallbackToken {
					t.Error("wrong fixed admission authority")
				}
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					t.Error("invalid admit")
					w.WriteHeader(400)
					return
				}
				reply := ownerReply(input)
				select {
				case proofCh <- reply.Dispatch.Closure:
				default:
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(reply)
			}))
			defer callback.Close()
			var denyAuthority atomic.Bool
			cfg := Config{Gateway: gateway, Custody: custody, Enrollment: Enrollment{"fixture-origin", ownerIncarnation, "fixture-qualification"},
				Profile:    QualifiedProfile{Profile: service.GatewayMiMoResponsesProfile, Model: "fixture-model", BaseURL: upstream.URL, QualificationRef: "fixture-qualification", RequestBytes: 4096, OutputBytes: 4096, Tokens: 100, ProviderTokenUpperBound: 200},
				MaxEntries: 4, EnvelopeBytes: 8192, CallbackBytes: 65536, CallbackOrigin: callback.URL, CallbackCredential: fixtureCallbackToken, CallbackTimeout: time.Second, IOTimeout: 5 * time.Second, ProviderReadIdle: time.Second, CleanupTimeout: time.Second,
				Authorize: func(r *http.Request) (Peer, error) {
					switch r.Header.Get("Authorization") {
					case "Bearer " + fixtureExecutionToken:
						return Peer{"fixture-consumer", "execution"}, nil
					case "Bearer " + fixtureCleanupToken:
						return Peer{"fixture-consumer", "cleanup"}, nil
					case "Bearer " + fixtureForeignToken:
						return Peer{"fixture-foreign", "execution"}, nil
					default:
						return Peer{}, errDenied
					}
				},
				VerifyEnrollment:   func(context.Context, Enrollment) error { return nil },
				VerifyDispatch:     func(context.Context, string, Proof, time.Time) error { return nil },
				AuthorizeCleanup:   func(context.Context, string, Proof, CleanupLease) error { return errDenied },
				AcknowledgeClosure: func(context.Context, string, Proof, CleanupLease, Receipt) error { return errDenied },
				AuthorizeOwnerClosure: func(_ context.Context, consumer string, p Proof) error {
					if denyAuthority.Load() || consumer != "fixture-consumer" || p.Worker != ownerWorker || p.Token != "fixture-proof" {
						return errDenied
					}
					return nil
				},
				AcknowledgeOwnerClosure: func(_ context.Context, _ string, _ Proof, r Receipt) error {
					if r.Phase != "closed" || !closed(r.Lifetime) {
						return errDenied
					}
					acks.Add(1)
					return nil
				},
			}
			if mode == "paused" {
				cfg.IOTimeout = time.Second
				cfg.ProviderReadIdle = 2 * time.Second
			}
			if mode == "steadyBeyondIO" {
				cfg.IOTimeout = time.Second
				cfg.ProviderReadIdle = 750 * time.Millisecond
			}
			h, err := New(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(h)
			defer server.Close()
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = h.Stop(ctx)
			}()
			client := &http.Client{Timeout: 3 * time.Second}
			call := func(path, token string, input any) (int, Receipt) {
				t.Helper()
				result := ownerPOST(client, server.URL, path, token, input)
				if result.err != nil {
					t.Fatal("private control response failed", result.err)
				}
				return result.code, result.receipt
			}
			input := ownerRequest()
			input.Descriptor, err = service.GatewayNativeDescriptor(row)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan ownerHTTPResult, 1)
			go func() { done <- ownerPOST(client, server.URL, "", fixtureExecutionToken, input) }()
			var proof Proof
			select {
			case proof = <-proofCh:
			case <-time.After(3 * time.Second):
				t.Fatal("actual admit missing")
			}
			select {
			case <-providerEntered:
			case <-time.After(3 * time.Second):
				t.Fatal("actual provider entry missing")
			}
			request := ownerClosureRequest{proof}
			if code, _ := call("/read-owner", fixtureCleanupToken, request); code != 403 {
				t.Fatal("delegate entered owner route")
			}
			if code, _ := call("/read-owner", fixtureForeignToken, request); code != 409 {
				t.Fatal("foreign consumer read original transport")
			}
			for _, mutate := range []func(*Proof){
				func(p *Proof) { p.ExecutionRef = ownerIssuer },
				func(p *Proof) { p.RequestRef = "fixture-foreign" },
				func(p *Proof) { p.Worker = ownerIssuer },
				func(p *Proof) { p.Token = "fixture-foreign" },
				func(p *Proof) { p.Native.OriginRef = "fixture-foreign" },
				func(p *Proof) { p.Native.EngineIncarnation = ownerIssuer },
				func(p *Proof) { p.Native.Generation = ownerIssuer },
				func(p *Proof) { p.Native.RequestNonce = ownerWorker },
			} {
				wrong := request
				mutate(&wrong.Proof)
				if code, _ := call("/ack-owner", fixtureExecutionToken, wrong); code != 409 {
					t.Fatal("foreign original/native proof acknowledged")
				}
			}
			if code, _ := call("/ack-owner", fixtureExecutionToken, cleanupRequest{Proof: proof}); code != 400 {
				t.Fatal("owner path accepted delegated wire")
			}
			if code, _ := call("/ack", fixtureExecutionToken, cleanupRequest{Proof: proof}); code != 403 {
				t.Fatal("owner entered delegated route")
			}
			if mode == "held" {
				if code, _ := call("/ack-owner", fixtureExecutionToken, request); code != 409 || acks.Load() != 0 {
					t.Fatal("unclosed transport acknowledged")
				}
				if code, _ := call("/cancel-owner", fixtureExecutionToken, request); code != 202 {
					t.Fatal("original cancel failed")
				}
			}
			if mode == "idleBlockedClose" {
				select {
				case <-closeStarted:
				case <-time.After(2 * time.Second):
					t.Fatal("idle did not cancel then attempt Close")
				}
				code, retained := call("/read-owner", fixtureExecutionToken, request)
				if code != 202 || retained.Phase == "closed" || retained.Effect != "effect_unknown" ||
					!retained.Lifetime.ContextDone || retained.Lifetime.BodyClosed || retained.Lifetime.ForwardingReturned || retained.Lifetime.Completed || retained.Acknowledged {
					t.Fatal("blocked Close invented closure/completion", retained)
				}
				if code, _ := call("/ack-owner", fixtureExecutionToken, request); code != 409 || acks.Load() != 0 {
					t.Fatal("blocked Close released occupancy")
				}
				call("", fixtureExecutionToken, input)
				if entries.Load() != 1 || admits.Load() != 1 {
					t.Fatal("blocked Close reentered provider")
				}
				select {
				case <-done:
					t.Fatal("forward returned before physical Close")
				default:
				}
				releaseClose.Do(func() { close(closeGate) })
			}
			select {
			case result := <-done:
				if (mode == "completed" || mode == "paused" || mode == "steadyBeyondIO") && (result.err != nil || result.code != 200) {
					t.Fatal("actual completed stream failed", result.code, result.err)
				}
				if (mode == "paused" || mode == "steadyBeyondIO") && (result.duration <= cfg.IOTimeout || !bytes.Contains(result.body, []byte("response.completed"))) {
					t.Fatal("stream failed to deliver completion beyond one write I/O timeout", result.duration)
				}
				if strings.HasPrefix(mode, "idle") && bytes.Contains(result.body, []byte("response.completed")) {
					t.Fatal("read-idle delivered a false completed event")
				}
				if strings.HasPrefix(mode, "idle") {
					// Distinguish the native 1s read-idle from this client's
					// independent 3s timeout and the handler's 5s I/O deadline.
					if result.duration < 800*time.Millisecond || result.duration >= 2*time.Second || !bytes.Contains(result.body, []byte("response.created")) {
						t.Fatal("partial stream did not return at native read-idle before client/absolute timeout", result.duration)
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("native forward did not return")
			}
			select {
			case <-providerClosed:
			case <-time.After(time.Second):
				t.Fatal("native lifetime did not close live provider body")
			}
			code, receipt := call("/read-owner", fixtureExecutionToken, request)
			settleBy := time.Now().Add(2 * time.Second)
			for {
				if code != 202 || receipt.Proof == nil || *receipt.Proof != proof {
					t.Fatal("original receipt unavailable")
				}
				if receipt.Lifetime.ForwardingReturned && (receipt.Lifetime.BodyClosed || receipt.Lifetime.CloseFailed) {
					break
				}
				if time.Now().After(settleBy) {
					t.Fatal("native physical lifetime did not settle")
				}
				time.Sleep(10 * time.Millisecond)
				code, receipt = call("/read-owner", fixtureExecutionToken, request)
			}
			if (mode == "closeFailed" || mode == "idleCloseFailed") && !receipt.Lifetime.CloseFailed {
				t.Fatal("physical Close failure observation lost")
			}
			if strings.HasPrefix(mode, "idle") {
				if receipt.Effect != "effect_unknown" || receipt.Lifetime.Completed || receipt.Acknowledged || acks.Load() != 0 || !receipt.Lifetime.ContextDone {
					t.Fatal("read-idle changed unknown effect or acknowledged occupancy before closure ACK")
				}
				if mode == "idleCloseFailed" && receipt.Phase == "closed" {
					t.Fatal("failed Close certified closure after read-idle")
				}
			}
			code, receipt = call("/ack-owner", fixtureExecutionToken, request)
			if mode == "closeFailed" || mode == "idleCloseFailed" {
				if code != 409 || acks.Load() != 0 {
					t.Fatal("failed physical Close acknowledged")
				}
			} else {
				if code != 202 || !receipt.Acknowledged || receipt.Phase != "closed" || acks.Load() != 1 {
					t.Fatal("original closed proof not acknowledged")
				}
				if (mode == "completed" || mode == "paused" || mode == "steadyBeyondIO") && (receipt.Effect != "completed" || !receipt.Lifetime.Completed) {
					t.Fatal("completed native outcome lost")
				}
				if code, _ := call("/ack-owner", fixtureExecutionToken, request); code != 202 {
					t.Fatal("exact original ACK not idempotent")
				}
			}
			call("", fixtureExecutionToken, input)
			if entries.Load() != 1 || admits.Load() != 1 {
				t.Fatal("closure reentered provider or issued another claim")
			}
			denyAuthority.Store(true)
			if code, _ := call("/read-owner", fixtureExecutionToken, request); code != 409 {
				t.Fatal("current owner authority denial bypassed by retained receipt")
			}
		})
	}
}

func TestOwnerClosureMissingPortsAndSealedLostAck(t *testing.T) {
	h := ownerRegistry()
	e := ownerReserve(t, h, ownerRequest())
	p := Proof{ExecutionRef: ownerExecution, RequestRef: e.request.RequestRef, Worker: ownerWorker, Token: "fixture-original", Native: e.native}
	if _, err := h.ownerClosure(context.Background(), "fixture-consumer", "read-owner", p); err == nil {
		t.Fatal("missing authority allowed owner closure")
	}
	h.cfg.AuthorizeOwnerClosure = func(context.Context, string, Proof) error { return nil }
	if _, err := h.ownerClosure(context.Background(), "fixture-consumer", "read-owner", p); err == nil || e.proof != nil {
		t.Fatal("unsealed reservation attached proof")
	}
	h.seal(e)
	e.life.NoForward()
	r, err := h.ownerClosure(context.Background(), "fixture-consumer", "read-owner", p)
	if err != nil || r.Proof == nil || *r.Proof != p || r.Lifetime.Entered || e.life.Admit(time.Now().Add(time.Minute)) {
		t.Fatal("lost ACK owner read rearmed dispatch")
	}
	changed := p
	changed.Token = "fixture-replacement"
	if _, err := h.ownerClosure(context.Background(), "fixture-consumer", "read-owner", changed); err == nil {
		t.Fatal("recovered original proof was replaced")
	}
	if _, err := h.ownerClosure(context.Background(), "fixture-consumer", "ack-owner", p); err == nil {
		t.Fatal("missing ACK authority allowed closure")
	}
}

// Controlled storage plus real private HTTP, callback and TLS provider. No DB
// claim/occupancy qualification is inferred from these boundary tests.
func ownerIngressFixture(t *testing.T, provider http.Handler, configure ...func(*Config, *ownerFixtureRepo)) (*Handler, *httptest.Server, *atomic.Int32, <-chan Proof) {
	t.Helper()
	upstream := httptest.NewTLSServer(provider)
	t.Cleanup(upstream.Close)
	custody, err := service.NewGatewayNativeCredentialCustody("fixture", map[string][]byte{"fixture": []byte(strings.Repeat("K", 32))})
	if err != nil {
		t.Fatal(err)
	}
	scope := service.GatewayNativeCredentialScope{Consumer: "fixture-consumer", Owner: "fixture-owner", Account: ownerAccount, Generation: ownerGeneration, Purpose: service.GatewayCredentialPurpose}
	envelope, err := custody.Seal(scope, fixtureProviderKey)
	if err != nil {
		t.Fatal(err)
	}
	row := &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, CreatedAt: ownerRequest().Descriptor.CreatedAt,
		Credentials: map[string]any{"api_key": envelope, "base_url": upstream.URL}, Extra: map[string]any{
			service.GatewayGenerationExtraKey: ownerGeneration, service.GatewayProfileExtraKey: service.GatewayMiMoResponsesProfile,
			service.GatewayModelExtraKey: "fixture-model", service.GatewayCredentialScopeExtraKey: scope.Metadata(),
			"openai_responses_mode": "force_responses", "openai_passthrough": true,
			"native_api_key_cancel_on_disconnect": true, "openai_preserve_compatible_reasoning": true}}
	u, err := service.NewGatewayNativeLifetimeUpstream(&ownerFixtureHTTP{client: upstream.Client()})
	if err != nil {
		t.Fatal(err)
	}
	repo := &ownerFixtureRepo{row: row}
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil,
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		nil, nil, nil, nil, nil, u, nil, nil, nil, nil, nil, nil, nil, nil)
	admits := &atomic.Int32{}
	proofs := make(chan Proof, fixtureExecutionAdmissionCap)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admits.Add(1)
		var input admitRequest
		if r.URL.Path != "/private/native/v1/admit" || r.Header.Get("Authorization") != "Bearer "+fixtureCallbackToken || json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("invalid authenticated callback")
			w.WriteHeader(400)
			return
		}
		reply := ownerReply(input)
		proofs <- reply.Dispatch.Closure
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(callback.Close)
	cfg := Config{Gateway: gateway, Custody: custody, Enrollment: Enrollment{"fixture-origin", ownerIncarnation, "fixture-qualification"},
		Profile: QualifiedProfile{Profile: service.GatewayMiMoResponsesProfile, Model: "fixture-model", BaseURL: upstream.URL, QualificationRef: "fixture-qualification",
			RequestBytes: 4096, OutputBytes: 8 << 20, Tokens: 100, ProviderTokenUpperBound: 200},
		MaxEntries: 4, EnvelopeBytes: 8192, CallbackBytes: 65536, CallbackOrigin: callback.URL, CallbackCredential: fixtureCallbackToken,
		CallbackTimeout: time.Second, IOTimeout: 15 * time.Second, CleanupTimeout: time.Second,
		Authorize: func(r *http.Request) (Peer, error) {
			switch r.Header.Get("Authorization") {
			case "Bearer " + fixtureExecutionToken:
				return Peer{"fixture-consumer", "execution"}, nil
			case "Bearer " + fixtureCleanupToken:
				return Peer{"fixture-consumer", "cleanup"}, nil
			case "Bearer " + gatewayIngressGuardFixture1:
				return Peer{"fixture-consumer", "management"}, nil
			}
			return Peer{}, errDenied
		},
		VerifyEnrollment:      func(context.Context, Enrollment) error { return nil },
		VerifyDispatch:        func(context.Context, string, Proof, time.Time) error { return nil },
		AuthorizeCleanup:      func(context.Context, string, Proof, CleanupLease) error { return errDenied },
		AcknowledgeClosure:    func(context.Context, string, Proof, CleanupLease, Receipt) error { return errDenied },
		AuthorizeOwnerClosure: func(context.Context, string, Proof) error { return nil },
		AcknowledgeOwnerClosure: func(_ context.Context, _ string, _ Proof, r Receipt) error {
			if !closed(r.Lifetime) {
				return errDenied
			}
			return nil
		},
	}
	for _, configure := range configure {
		configure(&cfg, repo)
	}
	h, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(h)
	// Real kernel socket bounds make downstream backpressure deterministic;
	// this is fixture listener configuration, not a production handler hook.
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			if err := conn.(*net.TCPConn).SetWriteBuffer(1024); err != nil {
				t.Error(err)
			}
		}
	}
	server.Start()
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = h.Stop(ctx)
	})
	return h, server, admits, proofs
}

// Expect: 100-continue observes the REAL first request body Read. No body is
// supplied, so accepted handlers remain blocked before decode/reserve/callback.
func ownerHeldBody(t *testing.T, origin, path, token string, want int) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(origin, "http://"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, err = fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: fixture\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: 2\r\nExpect: 100-continue\r\n\r\n", transportsPath+path, token)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "POST"})
	if err != nil {
		t.Fatal("held body did not receive immediate admission decision", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("held %s: got %d want %d", path, resp.StatusCode, want)
	}
	_ = conn.SetDeadline(time.Time{})
	return conn
}

// Regression: body buffering precedes execution admission, so authenticated
// slow bodies exhaust the listener and cleanup cannot get a bounded response.
func TestGatewayNativeIngressHeldBodiesReserveControlAndReleaseOnAbort(t *testing.T) {
	var upstreams atomic.Int32
	h, server, admits, _ := ownerIngressFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreams.Add(1)
		w.WriteHeader(500)
	}))
	var held []net.Conn
	for i := 0; i < fixtureExecutionAdmissionCap; i++ {
		held = append(held, ownerHeldBody(t, server.URL, "", fixtureExecutionToken, 100))
	}
	ownerHeldBody(t, server.URL, "", fixtureExecutionToken, 503)
	// Wrong-purpose credentials remain forbidden, including at saturation.
	ownerHeldBody(t, server.URL, "/read", fixtureExecutionToken, 403)
	for i := 0; i < fixtureControlAdmissionCap; i++ {
		ownerHeldBody(t, server.URL, "/read", fixtureCleanupToken, 100)
	}
	ownerHeldBody(t, server.URL, "/read", fixtureCleanupToken, 503)
	ownerHeldBody(t, server.URL, "/read-owner", fixtureExecutionToken, 503)
	management := httptest.NewServer(h.ManagementHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("saturated management entered handler")
	})))
	defer management.Close()
	req, _ := http.NewRequest("POST", management.URL, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+gatewayIngressGuardFixture1)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatal("management did not share the bounded reserved control budget")
	}
	// Failure: the new OAuth cleanup mount could allocate separate unlimited
	// control capacity, or accept management/execution role as cleanup authority.
	cleanup := httptest.NewServer(h.CleanupHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("saturated OAuth cleanup entered handler") })))
	defer cleanup.Close()
	for _, tc := range []struct {
		token string
		want  int
	}{{fixtureCleanupToken, 503}, {fixtureExecutionToken, 403}, {gatewayIngressGuardFixture1, 403}} {
		req, _ := http.NewRequest("POST", cleanup.URL, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		response, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != tc.want {
			t.Fatalf("cleanup purpose/control budget status %d, want %d", response.StatusCode, tc.want)
		}
	}

	_ = held[0].Close()
	by := time.Now().Add(2 * time.Second)
	for {
		result := ownerPOST(&http.Client{Timeout: time.Second}, server.URL, "", fixtureExecutionToken, map[string]string{})
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.code == 400 {
			break // Invalid envelope is read only AFTER the aborted slot releases.
		}
		if result.code != 503 || time.Now().After(by) {
			t.Fatal("aborted held execution did not release its handler slot", result.code)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if admits.Load() != 0 || upstreams.Load() != 0 || len(h.entries) != 0 {
		t.Fatal("pre-body saturation created callback/claim/upstream or retained occupancy")
	}
}

// Regression: releasing execution admission after body decode/first flush lets
// blocked downstream output multiply executions and starve exact cleanup.
func TestGatewayNativeIngressBlockedOutputRetainsSlotAndAllowsCleanup(t *testing.T) {
	var upstreams atomic.Int32
	providerReady := make(chan struct{}, 1)
	h, server, admits, proofs := ownerIngressFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreams.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		frame := ": " + strings.Repeat("x", 16380) + "\n\n"
		// More than the unread downstream TCP window; remain alive without a
		// terminal event. Cleanup must interrupt the actual blocked HTTP writer.
		for i := 0; i < 440; i++ {
			if _, err := io.WriteString(w, frame); err != nil {
				return
			}
			if http.NewResponseController(w).Flush() != nil {
				return
			}
			if i == 64 {
				providerReady <- struct{}{}
			}
		}
		<-r.Context().Done()
	}))
	input := ownerRequest()
	input.Descriptor.BaseURL = h.cfg.Profile.BaseURL
	input.Admission.Limits.OutputBytes = 8 << 20
	raw, _ := json.Marshal(input)
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(server.URL, "http://"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.(*net.TCPConn).SetReadBuffer(1024); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: fixture\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", transportsPath, fixtureExecutionToken, len(raw), raw)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "POST"})
	if err != nil || response.StatusCode != 200 {
		t.Fatal("live private stream did not start", err)
	}
	// Leave response.Body unread throughout saturation and cleanup.
	var proof Proof
	select {
	case proof = <-proofs:
	case <-time.After(2 * time.Second):
		t.Fatal("sole authenticated admit missing")
	}
	select {
	case <-providerReady:
	case <-time.After(3 * time.Second):
		t.Fatal("controlled provider did not fill unread downstream output")
	}
	for i := 1; i < fixtureExecutionAdmissionCap; i++ {
		ownerHeldBody(t, server.URL, "", fixtureExecutionToken, 100)
	}
	ownerHeldBody(t, server.URL, "", fixtureExecutionToken, 503)
	client := &http.Client{Timeout: 2 * time.Second}
	request := ownerClosureRequest{Proof: proof}
	read := ownerPOST(client, server.URL, "/read-owner", fixtureExecutionToken, request)
	if read.err != nil || read.code != 202 || read.receipt.Phase != "entered" || read.receipt.Lifetime.ForwardingReturned {
		t.Fatal("blocked output released lifetime or starved control", read.code, read.err, read.receipt)
	}
	ack := ownerPOST(client, server.URL, "/ack-owner", fixtureExecutionToken, request)
	if ack.err != nil || ack.code != 409 {
		t.Fatal("blocked output falsely acknowledged closure")
	}
	cancel := ownerPOST(client, server.URL, "/cancel-owner", fixtureExecutionToken, request)
	if cancel.err != nil || cancel.code != 202 {
		t.Fatal("reserved control could not cancel blocked writer")
	}
	by := time.Now().Add(2 * time.Second)
	for {
		read = ownerPOST(client, server.URL, "/read-owner", fixtureExecutionToken, request)
		if read.err != nil || read.code != 202 {
			t.Fatal("exact closure readback unavailable")
		}
		if read.receipt.Phase == "closed" {
			break
		}
		if time.Now().After(by) {
			t.Fatal("cancellation did not interrupt the blocked writer and close provider body")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if read.receipt.Effect != "effect_unknown" || read.receipt.Acknowledged || admits.Load() != 1 || upstreams.Load() != 1 {
		t.Fatal("local handler cleanup certified durable occupancy/effect or replayed inference")
	}
	// Closed but unacknowledged evidence stays retained after the HTTP slot frees.
	if len(h.entries) != 1 {
		t.Fatal("handler release evicted unacknowledged closure")
	}
	result := ownerPOST(client, server.URL, "", fixtureExecutionToken, map[string]string{})
	if result.err != nil || result.code != 400 {
		t.Fatal("forward return did not release execution slot", result.code, result.err)
	}
}

func gatewayIngressGuardFixtureOpaque() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("synthetic fixture entropy unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

var gatewayIngressGuardFixture1 = gatewayIngressGuardFixtureOpaque()

// Only canonical mapping is controlled here. Actual encrypted credential
// boundary, signed JWKS, explicit F2 version update and TLS inference run.
type oauthTransportRows struct {
	service.AccountRepository
	row      *service.Account
	physical service.GatewayNativeOAuthPhysical
	mode     string
	resolves int
	admitted *atomic.Int32
}

func (r *oauthTransportRows) LockGatewayNativeAccount(context.Context, int64) (*service.Account, func(), error) {
	return nil, nil, errDenied
}
func (r *oauthTransportRows) ReadGatewayNativeOAuthDispatch(_ context.Context, s service.GatewayNativeCredentialScope, op string) (service.GatewayNativeOAuthOutcome, error) {
	return service.GatewayNativeOAuthOutcome{Operation: op, AccountID: r.row.ID, Generation: s.Generation, State: "completed"}, nil
}
func (r *oauthTransportRows) LockGatewayNativeOAuthDispatch(ctx context.Context, s service.GatewayNativeCredentialScope, op string, id int64) (*service.Account, service.GatewayNativeOAuthPhysical, func(), error) {
	if !service.GatewayNativeOAuthScopeAuthorized(ctx, s) || s != r.physical.Scope || op != r.physical.Operation || id != r.row.ID {
		return nil, service.GatewayNativeOAuthPhysical{}, nil, errDenied
	}
	p := r.physical
	p.RefreshFence = r
	return r.row, p, func() {}, nil
}
func (r *oauthTransportRows) Check(context.Context) error { return nil }
func (r *oauthTransportRows) QualifyGatewayNativeOAuthDispatch(_ context.Context, p service.GatewayNativeOAuthPhysical, version int64) error {
	if !p.QualificationValid(version) {
		return errDenied
	}
	r.physical = p
	return nil
}
func (r *oauthTransportRows) ResolveGatewayNativeOAuthCanonical(ctx context.Context, route service.GatewayNativeRoute, account string) (service.GatewayNativeCredentialScope, string, error) {
	r.resolves++
	consumer, err := service.GatewayNativeConsumer(ctx)
	if err != nil || r.admitted.Load() != 1 || consumer != "fixture-consumer" || account != ownerAccount || !service.SameGatewayNativeDescriptor(route, r.physical.Route) {
		return service.GatewayNativeCredentialScope{}, "", errDenied
	}
	scope := r.physical.Scope
	switch r.mode {
	case "zero", "multi":
		return scope, "", errDenied
	case "foreign":
		scope.Consumer = "foreign"
	case "wrong-owner":
		scope.Owner = "foreign-owner"
	}
	return scope, r.physical.Operation, nil
}
func (r *oauthTransportRows) PrepareGatewayNativeOAuthRefresh(_ context.Context, in service.GatewayNativeOAuthRefreshIntent) (service.GatewayNativeOAuthRefreshPrepared, error) {
	return service.GatewayNativeOAuthRefreshPrepared{Outcome: service.GatewayNativeOAuthRefreshOutcome{Operation: in.Operation, State: "prepared"}, Fence: 1, Deadline: time.Now().Add(time.Second), Claimed: true, Envelope: r.row.GetCredential("oauth_bundle"), Issuer: r.physical.Issuer, Subject: r.physical.Subject}, nil
}
func (*oauthTransportRows) EnterGatewayNativeOAuthRefresh(context.Context, service.GatewayNativeOAuthRefreshIntent, int64) (bool, error) {
	return true, nil
}
func (r *oauthTransportRows) CompleteGatewayNativeOAuthRefresh(_ context.Context, in service.GatewayNativeOAuthRefreshIntent, _ int64, envelope string) (service.GatewayNativeOAuthRefreshOutcome, error) {
	r.row.Credentials = map[string]any{"oauth_bundle": envelope, "credential_version": "2"}
	return service.GatewayNativeOAuthRefreshOutcome{Operation: in.Operation, State: "completed", Version: 2}, nil
}
func (*oauthTransportRows) UnknownGatewayNativeOAuthRefresh(context.Context, service.GatewayNativeOAuthRefreshIntent, int64) error {
	return errDenied
}

func TestProtectedOAuthTransportPostAdmitCanonicalBinding(t *testing.T) {
	for _, mode := range []string{"valid", "dynamic", "unknown-model", "unsupported-effort", "raw-ultra", "legacy-raw-ultra", "disabled", "disabled-field", "zero", "multi", "foreign", "wrong-owner", "forged-descriptor"} {
		t.Run(mode, func(t *testing.T) {
			var entries, admits, refreshes, checks atomic.Int32
			dynamicCatalog := mode == "dynamic" || mode == "unknown-model" || mode == "unsupported-effort" || mode == "raw-ultra"
			disabledCatalog := mode == "disabled" || mode == "disabled-field"
			success := mode == "valid" || mode == "dynamic" || mode == "disabled"
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			require.NoError(t, err)
			subject := uuid.NewString()
			providerID := uuid.NewString()
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": service.GatewayOAuthIssuer, "sub": subject, "aud": "app_EMoamEEZ73f0CkXaXp7hrann", "exp": time.Now().Add(time.Hour).Unix()})
			token.Header["kid"] = "fixture"
			signed, err := token.SignedString(key)
			require.NoError(t, err)
			access := uuid.NewString()
			nextAccess := uuid.NewString()
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/.well-known/jwks.json":
					require.Equal(t, "auth.openai.com", r.Host)
					_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kid": "fixture", "kty": "RSA", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
				case "/backend-api/accounts/check/v4-2023-04-27":
					checks.Add(1)
					_, _ = fmt.Fprintf(w, `{"accounts":{"irrelevant-key":{"account":{"account_id":%q}}}}`, providerID)
				case "/oauth/token":
					refreshes.Add(1)
					require.Equal(t, "auth.openai.com", r.Host)
					_ = json.NewEncoder(w).Encode(map[string]any{"access_token": nextAccess, "refresh_token": uuid.NewString(), "id_token": signed, "token_type": "Bearer", "expires_in": 3600})
				case "/backend-api/codex/models":
					if dynamicCatalog {
						_, _ = io.WriteString(w, `{"models":[{"slug":"unseen-native-model","supported_in_api":false,"visibility":"list","supported_reasoning_levels":["ultra","max"],"default_reasoning_level":"ultra"}]}`)
					} else if disabledCatalog {
						_, _ = io.WriteString(w, `{"models":[{"slug":"unseen-native-model","supported_reasoning_levels":[],"default_reasoning_level":null}]}`)
					} else {
						w.WriteHeader(http.StatusServiceUnavailable)
					}
				case "/backend-api/codex/responses":
					entries.Add(1)
					body, readErr := io.ReadAll(r.Body)
					require.NoError(t, readErr)
					if mode == "dynamic" {
						require.Contains(t, string(body), `"reasoning":{"effort":"max"}`)
						require.NotContains(t, string(body), `"ultra"`)
					}
					if mode == "disabled" {
						require.Contains(t, string(body), `"reasoning":{}`)
					}
					require.Equal(t, "chatgpt.com", r.Host)
					require.Equal(t, "Bearer "+nextAccess, r.Header.Get("Authorization"))
					require.Equal(t, providerID, r.Header.Get("ChatGPT-Account-Id"))
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"custom_tool_call\",\"call_id\":\"call_fixture\",\"name\":\"exec\",\"input\":\"printf fixture\"}}\n\n")
					_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"controlled transport final\"}]}]}}\n\n")
				default:
					t.Errorf("unexpected OAuth route %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer upstream.Close()
			provider := upstream.Client().Transport.(*http.Transport).Clone()
			provider.DisableKeepAlives = true
			provider.TLSClientConfig.ServerName = "example.com"
			provider.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != "auth.openai.com:443" && address != "chatgpt.com:443" {
					return nil, errDenied
				}
				return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
			}
			defer provider.CloseIdleConnections()
			custody, err := service.NewGatewayNativeCredentialCustody("fixture", map[string][]byte{"fixture": []byte(strings.Repeat("K", 32))})
			require.NoError(t, err)
			scope := service.GatewayNativeCredentialScope{Consumer: "fixture-consumer", Owner: "saved-server-owner", Account: ownerAccount, Generation: ownerGeneration, Purpose: service.GatewayOAuthBundlePurpose}
			envelope, err := custody.SealOAuthBundle(scope, service.GatewayNativeOAuthBundle{AccessToken: access, RefreshToken: uuid.NewString(), IDToken: signed, SensitiveMetadata: json.RawMessage(`{"expires_in":3600}`)})
			require.NoError(t, err)
			birth := time.Now().UTC().Truncate(time.Microsecond)
			row := &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusDisabled, CreatedAt: birth, Credentials: map[string]any{"oauth_bundle": envelope}, Extra: map[string]any{service.GatewayGenerationExtraKey: scope.Generation, service.GatewayProfileExtraKey: service.GatewayOAuthStagingProfile, service.GatewayCredentialScopeExtraKey: scope.Metadata()}}
			physical := service.GatewayNativeOAuthPhysical{Scope: scope, Operation: "saved-private-connect-operation", Route: service.GatewayNativeRoute{AccountID: 7, Generation: scope.Generation, CreatedAt: birth, Profile: service.GatewayCodexOAuthResponsesProfile, BaseURL: service.GatewayCodexOAuthBaseURL, Model: service.GatewayCodexOAuthModel}, Issuer: service.GatewayOAuthIssuer, Subject: subject}
			rows := &oauthTransportRows{row: row, physical: physical, mode: mode, admitted: &admits}
			verifier := service.NewGatewayNativeOAuthVerifier(provider)
			oauth, err := service.NewGatewayNativeOAuthDispatch(rows, custody, verifier, provider)
			require.NoError(t, err)
			ownerCtx, err := service.WithGatewayNativeConsumer(context.Background(), scope.Consumer)
			require.NoError(t, err)
			ownerCtx, err = service.WithGatewayNativeOAuthOwner(ownerCtx, scope.Owner)
			require.NoError(t, err)
			descriptor, err := oauth.Descriptor(ownerCtx, scope, physical.Operation)
			require.NoError(t, err)
			require.NotNil(t, descriptor.Native)
			refresh, err := service.NewGatewayNativeOAuthRefresh(rows, custody, verifier, provider)
			require.NoError(t, err)
			if success {
				out, err := refresh.Refresh(ownerCtx, service.GatewayNativeOAuthRefreshIntent{Scope: scope, AccountID: 7, CreatedAt: birth, ExpectedVersion: 1, Operation: uuid.NewString(), Intent: uuid.NewString()})
				require.NoError(t, err)
				require.EqualValues(t, 2, out.Version)
			}

			ownedUpstream, err := service.NewGatewayNativeLifetimeUpstream(&ownerFixtureHTTP{client: &http.Client{Transport: provider}})
			require.NoError(t, err)
			gateway := service.NewOpenAIGatewayService(rows, nil, nil, nil, nil, nil, nil, &config.Config{}, nil, nil, nil, nil, nil, ownedUpstream, nil, nil, nil, nil, nil, nil, nil, nil)
			callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				admits.Add(1)
				var input admitRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
				reply := ownerReply(input)
				canonical := physical.Route
				reply.NativeDescriptor = &canonical
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(reply)
			}))
			defer callback.Close()
			cfg := Config{Gateway: gateway, Custody: custody, OAuth: oauth, Enrollment: Enrollment{"fixture-origin", ownerIncarnation, "fixture-qualification"},
				Profile:    QualifiedProfile{Profile: service.GatewayMiMoResponsesProfile, Model: "fixture-model", BaseURL: "https://fixture.invalid", QualificationRef: "fixture-qualification", RequestBytes: 2048, OutputBytes: 3072, Tokens: 50, ProviderTokenUpperBound: 80},
				Codex:      &QualifiedProfile{Profile: service.GatewayCodexOAuthResponsesProfile, Model: service.GatewayCodexOAuthModel, BaseURL: service.GatewayCodexOAuthBaseURL, QualificationRef: "codex-fixture-qualification", RequestBytes: 4096, OutputBytes: 4096, Tokens: 100, ProviderTokenUpperBound: 200},
				MaxEntries: 4, EnvelopeBytes: 8192, CallbackBytes: 65536, CallbackOrigin: callback.URL, CallbackCredential: fixtureCallbackToken, CallbackTimeout: time.Second, IOTimeout: 5 * time.Second, CleanupTimeout: time.Second, Authorize: func(r *http.Request) (Peer, error) {
					if r.Header.Get("Authorization") != "Bearer "+fixtureExecutionToken {
						return Peer{}, errDenied
					}
					return Peer{"fixture-consumer", "execution"}, nil
				}, VerifyEnrollment: func(context.Context, Enrollment) error { return nil }, VerifyDispatch: func(context.Context, string, Proof, time.Time) error { return nil }, AuthorizeCleanup: func(context.Context, string, Proof, CleanupLease) error { return errDenied }, AcknowledgeClosure: func(context.Context, string, Proof, CleanupLease, Receipt) error { return errDenied }}
			for _, bad := range []QualifiedProfile{{Profile: "arbitrary-profile", Model: service.GatewayCodexOAuthModel, BaseURL: service.GatewayCodexOAuthBaseURL}, {Profile: service.GatewayCodexOAuthResponsesProfile, Model: "wrong-model", BaseURL: service.GatewayCodexOAuthBaseURL}, {Profile: service.GatewayCodexOAuthResponsesProfile, Model: service.GatewayCodexOAuthModel, BaseURL: "https://caller.invalid"}} {
				wrong := cfg
				wrong.Codex = &bad
				_, err := New(context.Background(), wrong)
				require.Error(t, err)
			}
			handler, err := New(context.Background(), cfg)
			require.NoError(t, err)
			server := httptest.NewServer(handler)
			defer server.Close()
			defer handler.Stop(context.Background())
			input := ownerRequest()
			input.Descriptor = *descriptor.Native
			input.Admission.ProfileID = service.GatewayCodexOAuthResponsesProfile
			input.Payload = json.RawMessage(fmt.Sprintf(`{"model":%q,"input":"controlled","store":false,"stream":true,"service_tier":"default","tools":[{"type":"custom","name":"exec","format":{"type":"text"}}]}`, service.GatewayCodexOAuthModel))
			if dynamicCatalog {
				selected, effort := "unseen-native-model", "max"
				if mode == "unknown-model" {
					selected = "not-in-catalog"
				}
				if mode == "unsupported-effort" {
					effort = "high"
				}
				if mode == "raw-ultra" {
					effort = "ultra"
				}
				input.Payload = json.RawMessage(fmt.Sprintf(`{"model":%q,"reasoning":{"effort":%q},"input":"controlled","store":false,"stream":true,"service_tier":"default","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]}`, selected, effort))
			}
			if disabledCatalog {
				reasoning := "{}"
				if mode == "disabled-field" {
					reasoning = `{"summary":"none"}`
				}
				input.Payload = json.RawMessage(fmt.Sprintf(`{"model":"unseen-native-model","reasoning":%s,"input":"controlled","store":false,"stream":true,"service_tier":"default"}`, reasoning))
			}
			if mode == "legacy-raw-ultra" {
				input.Payload = json.RawMessage(strings.TrimSuffix(string(input.Payload), "}") + `,"reasoning":{"effort":"ultra"}}`)
			}
			if mode == "forged-descriptor" {
				input.Descriptor.CreatedAt = input.Descriptor.CreatedAt.Add(time.Microsecond)
			}
			result := ownerPOST(server.Client(), server.URL, "", fixtureExecutionToken, input)
			require.NoError(t, result.err)
			require.EqualValues(t, 1, admits.Load())
			if success {
				require.EqualValues(t, 1, entries.Load())
				require.EqualValues(t, 1, refreshes.Load())
				require.Equal(t, "2", rows.row.GetCredential("credential_version"))
				require.True(t, service.SameGatewayNativeDescriptor(*descriptor.Native, rows.physical.Route))
				require.Contains(t, string(result.body), "custom_tool_call")
				require.Contains(t, string(result.body), "controlled transport final")
			} else {
				require.Zero(t, entries.Load())
				require.NotContains(t, string(result.body), "controlled transport final")
			}
			require.EqualValues(t, 1, checks.Load(), "invocation must not run qualification probe")
			if mode == "forged-descriptor" {
				require.Zero(t, rows.resolves)
			} else {
				require.Equal(t, 1, rows.resolves)
			}
		})
	}
}

// Isolated maximum-copy component check. The process includes the real private
// handler/native service AND controlled HTTP clients/TLS peer. It is not an ELF,
// SQL, full memory fixture, or cgroup-enforced 2 GiB qualification. No forced GC.
func TestGatewayNativeMiMoMaximumCopy32(t *testing.T) {
	const capBytes = 4 << 20
	const count = fixtureExecutionAdmissionCap
	prefix := `{"service_tier":"default","model":"fixture-model","input":"`
	suffix := `","opaque":{"service_tier":"nested","n":9007199254740993,"v":1.2300,"s":"\u0061"},"store":false,"stream":true}`
	payload := []byte(prefix + strings.Repeat("x", capBytes-64-len(prefix)-len(suffix)) + suffix)
	originalDigest := sha256.Sum256(payload)
	// qualifyPayload adds only its approved token cap. The MiMo wire then deletes
	// only the tier span. Compute this expectation independently of both helpers.
	wire := append([]byte(nil), payload[len(`{"service_tier":"default",`):len(payload)-1]...)
	wire = append([]byte{'{'}, wire...)
	wire = append(wire, []byte(`,"max_output_tokens":100}`)...)
	wireDigest, wireBytes := sha256.Sum256(wire), len(wire)
	wire = nil
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	copyHeld := make(chan struct{}, count)
	dispatchRelease := make(chan struct{})
	var dispatchOnce sync.Once
	releaseDispatch := func() { dispatchOnce.Do(func() { close(dispatchRelease) }) }
	defer releaseDispatch()
	arrivals := make(chan error, count)
	var providerEntries atomic.Int32
	h, server, admits, proofs := ownerIngressFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerEntries.Add(1)
		hash := sha256.New()
		n, err := io.CopyBuffer(hash, io.LimitReader(r.Body, capBytes+1), make([]byte, 32<<10))
		if err == nil && (n != int64(wireBytes) || !bytes.Equal(hash.Sum(nil), wireDigest[:]) || r.URL.Path != "/v1/responses" || r.Header.Get("x-openai-internal-codex-responses-lite") != "true") {
			err = errors.New("controlled provider wire differed from approved tier-only omission")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush() // release the existing fresh-row lock at Do headers
		arrivals <- err
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n")
	}), func(cfg *Config, repo *ownerFixtureRepo) {
		// Each callback arrived after native created its tier-deleted HTTP body.
		// Hold all 32 copies at this controlled repository boundary before real Do.
		repo.beforeNativeLock = func() { copyHeld <- struct{}{}; <-dispatchRelease }
		cfg.Profile.RequestBytes = capBytes
		cfg.EnvelopeBytes = 5 << 20
		cfg.MaxEntries = count
		cfg.IOTimeout = 30 * time.Second
		cfg.ProviderReadIdle = 30 * time.Second
		cfg.CallbackTimeout = 30 * time.Second
	})
	// Capture raw observations from this combined test process. RSS is /proc/self,
	// not parent shell/compiler memory; Go observations are not external RSS.
	type observation struct {
		Milliseconds int64  `json:"milliseconds"`
		HeapAlloc    uint64 `json:"heapAlloc"`
		HeapInuse    uint64 `json:"heapInuse"`
		RSS          uint64 `json:"rss"`
	}
	var observations []observation
	var observationMu sync.Mutex
	epoch := time.Now()
	sample := func() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		stat, err := os.ReadFile("/proc/self/statm")
		if err != nil {
			t.Error("RSS observation unavailable")
			return
		}
		fields := strings.Fields(string(stat))
		if len(fields) < 2 {
			t.Error("invalid statm observation")
			return
		}
		pages, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			t.Error("invalid RSS pages")
			return
		}
		observationMu.Lock()
		observations = append(observations, observation{time.Since(epoch).Milliseconds(), m.HeapAlloc, m.HeapInuse, pages * uint64(os.Getpagesize())})
		observationMu.Unlock()
	}
	sample()
	stop, sampled := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(sampled)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sample()
			case <-stop:
				return
			}
		}
	}()
	var copyHeldAt, heldAt, drainedAt int64
	defer func() {
		close(stop)
		<-sampled
		observationMu.Lock()
		raw := append([]observation(nil), observations...)
		observationMu.Unlock()
		var peakRSS uint64
		for _, o := range raw {
			if o.RSS > peakRSS {
				peakRSS = o.RSS
			}
		}
		report := struct {
			Scope                              string `json:"scope"`
			Requests, InternalBytes, WireBytes int
			CopyHeldAt, HeldAt, DrainedAt      int64
			PeakRSS                            uint64
			CgroupLimitEnforced                bool
			Samples                            []observation
		}{"combined_native_private_http_controlled_peer_test_process", count, len(payload), wireBytes, copyHeldAt, heldAt, drainedAt, peakRSS, false, raw}
		encoded, err := json.Marshal(report)
		require.NoError(t, err)
		t.Log("MAXCOPY_RAW " + string(encoded))
		require.LessOrEqual(t, peakRSS, uint64(2<<30), "combined process observed peak exceeds existing 2 GiB risk ceiling")
	}()
	results := make(chan ownerHTTPResult, count)
	client := &http.Client{Timeout: 60 * time.Second}
	for i := 0; i < count; i++ {
		input := ownerRequest()
		input.RequestRef = fmt.Sprintf("maxcopy-%02d", i)
		input.Descriptor.BaseURL = h.cfg.Profile.BaseURL
		input.Admission.Limits.RequestBytes = capBytes
		input.Admission.Limits.Concurrency = count
		input.Admission.Limits.Requests = count
		input.Admission.ExpiresAt = time.Now().Add(90 * time.Second).UTC().Format(time.RFC3339Nano)
		input.Payload = payload
		go func() { results <- ownerPOST(client, server.URL, "", fixtureExecutionToken, input) }()
	}
	for i := 0; i < count; i++ {
		select {
		case <-copyHeld:
		case <-time.After(30 * time.Second):
			t.Fatal("32 native tier-deleted request copies not held")
		}
	}
	copyHeldAt = time.Since(epoch).Milliseconds()
	sample()
	releaseDispatch()
	for i := 0; i < count; i++ {
		select {
		case err := <-arrivals:
			require.NoError(t, err)
		case <-time.After(45 * time.Second):
			t.Fatal("32 simultaneous maximum-copy native entries not observed")
		}
	}
	require.EqualValues(t, count, admits.Load())
	require.EqualValues(t, count, providerEntries.Load())
	require.Len(t, h.executions, count, "actual HTTP execution admission must stay occupied")
	// A 33rd valid auth request must be rejected before its body is read/callback.
	excess := ownerHeldBody(t, server.URL, "", fixtureExecutionToken, http.StatusServiceUnavailable)
	_ = excess.Close()
	require.EqualValues(t, count, admits.Load())
	heldAt = time.Since(epoch).Milliseconds()
	sample()
	unblock()
	for i := 0; i < count; i++ {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			require.Equal(t, http.StatusOK, result.code)
			require.Contains(t, string(result.body), `"status":"completed"`)
		case <-time.After(10 * time.Second):
			t.Fatal("normal native forwarding did not drain")
		}
	}
	for i := 0; i < count; i++ {
		proof := <-proofs
		read := ownerPOST(client, server.URL, "/read-owner", fixtureExecutionToken, ownerClosureRequest{Proof: proof})
		require.NoError(t, read.err)
		require.Equal(t, http.StatusAccepted, read.code)
		require.True(t, closed(read.receipt.Lifetime))
		require.True(t, read.receipt.Lifetime.Entered)
		require.True(t, read.receipt.Lifetime.Completed)
		ack := ownerPOST(client, server.URL, "/ack-owner", fixtureExecutionToken, ownerClosureRequest{Proof: proof})
		require.NoError(t, ack.err)
		require.Equal(t, http.StatusAccepted, ack.code)
		require.True(t, ack.receipt.Acknowledged)
		require.Equal(t, "closed", ack.receipt.Phase)
	}
	require.Empty(t, h.executions)
	require.Equal(t, originalDigest, sha256.Sum256(payload), "internal default/opaque payload must remain unchanged")
	drainedAt = time.Since(epoch).Milliseconds()
	time.Sleep(time.Second) // bounded postdrain raw observation, no runtime.GC.
	sample()
	require.EqualValues(t, count, admits.Load())
	require.EqualValues(t, count, providerEntries.Load())
}
