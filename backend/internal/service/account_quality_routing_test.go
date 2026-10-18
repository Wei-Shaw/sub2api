package service

import (
	"context"
	"fmt"
	"math"
	rand "math/rand"
	"testing"
	"time"
)

func qualityRoutingFixture() (*AccountQualityService, context.Context, int64) {
	p := DefaultAccountQualityPolicy()
	p.Mode = "auto"
	p.Model = "fixture-model"
	s := NewAccountQualityService(nil, nil)
	s.routing.Store(&accountQualityRoutingSnapshot{LoadedAt: time.Now(), Groups: map[int64]accountQualityRoutingGroup{1: {Policy: p, Weights: map[int64]accountQualityRoutingEntry{1: {Multiplier: 1.8}, 2: {Multiplier: .1, ConfirmedBad: true}, 3: {Multiplier: 2}}}}})
	ctx := WithAccountQualityRequestScope(context.Background(), AccountQualityRequestScope{Model: p.Model, Effort: p.Effort, ServiceTier: p.ServiceTier, Endpoint: p.Endpoint})
	return s, ctx, 1
}
func TestAccountQualityRoutingScopeIsStrictAndExpires(t *testing.T) {
	s, ctx, id := qualityRoutingFixture()
	if _, ok := s.matchRouting(ctx, &id, "fixture-model"); !ok {
		t.Fatal("exact configured request must match")
	}
	other := int64(2)
	if _, ok := s.matchRouting(ctx, &other, "fixture-model"); ok {
		t.Fatal("weights crossed groups")
	}
	for _, scope := range []AccountQualityRequestScope{{Model: "other", Effort: "low", Endpoint: "responses"}, {Model: "fixture-model", Effort: "high", Endpoint: "responses"}, {Model: "fixture-model", Effort: "low", ServiceTier: "priority", Endpoint: "responses"}, {Model: "fixture-model", Effort: "low", Endpoint: "messages"}} {
		if _, ok := s.matchRouting(WithAccountQualityRequestScope(ctx, scope), &id, "fixture-model"); ok {
			t.Fatal("mismatched request received quality weights", scope)
		}
	}
	state := s.routing.Load()
	state = &accountQualityRoutingSnapshot{LoadedAt: time.Now().Add(-3 * time.Minute), Groups: state.Groups}
	s.routing.Store(state)
	if _, ok := s.matchRouting(ctx, &id, "fixture-model"); ok {
		t.Fatal("stale snapshot must fall back to native routing")
	}
}
func TestAccountQualityRoutingPreservesPriorityAndDisabledModes(t *testing.T) {
	s, ctx, id := qualityRoutingFixture()
	accounts := []*Account{{ID: 3, Priority: 3}, {ID: 1, Priority: 1}, {ID: 2, Priority: 1}}
	for n := 0; n < 200; n++ {
		ordered := s.orderAccounts(ctx, &id, "fixture-model", accounts)
		if ordered[0].Priority != 1 || ordered[2].ID != 3 {
			t.Fatal("manual priority tiers were bypassed")
		}
	}
	state := s.routing.Load()
	g := state.Groups[id]
	g.Policy.Mode = "observe"
	s.routing.Store(&accountQualityRoutingSnapshot{LoadedAt: time.Now(), Groups: map[int64]accountQualityRoutingGroup{id: g}})
	ordered := s.orderAccounts(ctx, &id, "fixture-model", accounts)
	if ordered[0].ID != 3 {
		t.Fatal("observation mode changed native ordering")
	}
	if s.shouldEscapeSticky(ctx, &id, "fixture-model", 2) {
		t.Fatal("observation mode changed sticky binding")
	}
}
func TestAccountQualityWeightedSamplingRetainsExploration(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	counts := [2]int{}
	for n := 0; n < 10000; n++ {
		indices := qualityWeightedPermutation([]float64{1000, .0001}, rng.Float64)
		if len(indices) != 2 || indices[0] == indices[1] {
			t.Fatal("invalid permutation")
		}
		counts[indices[0]]++
	}
	if counts[1] < 150 || counts[0] < 9000 {
		t.Fatalf("exploration / weighting mass incorrect: %v", counts)
	}
}
func TestAccountQualityModernSchedulerUsesQualityBeyondTopK(t *testing.T) {
	q, ctx, id := qualityRoutingFixture()
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{accountQuality: q}}
	plan := openAIAccountLoadPlan{topK: 1, candidates: []openAIAccountCandidateScore{{account: &Account{ID: 1}, score: 1}, {account: &Account{ID: 2}, score: 1}}}
	req := OpenAIAccountScheduleRequest{GroupID: &id, RequestedModel: "fixture-model"}
	scheduler.applyAccountQuality(ctx, req, &plan)
	if plan.topK != 2 || plan.candidates[0].qualityWeight <= plan.candidates[1].qualityWeight {
		t.Fatal("quality weights were lost to a top-one cutoff")
	}
	counts := [2]int{}
	for n := 0; n < 2000; n++ {
		req.SessionHash = fmt.Sprintf("session-%d", n)
		order := scheduler.buildOpenAISelectionOrder(req, plan)
		counts[order[0].account.ID-1]++
	}
	if counts[0] < counts[1]*5 || counts[1] == 0 {
		t.Fatalf("actual scheduler ordering ignored quality or exploration: %v", counts)
	}
}
func TestAccountQualityBodyScopeDoesNotAssumeMissingEffort(t *testing.T) {
	ctx := WithAccountQualityBodyScope(context.Background(), "fixture-model", []byte(`{"reasoning":{"effort":"high"},"service_tier":"fast"}`), "/v1/responses")
	v := ctx.Value(accountQualityScopeKey{}).(AccountQualityRequestScope)
	if v.Effort != "high" || v.ServiceTier != "priority" || v.Endpoint != "responses" {
		t.Fatal("request condition normalization failed", v)
	}
	ctx = WithAccountQualityBodyScope(context.Background(), "fixture-model", []byte(`{}`), "/v1/responses/compact")
	v = ctx.Value(accountQualityScopeKey{}).(AccountQualityRequestScope)
	if v.Effort != "" || v.Endpoint != "" {
		t.Fatal("unproven effort or compact endpoint was treated as an ordinary text request")
	}
}
func TestAccountQualityScoreWeightsAreConfigurable(t *testing.T) {
	p := DefaultAccountQualityPolicy()
	p.ScoreWeights = AccountQualityScoreWeights{Cache: 100}
	ttft := 3000.0
	cache := 20.0
	rows := []AccountQualityRow{{Requests: 50, TTFTSamples: 50, P50: &ttft, CacheRate: &cache}}
	ScoreAccountQualityWithPolicy(rows, p)
	if rows[0].Score == nil || math.Abs(*rows[0].Score-20) > .01 {
		t.Fatal("configured score weights were ignored")
	}
	if rows[0].Components[2].EffectiveWeight != 100 {
		t.Fatal("effective component normalization missing")
	}
	p.ScoreWeights = AccountQualityScoreWeights{}
	if p.Validate() == nil {
		t.Fatal("zero score weights accepted")
	}
}

type qualityWeightRepo struct {
	AccountQualityRepository
	groups  []AccountQualityGroup
	rows    []AccountQualityRow
	weights map[int64]AccountQualityStoredWeight
}

func (r *qualityWeightRepo) Groups(context.Context) ([]AccountQualityGroup, error) {
	return r.groups, nil
}
func (r *qualityWeightRepo) Rows(context.Context, int64, AccountQualityPolicy) ([]AccountQualityRow, error) {
	return append([]AccountQualityRow(nil), r.rows...), nil
}
func (r *qualityWeightRepo) LoadWeights(context.Context, int64, string) (map[int64]AccountQualityStoredWeight, error) {
	return r.weights, nil
}
func (r *qualityWeightRepo) SaveWeights(_ context.Context, _ int64, _ string, w map[int64]AccountQualityStoredWeight) error {
	r.weights = w
	return nil
}
func TestAccountQualityWeightRefreshIsGradualAndManualModeExits(t *testing.T) {
	p := DefaultAccountQualityPolicy()
	p.Mode = "auto"
	p.Model = "fixture-model"
	now := time.Now()
	repo := &qualityWeightRepo{groups: []AccountQualityGroup{{ID: 1, Policy: p}}, rows: []AccountQualityRow{{ID: 1, Status: "active", Schedulable: true, QualitySamples: 3, QualityPassed: 3, LastTestAt: &now}, {ID: 2, Status: "active", Schedulable: true, QualitySamples: 3, LastTestAt: &now}}, weights: map[int64]AccountQualityStoredWeight{}}
	s := NewAccountQualityService(repo, nil)
	if err := s.refreshRouting(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := repo.weights[1].Multiplier
	bad := repo.weights[2].Multiplier
	if math.Abs(first-1.15) > .001 || math.Abs(bad-.85) > .001 {
		t.Fatalf("weight moved too quickly or wrong direction: %v %v", first, bad)
	}
	if err := s.refreshRouting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.weights[1].Multiplier-first > .005 {
		t.Fatal("frequent refresh must not accelerate ramp")
	}
	repo.groups[0].Policy.Mode = "manual"
	s.invalidateRoutingGroup(1)
	if err := s.refreshRouting(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := int64(1)
	ctx := WithAccountQualityRequestScope(context.Background(), AccountQualityRequestScope{Model: p.Model, Effort: p.Effort, Endpoint: p.Endpoint})
	if _, ok := s.matchRouting(ctx, &id, p.Model); ok {
		t.Fatal("manual rollback retained routing weights")
	}
}

func TestAccountQualityFailureScopeContainsOnlyConditions(t *testing.T) {
	ctx := WithAccountQualityBodyScope(context.Background(), "fixture-model", []byte(`{"reasoning":{"effort":"high"},"service_tier":"priority","secret":"must-not-copy"}`), "/v1/responses")
	entry := &OpsInsertErrorLogInput{}
	AttachAccountQualityErrorScope(ctx, entry)
	if !entry.QualityScopeKnown || entry.QualityReasoningEffort != "high" || entry.QualityServiceTier != "priority" || entry.ErrorBody != "" {
		t.Fatal("only non-secret condition fields should be copied")
	}
	unknown := &OpsInsertErrorLogInput{}
	AttachAccountQualityErrorScope(context.Background(), unknown)
	if unknown.QualityScopeKnown {
		t.Fatal("legacy errors must remain unknown")
	}
}

func TestAccountQualityFullOpenAISelectionHonorsWeightsAdmissionAndRollback(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	q, ctx, g := qualityRoutingFixture()
	cfg := newSchedulerTestSubscriptionPriorityConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	accounts := []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: "active", Schedulable: true, Concurrency: 2, GroupIDs: []int64{g}}, {ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: "active", Schedulable: true, Concurrency: 2, GroupIDs: []int64{g}}, {ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: "disabled", Schedulable: false, Concurrency: 2, GroupIDs: []int64{g}}}
	gateway := &OpenAIGatewayService{accountQuality: q, accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cache: &schedulerTestGatewayCache{}, cfg: cfg, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{})}
	counts := [2]int{}
	for n := 0; n < 400; n++ {
		selection, _, err := gateway.SelectAccountWithScheduler(ctx, &g, "", fmt.Sprintf("quality-fixture-%d", n), "fixture-model", nil, OpenAIUpstreamTransportAny, false)
		if err != nil {
			t.Fatal("full quality selection failed", err)
		}
		if selection == nil || selection.Account == nil || selection.Account.ID == 3 {
			t.Fatal("admission allowed a disabled account")
		}
		counts[selection.Account.ID-1]++
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	}
	if counts[0] < counts[1]*3 || counts[1] == 0 {
		t.Fatalf("full request selection did not apply group quality weights with exploration: %v", counts)
	}
	if len(q.RoutingEvents(g, "")) == 0 {
		t.Fatal("selected account is not explained in routing events")
	}
	state := q.routing.Load()
	entry := state.Groups[g]
	entry.Policy.Mode = "manual"
	q.routing.Store(&accountQualityRoutingSnapshot{LoadedAt: time.Now(), Groups: map[int64]accountQualityRoutingGroup{g: entry}})
	before := len(q.RoutingEvents(g, ""))
	selection, _, err := gateway.SelectAccountWithScheduler(ctx, &g, "", "manual-rollback", "fixture-model", nil, OpenAIUpstreamTransportAny, false)
	if err != nil || selection == nil {
		t.Fatal("manual rollback failed", err)
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	if len(q.RoutingEvents(g, "")) != before {
		t.Fatal("manual mode still produced automatic routing events")
	}
}
