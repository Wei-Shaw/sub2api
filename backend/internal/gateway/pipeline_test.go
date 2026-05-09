//go:build unit

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// --- mock provider with controllable Forward / ShouldFailover ---

type forwardFunc func(ctx context.Context, w http.ResponseWriter, req *ForwardRequest) (*ForwardResult, error)
type shouldFailoverFunc func(ctx context.Context, req *ForwardRequest, err error) bool

type pipelineMockProvider struct {
	platform     string
	protocols    []string
	forwardFn    forwardFunc
	shouldFailFn shouldFailoverFunc
}

func (m *pipelineMockProvider) Platform() string    { return m.platform }
func (m *pipelineMockProvider) Protocols() []string { return m.protocols }

func (m *pipelineMockProvider) Forward(
	ctx context.Context, w http.ResponseWriter, req *ForwardRequest,
) (*ForwardResult, error) {
	if m.forwardFn != nil {
		return m.forwardFn(ctx, w, req)
	}
	return &ForwardResult{}, nil
}

func (m *pipelineMockProvider) ShouldFailover(
	ctx context.Context, req *ForwardRequest, err error,
) bool {
	if m.shouldFailFn != nil {
		return m.shouldFailFn(ctx, req, err)
	}
	return false
}

// --- helpers ---

func newTestRegistry(providers ...GatewayProvider) *ProviderRegistry {
	r := NewProviderRegistry()
	for _, p := range providers {
		r.Register(p)
	}
	return r
}

func newMinimalPipeline(
	registry *ProviderRegistry, maxFailovers int,
) *GatewayPipeline {
	return &GatewayPipeline{
		registry:     registry,
		maxFailovers: maxFailovers,
	}
}

func makeAccount(id int64, platform string) *service.Account {
	return &service.Account{ID: id, Platform: platform}
}

// --- NewGatewayPipeline constructor ---

func TestNewGatewayPipeline_DefaultMaxFailovers(t *testing.T) {
	p := NewGatewayPipeline(
		NewProviderRegistry(), nil, nil, nil, nil, nil,
	)
	if p.maxFailovers != defaultMaxFailovers {
		t.Fatalf("got maxFailovers=%d, want %d",
			p.maxFailovers, defaultMaxFailovers)
	}
}

func TestNewGatewayPipeline_ConfigOverride(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.MaxAccountSwitches = 5
	p := NewGatewayPipeline(
		NewProviderRegistry(), nil, nil, nil, nil, cfg,
	)
	if p.maxFailovers != 5 {
		t.Fatalf("got maxFailovers=%d, want 5", p.maxFailovers)
	}
}

func TestNewGatewayPipeline_ConfigZeroUsesDefault(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.MaxAccountSwitches = 0
	p := NewGatewayPipeline(
		NewProviderRegistry(), nil, nil, nil, nil, cfg,
	)
	if p.maxFailovers != defaultMaxFailovers {
		t.Fatalf("got maxFailovers=%d, want %d",
			p.maxFailovers, defaultMaxFailovers)
	}
}

// --- forwardToProvider ---

func TestForwardToProvider_HappyPath(t *testing.T) {
	expected := &ForwardResult{Model: "test-model"}
	provider := &pipelineMockProvider{
		platform:  "testplat",
		protocols: []string{"openai"},
		forwardFn: func(_ context.Context, _ http.ResponseWriter, _ *ForwardRequest,
		) (*ForwardResult, error) {
			return expected, nil
		},
	}
	p := newMinimalPipeline(newTestRegistry(provider), 3)
	req := &ForwardRequest{
		Account: makeAccount(1, "testplat"),
	}
	w := httptest.NewRecorder()
	result, err := p.forwardToProvider(
		context.Background(), w, req,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Model != "test-model" {
		t.Fatalf("got model %q, want %q", result.Model, "test-model")
	}
}

func TestForwardToProvider_NotFound(t *testing.T) {
	p := newMinimalPipeline(NewProviderRegistry(), 3)
	req := &ForwardRequest{
		Account: makeAccount(1, "unknown-platform"),
	}
	w := httptest.NewRecorder()
	_, err := p.forwardToProvider(
		context.Background(), w, req,
	)
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
	want := `no provider for platform "unknown-platform"`
	if !containsSubstring(err.Error(), want) {
		t.Fatalf("error %q should contain %q", err.Error(), want)
	}
}

func TestForwardToProvider_ForwardError(t *testing.T) {
	forwardErr := errors.New("upstream timeout")
	provider := &pipelineMockProvider{
		platform:  "plat",
		protocols: []string{"openai"},
		forwardFn: func(_ context.Context, _ http.ResponseWriter, _ *ForwardRequest,
		) (*ForwardResult, error) {
			return nil, forwardErr
		},
	}
	p := newMinimalPipeline(newTestRegistry(provider), 3)
	req := &ForwardRequest{Account: makeAccount(1, "plat")}
	w := httptest.NewRecorder()
	_, err := p.forwardToProvider(
		context.Background(), w, req,
	)
	if !errors.Is(err, forwardErr) {
		t.Fatalf("got %v, want %v", err, forwardErr)
	}
}

// --- handleForwardError ---

func TestHandleForwardError_Failover(t *testing.T) {
	provider := &pipelineMockProvider{
		platform:  "plat",
		protocols: []string{"openai"},
		shouldFailFn: func(_ context.Context, _ *ForwardRequest, _ error,
		) bool {
			return true
		},
	}
	p := newMinimalPipeline(newTestRegistry(provider), 3)
	req := &ForwardRequest{Account: makeAccount(42, "plat")}
	w := httptest.NewRecorder()
	fs := newTestFailoverState(3)

	_, done, err := p.handleForwardError(
		context.Background(), w, req, fs, errors.New("bad"),
	)
	if done {
		t.Fatal("expected done=false to signal retry")
	}
	if err != nil {
		t.Fatalf("expected nil error on failover, got %v", err)
	}
	if _, ok := fs.excludedIDs[42]; !ok {
		t.Fatal("account 42 should be in excluded set")
	}
}

func TestHandleForwardError_NoFailover(t *testing.T) {
	provider := &pipelineMockProvider{
		platform:  "plat",
		protocols: []string{"openai"},
		shouldFailFn: func(_ context.Context, _ *ForwardRequest, _ error,
		) bool {
			return false
		},
	}
	p := newMinimalPipeline(newTestRegistry(provider), 3)
	req := &ForwardRequest{Account: makeAccount(1, "plat")}
	w := httptest.NewRecorder()
	fs := newTestFailoverState(3)

	_, done, err := p.handleForwardError(
		context.Background(), w, req, fs, errors.New("fatal"),
	)
	if !done {
		t.Fatal("expected done=true for non-failover error")
	}
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestHandleForwardError_UpstreamAccepted(t *testing.T) {
	provider := &pipelineMockProvider{
		platform:  "plat",
		protocols: []string{"openai"},
		shouldFailFn: func(_ context.Context, _ *ForwardRequest, _ error,
		) bool {
			return true // would failover, but UpstreamAccepted blocks it
		},
	}
	p := newMinimalPipeline(newTestRegistry(provider), 3)
	req := &ForwardRequest{
		Account:          makeAccount(1, "plat"),
		UpstreamAccepted: true,
	}
	w := httptest.NewRecorder()
	fs := newTestFailoverState(3)

	_, done, err := p.handleForwardError(
		context.Background(), w, req, fs, errors.New("mid-stream"),
	)
	if !done {
		t.Fatal("expected done=true when upstream already accepted")
	}
	if err == nil {
		t.Fatal("expected error when upstream accepted")
	}
	if !containsSubstring(err.Error(), "upstream accepted") {
		t.Fatalf("error should mention upstream accepted: %v", err)
	}
}

func TestHandleForwardError_ProviderGone(t *testing.T) {
	// Provider unregistered between Forward and handleForwardError
	p := newMinimalPipeline(NewProviderRegistry(), 3)
	req := &ForwardRequest{Account: makeAccount(1, "gone")}
	w := httptest.NewRecorder()
	fs := newTestFailoverState(3)

	_, done, err := p.handleForwardError(
		context.Background(), w, req, fs, errors.New("err"),
	)
	if !done {
		t.Fatal("expected done=true when provider not found")
	}
	if err == nil {
		t.Fatal("expected error to be returned")
	}
}

// --- recordUsage ---

func TestRecordUsage_HappyPath(t *testing.T) {
	var recorded bool
	recordFn := func(_ context.Context, _ *service.Account, _ *ForwardResult,
	) error {
		recorded = true
		return nil
	}
	p := newMinimalPipeline(NewProviderRegistry(), 3)
	req := &ForwardRequest{Account: makeAccount(1, "plat")}
	result := &ForwardResult{Model: "m"}

	err := p.recordUsage(
		context.Background(), req, result, recordFn,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recorded {
		t.Fatal("record callback was not invoked")
	}
}

func TestRecordUsage_NilCallbackNoError(t *testing.T) {
	p := newMinimalPipeline(NewProviderRegistry(), 3)
	req := &ForwardRequest{Account: makeAccount(1, "plat")}
	err := p.recordUsage(
		context.Background(), req, &ForwardResult{}, nil,
	)
	if err != nil {
		t.Fatalf("expected nil error with nil callback, got %v", err)
	}
}

func TestRecordUsage_NilResult(t *testing.T) {
	called := false
	recordFn := func(_ context.Context, _ *service.Account, _ *ForwardResult,
	) error {
		called = true
		return nil
	}
	p := newMinimalPipeline(NewProviderRegistry(), 3)
	req := &ForwardRequest{Account: makeAccount(1, "plat")}
	err := p.recordUsage(
		context.Background(), req, nil, recordFn,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("record callback should not be called with nil result")
	}
}

func TestRecordUsage_NilAccount(t *testing.T) {
	called := false
	recordFn := func(_ context.Context, _ *service.Account, _ *ForwardResult,
	) error {
		called = true
		return nil
	}
	p := newMinimalPipeline(NewProviderRegistry(), 3)
	req := &ForwardRequest{Account: nil}
	err := p.recordUsage(
		context.Background(), req, &ForwardResult{}, recordFn,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("should not call record with nil account")
	}
}

// --- consumeBilling nil ticket ---

func TestConsumeBilling_NilTicket(t *testing.T) {
	p := newMinimalPipeline(NewProviderRegistry(), 3)
	req := &ForwardRequest{BillingTicket: nil}
	err := p.consumeBilling(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error with nil ticket: %v", err)
	}
}

// --- selectAndForward failover loop ---

// fakeSelectablePipeline wraps a GatewayPipeline but overrides
// selectAccount by injecting accounts in order. This lets us test
// the failover loop without mocking GatewayService.
//
// We test selectAndForward indirectly through tryOneAccount: we
// construct a pipeline with known maxFailovers and manually call
// tryOneAccount in a loop simulating selectAndForward, or we test
// the full selectAndForward by creating a subtype. Since
// selectAndForward is unexported and calls selectAccount (which
// needs gatewayService), we instead test the composed behavior
// through forwardToProvider + handleForwardError which are the
// building blocks.

func TestFailoverLoop_SuccessOnSecondAttempt(t *testing.T) {
	callCount := 0
	provider := &pipelineMockProvider{
		platform:  "plat",
		protocols: []string{"openai"},
		forwardFn: func(_ context.Context, _ http.ResponseWriter, _ *ForwardRequest,
		) (*ForwardResult, error) {
			callCount++
			if callCount == 1 {
				return nil, &service.UpstreamFailoverError{
					StatusCode: 429,
				}
			}
			return &ForwardResult{Model: "ok"}, nil
		},
		shouldFailFn: func(_ context.Context, _ *ForwardRequest, err error,
		) bool {
			return DefaultShouldFailover(err)
		},
	}
	reg := newTestRegistry(provider)
	p := newMinimalPipeline(reg, 5)
	w := httptest.NewRecorder()
	fs := newTestFailoverState(5)

	// Simulate the loop: attempt 1 fails with failover
	accounts := []*service.Account{
		makeAccount(1, "plat"),
		makeAccount(2, "plat"),
	}
	req := &ForwardRequest{Account: accounts[0]}

	result1, err1 := p.forwardToProvider(
		context.Background(), w, req,
	)
	if result1 != nil || err1 == nil {
		t.Fatal("first forward should fail")
	}
	_, done1, _ := p.handleForwardError(
		context.Background(), w, req, fs, err1,
	)
	if done1 {
		t.Fatal("first attempt should signal retry")
	}

	// Attempt 2 with next account
	req.Account = accounts[1]
	result2, err2 := p.forwardToProvider(
		context.Background(), w, req,
	)
	if err2 != nil {
		t.Fatalf("second forward should succeed: %v", err2)
	}
	if result2.Model != "ok" {
		t.Fatalf("got model %q, want %q", result2.Model, "ok")
	}
	if callCount != 2 {
		t.Fatalf("expected 2 forward calls, got %d", callCount)
	}
}

func TestFailoverLoop_MaxFailoversReached(t *testing.T) {
	const maxAttempts = 3
	provider := &pipelineMockProvider{
		platform:  "plat",
		protocols: []string{"openai"},
		forwardFn: func(_ context.Context, _ http.ResponseWriter, _ *ForwardRequest,
		) (*ForwardResult, error) {
			return nil, &service.UpstreamFailoverError{
				StatusCode: 500,
			}
		},
		shouldFailFn: func(_ context.Context, _ *ForwardRequest, err error,
		) bool {
			return DefaultShouldFailover(err)
		},
	}
	reg := newTestRegistry(provider)
	p := newMinimalPipeline(reg, maxAttempts)
	w := httptest.NewRecorder()
	fs := newTestFailoverState(maxAttempts)

	// Simulate maxAttempts iterations all triggering failover
	for i := 0; i < maxAttempts; i++ {
		acct := makeAccount(int64(i+1), "plat")
		req := &ForwardRequest{Account: acct}
		_, fwdErr := p.forwardToProvider(
			context.Background(), w, req,
		)
		if fwdErr == nil {
			t.Fatalf("attempt %d should fail", i)
		}
		_, done, _ := p.handleForwardError(
			context.Background(), w, req, fs, fwdErr,
		)
		if done {
			t.Fatalf("attempt %d should signal retry", i)
		}
	}
	// After maxAttempts failovers, the real selectAndForward
	// would return "failover limit reached"
	if len(fs.excludedIDs) != maxAttempts {
		t.Fatalf("expected %d excluded accounts, got %d",
			maxAttempts, len(fs.excludedIDs))
	}
}

// --- nil pipeline fields (no panic) ---

func TestNilPipelineFields_NoPanic(t *testing.T) {
	// A pipeline with only registry and maxFailovers set;
	// all service pointers are nil. Calling methods that
	// nil-check services should not panic.
	p := newMinimalPipeline(NewProviderRegistry(), 3)

	t.Run("consumeBilling_nil_ticket", func(t *testing.T) {
		req := &ForwardRequest{BillingTicket: nil}
		if err := p.consumeBilling(context.Background(), req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("recordUsage_all_nil", func(t *testing.T) {
		if err := p.recordUsage(
			context.Background(), &ForwardRequest{}, nil, nil,
		); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("forwardToProvider_missing_provider", func(t *testing.T) {
		req := &ForwardRequest{Account: makeAccount(1, "x")}
		_, err := p.forwardToProvider(
			context.Background(), httptest.NewRecorder(), req,
		)
		if err == nil {
			t.Fatal("expected error for missing provider")
		}
	})
}

// --- DefaultShouldFailover ---

func TestDefaultShouldFailover_WithFailoverError(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &service.UpstreamFailoverError{
		StatusCode: 429,
	})
	if !DefaultShouldFailover(err) {
		t.Fatal("should return true for UpstreamFailoverError")
	}
}

func TestDefaultShouldFailover_NonFailoverError(t *testing.T) {
	if DefaultShouldFailover(errors.New("generic")) {
		t.Fatal("should return false for non-failover error")
	}
}

// --- Registry accessor ---

func TestPipeline_RegistryAccessor(t *testing.T) {
	reg := NewProviderRegistry()
	p := newMinimalPipeline(reg, 3)
	if p.Registry() != reg {
		t.Fatal("Registry() should return the same registry")
	}
}

// --- helper ---

func newTestFailoverState(maxSwitches int) *pipelineFailoverState {
	return newPipelineFailoverState(maxSwitches)
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
