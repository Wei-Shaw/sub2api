//go:build unit

package signature

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// fakePool is an in-memory SignaturePool used by tests; implements the
// ordering contract (TopN returns most-recently-added first).
// Thread-safe: the async harvester goroutine calls Add concurrently.
type fakePool struct {
	mu      sync.Mutex
	entries map[string][]string // bucket -> signatures, index 0 = newest
}

func newFakePool() *fakePool { return &fakePool{entries: map[string][]string{}} }

func (p *fakePool) Add(_ context.Context, bucket, sig string, _ time.Time, _ int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[bucket] = append([]string{sig}, p.entries[bucket]...)
	return nil
}
func (p *fakePool) TopN(_ context.Context, bucket string, n int) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := p.entries[bucket]
	if n > len(list) {
		n = len(list)
	}
	out := make([]string, n)
	copy(out, list[:n])
	return out, nil
}
func (p *fakePool) Size(_ context.Context, bucket string) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return int64(len(p.entries[bucket])), nil
}

func TestBucketFor(t *testing.T) {
	if got := BucketFor("oauth", "anthropic", 1); got != "oauth:anthropic" {
		t.Errorf("anthropic oauth: got %q, want oauth:anthropic", got)
	}
	if got := BucketFor("oauth", "antigravity", 1); got != "oauth:antigravity" {
		t.Errorf("antigravity oauth: got %q, want oauth:antigravity", got)
	}
	if got := BucketFor("setup-token", "anthropic", 1); got != "oauth:anthropic" {
		t.Errorf("setup-token must share platform oauth bucket, got %q", got)
	}
	if got := BucketFor("oauth", "", 1); got != "oauth:anthropic" {
		t.Errorf("empty platform defaults to anthropic: got %q", got)
	}
	if got := BucketFor("apikey", "anthropic", 42); got != "apikey:42" {
		t.Errorf("apikey bucket: got %q, want apikey:42", got)
	}
	if got := BucketFor("bedrock", "", 1); got != "" {
		t.Errorf("bedrock has no pool: got %q, want empty", got)
	}
}

func TestStripClaudeRectifier_Stage1AlwaysApplied(t *testing.T) {
	r := &StripClaudeRectifier{
		FilterStage1: func(b []byte) []byte { return append([]byte("stripped1:"), b...) },
		FilterStage2: func(b []byte) []byte { return append([]byte("stripped2:"), b...) },
	}
	out, proceed := r.Apply(context.Background(), ClaudeInput{Body: []byte("orig")}, StageThinkingOnly)
	if !proceed || string(out) != "stripped1:orig" {
		t.Fatalf("stage 1: proceed=%v out=%q", proceed, out)
	}
}

func TestStripClaudeRectifier_Stage2GatedOnToolError(t *testing.T) {
	r := &StripClaudeRectifier{
		FilterStage1: func(b []byte) []byte { return b },
		FilterStage2: func(b []byte) []byte { return append([]byte("stripped2:"), b...) },
	}
	// Non-tool-related error: stage 2 must decline.
	out, proceed := r.Apply(context.Background(), ClaudeInput{Body: []byte("orig"), LastErrMsg: "invalid signature in thinking block"}, StageThinkingAndTools)
	if proceed {
		t.Fatalf("stage 2 should decline for non-tool-related errors")
	}
	if out != nil {
		t.Errorf("expected nil body when declining, got %q", out)
	}
	// Tool-related error: stage 2 must proceed.
	out, proceed = r.Apply(context.Background(), ClaudeInput{Body: []byte("orig"), LastErrMsg: "tool_use block signature invalid"}, StageThinkingAndTools)
	if !proceed || string(out) != "stripped2:orig" {
		t.Fatalf("stage 2 on tool error: proceed=%v out=%q", proceed, out)
	}
}

func TestPoolClaudeRectifier_EmptyPoolSignalsAbort(t *testing.T) {
	pool := newFakePool()
	r := &PoolClaudeRectifier{Pool: pool, Capacity: 10}
	in := ClaudeInput{AccountType: "oauth", AccountID: 1, Platform: "anthropic", Body: []byte(`{"messages":[]}`)}
	out, proceed := r.Apply(context.Background(), in, StageThinkingOnly)
	if proceed {
		t.Fatalf("empty pool must signal proceed=false (rule A)")
	}
	if out != nil {
		t.Errorf("expected nil body when pool empty, got %q", out)
	}
}

func TestPoolClaudeRectifier_NoThinkingBlocksSignalsAbort(t *testing.T) {
	pool := newFakePool()
	_ = pool.Add(context.Background(), "oauth:anthropic", "good", time.Now(), 10)
	r := &PoolClaudeRectifier{Pool: pool, Capacity: 10}
	// Pool has a sig, but the request has no thinking blocks to replace.
	in := ClaudeInput{AccountType: "oauth", AccountID: 1, Platform: "anthropic", Body: []byte(`{"messages":[{"role":"user","content":"hi"}]}`)}
	out, proceed := r.Apply(context.Background(), in, StageThinkingOnly)
	if proceed {
		t.Fatalf("replaced=0 must signal proceed=false")
	}
	if out != nil {
		t.Errorf("expected nil body when nothing replaced, got %q", out)
	}
}

func TestPoolClaudeRectifier_ReplacesAndStopsAtStage2(t *testing.T) {
	pool := newFakePool()
	_ = pool.Add(context.Background(), "oauth:anthropic", "good", time.Now(), 10)
	r := &PoolClaudeRectifier{Pool: pool, Capacity: 10}
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x","signature":"bad"}]}]}`)
	in := ClaudeInput{AccountType: "oauth", AccountID: 1, Platform: "anthropic", Body: body}

	out, proceed := r.Apply(context.Background(), in, StageThinkingOnly)
	if !proceed {
		t.Fatalf("pool with sigs + thinking blocks must proceed")
	}
	if string(out) == string(body) {
		t.Errorf("body should be mutated")
	}

	// Stage 2: pool is a one-shot strategy, must decline.
	out2, proceed2 := r.Apply(context.Background(), in, StageThinkingAndTools)
	if proceed2 || out2 != nil {
		t.Fatalf("pool must decline stage 2 (one-shot)")
	}
}

func TestPoolAntigravityRectifier_Stages(t *testing.T) {
	r := &PoolAntigravityRectifier{}
	st := r.Stages()
	if len(st) != 1 || st[0] != StageThinkingOnly {
		t.Errorf("pool antigravity stages: got %v, want [StageThinkingOnly]", st)
	}
}

func TestPoolAntigravityRectifier_EmptyPool(t *testing.T) {
	pool := newFakePool()
	r := &PoolAntigravityRectifier{Pool: pool, Capacity: 10}
	req := &antigravity.ClaudeRequest{
		Messages: []antigravity.ClaudeMessage{
			{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"x","signature":"bad"}]`)},
		},
	}
	applied, proceed, err := r.Apply(context.Background(), AntigravityInput{AccountType: "oauth", AccountID: 1, Platform: "antigravity", Request: req}, StageThinkingOnly)
	if applied || proceed || err != nil {
		t.Errorf("empty pool: applied=%v proceed=%v err=%v; want all false/nil", applied, proceed, err)
	}
}

func TestPoolAntigravityRectifier_AppliedAndOneShot(t *testing.T) {
	pool := newFakePool()
	_ = pool.Add(context.Background(), "oauth:antigravity", "g0", time.Now(), 10)
	r := &PoolAntigravityRectifier{Pool: pool, Capacity: 10}
	req := &antigravity.ClaudeRequest{
		Messages: []antigravity.ClaudeMessage{
			{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"x","signature":"bad"}]`)},
		},
	}
	applied, proceed, err := r.Apply(context.Background(), AntigravityInput{AccountType: "oauth", AccountID: 1, Platform: "antigravity", Request: req}, StageThinkingOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !applied {
		t.Errorf("expected applied=true when pool produces replacements")
	}
	if proceed {
		t.Errorf("expected proceed=false (pool is one-shot)")
	}
}

// errPool is a SignaturePool that always returns errors — used to confirm
// rectifiers handle pool failures gracefully.
type errPool struct{}

func (errPool) Add(context.Context, string, string, time.Time, int) error {
	return errors.New("add failed")
}
func (errPool) TopN(context.Context, string, int) ([]string, error) {
	return nil, errors.New("topn failed")
}
func (errPool) Size(context.Context, string) (int64, error) { return 0, errors.New("size failed") }

func TestPoolClaudeRectifier_PoolErrorIsTreatedAsEmpty(t *testing.T) {
	r := &PoolClaudeRectifier{Pool: errPool{}, Capacity: 10}
	in := ClaudeInput{AccountType: "oauth", AccountID: 1, Platform: "anthropic", Body: []byte(`{"messages":[]}`)}
	_, proceed := r.Apply(context.Background(), in, StageThinkingOnly)
	if proceed {
		t.Errorf("pool error must behave like empty pool (proceed=false)")
	}
}

func TestPoolAntigravityRectifier_NilRequestNoop(t *testing.T) {
	pool := newFakePool()
	_ = pool.Add(context.Background(), "oauth:antigravity", "g", time.Now(), 10)
	r := &PoolAntigravityRectifier{Pool: pool, Capacity: 10}
	applied, proceed, err := r.Apply(context.Background(), AntigravityInput{AccountType: "oauth", AccountID: 1, Platform: "antigravity", Request: nil}, StageThinkingOnly)
	if applied || proceed || err != nil {
		t.Errorf("nil request: applied=%v proceed=%v err=%v; want all false/nil", applied, proceed, err)
	}
}

func TestPoolClaudeRectifier_NilReceiverNoPanic(t *testing.T) {
	var r *PoolClaudeRectifier
	_, proceed := r.Apply(context.Background(), ClaudeInput{AccountType: "oauth", AccountID: 1, Platform: "anthropic", Body: []byte(`{}`)}, StageThinkingOnly)
	if proceed {
		t.Errorf("nil receiver must return proceed=false")
	}
}

func TestStripAntigravityRectifier_BothStages(t *testing.T) {
	called := [2]bool{}
	r := &StripAntigravityRectifier{
		StripStage1: func(req *antigravity.ClaudeRequest) (bool, error) { called[0] = true; return true, nil },
		StripStage2: func(req *antigravity.ClaudeRequest) (bool, error) { called[1] = true; return true, nil },
	}
	stages := r.Stages()
	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}
	for i, stage := range stages {
		in := AntigravityInput{AccountType: "oauth", AccountID: 1, Platform: "antigravity", Request: &antigravity.ClaudeRequest{}}
		applied, proceed, err := r.Apply(context.Background(), in, stage)
		if err != nil || !applied || !proceed {
			t.Errorf("stage %d: applied=%v proceed=%v err=%v", i, applied, proceed, err)
		}
		if !called[i] {
			t.Errorf("stage %d strip function not called", i)
		}
	}
}
