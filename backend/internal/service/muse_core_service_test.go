//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type museAccountsFixture struct {
	AccountRepository
	account *Account
}

func (f *museAccountsFixture) GetByID(context.Context, int64) (*Account, error) {
	return f.account, nil
}

type museRuntimeFixture struct {
	muse.RuntimeStore
	turn        *muse.Turn
	state       muse.State
	balanceHold float64
}

func (f *museRuntimeFixture) Bind(_ context.Context, i muse.Identity) (*muse.Workspace, error) {
	return &muse.Workspace{ID: 1, Identity: i, Generation: 1}, nil
}
func (f *museRuntimeFixture) Reserve(_ context.Context, r muse.Reservation) (*muse.Turn, muse.Lease, error) {
	if err := r.Validate(); err != nil {
		return nil, muse.Lease{}, err
	}
	f.turn = &muse.Turn{ID: r.TurnID, WorkspaceID: 1, Actor: r.Actor, AccountID: r.AccountID, Pricing: r.Pricing, State: muse.Reserved}
	f.state = muse.Reserved
	hold, _ := decimal.NewFromString(r.BalanceHold)
	f.balanceHold, _ = hold.Float64()
	return f.turn, muse.Lease{WorkspaceID: 1, TurnID: r.TurnID, Owner: r.LeaseOwner, Fence: 1}, nil
}
func (f *museRuntimeFixture) Advance(_ context.Context, _ muse.Lease, from, to muse.State, providerID string) error {
	if f.state != from || !muse.CanTransition(from, to) {
		return muse.ErrTransition
	}
	f.state = to
	if providerID != "" {
		f.turn.ProviderTurnID = providerID
	}
	return nil
}

type museSubmissionFixture struct {
	*museProviderFixture
	reference string
}

func (f *museSubmissionFixture) SubmissionID(muse.Request) (string, error) {
	return f.reference, nil
}

func TestMuseProbeReferenceIsDurableBeforeLostAcknowledgement(t *testing.T) {
	_, core, provider, store, runtime, account, key := newMuseCoreFixture()
	provider.execute = func(_ context.Context, request muse.Request, _ func(muse.Event) error) (*muse.Result, error) {
		require.Equal(t, muse.Submitting, runtime.state)
		require.Equal(t, "durable-native-probe", runtime.turn.ProviderTurnID)
		return nil, muse.ErrNoiseProtocol
	}
	core.SetProvider(&museSubmissionFixture{museProviderFixture: provider, reference: "durable-native-probe"})
	_, turn, err := core.Execute(context.Background(), key, account, &apicompat.ResponsesRequest{Model: "muse/assistant", Input: json.RawMessage(`"hello"`)}, nil, nil)
	require.ErrorIs(t, err, muse.ErrNoiseProtocol)
	require.Equal(t, muse.Ambiguous, turn.State)
	require.Equal(t, "durable-native-probe", turn.ProviderTurnID)
	require.Equal(t, 1, provider.calls)
	require.Zero(t, store.settlements, "uncertain accepted work must not settle or be replayed")
}
func (f *museRuntimeFixture) Renew(context.Context, muse.Lease, time.Duration) error { return nil }

type museStoreFixture struct {
	MuseProviderStore
	observation *muse.Observation
	charges     int
	settlements int
	command     *UsageBillingCommand
}

func (f *museStoreFixture) Profile(context.Context, *Account) (*muse.Observation, error) {
	return f.observation, nil
}
func (f *museStoreFixture) FreezeCharge(_ context.Context, _ string, c *UsageBillingCommand, _ *UsageLog) error {
	f.charges++
	f.command = c
	return nil
}
func (f *museStoreFixture) PendingSettlement(context.Context, int) ([]string, error) { return nil, nil }
func (f *museStoreFixture) PendingTurns(context.Context, int64) ([]*muse.Turn, error) {
	return nil, nil
}
func (f *museStoreFixture) RecordResult(context.Context, string, *muse.Result) error { return nil }
func (f *museStoreFixture) Settle(context.Context, string) (*MuseSettlement, error) {
	f.settlements++
	return &MuseSettlement{}, nil
}

type museProviderFixture struct {
	muse.DisabledProvider
	calls           int
	badCorrelation  bool
	execute         func(context.Context, muse.Request, func(muse.Event) error) (*muse.Result, error)
	confirmedCancel bool
}

func (f *museProviderFixture) Qualified() bool { return true }
func (f *museProviderFixture) Execute(ctx context.Context, r muse.Request, emit func(muse.Event) error) (*muse.Result, error) {
	f.calls++
	if f.execute != nil {
		return f.execute(ctx, r, emit)
	}
	response := &apicompat.ResponsesResponse{ID: "provider-response", Object: "response", Model: "fixture-model", Status: "completed", CreatedAt: 1, Output: []apicompat.ResponsesOutput{{ID: "msg_fixture", Type: "message", Role: "assistant", Status: "completed", Content: []apicompat.ResponsesContentPart{{Type: "output_text", Text: "fixture reply"}}}}}
	if r.Input.Stream {
		id := r.OperationID
		if f.badCorrelation {
			id = "different-operation"
		}
		events := []apicompat.ResponsesStreamEvent{{Type: "response.created", Response: response}, {Type: "response.output_text.delta", Delta: "fixture reply"}, {Type: "response.completed", Response: response}}
		for i, event := range events {
			if err := emit(muse.Event{OperationID: id, Sequence: int64(i + 1), ProviderTurnID: "provider-turn", Data: event}); err != nil {
				return nil, err
			}
		}
	}
	return &muse.Result{ProviderTurnID: "provider-turn", Response: response}, nil
}

func newMuseCoreFixture() (*OpenAIGatewayService, *MuseCoreService, *museProviderFixture, *museStoreFixture, *museRuntimeFixture, *Account, *APIKey) {
	a := &Account{ID: 1, Platform: PlatformMuse, Type: AccountTypeSession, Status: StatusActive, UpdatedAt: time.Now(), Credentials: map[string]any{"muse_session": map[string]any{"opaque": "fixture"}}, Extra: map[string]any{MuseOwnerUserIDKey: int64(10)}}
	profile := &muse.Observation{InferenceAllowed: true, Identity: muse.Identity{PrincipalID: "fixture-principal", WorkspaceID: "fixture-workspace", OwnerUserID: 10, AccountID: 1, AccountUpdatedAt: a.UpdatedAt}, Capabilities: muse.Capabilities{Models: []string{"muse/assistant"}, Streaming: true}}
	store := &museStoreFixture{observation: profile}
	runtime := &museRuntimeFixture{}
	provider := &museProviderFixture{}
	g := &OpenAIGatewayService{cfg: &config.Config{RunMode: config.RunModeSimple}}
	core := NewMuseCoreService(&museAccountsFixture{account: a}, runtime, store, g)
	core.SetProvider(provider)
	key := &APIKey{ID: 20, UserID: 10, User: &User{ID: 10}, Group: &Group{ID: 30, Platform: PlatformMuse}}
	return g, core, provider, store, runtime, a, key
}

func TestMuseGatewayRendersAllPublicProtocols(t *testing.T) {
	for _, tc := range []struct{ endpoint, body, want string }{
		{"responses", `{"model":"muse/assistant","input":"hello"}`, `"object":"response"`},
		{"chat_completions", `{"model":"muse/assistant","messages":[{"role":"user","content":"hello"}]}`, `"object":"chat.completion"`},
		{"messages", `{"model":"muse/assistant","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`, `"type":"message"`},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			g, _, p, store, runtime, a, key := newMuseCoreFixture()
			store.observation.Capabilities.Controls = true
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/"+tc.endpoint, strings.NewReader(tc.body))
			c.Set("api_key", key)
			result, err := g.forwardMuse(c.Request.Context(), c, a, []byte(tc.body), tc.endpoint)
			require.NoError(t, err)
			require.Equal(t, 1, p.calls)
			require.Equal(t, muse.Completed, runtime.state)
			require.Equal(t, 1, store.settlements)
			require.Contains(t, recorder.Body.String(), tc.want)
			require.Contains(t, recorder.Body.String(), "fixture reply")
			require.NotEmpty(t, result.MuseTurnID)
		})
	}
}

func TestMuseGatewayUnqualifiedNeverInvokesProviderOrFreezesCharge(t *testing.T) {
	g, core, p, store, _, a, key := newMuseCoreFixture()
	core.SetProvider(muse.DisabledProvider{})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("api_key", key)
	_, err := g.forwardMuse(c.Request.Context(), c, a, []byte(`{"model":"muse/assistant","input":"hello"}`), "responses")
	require.ErrorIs(t, err, muse.ErrTransportUnqualified)
	require.Equal(t, 503, recorder.Code)
	require.Zero(t, p.calls)
	require.Zero(t, store.charges)
}

func TestMuseGatewayRejectsOwnershipAndUnsupportedToolsBeforeSubmission(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		g, _, p, store, _, a, key := newMuseCoreFixture()
		body := `{"model":"muse/assistant","input":"hello","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
		if foreign {
			key.UserID = 11
			body = `{"model":"muse/assistant","input":"hello"}`
		}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		c.Set("api_key", key)
		_, err := g.forwardMuse(c.Request.Context(), c, a, []byte(body), "responses")
		require.Error(t, err)
		require.Zero(t, p.calls)
		require.Zero(t, store.charges)
	}
}

func TestMuseGatewayStreamingRejectsUnrelatedOperation(t *testing.T) {
	g, _, p, store, runtime, a, key := newMuseCoreFixture()
	p.badCorrelation = true
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("api_key", key)
	_, err := g.forwardMuse(c.Request.Context(), c, a, []byte(`{"model":"muse/assistant","input":"hello","stream":true}`), "responses")
	require.Error(t, err)
	require.Equal(t, muse.Ambiguous, runtime.state)
	require.Zero(t, store.settlements)
	require.NotContains(t, recorder.Body.String(), "fixture reply")
}

func (f *museProviderFixture) Cancel(context.Context, muse.Session, string) (bool, error) {
	return f.confirmedCancel, nil
}

func TestMuseStreamingProtocolsAndClientControls(t *testing.T) {
	for _, tc := range []struct{ endpoint, body, want string }{
		{"responses", `{"model":"muse/assistant","input":"hello","stream":true}`, "event: response.completed"},
		{"chat_completions", `{"model":"muse/assistant","messages":[{"role":"user","content":"hello"}],"stream":true}`, "data: [DONE]"},
		{"messages", `{"model":"muse/assistant","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`, "event: message_stop"},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			g, _, _, store, runtime, a, key := newMuseCoreFixture()
			store.observation.Capabilities.Controls = true
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/"+tc.endpoint, nil)
			c.Set("api_key", key)
			_, err := g.forwardMuse(c.Request.Context(), c, a, []byte(tc.body), tc.endpoint)
			require.NoError(t, err)
			require.Equal(t, muse.Completed, runtime.state)
			require.Contains(t, recorder.Body.String(), tc.want)
			require.Contains(t, recorder.Body.String(), "fixture reply")
		})
	}
	input, err := parseMuseRequest([]byte(`{"model":"muse/assistant","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`), "messages")
	require.NoError(t, err)
	require.Equal(t, 16, *input.MaxOutputTokens)
	require.Nil(t, input.Reasoning)
	require.Nil(t, input.Store)
	require.Nil(t, input.ParallelToolCalls)
	require.Nil(t, input.Text)
	_, err = parseMuseRequest([]byte(`{"model":"muse/assistant","messages":[{"role":"user","content":"hello"}],"seed":42}`), "chat_completions")
	require.ErrorIs(t, err, muse.ErrInvalid)
}

func TestMuseConfirmedCancellationSettlesOriginalTurn(t *testing.T) {
	_, core, p, store, runtime, a, key := newMuseCoreFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.confirmedCancel = true
	p.execute = func(op context.Context, r muse.Request, emit func(muse.Event) error) (*muse.Result, error) {
		require.NoError(t, emit(muse.Event{OperationID: r.OperationID, ProviderTurnID: "fixture-turn", Sequence: 1, Data: apicompat.ResponsesStreamEvent{Type: "response.created"}}))
		cancel()
		<-op.Done()
		return nil, op.Err()
	}
	input := &apicompat.ResponsesRequest{Model: "muse/assistant", Input: json.RawMessage(`"hello"`)}
	_, turn, err := core.Execute(ctx, key, a, input, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, muse.Cancelled, turn.State)
	require.Equal(t, muse.Cancelled, runtime.state)
	require.Equal(t, 1, store.settlements)
	require.Equal(t, 1, p.calls)
}

func TestMuseRemoteFailureIsTerminalAndSettlesZero(t *testing.T) {
	_, core, p, store, runtime, a, key := newMuseCoreFixture()
	p.execute = func(context.Context, muse.Request, func(muse.Event) error) (*muse.Result, error) {
		return &muse.Result{ProviderTurnID: "fixture-turn", Response: &apicompat.ResponsesResponse{Status: "failed", Model: "fixture-model"}}, errors.New("synthetic remote failure")
	}
	_, turn, err := core.Execute(context.Background(), key, a, &apicompat.ResponsesRequest{Model: "muse/assistant", Input: json.RawMessage(`"hello"`)}, nil, nil)
	require.ErrorIs(t, err, muse.ErrRemoteFailed)
	require.Equal(t, muse.Failed, turn.State)
	require.Equal(t, muse.Failed, runtime.state)
	require.Equal(t, 1, store.settlements)
	require.Equal(t, 1, p.calls)
}

func TestMuseSchedulerUsesAuthenticatedOwner(t *testing.T) {
	_, _, _, _, _, a, _ := newMuseCoreFixture()
	a.Schedulable = true
	foreign := context.WithValue(context.Background(), ctxkey.UserID, int64(11))
	require.Equal(t, "muse_owner_mismatch", openAICompatibleAccountEligibilityFailureReasonBeforeProfit(foreign, a, PlatformMuse, "muse/assistant", false, ""))
	require.Equal(t, "muse_owner_mismatch", openAICompatibleAccountEligibilityFailureReasonBeforeProfit(context.Background(), a, PlatformMuse, "muse/assistant", false, ""))
}

func TestMuseCausalCallbackFailureCannotBeIgnoredByAdapter(t *testing.T) {
	_, core, p, store, runtime, a, key := newMuseCoreFixture()
	p.execute = func(_ context.Context, r muse.Request, emit func(muse.Event) error) (*muse.Result, error) {
		require.ErrorIs(t, emit(muse.Event{OperationID: "unrelated", ProviderTurnID: "fixture-turn", Sequence: 1}), muse.ErrGeneration)
		return &muse.Result{ProviderTurnID: "fixture-turn", Response: &apicompat.ResponsesResponse{Status: "completed", Model: "fixture-model"}}, nil
	}
	_, _, err := core.Execute(context.Background(), key, a, &apicompat.ResponsesRequest{Model: "muse/assistant", Input: json.RawMessage(`"hello"`)}, nil, nil)
	require.Error(t, err)
	require.Equal(t, muse.Ambiguous, runtime.state)
	require.Zero(t, store.settlements)
}

func TestMuseRejectsAccountSnapshotNewerThanLoadedCredentials(t *testing.T) {
	_, core, p, store, _, a, key := newMuseCoreFixture()
	store.observation.Identity.AccountUpdatedAt = a.UpdatedAt.Add(time.Second)
	_, _, err := core.Execute(context.Background(), key, a, &apicompat.ResponsesRequest{Model: "muse/assistant", Input: json.RawMessage(`"hello"`)}, nil, nil)
	require.ErrorIs(t, err, muse.ErrGeneration)
	require.Zero(t, p.calls)
	require.Zero(t, store.charges)
}

func TestMuseRejectsProxySnapshotNewerThanLoadedProxy(t *testing.T) {
	_, core, p, store, _, a, key := newMuseCoreFixture()
	id := int64(2)
	at := time.Now()
	a.ProxyID = &id
	a.Proxy = &Proxy{ID: id, UpdatedAt: at}
	newer := at.Add(time.Second)
	store.observation.ProxyUpdatedAt = &newer
	_, _, err := core.Execute(context.Background(), key, a, &apicompat.ResponsesRequest{Model: "muse/assistant", Input: json.RawMessage(`"hello"`)}, nil, nil)
	require.ErrorIs(t, err, muse.ErrGeneration)
	require.Zero(t, p.calls)
}

func TestMuseSchedulerFiltersObservedModelsAndMappedAliases(t *testing.T) {
	g, _, _, store, _, a, key := newMuseCoreFixture()
	a.Schedulable = true
	store.observation.Capabilities.Models = []string{"muse/basic"}
	ctx := context.WithValue(context.Background(), ctxkey.UserID, key.UserID)
	candidates := g.filterMuseAccounts(ctx, []Account{*a})
	require.Len(t, candidates, 1)
	require.Equal(t, "model_not_supported", openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, &candidates[0], PlatformMuse, "muse/premium", false, ""))
	require.Empty(t, openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, &candidates[0], PlatformMuse, "muse/basic", false, ""))
	a.Credentials["model_mapping"] = map[string]any{"client-basic": "muse/basic"}
	candidates = g.filterMuseAccounts(ctx, []Account{*a})
	require.True(t, candidates[0].IsModelSupported("client-basic"))
	require.False(t, candidates[0].IsModelSupported("muse/premium"))
}

type museProfileSetFixture struct {
	*museStoreFixture
	profiles map[int64]*muse.Observation
}

func (f *museProfileSetFixture) Profile(_ context.Context, a *Account) (*muse.Observation, error) {
	return f.profiles[a.ID], nil
}

type museAccountSetFixture struct {
	AccountRepository
	accounts map[int64]*Account
}

func (f *museAccountSetFixture) GetByID(_ context.Context, id int64) (*Account, error) {
	return f.accounts[id], nil
}

func TestMuseSelectionSkipsHigherPriorityUnsupportedAccountAndRechecksSticky(t *testing.T) {
	g, core, _, store, _, a, key := newMuseCoreFixture()
	a.Schedulable = true
	a.Priority = 1
	other := *a
	other.ID = 2
	other.Priority = 10
	first := *store.observation
	first.Capabilities.Models = []string{"muse/basic"}
	second := *store.observation
	second.Identity.AccountID = 2
	second.Capabilities.Models = []string{"muse/premium"}
	core.store = &museProfileSetFixture{museStoreFixture: store, profiles: map[int64]*muse.Observation{1: &first, 2: &second}}
	g.accountRepo = &museAccountSetFixture{accounts: map[int64]*Account{1: a, 2: &other}}
	ctx := context.WithValue(context.Background(), ctxkey.UserID, key.UserID)
	candidates := g.filterMuseAccounts(ctx, []Account{*a, other})
	selected, _, _ := g.selectBestAccount(ctx, nil, PlatformMuse, candidates, "muse/premium", nil, false, "", false)
	require.NotNil(t, selected)
	require.EqualValues(t, 2, selected.ID)
	sticky, err := g.getSchedulableAccount(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "model_not_supported", openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, sticky, PlatformMuse, "muse/premium", false, ""))
	require.Empty(t, openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, sticky, PlatformMuse, "muse/basic", false, ""))
}

func TestMuseChatStreamHonorsIncludeUsageWithoutInventingCounts(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		include, reported, want bool
	}{{"requested-known", true, true, true}, {"not-requested", false, true, false}, {"unknown", true, false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			g, _, p, _, _, a, key := newMuseCoreFixture()
			p.execute = func(_ context.Context, r muse.Request, emit func(muse.Event) error) (*muse.Result, error) {
				response := &apicompat.ResponsesResponse{ID: "remote", Object: "response", Status: "completed", Model: "fixture-model"}
				if tc.reported {
					response.Usage = &apicompat.ResponsesUsage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30}
				}
				for i, event := range []apicompat.ResponsesStreamEvent{{Type: "response.created", Response: response}, {Type: "response.output_text.delta", Delta: "reply"}, {Type: "response.completed", Response: response}} {
					require.NoError(t, emit(muse.Event{OperationID: r.OperationID, ProviderTurnID: "fixture-turn", Sequence: int64(i + 1), Data: event}))
				}
				return &muse.Result{ProviderTurnID: "fixture-turn", Response: response}, nil
			}
			body := `{"model":"muse/assistant","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":` + fmt.Sprint(tc.include) + `}}`
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c.Set("api_key", key)
			_, err := g.forwardMuse(c.Request.Context(), c, a, []byte(body), "chat_completions")
			require.NoError(t, err)
			if tc.want {
				require.Contains(t, recorder.Body.String(), `"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30`)
			} else {
				require.NotContains(t, recorder.Body.String(), `"usage"`)
			}
		})
	}
}

type museAuthStoreFixture struct {
	*museStoreFixture
	busy     bool
	document map[string]any
}

func (f *museAuthStoreFixture) RenewSession(ctx context.Context, _ *Account, renew func(context.Context) (map[string]any, error)) error {
	if f.busy {
		return muse.ErrBusy
	}
	document, err := renew(ctx)
	if err == nil {
		f.document = document
	}
	return err
}

type museAuthHTTPFixture struct {
	HTTPUpstream
	t             *testing.T
	calls         int
	expectedProxy string
}

func (f *museAuthHTTPFixture) Do(req *http.Request, proxy string, accountID int64, concurrency int) (*http.Response, error) {
	f.calls++
	require.True(f.t, HTTPUpstreamRedirectsDisabled(req.Context()))
	require.Equal(f.t, f.expectedProxy, proxy)
	require.EqualValues(f.t, 1, accountID)
	require.Equal(f.t, 1, concurrency)
	require.Equal(f.t, muse.SessionEndpoint, req.URL.String())
	require.Equal(f.t, "GET", req.Method)
	return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"hatch_vml=rotated-fixture; Path=/; Secure; HttpOnly; Max-Age=7200"}}, Body: io.NopCloser(strings.NewReader(`{"status":"assigned","vm_id":"fixture-vm","vm_state":"RUNNING"}`))}, nil
}

func TestMuseCookieAuthenticationWorksWithoutQualifyingInference(t *testing.T) {
	g, core, p, store, _, a, _ := newMuseCoreFixture()
	core.SetProvider(muse.DisabledProvider{})
	a.Credentials["muse_session"] = map[string]any{"cookies": map[string]any{"hatch_sess": "fixture", "hatch_gw": "fixture", "hatch_vml": "fixture", "hatch_native_auth_device": "fixture"}}
	proxyID := int64(9)
	a.ProxyID = &proxyID
	a.Proxy = &Proxy{ID: proxyID, Protocol: "http", Host: "fixture-proxy.local", Port: 8080}
	authStore := &museAuthStoreFixture{museStoreFixture: store}
	core.store = authStore
	httpPort := &museAuthHTTPFixture{t: t, expectedProxy: "http://fixture-proxy.local:8080"}
	g.httpUpstream = httpPort
	check, err := core.CheckSession(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, check.Authenticated)
	require.False(t, core.Qualified())
	require.Zero(t, p.calls)
	require.Zero(t, store.charges)
	cookies, err := muse.ParseCookieSession(authStore.document)
	require.NoError(t, err)
	require.Equal(t, "rotated-fixture", cookies.Cookies["hatch_vml"])
	require.NoError(t, core.RenewSession(context.Background(), 1))
	require.Equal(t, 2, httpPort.calls)
	authStore.busy = true
	_, err = core.CheckSession(context.Background(), 1)
	require.ErrorIs(t, err, muse.ErrBusy)
	require.Equal(t, 2, httpPort.calls)
}

func TestMuseSnapshotRegistryAndAvailabilityUseVerifiedModels(t *testing.T) {
	require.Contains(t, schedulerSnapshotPlatforms(), PlatformMuse)
	found := false
	for _, bucket := range schedulerBucketsForGroup(77) {
		if bucket.Platform == PlatformMuse {
			found = true
		}
	}
	require.True(t, found, "group warming must include the Muse bucket")
	g, _, _, store, _, a, key := newMuseCoreFixture()
	a.Schedulable = true
	store.observation.Capabilities.Models = []string{"auto/auto"}
	g.accountRepo = &mockAccountRepoForPlatform{accounts: []Account{*a}, accountsByID: map[int64]*Account{a.ID: a}}
	ctx := context.WithValue(context.Background(), ctxkey.UserID, key.UserID)
	known := g.DiagnoseModelAvailabilityForPlatform(ctx, nil, "auto/auto", PlatformMuse)
	require.True(t, known.HasAccountsInPool)
	require.True(t, known.HasModelSupport, "diagnosis must hydrate the observed catalog")
	unknown := g.DiagnoseModelAvailabilityForPlatform(ctx, nil, "unobserved", PlatformMuse)
	require.True(t, unknown.HasAccountsInPool)
	require.False(t, unknown.HasModelSupport)
	foreign := g.DiagnoseModelAvailabilityForPlatform(context.Background(), nil, "auto/auto", PlatformMuse)
	require.False(t, foreign.HasModelSupport, "model diagnosis must preserve the owner boundary")
}

func TestMuseSchedulerHydratesMetadataProjectionBeforeOwnerAndProfileCheck(t *testing.T) {
	g, _, _, _, _, account, key := newMuseCoreFixture()
	g.accountRepo = &museAccountsFixture{account: account}
	projection := *account
	projection.Extra = nil
	projection.Credentials = nil
	projection.UpdatedAt = time.Time{}
	ctx := context.WithValue(context.Background(), ctxkey.UserID, key.UserID)
	candidates := g.filterMuseAccounts(ctx, []Account{projection})
	require.Len(t, candidates, 1)
	require.Equal(t, account.UpdatedAt, candidates[0].UpdatedAt)
	require.True(t, candidates[0].IsModelSupported("muse/assistant"))
	foreign := context.WithValue(context.Background(), ctxkey.UserID, key.UserID+1)
	require.Empty(t, g.filterMuseAccounts(foreign, []Account{projection}))
}

func TestMuseUnknownChatRolesRejectedBeforeReservation(t *testing.T) {
	for _, role := range []string{"", "tool_typo"} {
		g, _, p, _, _, account, key := newMuseCoreFixture()
		body := fmt.Sprintf(`{"model":"muse/assistant","messages":[{"role":%q,"content":"hello"}]}`, role)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		c.Set("api_key", key)
		_, err := g.forwardMuse(c.Request.Context(), c, account, []byte(body), "chat_completions")
		require.ErrorIs(t, err, muse.ErrInvalid)
		require.Zero(t, p.calls)
		require.Equal(t, 0, g.muse.store.(*museStoreFixture).charges)
	}
}

func (f *museStoreFixture) WithSession(_ context.Context, a *Account, use func(map[string]any, func(map[string]any) error) error) error {
	document := a.Credentials["muse_session"].(map[string]any)
	return use(document, func(next map[string]any) error { a.Credentials["muse_session"] = next; return nil })
}
func (f *museStoreFixture) PendingBalance(context.Context, int64) (float64, error) { return 0, nil }

func (f *museStoreFixture) AvailableBalance(context.Context, int64) (float64, error) { return 0, nil }

func TestMuseBalanceAdmissionReturnsBillingError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	err := writeMuseError(c, "chat_completions", ErrInsufficientBalance, false)
	require.ErrorIs(t, err, ErrInsufficientBalance)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "insufficient_balance", response.Error.Code)
}
