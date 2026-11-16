package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func quotaTestConfig() GroupQuotaPolicyConfig {
	return GroupQuotaPolicyConfig{Rules: []GroupQuotaPolicyRule{{ID: "first", Name: "test", Priority: 10, Mode: "enforce", AccountIDs: []int64{1},
		AccountMatch: "any", WindowMatch: "any", Conditions: []GroupQuotaPolicyCondition{{Window: "5h", TriggerRemaining: 20, RecoverRemaining: 40}},
		Recovery: "auto", Mappings: []GroupQuotaPolicyMapping{{Source: "source", Target: "target"}}}}}
}

func quotaTestRuntime(now time.Time, remaining float64) GroupQuotaPolicyRuntime {
	return GroupQuotaPolicyRuntime{Snapshots: []GroupQuotaPolicySnapshot{{AccountID: 1, SampledAt: now.Unix(), CheckedAt: now.Unix(),
		FiveHour: &GroupQuotaPolicyWindow{RemainingPercent: remaining, ResetAt: now.Add(time.Hour).Unix()}}}}
}

func TestGroupQuotaPolicyHysteresisAndUnknown(t *testing.T) {
	now, config := time.Now(), quotaTestConfig()
	runtime := evaluateGroupQuotaPolicy(config, quotaTestRuntime(now, 20), now)
	require.True(t, runtime.Rules[0].Active, "trigger is inclusive")
	runtime.Snapshots[0].FiveHour.RemainingPercent = 30
	runtime = evaluateGroupQuotaPolicy(config, runtime, now)
	require.True(t, runtime.Rules[0].Active, "between thresholds preserves state")
	runtime.Snapshots[0].FiveHour.RemainingPercent = 80
	runtime.Snapshots[0].SampledAt = now.Add(-4 * time.Minute).Unix()
	runtime = evaluateGroupQuotaPolicy(config, runtime, now)
	require.True(t, runtime.Rules[0].Active, "stale data must not recover")
	require.Equal(t, "unknown", runtime.Rules[0].Status)
	runtime.Snapshots[0].SampledAt = now.Unix()
	runtime.Snapshots[0].FiveHour.RemainingPercent = 40
	runtime = evaluateGroupQuotaPolicy(config, runtime, now)
	require.False(t, runtime.Rules[0].Active, "recovery is inclusive")
	runtime.Snapshots[0].FiveHour = nil
	runtime = evaluateGroupQuotaPolicy(config, runtime, now)
	require.False(t, runtime.Rules[0].Active, "missing window cannot trigger")
}

func TestGroupQuotaPolicyAnyAllAndConservativeRecovery(t *testing.T) {
	now, config := time.Now(), quotaTestConfig()
	rule := &config.Rules[0]
	rule.AccountIDs = []int64{1, 2}
	rule.Conditions = append(rule.Conditions, GroupQuotaPolicyCondition{Window: "7d", TriggerRemaining: 10, RecoverRemaining: 30})
	runtime := quotaTestRuntime(now, 10)
	runtime.Snapshots[0].SevenDay = &GroupQuotaPolicyWindow{RemainingPercent: 50}
	runtime.Snapshots = append(runtime.Snapshots, GroupQuotaPolicySnapshot{AccountID: 2, SampledAt: now.Unix(), FiveHour: &GroupQuotaPolicyWindow{RemainingPercent: 60}, SevenDay: &GroupQuotaPolicyWindow{RemainingPercent: 60}})
	for _, tc := range []struct {
		accounts, windows string
		active            bool
	}{{"any", "any", true}, {"all", "any", false}, {"any", "all", false}, {"all", "all", false}} {
		rule.AccountMatch, rule.WindowMatch = tc.accounts, tc.windows
		evaluated := evaluateGroupQuotaPolicy(config, runtime, now)
		require.Equal(t, tc.active, evaluated.Rules[0].Active, "%s/%s", tc.accounts, tc.windows)
	}
	rule.AccountMatch, rule.WindowMatch = "any", "any"
	runtime = evaluateGroupQuotaPolicy(config, runtime, now)
	runtime.Snapshots[0].FiveHour.RemainingPercent = 39
	require.True(t, evaluateGroupQuotaPolicy(config, runtime, now).Rules[0].Active, "all selected windows/accounts must recover")
	runtime.Snapshots[0].FiveHour.RemainingPercent = 40
	require.False(t, evaluateGroupQuotaPolicy(config, runtime, now).Rules[0].Active)
	runtime.Snapshots[1].SevenDay = nil
	runtime.Rules = nil
	runtime.Snapshots[0].FiveHour.RemainingPercent = 5
	require.False(t, evaluateGroupQuotaPolicy(config, runtime, now).Rules[0].Active, "ANY still needs complete fresh evidence")
}

func TestGroupQuotaPolicyManualReleaseAndModes(t *testing.T) {
	now, config := time.Now(), quotaTestConfig()
	config.Rules[0].Recovery = "manual"
	runtime := evaluateGroupQuotaPolicy(config, quotaTestRuntime(now, 5), now)
	runtime.Snapshots[0].FiveHour.RemainingPercent = 95
	require.True(t, evaluateGroupQuotaPolicy(config, runtime, now).Rules[0].Active)
	runtime.Rules[0].Active, runtime.Rules[0].Suppressed = false, true
	runtime.Snapshots[0].FiveHour.RemainingPercent = 5
	runtime = evaluateGroupQuotaPolicy(config, runtime, now)
	require.False(t, runtime.Rules[0].Active)
	require.True(t, runtime.Rules[0].Suppressed)
	runtime.Snapshots[0].FiveHour.RemainingPercent = 50
	runtime = evaluateGroupQuotaPolicy(config, runtime, now)
	require.False(t, runtime.Rules[0].Suppressed)
	runtime.Snapshots[0].FiveHour.RemainingPercent = 5
	config.Rules[0].Mode = "observe"
	runtime = evaluateGroupQuotaPolicy(config, runtime, now)
	require.True(t, runtime.Rules[0].Active)
	require.False(t, resolveGroupQuotaPolicy(config, runtime, 1, "source").Applied)
	config.Rules[0].Mode = "disabled"
	require.False(t, evaluateGroupQuotaPolicy(config, runtime, now).Rules[0].Active)
}

func TestGroupQuotaPolicyRemovalVersusQueryFailure(t *testing.T) {
	now, config := time.Now(), quotaTestConfig()
	runtime := evaluateGroupQuotaPolicy(config, quotaTestRuntime(now, 5), now)
	runtime.Snapshots[0].ErrorCode = "QUOTA_QUERY_FAILED"
	require.True(t, evaluateGroupQuotaPolicy(config, runtime, now).Rules[0].Active)
	runtime.Snapshots[0].ErrorCode = "ACCOUNT_LOOKUP_FAILED"
	require.True(t, evaluateGroupQuotaPolicy(config, runtime, now).Rules[0].Active)
	runtime.Snapshots[0].ErrorCode = "ACCOUNT_UNAVAILABLE"
	require.False(t, evaluateGroupQuotaPolicy(config, runtime, now).Rules[0].Active)
}

func TestGroupQuotaPolicyExactPriorityNoChaining(t *testing.T) {
	config := quotaTestConfig()
	second := config.Rules[0]
	second.ID, second.Priority = "second", 20
	second.Mappings = []GroupQuotaPolicyMapping{{Source: "source", Target: "higher"}, {Source: "target", Target: "chained"}}
	config.Rules = append(config.Rules, second)
	runtime := GroupQuotaPolicyRuntime{Rules: []GroupQuotaPolicyRuleState{{RuleID: "first", Active: true}, {RuleID: "second", Active: true}}}
	require.Equal(t, "higher", resolveGroupQuotaPolicy(config, runtime, 7, "source").TargetModel)
	config.Rules[1].Mode = "observe"
	require.Equal(t, "target", resolveGroupQuotaPolicy(config, runtime, 7, "source").TargetModel)
	require.False(t, resolveGroupQuotaPolicy(config, runtime, 7, "source-version").Applied)
	config.Rules = nil
	require.Equal(t, "source", resolveGroupQuotaPolicy(config, runtime, 7, "source").TargetModel)
}

func TestGroupQuotaPolicyWindowNormalizationAndPresence(t *testing.T) {
	now := time.Now()
	usage := &OpenAIQuotaUsage{FetchedAt: now.Unix(), RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 90, LimitWindowSeconds: 604800}, SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 30, LimitWindowSeconds: 18000}}}
	snapshot := groupQuotaSnapshot(1, usage, now)
	require.Equal(t, float64(70), snapshot.FiveHour.RemainingPercent)
	require.Equal(t, float64(10), snapshot.SevenDay.RemainingPercent)
	validateGroupQuotaUsageWindowPresence(usage, []byte(`{"rate_limit":{"primary_window":{"used_percent":0},"secondary_window":{"used_percent":null}}}`))
	require.NotNil(t, usage.RateLimit.PrimaryWindow, "explicit zero is valid")
	require.Nil(t, usage.RateLimit.SecondaryWindow, "null is unknown")
	validateGroupQuotaUsageWindowPresence(usage, []byte(`{"rate_limit":{"primary_window":{"limit_window_seconds":604800}}}`))
	require.Nil(t, usage.RateLimit.PrimaryWindow, "missing is unknown")
	usage.RateLimit.PrimaryWindow = &OpenAIRateLimitWindow{UsedPercent: math.NaN(), LimitWindowSeconds: 18000}
	require.Nil(t, groupQuotaSnapshot(1, usage, now).FiveHour)
}

func TestGroupQuotaPolicyOlderSnapshotsCannotOverwrite(t *testing.T) {
	now, config := time.Now(), quotaTestConfig()
	runtime := quotaTestRuntime(now, 80)
	old := quotaTestRuntime(now.Add(-time.Minute), 5).Snapshots[0]
	old.CheckedAt = now.Add(time.Minute).Unix()
	merged := mergeGroupQuotaSamples(runtime, map[int64]GroupQuotaPolicySnapshot{1: old}, config)
	require.Equal(t, float64(80), merged.Snapshots[0].FiveHour.RemainingPercent)
	failure := GroupQuotaPolicySnapshot{AccountID: 1, CheckedAt: now.Add(time.Second).Unix(), ErrorCode: "QUOTA_QUERY_FAILED"}
	merged = mergeGroupQuotaSamples(runtime, map[int64]GroupQuotaPolicySnapshot{1: failure}, config)
	require.Equal(t, float64(80), merged.Snapshots[0].FiveHour.RemainingPercent)
	require.Equal(t, failure.ErrorCode, merged.Snapshots[0].ErrorCode)
}

func TestGroupQuotaPolicyValidationAndEmergencyDisable(t *testing.T) {
	config := quotaTestConfig()
	require.NoError(t, validateGroupQuotaPolicy(config, map[int64]bool{1: true}))
	require.Error(t, validateGroupQuotaPolicy(config, nil))
	config.Rules[0].Mode = "disabled"
	require.NoError(t, validateGroupQuotaPolicy(config, nil), "removed account must not prevent disabling rule")
	config.Rules[0].Conditions[0].RecoverRemaining = 20
	require.Error(t, validateGroupQuotaPolicy(config, nil))
	config = quotaTestConfig()
	config.Rules[0].Mappings[0].Source = "source*"
	require.Error(t, validateGroupQuotaPolicy(config, map[int64]bool{1: true}))
	shadowID := int64(2)
	require.False(t, groupQuotaEligible(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &shadowID}))
}

type quotaPolicyMemoryRepo struct {
	mu      sync.Mutex
	records map[int64]*GroupQuotaPolicyRecord
}

func cloneQuotaRecord(record *GroupQuotaPolicyRecord) *GroupQuotaPolicyRecord {
	body, _ := json.Marshal(record)
	var result GroupQuotaPolicyRecord
	_ = json.Unmarshal(body, &result)
	return &result
}

func (r *quotaPolicyMemoryRepo) Get(_ context.Context, id int64) (*GroupQuotaPolicyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if record := r.records[id]; record != nil {
		return cloneQuotaRecord(record), nil
	}
	return EmptyGroupQuotaPolicyRecord(id), nil
}
func (r *quotaPolicyMemoryRepo) List(_ context.Context) ([]*GroupQuotaPolicyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := []*GroupQuotaPolicyRecord{}
	for _, record := range r.records {
		result = append(result, cloneQuotaRecord(record))
	}
	return result, nil
}
func (r *quotaPolicyMemoryRepo) save(record *GroupQuotaPolicyRecord, revision, runtimeVersion int64, config bool) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.records[record.GroupID]
	if old == nil {
		old = EmptyGroupQuotaPolicyRecord(record.GroupID)
	}
	if old.Revision != revision || old.RuntimeVersion != runtimeVersion {
		return false, nil
	}
	copy := cloneQuotaRecord(record)
	copy.RuntimeVersion++
	if config {
		copy.Revision++
	}
	r.records[record.GroupID] = copy
	return true, nil
}
func (r *quotaPolicyMemoryRepo) SaveConfig(_ context.Context, record *GroupQuotaPolicyRecord, rev, version int64) (bool, error) {
	return r.save(record, rev, version, true)
}
func (r *quotaPolicyMemoryRepo) SaveRuntime(_ context.Context, record *GroupQuotaPolicyRecord, rev, version int64) (bool, error) {
	return r.save(record, rev, version, false)
}

type quotaPolicyGroups struct{ GroupRepository }

func (quotaPolicyGroups) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	return &Group{ID: id, Platform: PlatformOpenAI}, nil
}

type quotaPolicyAccounts struct{ AccountRepository }

func (quotaPolicyAccounts) ListByGroup(context.Context, int64) ([]Account, error) {
	return []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}}, nil
}

type quotaPolicyQuerier struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (q *quotaPolicyQuerier) QueryUsageWindows(context.Context, int64) (*OpenAIQuotaUsage, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls++
	if q.err != nil {
		return nil, q.err
	}
	return &OpenAIQuotaUsage{FetchedAt: time.Now().Unix(), RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 95, LimitWindowSeconds: 18000}}}, nil
}

func TestGroupQuotaPolicyServicePersistenceDedupAndReadOnlyPreview(t *testing.T) {
	repo := &quotaPolicyMemoryRepo{records: make(map[int64]*GroupQuotaPolicyRecord)}
	service := NewGroupQuotaPolicyService(repo, quotaPolicyGroups{}, quotaPolicyAccounts{}, nil, nil)
	query := &quotaPolicyQuerier{}
	service.quota = query
	ctx := context.Background()
	for _, id := range []int64{10, 20} {
		_, err := service.Save(ctx, id, 0, quotaTestConfig())
		require.NoError(t, err)
	}
	service.scan()
	require.Equal(t, 1, query.calls, "same account is sampled once across groups")
	for _, id := range []int64{10, 20} {
		require.True(t, service.Resolve(id, "source").Applied)
	}
	for i := 0; i < 100; i++ {
		service.Resolve(10, "source")
	}
	require.Equal(t, 1, query.calls, "gateway resolution must never query upstream")
	before, _ := repo.Get(ctx, 10)
	preview, err := service.Preview(ctx, 10, quotaTestConfig(), "source")
	require.NoError(t, err)
	require.True(t, preview.Resolution.Applied)
	after, _ := repo.Get(ctx, 10)
	require.Equal(t, before, after, "preview must not write or query")
	_, err = service.Save(ctx, 10, 0, quotaTestConfig())
	require.ErrorIs(t, err, ErrGroupQuotaPolicyConflict)
	_, err = service.Release(ctx, 10, 1, "first")
	require.NoError(t, err)
	require.False(t, service.Resolve(10, "source").Applied)
	restarted := NewGroupQuotaPolicyService(repo, quotaPolicyGroups{}, quotaPolicyAccounts{}, nil, nil)
	_, err = restarted.syncRecords(ctx)
	require.NoError(t, err)
	require.False(t, restarted.Resolve(10, "source").Applied)
	require.True(t, restarted.Resolve(20, "source").Applied)
	service.scan()
	require.False(t, service.Resolve(10, "source").Applied, "low quota must not retrigger manual release")
	query.err = errors.New("credentials must never be persisted")
	service.sampleMu.Lock()
	service.samples = map[int64]GroupQuotaPolicySnapshot{}
	service.sampleMu.Unlock()
	service.scan()
	view, err := service.Get(ctx, 20)
	require.NoError(t, err)
	require.True(t, view.Runtime.Rules[0].Active)
	require.Equal(t, "QUOTA_QUERY_FAILED", view.Runtime.Snapshots[0].ErrorCode)
}

func TestGroupQuotaPolicyPublishRejectsOlderRuntime(t *testing.T) {
	s := NewGroupQuotaPolicyService(nil, nil, nil, nil, nil)
	record := EmptyGroupQuotaPolicyRecord(1)
	record.Revision, record.RuntimeVersion, record.Config = 1, 2, quotaTestConfig()
	record.Runtime.Rules = []GroupQuotaPolicyRuleState{{RuleID: "first", Active: false, Suppressed: true}}
	s.publish(record)
	record.RuntimeVersion = 1
	record.Runtime.Rules[0].Active, record.Runtime.Rules[0].Suppressed = true, false
	s.publish(record)
	require.False(t, s.Resolve(1, "source").Applied, "a late read must not reverse a manual release")
}
