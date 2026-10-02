//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// parkForeignOpenAIAccounts 把本次测试之外的 OpenAI 活跃账号临时置为 disabled，
// 让降级守卫的全局比例统计只看到本用例创建的账号，测试结束后原样恢复。
//
// CountOpenAIModelDowngradeBlocked 的分母是全库 OpenAI 活跃账号，其他集成用例
// 留下的账号会把比例冲淡，断言就没意义了。
func parkForeignOpenAIAccounts(t *testing.T, ctx context.Context, keep []int64) {
	t.Helper()

	rows, err := integrationDB.QueryContext(ctx, `
		SELECT id FROM accounts
		WHERE deleted_at IS NULL AND status = 'active' AND platform = $1 AND NOT (id = ANY($2))
	`, service.PlatformOpenAI, pq.Array(keep))
	require.NoError(t, err)
	foreign := make([]int64, 0)
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		foreign = append(foreign, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	if len(foreign) == 0 {
		return
	}

	_, err = integrationDB.ExecContext(ctx,
		`UPDATE accounts SET status = 'disabled' WHERE id = ANY($1)`, pq.Array(foreign))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(),
			`UPDATE accounts SET status = 'active' WHERE id = ANY($1)`, pq.Array(foreign))
	})
}

func mustModelDowngradeReason(t *testing.T, sentModel, responseModel string, until time.Time) string {
	t.Helper()
	raw, err := json.Marshal(&service.TempUnschedState{
		UntilUnix:            until.Unix(),
		TriggeredAtUnix:      time.Now().Unix(),
		MatchedKeyword:       service.ModelDowngradeGuardKeyword,
		RuleIndex:            -1,
		ErrorMessage:         fmt.Sprintf("%s → %s (5 hits in 30 min)", sentModel, responseModel),
		TriggerCount:         5,
		TriggerThreshold:     5,
		TriggerWindowMinutes: 30,
	})
	require.NoError(t, err)
	return string(raw)
}

// createModelDowngradeGuardAccounts 建 n 个 OpenAI 活跃账号，并登记清理。
func createModelDowngradeGuardAccounts(t *testing.T, ctx context.Context, prefix string, n int) []int64 {
	t.Helper()
	suffix := time.Now().UnixNano()
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		account := mustCreateAccount(t, integrationEntClient, &service.Account{
			Name:        fmt.Sprintf("%s-%d-%d", prefix, suffix, i),
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{"api_key": fmt.Sprintf("%s-%d-%d", prefix, suffix, i)},
		})
		ids = append(ids, account.ID)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM scheduler_outbox WHERE account_id = ANY($1)`, pq.Array(ids))
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM account_groups WHERE account_id = ANY($1)`, pq.Array(ids))
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM accounts WHERE id = ANY($1)`, pq.Array(ids))
	})
	_ = ctx
	return ids
}

// TestCountOpenAIModelDowngradeBlockedCountsModelScope：
// 「仅屏蔽该模型」写进 extra.model_rate_limits，同样必须计入安全阀的分子，
// 否则 model_block 模式下守卫能绕过比例上限把整个号池屏蔽干净。
func TestCountOpenAIModelDowngradeBlockedCountsModelScope(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-count", 5)
	parkForeignOpenAIAccounts(t, ctx, ids)

	now := time.Now()
	until := now.Add(6 * time.Hour)
	reason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)

	// (a) 整账号临时不可调度
	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[0], until, reason))
	// (b) 仅屏蔽单个模型
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[1], "gpt-6-astra", until, reason))
	// (a) + (b) 同时命中的账号只能算一个
	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[2], until, reason))
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[2], "gpt-6-astra", until, reason))
	// 已过期的模型屏蔽不算
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[3], "gpt-6-astra", now.Add(-time.Hour), reason))
	// 别的原因写的模型限流不算
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[4], "gpt-6-astra", until, "upstream 429"))

	blocked, total, err := repo.CountOpenAIModelDowngradeBlocked(ctx, now)
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Equal(t, int64(3), blocked, "整账号、仅模型、以及两者同时命中的账号各算一个")

	items, err := repo.ListOpenAIModelDowngradeBlocked(ctx, now)
	require.NoError(t, err)
	byAccount := map[int64][]string{}
	for _, item := range items {
		byAccount[item.AccountID] = append(byAccount[item.AccountID], item.Scope)
	}
	require.Len(t, byAccount, 3, "列表的候选口径必须和计数一致")
	require.ElementsMatch(t, []string{service.ModelDowngradeBlockedScopeAccount}, byAccount[ids[0]])
	require.ElementsMatch(t, []string{service.ModelDowngradeBlockedScopeModel}, byAccount[ids[1]])
	require.ElementsMatch(t, []string{
		service.ModelDowngradeBlockedScopeAccount,
		service.ModelDowngradeBlockedScopeModel,
	}, byAccount[ids[2]])
}

// TestApplyOpenAIModelDowngradeBlockRespectsRatioUnderConcurrency：
// 10 个 OpenAI 活跃账号、2 个已经因降级下线，8 个 goroutine 同时申请下线剩下的 8 个。
// 上限 30% 意味着最多只能有 3 个账号处于受限状态；拆成 count + write 的写法在这里
// 会让 8 个请求读到同一个 blocked=2 后全部通过。
func TestApplyOpenAIModelDowngradeBlockRespectsRatioUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-race", 10)
	parkForeignOpenAIAccounts(t, ctx, ids)

	now := time.Now()
	until := now.Add(6 * time.Hour)
	reason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)

	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[0], until, reason))
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[1], "gpt-6-astra", until, reason))

	blocked, total, err := repo.CountOpenAIModelDowngradeBlocked(ctx, now)
	require.NoError(t, err)
	require.Equal(t, int64(2), blocked)
	require.Equal(t, int64(10), total)

	const maxRatio = 0.3
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		applied int
		errs    []error
	)
	start := make(chan struct{})
	for i := 2; i < 10; i++ {
		accountID := ids[i]
		// 一半走整账号、一半走仅模型，两种范围共享同一个比例上限。
		scope, model := service.ModelDowngradeBlockedScopeAccount, ""
		if i%2 == 0 {
			scope, model = service.ModelDowngradeBlockedScopeModel, "gpt-6-astra"
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := repo.ApplyOpenAIModelDowngradeBlock(
				context.Background(), accountID, scope, model, until, reason, maxRatio, time.Now(),
			)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if res.Applied {
				applied++
			}
		}()
	}
	close(start)
	wg.Wait()
	require.Empty(t, errs)
	require.Equal(t, 1, applied, "2 个已下线 + 1 个新下线 = 30%，再多一个就越界")

	finalBlocked, finalTotal, err := repo.CountOpenAIModelDowngradeBlocked(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(10), finalTotal)
	require.LessOrEqual(t, finalBlocked, int64(3), "比例上限在并发下也不能被击穿")
}

// TestApplyOpenAIModelDowngradeBlockCountsAlreadyBlockedAccountOnce：
// model_block 模式下同一个账号屏蔽第二个模型时受限账号数没变，比例判定必须按 blocked
// 而不是 blocked+1；对照组换一个还没受限的账号，同样的比例下必须被拦。
func TestApplyOpenAIModelDowngradeBlockCountsAlreadyBlockedAccountOnce(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-recount", 10)
	parkForeignOpenAIAccounts(t, ctx, ids)

	now := time.Now()
	until := now.Add(6 * time.Hour)
	reason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)
	solReason := mustModelDowngradeReason(t, "gpt-6-sol", "gpt-5.6-luna", until)

	// 3/10 已受限，上限 0.3：此刻正好卡在上限上。
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[0], "gpt-6-astra", until, reason))
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[1], "gpt-6-astra", until, reason))
	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[2], until, reason))

	blocked, total, err := repo.CountOpenAIModelDowngradeBlocked(ctx, now)
	require.NoError(t, err)
	require.Equal(t, int64(3), blocked)
	require.Equal(t, int64(10), total)

	const maxRatio = 0.3

	// 已受限的账号再屏蔽一个模型：分子不变，必须放行。
	result, err := repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-6-sol",
		until, solReason, maxRatio, time.Now(),
	)
	require.NoError(t, err)
	require.True(t, result.Applied, "已受限账号再屏蔽一个模型不该被比例上限拦下")
	require.False(t, result.AlreadyBlocked, "屏蔽的是另一个模型，不是重复动作")
	require.Equal(t, int64(3), result.Blocked)
	require.Equal(t, int64(10), result.Total)

	// 分子确实没变。
	blocked, _, err = repo.CountOpenAIModelDowngradeBlocked(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(3), blocked)

	// 对照组：还没受限的账号会把比例推到 0.4，必须被拦。
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[4], service.ModelDowngradeBlockedScopeModel, "gpt-6-sol",
		until, solReason, maxRatio, time.Now(),
	)
	require.NoError(t, err)
	require.False(t, result.Applied, "新增受限账号会越过上限，必须拒绝")

	blocked, _, err = repo.CountOpenAIModelDowngradeBlocked(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(3), blocked)
}

// TestApplyOpenAIModelDowngradeBlockModelScopeIsVisibleToScheduling：守卫写进
// extra.model_rate_limits 的形状必须能被调度侧的 Account.IsSchedulableForModel 读懂——
// 只屏蔽被降级的那个模型，账号本身和其他模型照常可调度；写入同时要进 scheduler outbox，
// 调度快照才会刷新。
func TestApplyOpenAIModelDowngradeBlockModelScopeIsVisibleToScheduling(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-visible", 1)
	parkForeignOpenAIAccounts(t, ctx, ids)

	var outboxBefore int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1`, ids[0]).Scan(&outboxBefore))

	until := time.Now().Add(6 * time.Hour)
	reason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)
	result, err := repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra", until, reason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.True(t, result.Applied)

	account, err := repo.GetByID(ctx, ids[0])
	require.NoError(t, err)
	require.True(t, account.IsSchedulable(), "仅屏蔽模型不该动账号级可调度状态")
	require.False(t, account.IsSchedulableForModel("gpt-6-astra"))
	require.True(t, account.IsSchedulableForModel("gpt-5.6-sol"))

	var outboxAfter int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1`, ids[0]).Scan(&outboxAfter))
	require.Greater(t, outboxAfter, outboxBefore, "写入必须进 scheduler outbox")

	// 提前恢复后模型重新可调度。
	released, err := repo.ReleaseOpenAIModelDowngradeBlock(ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra")
	require.NoError(t, err)
	require.True(t, released)
	account, err = repo.GetByID(ctx, ids[0])
	require.NoError(t, err)
	require.True(t, account.IsSchedulableForModel("gpt-6-astra"))
}

// modelDowngradeAccountSnapshot 读出账号在幂等断言里关心的全部字段。
type modelDowngradeAccountSnapshot struct {
	TempUntil   *time.Time
	TempReason  *string
	ModelLimits map[string]map[string]string
	UpdatedAt   time.Time
}

func readModelDowngradeAccountSnapshot(t *testing.T, ctx context.Context, id int64) modelDowngradeAccountSnapshot {
	t.Helper()
	var (
		snapshot modelDowngradeAccountSnapshot
		raw      []byte
	)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT temp_unschedulable_until,
			temp_unschedulable_reason,
			COALESCE(extra -> 'model_rate_limits', '{}'::jsonb),
			updated_at
		FROM accounts WHERE id = $1
	`, id).Scan(&snapshot.TempUntil, &snapshot.TempReason, &raw, &snapshot.UpdatedAt))
	require.NoError(t, json.Unmarshal(raw, &snapshot.ModelLimits))
	return snapshot
}

// TestApplyOpenAIModelDowngradeBlockIsIdempotentForBlockedAccount：
// 整账号被守卫下线后，这个号上进行中的 WS 连接继续跑、命中计数继续涨，阈值后守卫
// 再次触发；此时配置可能已切成「仅屏蔽该模型」，不能再写一条 model_rate_limits，
// 否则页面上同一个账号出现「整账号」和「仅模型」两行。
//
// 整账号下线覆盖该账号的所有模型：两种范围的重复请求都必须原地返回 AlreadyBlocked，
// temp_unschedulable_* / model_rate_limits / updated_at 一个都不许动。
func TestApplyOpenAIModelDowngradeBlockIsIdempotentForBlockedAccount(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-idem-account", 4)
	parkForeignOpenAIAccounts(t, ctx, ids)

	now := time.Now()
	until := now.Add(6 * time.Hour)
	reason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)

	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[0], until, reason))
	before := readModelDowngradeAccountSnapshot(t, ctx, ids[0])

	// (a) 再来一次整账号下线：重复动作。
	result, err := repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[0], service.ModelDowngradeBlockedScopeAccount, "",
		until.Add(time.Hour), reason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.True(t, result.AlreadyBlocked)
	require.False(t, result.Applied)

	// (b) 配置切成「仅屏蔽该模型」后再触发：整账号下线已经覆盖了所有模型。
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra",
		until, reason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.True(t, result.AlreadyBlocked)
	require.False(t, result.Applied)

	after := readModelDowngradeAccountSnapshot(t, ctx, ids[0])
	require.Equal(t, before.TempUntil, after.TempUntil, "重复下线不能把下线窗口往后顶")
	require.Equal(t, before.TempReason, after.TempReason)
	require.Empty(t, after.ModelLimits, "整账号下线期间不该再写一条模型屏蔽")
	require.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "一行都没写，updated_at 不该变")

	// 对照组：别的来源（不是守卫）写的临时下线不构成覆盖，守卫照常写自己的那条。
	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[1], until, `{"matched_keyword":"stream_timeout"}`))
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[1], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra",
		until, reason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.False(t, result.AlreadyBlocked)
	require.True(t, result.Applied)
	require.Contains(t, readModelDowngradeAccountSnapshot(t, ctx, ids[1]).ModelLimits, "gpt-6-astra")

	// 对照组：已过期的整账号下线同样不构成覆盖。
	expired := now.Add(-time.Hour)
	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[2], expired,
		mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", expired)))
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[2], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra",
		until, reason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.False(t, result.AlreadyBlocked)
	require.True(t, result.Applied)
}

// TestApplyOpenAIModelDowngradeBlockIsIdempotentForBlockedModel：同一个模型已被守卫
// 屏蔽时，model 范围的重复请求跳过；换一个模型照常写；配置切成整账号时也照常写
// ——那是「仅模型 → 整账号」的升级，不是重复。
func TestApplyOpenAIModelDowngradeBlockIsIdempotentForBlockedModel(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-idem-model", 4)
	parkForeignOpenAIAccounts(t, ctx, ids)

	now := time.Now()
	until := now.Add(6 * time.Hour)
	astraReason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)
	solReason := mustModelDowngradeReason(t, "gpt-6-sol", "gpt-5.6-luna", until)

	require.NoError(t, repo.SetModelRateLimit(ctx, ids[0], "gpt-6-astra", until, astraReason))
	before := readModelDowngradeAccountSnapshot(t, ctx, ids[0])

	// (a) 同一个模型再屏蔽一次：重复动作，窗口不该被顶到更晚。
	result, err := repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra",
		until.Add(time.Hour), astraReason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.True(t, result.AlreadyBlocked)
	require.False(t, result.Applied)

	after := readModelDowngradeAccountSnapshot(t, ctx, ids[0])
	require.Equal(t, before.ModelLimits, after.ModelLimits, "重复屏蔽同一个模型不能改写限流窗口")
	require.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "一行都没写，updated_at 不该变")

	// (b) 换一个模型：不是重复，照常写。
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-6-sol",
		until, solReason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.False(t, result.AlreadyBlocked)
	require.True(t, result.Applied)
	require.ElementsMatch(t, []string{"gpt-6-astra", "gpt-6-sol"},
		modelDowngradeModelRateLimitKeys(t, ctx, ids[0]))

	// (c) 配置切成「整账号临时下线」：模型屏蔽不构成覆盖，这是升级。
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[0], service.ModelDowngradeBlockedScopeAccount, "",
		until, astraReason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.False(t, result.AlreadyBlocked)
	require.True(t, result.Applied)
	require.NotNil(t, readModelDowngradeAccountSnapshot(t, ctx, ids[0]).TempUntil)

	// 对照组：别的来源写的模型限流（例如上游 429）不构成覆盖。
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[1], "gpt-6-astra", until, "upstream 429"))
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[1], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra",
		until, astraReason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.False(t, result.AlreadyBlocked)
	require.True(t, result.Applied)

	// 对照组：已过期的模型屏蔽同样不构成覆盖。
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[2], "gpt-6-astra", now.Add(-time.Hour), astraReason))
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[2], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra",
		until, astraReason, 1.0, time.Now(),
	)
	require.NoError(t, err)
	require.False(t, result.AlreadyBlocked)
	require.True(t, result.Applied)
}

// TestApplyOpenAIModelDowngradeBlockIdempotencyPrecedesRatioCap：幂等判定排在比例
// 上限之前——已经下线的账号再触发时号池状态没有任何变化，不该被当成「被安全阀拦下」，
// 否则守卫会给一个早就下线的账号反复写观察记录、反复告警。
func TestApplyOpenAIModelDowngradeBlockIdempotencyPrecedesRatioCap(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-idem-ratio", 10)
	parkForeignOpenAIAccounts(t, ctx, ids)

	now := time.Now()
	until := now.Add(6 * time.Hour)
	reason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)

	// 5/10 已受限，远超 0.3 的上限：任何新增下线都会被安全阀拦下。
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.SetTempUnschedulable(ctx, ids[i], until, reason))
	}

	result, err := repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra",
		until, reason, 0.3, time.Now(),
	)
	require.NoError(t, err)
	require.True(t, result.AlreadyBlocked, "已下线的账号是重复动作，不是被比例上限拦下")
	require.False(t, result.Applied)

	// 对照组：还没受限的账号在同样的比例下被安全阀拦下，两条路径可区分。
	result, err = repo.ApplyOpenAIModelDowngradeBlock(
		ctx, ids[9], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra",
		until, reason, 0.3, time.Now(),
	)
	require.NoError(t, err)
	require.False(t, result.AlreadyBlocked)
	require.False(t, result.Applied)
	require.Equal(t, int64(5), result.Blocked)
	require.Equal(t, int64(10), result.Total)
}

// modelDowngradeModelRateLimitKeys 读出账号 extra.model_rate_limits 里的所有模型 key。
func modelDowngradeModelRateLimitKeys(t *testing.T, ctx context.Context, id int64) []string {
	t.Helper()
	var raw []byte
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COALESCE(extra -> 'model_rate_limits', '{}'::jsonb) FROM accounts WHERE id = $1`, id,
	).Scan(&raw))
	entries := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &entries))
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	return keys
}

// TestReleaseOpenAIModelDowngradeBlockAccountScopeKeepsModelRateLimits：
// 账号同时有降级临时下线和图片模型的 429 冷却时，account 范围的提前恢复只清临时下线，
// 无关的模型冷却必须原样保留（ClearTempUnschedulable 会把它一起清掉）。
func TestReleaseOpenAIModelDowngradeBlockAccountScopeKeepsModelRateLimits(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-release-account", 2)
	now := time.Now()
	until := now.Add(6 * time.Hour)
	reason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)

	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[0], until, reason))
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[0], "gpt-image-1", until, "upstream 429"))

	released, err := repo.ReleaseOpenAIModelDowngradeBlock(ctx, ids[0], service.ModelDowngradeBlockedScopeAccount, "")
	require.NoError(t, err)
	require.True(t, released)

	var tempUntil *time.Time
	var tempReason *string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT temp_unschedulable_until, temp_unschedulable_reason FROM accounts WHERE id = $1`, ids[0],
	).Scan(&tempUntil, &tempReason))
	require.Nil(t, tempUntil)
	require.Nil(t, tempReason)
	require.ElementsMatch(t, []string{"gpt-image-1"}, modelDowngradeModelRateLimitKeys(t, ctx, ids[0]),
		"图片模型的 429 冷却与降级守卫无关，不能被提前恢复误清")

	// 别的来源写的临时下线不属于守卫的解除范围。
	require.NoError(t, repo.SetTempUnschedulable(ctx, ids[1], until, `{"matched_keyword":"stream_timeout"}`))
	released, err = repo.ReleaseOpenAIModelDowngradeBlock(ctx, ids[1], service.ModelDowngradeBlockedScopeAccount, "")
	require.NoError(t, err)
	require.False(t, released, "reason 不是降级来源时一行都不该动")

	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT temp_unschedulable_until FROM accounts WHERE id = $1`, ids[1],
	).Scan(&tempUntil))
	require.NotNil(t, tempUntil)
}

// TestReleaseOpenAIModelDowngradeBlockModelScopeRemovesOnlyThatModel：
// model 范围只删对应的那个 key，其他模型（无论是不是降级来源）都保留；
// 目标模型的 reason 不是降级来源时返回 false。
func TestReleaseOpenAIModelDowngradeBlockModelScopeRemovesOnlyThatModel(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)

	ids := createModelDowngradeGuardAccounts(t, ctx, "downgrade-release-model", 2)
	now := time.Now()
	until := now.Add(6 * time.Hour)
	astraReason := mustModelDowngradeReason(t, "gpt-6-astra", "gpt-5.6-luna", until)
	solReason := mustModelDowngradeReason(t, "gpt-6-sol", "gpt-5.6-luna", until)

	require.NoError(t, repo.SetModelRateLimit(ctx, ids[0], "gpt-6-astra", until, astraReason))
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[0], "gpt-6-sol", until, solReason))
	require.NoError(t, repo.SetModelRateLimit(ctx, ids[0], "gpt-image-1", until, "upstream 429"))

	released, err := repo.ReleaseOpenAIModelDowngradeBlock(ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra")
	require.NoError(t, err)
	require.True(t, released)
	require.ElementsMatch(t, []string{"gpt-6-sol", "gpt-image-1"},
		modelDowngradeModelRateLimitKeys(t, ctx, ids[0]), "只应删掉被解除的那一个模型")

	// 非降级来源的模型限流不属于守卫的解除范围。
	released, err = repo.ReleaseOpenAIModelDowngradeBlock(ctx, ids[0], service.ModelDowngradeBlockedScopeModel, "gpt-image-1")
	require.NoError(t, err)
	require.False(t, released)
	require.ElementsMatch(t, []string{"gpt-6-sol", "gpt-image-1"}, modelDowngradeModelRateLimitKeys(t, ctx, ids[0]))

	// 账号上根本没有这个模型时同样返回 false。
	released, err = repo.ReleaseOpenAIModelDowngradeBlock(ctx, ids[1], service.ModelDowngradeBlockedScopeModel, "gpt-6-astra")
	require.NoError(t, err)
	require.False(t, released)
}
