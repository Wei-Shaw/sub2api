//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type modelDowngradeSettingRepo struct {
	SettingRepository
	value    string
	getErr   error
	getCalls int
}

func (r *modelDowngradeSettingRepo) GetValue(context.Context, string) (string, error) {
	r.getCalls++
	if r.getErr != nil {
		return "", r.getErr
	}
	if r.value == "" {
		return "", ErrSettingNotFound
	}
	return r.value, nil
}

func (r *modelDowngradeSettingRepo) Set(_ context.Context, _, value string) error {
	r.value = value
	return nil
}

type modelDowngradeAccountRepo struct {
	AccountRepository
	tempCalls       int
	tempID          int64
	tempUntil       time.Time
	tempReason      string
	modelCalls      int
	modelScope      string
	modelUntil      time.Time
	modelReason     string
	blockedAccounts int64
	totalAccounts   int64
	countErr        error
	countCalls      int
	listItems       []ModelDowngradeBlockedAccount
	listErr         error
	listCalls       int
	applyErr        error
	applyScopes     []string
	applyMaxRatios  []float64
	// alreadyBlocked 复刻仓储里的「当前账号是否已经计入 blocked」判定：
	// 命中的账号再加一条限制不会让分子变大。
	alreadyBlocked map[int64]bool
	// accountBlocked / modelBlocked 复刻幂等判定：前者是守卫写的整账号下线（覆盖所有
	// 模型），后者按「账号:模型」记守卫写的模型屏蔽。
	accountBlocked map[int64]bool
	modelBlocked   map[string]bool
	releaseErr     error
	releaseOK      bool
	releaseCalls   int
	releaseID      int64
	releaseScope   string
	releaseModel   string
	account        *Account
	getByIDErr     error
	getByIDCalls   int
}

func (r *modelDowngradeAccountRepo) CountOpenAIModelDowngradeBlocked(context.Context, time.Time) (int64, int64, error) {
	r.countCalls++
	if r.countErr != nil {
		return 0, 0, r.countErr
	}
	return r.blockedAccounts, r.totalAccounts, nil
}

func (r *modelDowngradeAccountRepo) ListOpenAIModelDowngradeBlocked(context.Context, time.Time) ([]ModelDowngradeBlockedAccount, error) {
	r.listCalls++
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.listItems, nil
}

// ApplyOpenAIModelDowngradeBlock 复刻真实仓储的判定：覆盖判定 → 统计 → 比例上限 →
// 条件写入，让 service 层的单测能覆盖「被上限拦下时什么都不该发生」和「已受限时
// 不重复处理」两条路径。
func (r *modelDowngradeAccountRepo) ApplyOpenAIModelDowngradeBlock(
	_ context.Context,
	id int64,
	scope string,
	model string,
	until time.Time,
	reason string,
	maxRatio float64,
	_ time.Time,
) (ModelDowngradeBlockApplyResult, error) {
	r.countCalls++
	r.applyScopes = append(r.applyScopes, scope)
	r.applyMaxRatios = append(r.applyMaxRatios, maxRatio)
	if r.countErr != nil {
		return ModelDowngradeBlockApplyResult{}, r.countErr
	}
	if r.applyErr != nil {
		return ModelDowngradeBlockApplyResult{Blocked: r.blockedAccounts, Total: r.totalAccounts}, r.applyErr
	}
	// 整账号下线覆盖所有模型；同一个模型的屏蔽只覆盖同模型的 model 范围请求，
	// account 范围请求照常写（升级而不是重复）。
	if r.accountBlocked[id] || (scope == ModelDowngradeBlockedScopeModel && r.modelBlocked[modelDowngradeObservedDedupeKey(id, model)]) {
		return ModelDowngradeBlockApplyResult{AlreadyBlocked: true}, nil
	}
	blocked, total := r.blockedAccounts, r.totalAccounts
	projected := blocked + 1
	if r.alreadyBlocked[id] {
		projected = blocked
	}
	if total <= 0 || float64(projected)/float64(total) > maxRatio {
		return ModelDowngradeBlockApplyResult{Blocked: blocked, Total: total}, nil
	}
	switch scope {
	case ModelDowngradeBlockedScopeModel:
		r.modelCalls++
		r.modelScope = model
		r.modelUntil = until
		r.modelReason = reason
	case ModelDowngradeBlockedScopeAccount:
		r.tempCalls++
		r.tempID = id
		r.tempUntil = until
		r.tempReason = reason
	default:
		return ModelDowngradeBlockApplyResult{Blocked: blocked, Total: total}, nil
	}
	return ModelDowngradeBlockApplyResult{Applied: true, Blocked: blocked, Total: total}, nil
}

// GetByID 供手动处理用：它得先把账号捞出来才能写 runtime blocker。
func (r *modelDowngradeAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.getByIDCalls++
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	if r.account == nil {
		return nil, nil
	}
	clone := *r.account
	clone.ID = id
	return &clone, nil
}

func (r *modelDowngradeAccountRepo) ReleaseOpenAIModelDowngradeBlock(_ context.Context, id int64, scope, model string) (bool, error) {
	r.releaseCalls++
	r.releaseID = id
	r.releaseScope = scope
	r.releaseModel = model
	if r.releaseErr != nil {
		return false, r.releaseErr
	}
	return r.releaseOK, nil
}

type modelDowngradeCounterStub struct {
	counts     []int64
	incrCalls  []string
	resetCalls []string
	incrErr    error
	lastWindow int
}

func (c *modelDowngradeCounterStub) IncrementModelDowngradeCount(_ context.Context, _ int64, sentModel string, windowMinutes int) (int64, error) {
	c.incrCalls = append(c.incrCalls, sentModel)
	c.lastWindow = windowMinutes
	if c.incrErr != nil {
		return 0, c.incrErr
	}
	if len(c.counts) == 0 {
		return int64(len(c.incrCalls)), nil
	}
	count := c.counts[0]
	c.counts = c.counts[1:]
	return count, nil
}

func (c *modelDowngradeCounterStub) ResetModelDowngradeCount(_ context.Context, _ int64, sentModel string) error {
	c.resetCalls = append(c.resetCalls, sentModel)
	return nil
}

type modelDowngradeTempUnschedCacheStub struct {
	TempUnschedCache
	setCalls    int
	deleteCalls int
	lastState   *TempUnschedState
}

func (c *modelDowngradeTempUnschedCacheStub) SetTempUnsched(_ context.Context, _ int64, state *TempUnschedState) error {
	c.setCalls++
	c.lastState = state
	return nil
}

func (c *modelDowngradeTempUnschedCacheStub) DeleteTempUnsched(_ context.Context, _ int64) error {
	c.deleteCalls++
	return nil
}

type modelDowngradeRuntimeBlocker struct {
	calls      int
	clearCalls int
	reasons    []string
}

func (b *modelDowngradeRuntimeBlocker) BlockAccountScheduling(_ *Account, _ time.Time, reason string) {
	b.calls++
	b.reasons = append(b.reasons, reason)
}

func (b *modelDowngradeRuntimeBlocker) ClearAccountSchedulingBlock(int64) { b.clearCalls++ }

// modelDowngradeObservedStub 用 map 建模真实实现的语义：同一账号 + 模型覆盖，
// 删除返回是否命中。只记调用次数的 stub 覆盖不到「写入后又被删掉」这类时序。
type modelDowngradeObservedStub struct {
	entries     map[string]ModelDowngradeObservedEntry
	recordTTLs  []time.Duration
	recordErr   error
	listErr     error
	deleteErr   error
	deleteCalls []string
}

func newModelDowngradeObservedStub() *modelDowngradeObservedStub {
	return &modelDowngradeObservedStub{entries: map[string]ModelDowngradeObservedEntry{}}
}

func (c *modelDowngradeObservedStub) RecordObserved(_ context.Context, entry ModelDowngradeObservedEntry, ttl time.Duration) error {
	if c.recordErr != nil {
		return c.recordErr
	}
	c.recordTTLs = append(c.recordTTLs, ttl)
	c.entries[modelDowngradeObservedDedupeKey(entry.AccountID, entry.SentModel)] = entry
	return nil
}

func (c *modelDowngradeObservedStub) ListObserved(context.Context) ([]ModelDowngradeObservedEntry, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	out := make([]ModelDowngradeObservedEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccountID < out[j].AccountID })
	return out, nil
}

func (c *modelDowngradeObservedStub) DeleteObserved(_ context.Context, accountID int64, sentModel string) (bool, error) {
	key := modelDowngradeObservedDedupeKey(accountID, sentModel)
	c.deleteCalls = append(c.deleteCalls, key)
	if c.deleteErr != nil {
		return false, c.deleteErr
	}
	if _, ok := c.entries[key]; !ok {
		return false, nil
	}
	delete(c.entries, key)
	return true, nil
}

func (c *modelDowngradeObservedStub) get(t *testing.T, accountID int64, sentModel string) ModelDowngradeObservedEntry {
	t.Helper()
	entry, ok := c.entries[modelDowngradeObservedDedupeKey(accountID, sentModel)]
	require.True(t, ok, "expected an observed entry for %d/%s", accountID, sentModel)
	return entry
}

func modelDowngradeGuardJSON(t *testing.T, settings *ModelDowngradeGuardSettings) string {
	t.Helper()
	raw, err := json.Marshal(settings)
	require.NoError(t, err)
	return string(raw)
}

// enabledModelDowngradeGuardSettings 是默认配置打开总开关：动作是默认的「仅屏蔽该模型」。
func enabledModelDowngradeGuardSettings() *ModelDowngradeGuardSettings {
	settings := DefaultModelDowngradeGuardSettings()
	settings.Enabled = true
	return settings
}

// tempUnschedModelDowngradeGuardSettings 是「整账号临时不可调度」的可选动作。
func tempUnschedModelDowngradeGuardSettings() *ModelDowngradeGuardSettings {
	settings := enabledModelDowngradeGuardSettings()
	settings.Action = ModelDowngradeGuardActionTempUnsched
	return settings
}

const modelDowngradeTestAccountID int64 = 101

func modelDowngradeTestAccount() *Account {
	return &Account{ID: modelDowngradeTestAccountID, Name: "openai-pool-1", Platform: PlatformOpenAI}
}

type modelDowngradeGuardHarness struct {
	svc      *RateLimitService
	repo     *modelDowngradeAccountRepo
	counter  *modelDowngradeCounterStub
	observed *modelDowngradeObservedStub
	cache    *modelDowngradeTempUnschedCacheStub
	blocker  *modelDowngradeRuntimeBlocker
	settings *modelDowngradeSettingRepo
}

func newModelDowngradeGuardHarness(t *testing.T, settings *ModelDowngradeGuardSettings) *modelDowngradeGuardHarness {
	t.Helper()
	settingRepo := &modelDowngradeSettingRepo{}
	if settings != nil {
		settingRepo.value = modelDowngradeGuardJSON(t, settings)
	}
	repo := &modelDowngradeAccountRepo{totalAccounts: 10, account: modelDowngradeTestAccount()}
	cache := &modelDowngradeTempUnschedCacheStub{}
	counter := &modelDowngradeCounterStub{}
	blocker := &modelDowngradeRuntimeBlocker{}
	observed := newModelDowngradeObservedStub()

	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)
	svc.SetSettingService(NewSettingService(settingRepo, &config.Config{}))
	svc.SetModelDowngradeCounterCache(counter)
	svc.SetModelDowngradeObservedCache(observed)
	svc.SetAccountRuntimeBlocker(blocker)

	return &modelDowngradeGuardHarness{
		svc:      svc,
		repo:     repo,
		counter:  counter,
		observed: observed,
		cache:    cache,
		blocker:  blocker,
		settings: settingRepo,
	}
}

func TestModelDowngradeGuardDisabledDoesNothing(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, DefaultModelDowngradeGuardSettings())

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Empty(t, h.counter.incrCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
}

func TestModelDowngradeGuardIgnoresUnconfiguredPair(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())

	// 别名映射：不在降级对列表里，不应该计数。
	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "codex-auto-review", "gpt-5.6-luna")
	// 升级：同样不该计数。
	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-5.6-sol", "gpt-6-sol")
	// 上游没有自报模型（或与发送一致）：不计数。
	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "")
	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-6-astra")

	require.Empty(t, h.counter.incrCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
}

func TestModelDowngradeGuardIgnoresNonOpenAIAccount(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	account := modelDowngradeTestAccount()
	account.Platform = PlatformAnthropic

	h.svc.HandleModelDowngrade(context.Background(), account, "gpt-6-astra", "gpt-5.6-luna")

	require.Empty(t, h.counter.incrCalls)
	require.Zero(t, h.settings.getCalls)
}

func TestModelDowngradeGuardBelowThresholdDoesNotBlock(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{1, 2, 3, 4}

	for i := 0; i < 4; i++ {
		h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")
	}

	require.Len(t, h.counter.incrCalls, 4)
	require.Empty(t, h.counter.resetCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
	require.Zero(t, h.repo.countCalls)
}

// 默认动作是「仅屏蔽该模型」：写 model_rate_limits[发往上游的模型]，
// 不动账号级可调度状态（runtime blocker、临时不可调度缓存都不碰）。
func TestModelDowngradeGuardDefaultActionBlocksOnlyThatModel(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	require.Equal(t, ModelDowngradeGuardActionModelBlock, settings.Action)
	settings.BlockHours = 6
	h := newModelDowngradeGuardHarness(t, settings)
	h.counter.counts = []int64{5}
	before := time.Now()

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Equal(t, []string{ModelDowngradeBlockedScopeModel}, h.repo.applyScopes)
	require.Equal(t, 1, h.repo.modelCalls)
	require.Equal(t, "gpt-6-astra", h.repo.modelScope)
	require.WithinDuration(t, before.Add(6*time.Hour), h.repo.modelUntil, time.Minute)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.cache.setCalls)
	require.Zero(t, h.blocker.calls)
	require.Equal(t, []string{"gpt-6-astra"}, h.counter.resetCalls)
	require.Equal(t, 30, h.counter.lastWindow)

	state, ok := ModelDowngradeStateFromReason(h.repo.modelReason)
	require.True(t, ok, "model_rate_limits.reason 必须能被守卫自己认出来，列表和提前恢复都靠它")
	require.Equal(t, int64(5), state.TriggerCount)
	require.Equal(t, "gpt-6-astra → gpt-5.6-luna (5 hits in 30 min)", state.ErrorMessage)
}

func TestModelDowngradeGuardTempUnschedAtThreshold(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, tempUnschedModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	before := time.Now()

	// 大小写和空格都应该被规整掉后再匹配。
	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), " GPT-6-Astra ", "GPT-5.6-LUNA")

	require.Equal(t, 1, h.repo.tempCalls)
	require.Equal(t, modelDowngradeTestAccountID, h.repo.tempID)
	require.WithinDuration(t, before.Add(24*time.Hour), h.repo.tempUntil, time.Minute)
	require.Zero(t, h.repo.modelCalls)
	require.Equal(t, []string{"GPT-6-Astra"}, h.counter.resetCalls)

	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(h.repo.tempReason), &state))
	require.Equal(t, ModelDowngradeGuardKeyword, state.MatchedKeyword)
	require.Equal(t, -1, state.RuleIndex)
	require.Zero(t, state.StatusCode)
	require.Equal(t, int64(5), state.TriggerCount)
	require.Equal(t, "GPT-6-Astra → GPT-5.6-LUNA (5 hits in 30 min)", state.ErrorMessage)

	require.Equal(t, 1, h.cache.setCalls)
	require.Equal(t, 1, h.blocker.calls)
	require.Equal(t, []string{"model_downgrade_temp_unschedulable"}, h.blocker.reasons)
}

func TestModelDowngradeGuardActionNoneOnlyRecords(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.Action = ModelDowngradeGuardActionNone
	h := newModelDowngradeGuardHarness(t, settings)
	h.counter.counts = []int64{5}

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
	require.Zero(t, h.repo.countCalls)
	require.Zero(t, h.cache.setCalls)
	require.Zero(t, h.blocker.calls)
	// 观察模式仍然清零计数，让 dry-run 日志的频率与真实动作一致。
	require.Equal(t, []string{"gpt-6-astra"}, h.counter.resetCalls)
}

func TestModelDowngradeGuardRatioCapSkipsBlocking(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, tempUnschedModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	// (3+1)/10 = 0.4 > 默认上限 0.3
	h.repo.blockedAccounts = 3
	h.repo.totalAccounts = 10

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Equal(t, 1, h.repo.countCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
	// 被上限拦下时不能留下任何"已下线"的痕迹：runtime blocker 和临时不可调度缓存
	// 都必须和库里的状态一致。
	require.Zero(t, h.blocker.calls)
	require.Zero(t, h.cache.setCalls)
	require.Equal(t, []string{"gpt-6-astra"}, h.counter.resetCalls)
}

func TestModelDowngradeGuardRatioCapSkipsModelBlock(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	// 「仅屏蔽该模型」同样要计入比例上限，不能绕过安全阀。
	h.repo.blockedAccounts = 3
	h.repo.totalAccounts = 10

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Equal(t, 1, h.repo.countCalls)
	require.Equal(t, []string{ModelDowngradeBlockedScopeModel}, h.repo.applyScopes)
	require.Zero(t, h.repo.modelCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.blocker.calls)
}

// 统计和写入合并成一次原子调用后，service 层必须把当前配置的比例上限原样传下去，
// 判定口径才不会在两处漂移。
func TestModelDowngradeGuardForwardsScopeAndRatioToRepository(t *testing.T) {
	settings := tempUnschedModelDowngradeGuardSettings()
	settings.MaxBlockedRatio = 0.5
	h := newModelDowngradeGuardHarness(t, settings)
	h.counter.counts = []int64{5}
	h.repo.blockedAccounts = 4
	h.repo.totalAccounts = 10

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Equal(t, []string{ModelDowngradeBlockedScopeAccount}, h.repo.applyScopes)
	require.Len(t, h.repo.applyMaxRatios, 1)
	require.InDelta(t, 0.5, h.repo.applyMaxRatios[0], 1e-9)
	// (4+1)/10 = 0.5，正好不超过上限，应当放行。
	require.Equal(t, 1, h.repo.tempCalls)
	require.Equal(t, 1, h.blocker.calls)
	require.Equal(t, 1, h.cache.setCalls)
}

func TestModelDowngradeGuardApplyFailureSkipsSideEffects(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, tempUnschedModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	h.repo.applyErr = errors.New("update failed")

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.blocker.calls)
	require.Zero(t, h.cache.setCalls)
	require.Empty(t, h.observed.entries)
}

func TestModelDowngradeGuardCountFailureSkipsBlocking(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	h.repo.countErr = errors.New("db down")

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Equal(t, 1, h.repo.countCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
}

// Redis 计数失败时退回 count=1：宁可不熔断，也不要凭单次命中处理账号。
func TestModelDowngradeGuardCounterFailureNeverBlocksOnSingleHit(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.incrErr = errors.New("redis down")

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Len(t, h.counter.incrCalls, 1)
	require.Zero(t, h.repo.countCalls)
	require.Zero(t, h.repo.modelCalls)
}

func TestModelDowngradeGuardSettingsAreCachedOnHotPath(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{1, 2, 3}

	for i := 0; i < 3; i++ {
		h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")
	}

	require.Equal(t, 1, h.settings.getCalls)
}

func TestGetModelDowngradeGuardSettingsDefaultsAndClamping(t *testing.T) {
	t.Run("missing key returns defaults", func(t *testing.T) {
		svc := NewSettingService(&modelDowngradeSettingRepo{}, &config.Config{})

		settings, err := svc.GetModelDowngradeGuardSettings(context.Background())

		require.NoError(t, err)
		require.False(t, settings.Enabled, "守卫默认关闭")
		require.Equal(t, ModelDowngradeGuardActionModelBlock, settings.Action, "默认只屏蔽命中的模型")
		require.Equal(t, 5, settings.ThresholdCount)
		require.Equal(t, 30, settings.ThresholdWindowMinutes)
		require.Equal(t, 24, settings.BlockHours)
		require.InDelta(t, 0.3, settings.MaxBlockedRatio, 1e-9)
		require.Equal(t, []ModelDowngradePair{{SentModel: "gpt-6-astra", ResponseModel: "gpt-5.6-luna"}}, settings.Pairs)
	})

	t.Run("out of range values are clamped", func(t *testing.T) {
		repo := &modelDowngradeSettingRepo{value: `{
			"enabled": true,
			"action": "bogus",
			"pairs": [
				{"sent_model": " gpt-6-astra ", "response_model": " gpt-5.6-luna "},
				{"sent_model": "GPT-6-ASTRA", "response_model": "GPT-5.6-LUNA"},
				{"sent_model": "  ", "response_model": "x"}
			],
			"threshold_count": 0,
			"threshold_window_minutes": 100000,
			"block_hours": 500,
			"max_blocked_ratio": 7
		}`}
		svc := NewSettingService(repo, &config.Config{})

		settings, err := svc.GetModelDowngradeGuardSettings(context.Background())

		require.NoError(t, err)
		require.Equal(t, ModelDowngradeGuardActionModelBlock, settings.Action)
		require.Equal(t, 1, settings.ThresholdCount)
		require.Equal(t, 1440, settings.ThresholdWindowMinutes)
		require.Equal(t, 72, settings.BlockHours)
		require.InDelta(t, 1.0, settings.MaxBlockedRatio, 1e-9)
		// 空白项被丢掉，大小写重复项被去重。
		require.Equal(t, []ModelDowngradePair{{SentModel: "gpt-6-astra", ResponseModel: "gpt-5.6-luna"}}, settings.Pairs)
	})

	t.Run("broken json falls back to defaults", func(t *testing.T) {
		svc := NewSettingService(&modelDowngradeSettingRepo{value: "not json"}, &config.Config{})

		settings, err := svc.GetModelDowngradeGuardSettings(context.Background())

		require.NoError(t, err)
		require.False(t, settings.Enabled)
		require.Equal(t, 24, settings.BlockHours)
	})

	t.Run("storage errors are surfaced", func(t *testing.T) {
		svc := NewSettingService(&modelDowngradeSettingRepo{getErr: errors.New("db down")}, &config.Config{})

		_, err := svc.GetModelDowngradeGuardSettings(context.Background())

		require.Error(t, err)
	})
}

func TestSetModelDowngradeGuardSettingsValidation(t *testing.T) {
	validPairs := []ModelDowngradePair{{SentModel: "gpt-6-astra", ResponseModel: "gpt-5.6-luna"}}

	tests := []struct {
		name     string
		settings *ModelDowngradeGuardSettings
		wantErr  string
	}{
		{
			name:     "threshold count too large",
			settings: &ModelDowngradeGuardSettings{Action: ModelDowngradeGuardActionModelBlock, Pairs: validPairs, ThresholdCount: 101, ThresholdWindowMinutes: 30, BlockHours: 24, MaxBlockedRatio: 0.3},
			wantErr:  "threshold_count must be between 1-100",
		},
		{
			name:     "window out of range",
			settings: &ModelDowngradeGuardSettings{Action: ModelDowngradeGuardActionModelBlock, Pairs: validPairs, ThresholdCount: 5, ThresholdWindowMinutes: 1441, BlockHours: 24, MaxBlockedRatio: 0.3},
			wantErr:  "threshold_window_minutes must be between 1-1440",
		},
		{
			name:     "block hours out of range",
			settings: &ModelDowngradeGuardSettings{Action: ModelDowngradeGuardActionModelBlock, Pairs: validPairs, ThresholdCount: 5, ThresholdWindowMinutes: 30, BlockHours: 73, MaxBlockedRatio: 0.3},
			wantErr:  "block_hours must be between 1-72",
		},
		{
			name:     "ratio out of range",
			settings: &ModelDowngradeGuardSettings{Action: ModelDowngradeGuardActionModelBlock, Pairs: validPairs, ThresholdCount: 5, ThresholdWindowMinutes: 30, BlockHours: 24, MaxBlockedRatio: 1.5},
			wantErr:  "max_blocked_ratio must be between 0-1",
		},
		{
			name:     "invalid action",
			settings: &ModelDowngradeGuardSettings{Action: "disable", Pairs: validPairs, ThresholdCount: 5, ThresholdWindowMinutes: 30, BlockHours: 24, MaxBlockedRatio: 0.3},
			wantErr:  "invalid action: disable",
		},
		{
			name:     "enabled without any pair",
			settings: &ModelDowngradeGuardSettings{Enabled: true, Action: ModelDowngradeGuardActionModelBlock, ThresholdCount: 5, ThresholdWindowMinutes: 30, BlockHours: 24, MaxBlockedRatio: 0.3},
			wantErr:  "at least one downgrade pair is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSettingService(&modelDowngradeSettingRepo{}, &config.Config{})

			err := svc.SetModelDowngradeGuardSettings(context.Background(), tt.settings)

			require.ErrorContains(t, err, tt.wantErr)
		})
	}

	t.Run("valid settings are normalized before persisting", func(t *testing.T) {
		repo := &modelDowngradeSettingRepo{}
		svc := NewSettingService(repo, &config.Config{})

		err := svc.SetModelDowngradeGuardSettings(context.Background(), &ModelDowngradeGuardSettings{
			Enabled:                true,
			Action:                 ModelDowngradeGuardActionTempUnsched,
			Pairs:                  []ModelDowngradePair{{SentModel: " gpt-6-astra ", ResponseModel: " gpt-5.6-luna "}, {SentModel: "", ResponseModel: "x"}},
			ThresholdCount:         5,
			ThresholdWindowMinutes: 30,
			BlockHours:             24,
			MaxBlockedRatio:        0.3,
		})

		require.NoError(t, err)
		var stored ModelDowngradeGuardSettings
		require.NoError(t, json.Unmarshal([]byte(repo.value), &stored))
		require.Equal(t, []ModelDowngradePair{{SentModel: "gpt-6-astra", ResponseModel: "gpt-5.6-luna"}}, stored.Pairs)
		require.Equal(t, ModelDowngradeGuardActionTempUnsched, stored.Action)
	})
}

// model_block 模式下同一个账号屏蔽第二个模型时，受限账号数并没有变，
// 比例判定必须按 blocked 而不是 blocked+1，否则已经受限的账号会被重复计数、
// 明明没超上限却拒绝执行。
func TestModelDowngradeGuardRatioCapCountsAlreadyBlockedAccountOnce(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.Pairs = append(settings.Pairs, ModelDowngradePair{
		SentModel:     "gpt-6-sol",
		ResponseModel: "gpt-5.6-luna",
	})
	h := newModelDowngradeGuardHarness(t, settings)
	h.counter.counts = []int64{5}
	// 3/10 已受限，上限 0.3：当前账号已在其中，屏蔽第二个模型不改变分子。
	h.repo.blockedAccounts = 3
	h.repo.totalAccounts = 10
	account := modelDowngradeTestAccount()
	h.repo.alreadyBlocked = map[int64]bool{account.ID: true}

	h.svc.HandleModelDowngrade(context.Background(), account, "gpt-6-sol", "gpt-5.6-luna")

	require.Equal(t, []string{ModelDowngradeBlockedScopeModel}, h.repo.applyScopes)
	require.Equal(t, 1, h.repo.modelCalls, "已受限账号再屏蔽一个模型不该被比例上限拦下")
	require.Equal(t, "gpt-6-sol", h.repo.modelScope)
	require.Zero(t, h.repo.tempCalls)
}

// 上一个用例的对照组：同样 3/10、上限 0.3，但当前账号还没被计入分子，
// 这次处理会让比例变成 0.4，必须拒绝。
func TestModelDowngradeGuardRatioCapBlocksNewAccountAtSameRatio(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.Pairs = append(settings.Pairs, ModelDowngradePair{
		SentModel:     "gpt-6-sol",
		ResponseModel: "gpt-5.6-luna",
	})
	h := newModelDowngradeGuardHarness(t, settings)
	h.counter.counts = []int64{5}
	h.repo.blockedAccounts = 3
	h.repo.totalAccounts = 10
	account := modelDowngradeTestAccount()
	// 受限集合里是别的账号，当前账号属于新增。
	h.repo.alreadyBlocked = map[int64]bool{account.ID + 1: true}

	h.svc.HandleModelDowngrade(context.Background(), account, "gpt-6-sol", "gpt-5.6-luna")

	require.Equal(t, []string{ModelDowngradeBlockedScopeModel}, h.repo.applyScopes)
	require.Zero(t, h.repo.modelCalls, "新增受限账号会把比例推到 0.4，必须被上限拦下")
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.blocker.calls)
}

// ==================== 幂等：已受限的账号不再重复处理 ====================

// 守卫按整账号把号下线后，这个号上进行中的 WS 连接继续跑、命中计数继续涨，
// 阈值后守卫再次触发；此时配置可能已经切成「仅屏蔽该模型」。整账号下线覆盖该账号的
// 所有模型，这一次必须完全跳过，不能再写一条 model_rate_limits。
func TestModelDowngradeGuardSkipsWhenAccountAlreadyBlocked(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	account := modelDowngradeTestAccount()
	h.repo.accountBlocked = map[int64]bool{account.ID: true}

	h.svc.HandleModelDowngrade(context.Background(), account, "gpt-6-astra", "gpt-5.6-luna")

	// 仓储被调用了（幂等判定在事务里做），但一行都没写。
	require.Equal(t, []string{ModelDowngradeBlockedScopeModel}, h.repo.applyScopes)
	require.Zero(t, h.repo.modelCalls)
	require.Zero(t, h.repo.tempCalls)
	// 重复处理不该产生任何副作用：缓存、runtime blocker 一个都不碰。
	require.Zero(t, h.cache.setCalls)
	require.Zero(t, h.blocker.calls)
	// 也不该留下「本应处理」的观察记录——它真的已经受限了。
	require.Empty(t, h.observed.entries)
	// 计数照常清零，下一个窗口重新计。
	require.Equal(t, []string{"gpt-6-astra"}, h.counter.resetCalls)
}

// model 范围下同一个模型已经被守卫屏蔽时再次触发，同样是重复动作。
func TestModelDowngradeGuardSkipsWhenSameModelAlreadyBlocked(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	account := modelDowngradeTestAccount()
	h.repo.modelBlocked = map[string]bool{
		modelDowngradeObservedDedupeKey(account.ID, "gpt-6-astra"): true,
	}

	h.svc.HandleModelDowngrade(context.Background(), account, "gpt-6-astra", "gpt-5.6-luna")

	require.Zero(t, h.repo.modelCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.cache.setCalls)
	require.Zero(t, h.blocker.calls)
	require.Empty(t, h.observed.entries)
}

// 跳过重复处理时也要清掉观察记录，否则同一个账号会同时出现「已受限」和「观察中」两行。
func TestModelDowngradeGuardAlreadyBlockedDeletesObservedEntry(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	account := modelDowngradeTestAccount()
	h.repo.accountBlocked = map[int64]bool{account.ID: true}
	require.NoError(t, h.observed.RecordObserved(context.Background(), ModelDowngradeObservedEntry{
		AccountID: account.ID, SentModel: "gpt-6-astra", Cause: ModelDowngradeObservedCauseRatioCap,
	}, time.Hour))

	h.svc.HandleModelDowngrade(context.Background(), account, "gpt-6-astra", "gpt-5.6-luna")

	require.Equal(t, []string{"101:gpt-6-astra"}, h.observed.deleteCalls)
	require.Empty(t, h.observed.entries)
}

// 只屏蔽了模型的账号遇到「整账号下线」的配置时照常写整账号——那是升级，不是重复。
func TestModelDowngradeGuardUpgradesModelBlockToAccountBlock(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, tempUnschedModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	account := modelDowngradeTestAccount()
	h.repo.modelBlocked = map[string]bool{
		modelDowngradeObservedDedupeKey(account.ID, "gpt-6-astra"): true,
	}

	h.svc.HandleModelDowngrade(context.Background(), account, "gpt-6-astra", "gpt-5.6-luna")

	require.Equal(t, []string{ModelDowngradeBlockedScopeAccount}, h.repo.applyScopes)
	require.Equal(t, 1, h.repo.tempCalls)
	require.Equal(t, 1, h.cache.setCalls)
	require.Equal(t, 1, h.blocker.calls)
}

// 整账号范围的提前恢复只解除降级来源的临时下线，并清掉缓存、通知 runtime blocker，
// 绝不连带清空模型级限流。
func TestReleaseModelDowngradeBlockAccountScope(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.repo.releaseOK = true

	require.NoError(t, h.svc.ReleaseModelDowngradeBlock(
		context.Background(), modelDowngradeTestAccountID, ModelDowngradeBlockedScopeAccount, "",
	))

	require.Equal(t, 1, h.repo.releaseCalls)
	require.Equal(t, modelDowngradeTestAccountID, h.repo.releaseID)
	require.Equal(t, ModelDowngradeBlockedScopeAccount, h.repo.releaseScope)
	require.Equal(t, "", h.repo.releaseModel)
	require.Equal(t, 1, h.cache.deleteCalls)
	require.Equal(t, 1, h.blocker.clearCalls)
}

// 仅模型范围只删对应的那条限流，不碰整账号的临时下线缓存，也不通知 runtime blocker。
func TestReleaseModelDowngradeBlockModelScope(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.repo.releaseOK = true

	require.NoError(t, h.svc.ReleaseModelDowngradeBlock(
		context.Background(), modelDowngradeTestAccountID, ModelDowngradeBlockedScopeModel, " gpt-6-astra ",
	))

	require.Equal(t, 1, h.repo.releaseCalls)
	require.Equal(t, ModelDowngradeBlockedScopeModel, h.repo.releaseScope)
	require.Equal(t, "gpt-6-astra", h.repo.releaseModel)
	require.Zero(t, h.cache.deleteCalls)
	require.Zero(t, h.blocker.clearCalls)
}

// 仓储没匹配到降级来源的记录时，service 必须返回可识别的 404 错误，而不是静默成功。
func TestReleaseModelDowngradeBlockNotFound(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.repo.releaseOK = false

	err := h.svc.ReleaseModelDowngradeBlock(context.Background(), modelDowngradeTestAccountID, ModelDowngradeBlockedScopeAccount, "")

	require.ErrorIs(t, err, ErrModelDowngradeBlockNotFound)
	require.Zero(t, h.cache.deleteCalls)
	require.Zero(t, h.blocker.clearCalls)
}

// scope 不合法或 model 范围缺模型名时直接拒绝，不打到仓储。
func TestReleaseModelDowngradeBlockRejectsInvalidScope(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())

	require.ErrorIs(t,
		h.svc.ReleaseModelDowngradeBlock(context.Background(), modelDowngradeTestAccountID, "whatever", ""),
		ErrModelDowngradeBlockScopeInvalid,
	)
	require.ErrorIs(t,
		h.svc.ReleaseModelDowngradeBlock(context.Background(), modelDowngradeTestAccountID, ModelDowngradeBlockedScopeModel, "  "),
		ErrModelDowngradeBlockScopeInvalid,
	)
	require.Zero(t, h.repo.releaseCalls)
}

// ==================== 观察记录（本应处理但没处理） ====================

// 观察模式到阈值时除了打 dry-run 日志，还要写一条观察记录，管理页上必须看得到。
func TestModelDowngradeGuardActionNoneRecordsObservedEntry(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.Action = ModelDowngradeGuardActionNone
	settings.BlockHours = 12
	h := newModelDowngradeGuardHarness(t, settings)
	h.counter.counts = []int64{5}
	before := time.Now()

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	entry := h.observed.get(t, modelDowngradeTestAccountID, "gpt-6-astra")
	require.Equal(t, ModelDowngradeObservedCauseDryRun, entry.Cause)
	require.Equal(t, "openai-pool-1", entry.AccountName)
	require.Equal(t, "gpt-6-astra", entry.SentModel)
	require.Equal(t, "gpt-5.6-luna", entry.ResponseModel)
	require.Equal(t, int64(5), entry.TriggerCount)
	require.Equal(t, 5, entry.TriggerThreshold)
	require.Equal(t, 30, entry.TriggerWindowMinutes)
	// 观察模式没有分子分母可言。
	require.Zero(t, entry.Blocked)
	require.Zero(t, entry.Total)
	require.Zero(t, entry.MaxBlockedRatio)
	// 记录保留时长 = 配置里的处理时长。
	require.Equal(t, []time.Duration{12 * time.Hour}, h.observed.recordTTLs)
	require.WithinDuration(t, before.Add(12*time.Hour), entry.ExpiresAt, time.Minute)
	require.WithinDuration(t, before, entry.ObservedAt, time.Minute)

	// 主流程行为不变：不动账号。
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
}

// 比例上限拦下时同样留痕，并带上当时的分子分母，页面能解释「为什么没处理」。
func TestModelDowngradeGuardRatioCapRecordsObservedEntry(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	h.repo.blockedAccounts = 3
	h.repo.totalAccounts = 10

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	entry := h.observed.get(t, modelDowngradeTestAccountID, "gpt-6-astra")
	require.Equal(t, ModelDowngradeObservedCauseRatioCap, entry.Cause)
	require.Equal(t, int64(3), entry.Blocked)
	require.Equal(t, int64(10), entry.Total)
	require.InDelta(t, 0.3, entry.MaxBlockedRatio, 1e-9)
	require.Equal(t, []time.Duration{24 * time.Hour}, h.observed.recordTTLs)
	require.Zero(t, h.repo.modelCalls)
}

// 真的处理成功后要顺手删掉观察记录，否则同一个账号会同时出现「观察中」和「已受限」两行。
func TestModelDowngradeGuardAppliedDeletesObservedEntry(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.counter.counts = []int64{5}
	require.NoError(t, h.observed.RecordObserved(context.Background(), ModelDowngradeObservedEntry{
		AccountID: modelDowngradeTestAccountID, SentModel: "gpt-6-astra", Cause: ModelDowngradeObservedCauseDryRun,
	}, time.Hour))

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Equal(t, 1, h.repo.modelCalls)
	require.Equal(t, []string{"101:gpt-6-astra"}, h.observed.deleteCalls)
	require.Empty(t, h.observed.entries)
}

// 记录失败（Redis 挂了）不能影响熔断主流程：账号照样处理。
func TestModelDowngradeGuardObservedFailureDoesNotBreakMainFlow(t *testing.T) {
	t.Run("dry run", func(t *testing.T) {
		settings := enabledModelDowngradeGuardSettings()
		settings.Action = ModelDowngradeGuardActionNone
		h := newModelDowngradeGuardHarness(t, settings)
		h.counter.counts = []int64{5}
		h.observed.recordErr = errors.New("redis down")

		h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

		require.Equal(t, []string{"gpt-6-astra"}, h.counter.resetCalls)
		require.Zero(t, h.repo.tempCalls)
		require.Empty(t, h.observed.entries)
	})

	t.Run("applied", func(t *testing.T) {
		h := newModelDowngradeGuardHarness(t, tempUnschedModelDowngradeGuardSettings())
		h.counter.counts = []int64{5}
		h.observed.deleteErr = errors.New("redis down")

		h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

		require.Equal(t, 1, h.repo.tempCalls)
		require.Equal(t, 1, h.blocker.calls)
		require.Equal(t, 1, h.cache.setCalls)
	})
}

// 没装配观察记录存储时一切照旧，只是页面上看不到观察行。
func TestModelDowngradeGuardWithoutObservedCacheStillWorks(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.Action = ModelDowngradeGuardActionNone
	h := newModelDowngradeGuardHarness(t, settings)
	h.svc.SetModelDowngradeObservedCache(nil)
	h.counter.counts = []int64{5}

	h.svc.HandleModelDowngrade(context.Background(), modelDowngradeTestAccount(), "gpt-6-astra", "gpt-5.6-luna")

	require.Empty(t, h.observed.entries)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
}

// ==================== 挂点：OpenAI RecordUsage ====================

// RecordUsage 必须把「实际发往上游的模型」（而不是客户端请求的模型）和上游自报的模型
// 交给守卫；非 OpenAI 平台不进守卫。
func TestOpenAIRecordUsageFeedsModelDowngradeGuard(t *testing.T) {
	newRecordUsage := func(t *testing.T) (*OpenAIGatewayService, *modelDowngradeGuardHarness) {
		t.Helper()
		h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		svc.rateLimitService = h.svc
		return svc, h
	}
	record := func(t *testing.T, svc *OpenAIGatewayService, account *Account, result *OpenAIForwardResult) {
		t.Helper()
		result.Usage = OpenAIUsage{InputTokens: 10, OutputTokens: 5}
		result.Duration = time.Second
		require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result:  result,
			APIKey:  &APIKey{ID: 10},
			User:    &User{ID: 20},
			Account: account,
		}))
	}

	t.Run("uses the model actually sent upstream", func(t *testing.T) {
		svc, h := newRecordUsage(t)
		h.counter.counts = []int64{5}

		record(t, svc, modelDowngradeTestAccount(), &OpenAIForwardResult{
			RequestID:             "req_downgrade_mapped",
			Model:                 "astra-alias",
			UpstreamModel:         "gpt-6-astra",
			UpstreamResponseModel: "gpt-5.6-luna",
		})

		require.Equal(t, []string{"gpt-6-astra"}, h.counter.incrCalls)
		require.Equal(t, 1, h.repo.modelCalls)
		require.Equal(t, "gpt-6-astra", h.repo.modelScope)
	})

	t.Run("falls back to the request model when nothing was rewritten", func(t *testing.T) {
		svc, h := newRecordUsage(t)

		record(t, svc, modelDowngradeTestAccount(), &OpenAIForwardResult{
			RequestID:             "req_downgrade_plain",
			Model:                 "gpt-6-astra",
			UpstreamResponseModel: "gpt-5.6-luna",
		})

		require.Equal(t, []string{"gpt-6-astra"}, h.counter.incrCalls)
	})

	t.Run("ignores non-OpenAI accounts", func(t *testing.T) {
		svc, h := newRecordUsage(t)
		account := modelDowngradeTestAccount()
		account.Platform = PlatformGrok

		record(t, svc, account, &OpenAIForwardResult{
			RequestID:             "req_downgrade_grok",
			Model:                 "gpt-6-astra",
			UpstreamResponseModel: "gpt-5.6-luna",
		})

		require.Empty(t, h.counter.incrCalls)
	})
}
