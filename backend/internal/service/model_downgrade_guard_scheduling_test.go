//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 这组用例把守卫的写入端和 HTTP 选号的读取端串起来验证：守卫按
// （账号, 发往上游的模型）写进 extra.model_rate_limits 的键，必须能被 OpenAI HTTP
// 调度路径（IsSchedulableForModelWithContext → GetMappedModel(请求模型)）识别，
// 从而在该模型上跳过这个账号，同时不影响同账号的其他模型。

// modelDowngradeSchedulingRepo 在调度测试仓储之上实现守卫的原子写入：
// 与真实仓储一样把限制写进 extra.model_rate_limits[model]（形状同 SetModelRateLimit），
// 调度随后从同一份账号数据里读到它。比例上限在别的用例里覆盖，这里始终放行。
type modelDowngradeSchedulingRepo struct {
	schedulerTestOpenAIAccountRepo
}

func (r modelDowngradeSchedulingRepo) ApplyOpenAIModelDowngradeBlock(
	_ context.Context,
	id int64,
	scope string,
	model string,
	until time.Time,
	reason string,
	_ float64,
	now time.Time,
) (ModelDowngradeBlockApplyResult, error) {
	for i := range r.accounts {
		if r.accounts[i].ID != id {
			continue
		}
		switch scope {
		case ModelDowngradeBlockedScopeModel:
			setAccountModelRateLimitSnapshot(&r.accounts[i], model, until, reason, now)
		case ModelDowngradeBlockedScopeAccount:
			r.accounts[i].TempUnschedulableUntil = &until
			r.accounts[i].TempUnschedulableReason = reason
		}
		return ModelDowngradeBlockApplyResult{Applied: true, Total: int64(len(r.accounts))}, nil
	}
	return ModelDowngradeBlockApplyResult{}, ErrAccountNotFound
}

// modelDowngradeForwardSentModel 复刻 HTTP Forward 算出的「发往上游的模型」，
// 也就是 RecordUsage 里 upstreamSentModel(result.Model, result.UpstreamModel) 交给守卫的值：
//   - 普通 Forward：result.UpstreamModel 取自 resolveOpenAIForwardMappedModels
//     （账号 model_mapping + Codex 归一），见 openai_gateway_forward.go；
//   - openai_passthrough：result.UpstreamModel 为空（非 compact），发往上游的就是
//     渠道映射后的请求模型本身，见 openai_gateway_passthrough.go。
func modelDowngradeForwardSentModel(account *Account, forwardModel string) string {
	if account.IsOpenAIPassthroughEnabled() {
		return upstreamSentModel(forwardModel, "")
	}
	_, upstreamModel := resolveOpenAIForwardMappedModels(account, forwardModel, false)
	return upstreamSentModel(forwardModel, upstreamModel)
}

type modelDowngradeSchedulerMode struct {
	name      string
	loadBatch bool
	advanced  bool
}

var modelDowngradeSchedulerModes = []modelDowngradeSchedulerMode{
	{name: "legacy"},
	{name: "legacy load batch", loadBatch: true},
	{name: "advanced scheduler", advanced: true},
}

func newModelDowngradeSchedulingGateway(mode modelDowngradeSchedulerMode, repo modelDowngradeSchedulingRepo, cache *schedulerTestGatewayCache) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = mode.loadBatch
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	if mode.advanced {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	} else {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
	}
	return svc
}

// selectModelDowngradeTestAccount 走一次真实的 OpenAI HTTP 选号，返回选中的账号 ID；
// 没有可用账号时返回 0 和错误。
func selectModelDowngradeTestAccount(t *testing.T, svc *OpenAIGatewayService, groupID int64, sessionHash, model string, excluded ...int64) (int64, error) {
	t.Helper()
	var excludedIDs map[int64]struct{}
	if len(excluded) > 0 {
		excludedIDs = make(map[int64]struct{}, len(excluded))
		for _, id := range excluded {
			excludedIDs[id] = struct{}{}
		}
	}
	selection, _, err := svc.SelectAccountWithScheduler(
		context.Background(), &groupID, "", sessionHash, model, excludedIDs, OpenAIUpstreamTransportAny, false,
	)
	if err != nil {
		return 0, err
	}
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	return selection.Account.ID, nil
}

func mustSelectModelDowngradeTestAccount(t *testing.T, svc *OpenAIGatewayService, groupID int64, sessionHash, model string, excluded ...int64) int64 {
	t.Helper()
	id, err := selectModelDowngradeTestAccount(t, svc, groupID, sessionHash, model, excluded...)
	require.NoError(t, err)
	return id
}

func TestModelDowngradeGuardModelBlockIsHonoredByOpenAIHTTPScheduling(t *testing.T) {
	const (
		groupID      = int64(20901)
		targetID     = int64(20911)
		fallbackID   = int64(20912)
		responseSeen = "gpt-5.6-luna"
		otherModel   = "gpt-5.6-sol"
	)

	cases := []struct {
		name string
		// target 构造会被守卫屏蔽的账号；每个子用例都要一份新的 Extra / Credentials，
		// 守卫会往 Extra 里写 model_rate_limits。
		target       func() Account
		forwardModel string
		wantSent     string
	}{
		{
			name: "api key account without model mapping",
			target: func() Account {
				return Account{
					Type: AccountTypeAPIKey,
				}
			},
			forwardModel: "gpt-6-astra",
			wantSent:     "gpt-6-astra",
		},
		{
			name: "oauth account without model mapping",
			target: func() Account {
				return Account{
					Type: AccountTypeOAuth,
				}
			},
			forwardModel: "gpt-6-astra",
			wantSent:     "gpt-6-astra",
		},
		{
			name: "api key account with model mapping",
			target: func() Account {
				return Account{
					Type: AccountTypeAPIKey,
					Credentials: map[string]any{
						"model_mapping": map[string]any{"astra": "gpt-6-astra", otherModel: otherModel},
					},
				}
			},
			forwardModel: "astra",
			wantSent:     "gpt-6-astra",
		},
		{
			name: "oauth account with model mapping",
			target: func() Account {
				return Account{
					Type: AccountTypeOAuth,
					Credentials: map[string]any{
						"model_mapping": map[string]any{"astra": "gpt-6-astra", otherModel: otherModel},
					},
				}
			},
			forwardModel: "astra",
			wantSent:     "gpt-6-astra",
		},
		{
			name: "api key account with openai_passthrough",
			target: func() Account {
				return Account{
					Type:  AccountTypeAPIKey,
					Extra: map[string]any{"openai_passthrough": true},
				}
			},
			forwardModel: "gpt-6-astra",
			wantSent:     "gpt-6-astra",
		},
		{
			name: "oauth account with openai_passthrough",
			target: func() Account {
				return Account{
					Type:  AccountTypeOAuth,
					Extra: map[string]any{"openai_passthrough": true},
				}
			},
			forwardModel: "gpt-6-astra",
			wantSent:     "gpt-6-astra",
		},
	}

	for _, mode := range modelDowngradeSchedulerModes {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				target := tc.target()
				target.ID = targetID
				target.Name = "downgraded"
				target.Platform = PlatformOpenAI
				target.Status = StatusActive
				target.Schedulable = true
				target.Concurrency = 5
				fallback := Account{
					ID:          fallbackID,
					Name:        "healthy",
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 5,
				}
				repo := modelDowngradeSchedulingRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{target, fallback}}}
				cache := &schedulerTestGatewayCache{}
				gateway := newModelDowngradeSchedulingGateway(mode, repo, cache)

				// 前置条件：屏蔽前（排除 fallback 后）target 可以被选中，否则后面的断言证明不了任何事。
				require.Equal(t, targetID, mustSelectModelDowngradeTestAccount(t, gateway, groupID, "", tc.forwardModel, fallbackID))

				// 守卫侧：RecordUsage 交给守卫的是「发往上游的模型」，达到阈值后按默认动作
				// （仅屏蔽该模型）写 model_rate_limits。
				sentModel := modelDowngradeForwardSentModel(&repo.accounts[0], tc.forwardModel)
				require.Equal(t, tc.wantSent, sentModel)

				guard := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
				guard.SetSettingService(NewSettingService(&modelDowngradeSettingRepo{
					value: modelDowngradeGuardJSON(t, enabledModelDowngradeGuardSettings()),
				}, &config.Config{}))
				guard.SetModelDowngradeCounterCache(&modelDowngradeCounterStub{counts: []int64{5}})
				guard.HandleModelDowngrade(context.Background(), &repo.accounts[0], sentModel, responseSeen)

				limits, ok := repo.accounts[0].Extra[modelRateLimitsKey].(map[string]any)
				require.True(t, ok, "guard must have written extra.model_rate_limits")
				require.Contains(t, limits, sentModel)
				require.Nil(t, repo.accounts[0].TempUnschedulableUntil, "默认动作不动整账号")

				// 调度侧：同一个请求模型不再选到被屏蔽的账号——只剩它时无号可选，
				// 有其他账号时落到其他账号……
				require.False(t, repo.accounts[0].IsSchedulableForModelWithContext(context.Background(), tc.forwardModel))
				_, err := selectModelDowngradeTestAccount(t, gateway, groupID, "", tc.forwardModel, fallbackID)
				require.ErrorIs(t, err, ErrNoAvailableAccounts)
				require.Equal(t, fallbackID, mustSelectModelDowngradeTestAccount(t, gateway, groupID, "", tc.forwardModel))

				// ……会话粘性也不能把请求钉回被屏蔽的账号……
				cache.sessionBindings = map[string]int64{"sess_downgrade": targetID}
				require.Equal(t, fallbackID, mustSelectModelDowngradeTestAccount(t, gateway, groupID, "sess_downgrade", tc.forwardModel))

				// ……而同一个账号的其他模型不受影响。
				require.True(t, repo.accounts[0].IsSchedulableForModelWithContext(context.Background(), otherModel))
				require.Equal(t, targetID, mustSelectModelDowngradeTestAccount(t, gateway, groupID, "", otherModel, fallbackID))
			})
		}
	}
}
