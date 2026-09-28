//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/gin-gonic/gin"
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
	turn  *muse.Turn
	state muse.State
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
	return f.turn, muse.Lease{WorkspaceID: 1, TurnID: r.TurnID, Owner: r.LeaseOwner, Fence: 1}, nil
}
func (f *museRuntimeFixture) Advance(_ context.Context, _ muse.Lease, from, to muse.State, _ string) error {
	if f.state != from || !muse.CanTransition(from, to) {
		return muse.ErrTransition
	}
	f.state = to
	return nil
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
