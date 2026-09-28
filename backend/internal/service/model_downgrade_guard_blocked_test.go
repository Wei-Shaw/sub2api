//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestSplitModelDowngradeModels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		errorMessage string
		wantSent     string
		wantResponse string
	}{
		{
			name:         "guard format",
			errorMessage: buildModelDowngradeErrorMessage("gpt-6-astra", "gpt-5.6-luna", 5, 30),
			wantSent:     "gpt-6-astra",
			wantResponse: "gpt-5.6-luna",
		},
		{
			name:         "without suffix",
			errorMessage: "gpt-6-astra → gpt-5.6-luna",
			wantSent:     "gpt-6-astra",
			wantResponse: "gpt-5.6-luna",
		},
		{
			name:         "model name containing parenthesis keeps the last group",
			errorMessage: "gpt-6-astra → gpt-5.6-luna(preview) (5 hits in 30 min)",
			wantSent:     "gpt-6-astra",
			wantResponse: "gpt-5.6-luna(preview)",
		},
		{
			name:         "no arrow",
			errorMessage: "account temporarily unschedulable",
			wantSent:     "",
			wantResponse: "",
		},
		{
			name:         "empty",
			errorMessage: "   ",
			wantSent:     "",
			wantResponse: "",
		},
		{
			name:         "missing response model",
			errorMessage: "gpt-6-astra → (5 hits in 30 min)",
			wantSent:     "",
			wantResponse: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sent, response := SplitModelDowngradeModels(tc.errorMessage)
			require.Equal(t, tc.wantSent, sent)
			require.Equal(t, tc.wantResponse, response)
		})
	}
}

func TestModelDowngradeStateFromReason(t *testing.T) {
	t.Parallel()

	guardState := &TempUnschedState{
		UntilUnix:            1_700_000_000,
		TriggeredAtUnix:      1_699_990_000,
		MatchedKeyword:       ModelDowngradeGuardKeyword,
		RuleIndex:            -1,
		ErrorMessage:         buildModelDowngradeErrorMessage("gpt-6-astra", "gpt-5.6-luna", 5, 30),
		TriggerCount:         5,
		TriggerThreshold:     5,
		TriggerWindowMinutes: 30,
	}
	guardRaw, err := json.Marshal(guardState)
	require.NoError(t, err)

	otherRaw, err := json.Marshal(&TempUnschedState{
		MatchedKeyword: "model_downgrade_lookalike",
		ErrorMessage:   "not the guard",
	})
	require.NoError(t, err)

	t.Run("guard reason", func(t *testing.T) {
		t.Parallel()
		state, ok := ModelDowngradeStateFromReason(string(guardRaw))
		require.True(t, ok)
		require.Equal(t, int64(5), state.TriggerCount)
		require.Equal(t, 30, state.TriggerWindowMinutes)
	})

	t.Run("other keyword rejected", func(t *testing.T) {
		t.Parallel()
		_, ok := ModelDowngradeStateFromReason(string(otherRaw))
		require.False(t, ok)
	})

	t.Run("plain text rejected", func(t *testing.T) {
		t.Parallel()
		_, ok := ModelDowngradeStateFromReason("model_downgrade happened")
		require.False(t, ok)
	})

	t.Run("empty rejected", func(t *testing.T) {
		t.Parallel()
		_, ok := ModelDowngradeStateFromReason("")
		require.False(t, ok)
	})
}

func TestRateLimitService_ListModelDowngradeBlocked_SortsAndSummarizes(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.MaxBlockedRatio = 0.25
	h := newModelDowngradeGuardHarness(t, settings)

	base := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	h.repo.listItems = []ModelDowngradeBlockedAccount{
		{AccountID: 1, Scope: ModelDowngradeBlockedScopeAccount, Until: base.Add(time.Hour)},
		{AccountID: 2, Scope: ModelDowngradeBlockedScopeModel, Model: "gpt-6-astra", Until: base.Add(5 * time.Hour)},
		{AccountID: 3, Scope: ModelDowngradeBlockedScopeAccount, Until: base.Add(3 * time.Hour)},
	}
	h.repo.blockedAccounts = 2
	h.repo.totalAccounts = 12

	result, err := h.svc.ListModelDowngradeBlocked(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, h.repo.listCalls)
	require.Len(t, result.Items, 3)
	require.Equal(t, []int64{2, 3, 1}, []int64{
		result.Items[0].AccountID,
		result.Items[1].AccountID,
		result.Items[2].AccountID,
	})
	require.Equal(t, int64(2), result.Summary.Blocked)
	require.Equal(t, int64(12), result.Summary.TotalActive)
	require.InDelta(t, 0.25, result.Summary.MaxBlockedRatio, 1e-9)
}

func TestRateLimitService_ListModelDowngradeBlocked_EmptyIsNeverNil(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.repo.listItems = nil
	h.repo.blockedAccounts = 0
	h.repo.totalAccounts = 7

	result, err := h.svc.ListModelDowngradeBlocked(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result.Items)
	require.Empty(t, result.Items)
	require.Equal(t, int64(7), result.Summary.TotalActive)
}

func TestRateLimitService_ListModelDowngradeBlocked_PropagatesErrors(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.repo.listErr = errors.New("boom")

	_, err := h.svc.ListModelDowngradeBlocked(context.Background())
	require.Error(t, err)

	h.repo.listErr = nil
	h.repo.countErr = errors.New("count boom")
	_, err = h.svc.ListModelDowngradeBlocked(context.Background())
	require.Error(t, err)
}

// ==================== 观察记录合并进列表 ====================

func recordObservedForTest(t *testing.T, h *modelDowngradeGuardHarness, entry ModelDowngradeObservedEntry) {
	t.Helper()
	require.NoError(t, h.observed.RecordObserved(context.Background(), entry, time.Hour))
}

// 真实下线行和观察行合并成一张表，按 until 倒序，汇总里 observed 单独计数、
// 不混进 blocked（页面上的占比只能按真实下线算）。
func TestRateLimitService_ListModelDowngradeBlocked_MergesObservedEntries(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())

	base := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	h.repo.listItems = []ModelDowngradeBlockedAccount{
		{AccountID: 1, Scope: ModelDowngradeBlockedScopeAccount, SentModel: "gpt-6-astra", Until: base.Add(2 * time.Hour)},
	}
	h.repo.blockedAccounts = 1
	h.repo.totalAccounts = 10

	recordObservedForTest(t, h, ModelDowngradeObservedEntry{
		AccountID: 2, AccountName: "pool-2", Cause: ModelDowngradeObservedCauseDryRun,
		SentModel: "gpt-6-astra", ResponseModel: "gpt-5.6-luna",
		TriggerCount: 5, TriggerThreshold: 5, TriggerWindowMinutes: 30,
		ObservedAt: base, ExpiresAt: base.Add(6 * time.Hour),
	})
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{
		AccountID: 3, AccountName: "pool-3", Cause: ModelDowngradeObservedCauseRatioCap,
		SentModel: "gpt-6-sol", ResponseModel: "gpt-5.6-luna",
		TriggerCount: 7, TriggerThreshold: 5, TriggerWindowMinutes: 30,
		ObservedAt: base, ExpiresAt: base.Add(time.Hour),
		Blocked: 3, Total: 10, MaxBlockedRatio: 0.3,
	})

	result, err := h.svc.ListModelDowngradeBlocked(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Items, 3)

	// until 倒序：观察记录 6h > 真实下线 2h > 观察记录 1h
	require.Equal(t, []int64{2, 1, 3}, []int64{
		result.Items[0].AccountID, result.Items[1].AccountID, result.Items[2].AccountID,
	})

	observedRow := result.Items[0]
	require.Equal(t, ModelDowngradeBlockedStatusObserved, observedRow.Status)
	require.Equal(t, ModelDowngradeBlockedScopeObserved, observedRow.Scope)
	require.Equal(t, ModelDowngradeObservedCauseDryRun, observedRow.Cause)
	require.Equal(t, "gpt-6-astra", observedRow.Model, "观察行的模型列填发往上游的模型")
	require.Equal(t, base, observedRow.TriggeredAt)
	require.Equal(t, base.Add(6*time.Hour), observedRow.Until)

	require.Equal(t, ModelDowngradeBlockedStatusBlocked, result.Items[1].Status)

	cappedRow := result.Items[2]
	require.Equal(t, ModelDowngradeBlockedStatusRatioCapped, cappedRow.Status)
	require.Equal(t, int64(3), cappedRow.Blocked)
	require.Equal(t, int64(10), cappedRow.Total)
	require.InDelta(t, 0.3, cappedRow.MaxBlockedRatio, 1e-9)

	require.Equal(t, int64(1), result.Summary.Blocked)
	require.Equal(t, int64(2), result.Summary.Observed)
	require.Equal(t, int64(10), result.Summary.TotalActive)
}

// 同一账号 + 模型既有真实下线行又有残留观察记录时只留真实下线行。
// 写入时本应已经删掉，这里是容错。
func TestRateLimitService_ListModelDowngradeBlocked_DedupesObservedShadowedByBlock(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	until := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	h.repo.listItems = []ModelDowngradeBlockedAccount{
		{AccountID: 101, Scope: ModelDowngradeBlockedScopeAccount, SentModel: "GPT-6-Astra", Until: until},
	}
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{
		AccountID: 101, Cause: ModelDowngradeObservedCauseDryRun,
		SentModel: "gpt-6-astra", ExpiresAt: until.Add(time.Hour),
	})
	// 别的模型的观察记录不受影响。
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{
		AccountID: 101, Cause: ModelDowngradeObservedCauseDryRun,
		SentModel: "gpt-6-sol", ExpiresAt: until.Add(2 * time.Hour),
	})

	result, err := h.svc.ListModelDowngradeBlocked(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.Equal(t, "gpt-6-sol", result.Items[0].SentModel)
	require.Equal(t, ModelDowngradeBlockedStatusObserved, result.Items[0].Status)
	require.Equal(t, ModelDowngradeBlockedStatusBlocked, result.Items[1].Status)
	require.Equal(t, int64(1), result.Summary.Observed)
}

// 观察记录读失败不能把「真实下线」这张更重要的表一起打挂。
func TestRateLimitService_ListModelDowngradeBlocked_ObservedFailureIsTolerated(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.repo.listItems = []ModelDowngradeBlockedAccount{
		{AccountID: 1, Scope: ModelDowngradeBlockedScopeAccount, Until: time.Now().Add(time.Hour)},
	}
	h.observed.listErr = errors.New("redis down")

	result, err := h.svc.ListModelDowngradeBlocked(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Zero(t, result.Summary.Observed)
}

// ==================== 清除观察记录 ====================

func TestReleaseModelDowngradeBlockObservedScope(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{AccountID: 101, SentModel: "gpt-6-astra"})

	require.NoError(t, h.svc.ReleaseModelDowngradeBlock(
		context.Background(), 101, ModelDowngradeBlockedScopeObserved, " GPT-6-Astra ",
	))

	require.Empty(t, h.observed.entries)
	// 观察记录只在 Redis 里：不碰库、不动缓存、不发恢复通知。
	require.Zero(t, h.repo.releaseCalls)
	require.Zero(t, h.cache.deleteCalls)
	require.Zero(t, h.blocker.clearCalls)
}

func TestReleaseModelDowngradeBlockObservedNotFound(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())

	err := h.svc.ReleaseModelDowngradeBlock(context.Background(), 101, ModelDowngradeBlockedScopeObserved, "gpt-6-astra")

	require.ErrorIs(t, err, ErrModelDowngradeBlockNotFound)
	require.Zero(t, h.repo.releaseCalls)
}

func TestReleaseModelDowngradeBlockObservedRequiresModel(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())

	require.ErrorIs(t,
		h.svc.ReleaseModelDowngradeBlock(context.Background(), 101, ModelDowngradeBlockedScopeObserved, "  "),
		ErrModelDowngradeBlockScopeInvalid,
	)
}

// ==================== 手动把观察记录转成真实下线 ====================

func TestApplyModelDowngradeBlockNow_UsesObservedEntryAndDeletesIt(t *testing.T) {
	settings := tempUnschedModelDowngradeGuardSettings()
	settings.BlockHours = 6
	h := newModelDowngradeGuardHarness(t, settings)
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{
		AccountID: 101, Cause: ModelDowngradeObservedCauseRatioCap,
		SentModel: "gpt-6-astra", ResponseModel: "gpt-5.6-luna", TriggerCount: 7,
	})
	before := time.Now()

	result, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, " gpt-6-astra ")
	require.NoError(t, err)
	require.Equal(t, int64(101), result.AccountID)
	require.Equal(t, ModelDowngradeBlockedScopeAccount, result.Scope)
	require.WithinDuration(t, before.Add(6*time.Hour), result.Until, time.Minute)

	require.Equal(t, 1, h.repo.tempCalls)
	require.Equal(t, 1, h.cache.setCalls)
	require.Equal(t, 1, h.blocker.calls)
	// 转正之后这条记录就不该再以「观察中」的身份留在表里。
	require.Empty(t, h.observed.entries)

	// reason 里沿用观察记录的响应模型和次数，并标注是手动触发的。
	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(h.repo.tempReason), &state))
	require.Equal(t, ModelDowngradeGuardKeyword, state.MatchedKeyword)
	require.Equal(t, int64(7), state.TriggerCount)
	require.Equal(t, "gpt-6-astra → gpt-5.6-luna (7 hits in 30 min) [manual]", state.ErrorMessage)
	// 手动标记不干扰模型名解析。
	sent, response := SplitModelDowngradeModels(state.ErrorMessage)
	require.Equal(t, "gpt-6-astra", sent)
	require.Equal(t, "gpt-5.6-luna", response)
}

// 没有观察记录（已过期／从没写过）也允许执行：次数记 0，响应模型记 "unknown"。
func TestApplyModelDowngradeBlockNow_WithoutObservedEntry(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, tempUnschedModelDowngradeGuardSettings())

	result, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")
	require.NoError(t, err)
	require.Equal(t, ModelDowngradeBlockedScopeAccount, result.Scope)

	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(h.repo.tempReason), &state))
	require.Zero(t, state.TriggerCount)
	require.Equal(t, "gpt-6-astra → unknown (0 hits in 30 min) [manual]", state.ErrorMessage)
}

// 观察模式下管理员手动点了就是要处理：按默认动作（仅屏蔽该模型）执行，不被 action=none 拦住。
func TestApplyModelDowngradeBlockNow_ObservationModeUsesDefaultAction(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.Action = ModelDowngradeGuardActionNone
	h := newModelDowngradeGuardHarness(t, settings)
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{
		AccountID: 101, Cause: ModelDowngradeObservedCauseDryRun,
		SentModel: "gpt-6-astra", ResponseModel: "gpt-5.6-luna", TriggerCount: 5,
	})

	result, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")
	require.NoError(t, err)
	require.Equal(t, ModelDowngradeBlockedScopeModel, result.Scope)
	require.Equal(t, "gpt-6-astra", result.Model)
	require.Equal(t, []string{ModelDowngradeBlockedScopeModel}, h.repo.applyScopes)
	require.Equal(t, 1, h.repo.modelCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Empty(t, h.observed.entries)
}

// 处理方式是「仅屏蔽该模型」时，手动下线也只屏蔽那个模型。
func TestApplyModelDowngradeBlockNow_ModelBlockScope(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.Action = ModelDowngradeGuardActionModelBlock
	h := newModelDowngradeGuardHarness(t, settings)

	result, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")
	require.NoError(t, err)
	require.Equal(t, ModelDowngradeBlockedScopeModel, result.Scope)
	require.Equal(t, "gpt-6-astra", result.Model)
	require.Equal(t, 1, h.repo.modelCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.cache.setCalls)
}

// 管理员刚把处理方式从「整账号临时不可调度」改成「仅屏蔽该模型」就点「立即处理」：
// 手动路径直读配置，不能拿 30s 缓存里的旧值按整账号下线。
func TestApplyModelDowngradeBlockNow_BypassesSettingsCache(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, tempUnschedModelDowngradeGuardSettings())
	// 先让缓存里存下 action=temp_unsched。
	cached := h.svc.modelDowngradeGuardSettings(context.Background())
	require.Equal(t, ModelDowngradeGuardActionTempUnsched, cached.Action)

	updated := enabledModelDowngradeGuardSettings()
	updated.Action = ModelDowngradeGuardActionModelBlock
	h.settings.value = modelDowngradeGuardJSON(t, updated)

	result, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")
	require.NoError(t, err)
	require.Equal(t, ModelDowngradeBlockedScopeModel, result.Scope)
	require.Equal(t, 1, h.repo.modelCalls)
	require.Zero(t, h.repo.tempCalls)

	// 直读到的最新值顺手回填缓存，自动路径也立刻跟上。
	require.Equal(t, ModelDowngradeGuardActionModelBlock,
		h.svc.modelDowngradeGuardSettings(context.Background()).Action)
}

// 配置读失败宁可报错让管理员重试，也不回落缓存或默认值按过期配置下线。
func TestApplyModelDowngradeBlockNow_SettingsReadFailure(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{AccountID: 101, SentModel: "gpt-6-astra"})
	h.settings.getErr = errors.New("settings store down")

	_, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")

	require.ErrorIs(t, err, ErrModelDowngradeGuardSettingsUnavailable)
	var appErr *infraerrors.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.EqualValues(t, http.StatusServiceUnavailable, appErr.Code)

	// 没读到配置就什么都不做：不碰库，观察记录原样留着。
	require.Zero(t, h.repo.getByIDCalls)
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
	require.Len(t, h.observed.entries, 1)
}

// 比例上限对手动下线同样生效：安全阀保护号池的口径不能因为「管理员点的」就放宽。
func TestApplyModelDowngradeBlockNow_RatioCapReturnsConflict(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.repo.blockedAccounts = 3
	h.repo.totalAccounts = 10
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{
		AccountID: 101, Cause: ModelDowngradeObservedCauseRatioCap, SentModel: "gpt-6-astra",
	})

	_, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")

	require.ErrorIs(t, err, ErrModelDowngradeBlockRatioCapped)
	var appErr *infraerrors.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.EqualValues(t, http.StatusConflict, appErr.Code)
	require.Equal(t, "3", appErr.Metadata["blocked"])
	require.Equal(t, "10", appErr.Metadata["total"])
	require.Equal(t, "0.3", appErr.Metadata["max_blocked_ratio"])

	// 没处理成就不该动账号，观察记录也必须留着。
	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
	require.Len(t, h.observed.entries, 1)
}

// 手动下线同样幂等：管理员多半是对着一份没刷新的列表点的，重复写一条只会让页面上
// 同一个账号出现两行。409 直说「已被守卫限制」。
func TestApplyModelDowngradeBlockNow_AlreadyBlockedReturnsConflict(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, tempUnschedModelDowngradeGuardSettings())
	h.repo.accountBlocked = map[int64]bool{101: true}
	recordObservedForTest(t, h, ModelDowngradeObservedEntry{
		AccountID: 101, Cause: ModelDowngradeObservedCauseRatioCap, SentModel: "gpt-6-astra",
	})

	_, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")

	require.ErrorIs(t, err, ErrModelDowngradeBlockAlreadyActive)
	var appErr *infraerrors.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.EqualValues(t, http.StatusConflict, appErr.Code)
	require.Equal(t, "account is already blocked by the model downgrade guard", appErr.Message)
	require.Equal(t, ModelDowngradeBlockedScopeAccount, appErr.Metadata["scope"])

	require.Zero(t, h.repo.tempCalls)
	require.Zero(t, h.repo.modelCalls)
	// 已经下线的账号不该还挂着观察记录。
	require.Empty(t, h.observed.entries)
}

// 同一个模型已被屏蔽时，手动对该模型再点一次「立即下线」同样是重复动作。
func TestApplyModelDowngradeBlockNow_SameModelAlreadyBlockedReturnsConflict(t *testing.T) {
	settings := enabledModelDowngradeGuardSettings()
	settings.Action = ModelDowngradeGuardActionModelBlock
	h := newModelDowngradeGuardHarness(t, settings)
	h.repo.modelBlocked = map[string]bool{
		modelDowngradeObservedDedupeKey(101, "gpt-6-astra"): true,
	}

	_, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")

	require.ErrorIs(t, err, ErrModelDowngradeBlockAlreadyActive)
	var appErr *infraerrors.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, ModelDowngradeBlockedScopeModel, appErr.Metadata["scope"])
	require.Zero(t, h.repo.modelCalls)
}

func TestApplyModelDowngradeBlockNow_RejectsInvalidInput(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())

	_, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 0, "gpt-6-astra")
	require.ErrorIs(t, err, ErrAccountNotFound)

	_, err = h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "   ")
	require.ErrorIs(t, err, ErrModelDowngradeBlockScopeInvalid)

	require.Zero(t, h.repo.getByIDCalls)
}

func TestApplyModelDowngradeBlockNow_MissingAccount(t *testing.T) {
	h := newModelDowngradeGuardHarness(t, enabledModelDowngradeGuardSettings())
	h.repo.account = nil

	_, err := h.svc.ApplyModelDowngradeBlockNow(context.Background(), 101, "gpt-6-astra")
	require.ErrorIs(t, err, ErrAccountNotFound)
	require.Zero(t, h.repo.tempCalls)
}
