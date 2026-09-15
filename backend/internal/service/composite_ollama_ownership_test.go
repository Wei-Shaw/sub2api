package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/stretchr/testify/require"
)

// compositeOwnershipScopedRepo 是尊重查询契约的 fake account repo：只返回同时
// 满足 groupID、platforms 白名单与 schedulable 的记录，语义与
// accountRepository.ListModelAvailabilityCandidates 的 group 分支一致
// （platforms 为空时返回空结果，组隔离由 groupID 保证，配置态由 schedulable
// 保证）。lastPlatforms 记录调用方实际传入的平台集合，供回归断言直接检查。
type compositeOwnershipScopedRepo struct {
	AccountRepository
	records       []compositeOwnershipScopedRecord
	lastPlatforms []string
}

type compositeOwnershipScopedRecord struct {
	groupID     int64
	platform    string
	schedulable bool
	account     Account
}

func (r *compositeOwnershipScopedRepo) ListModelAvailabilityCandidates(_ context.Context, groupID *int64, platforms []string, _ bool) ([]Account, error) {
	r.lastPlatforms = append([]string(nil), platforms...)
	if groupID == nil || len(platforms) == 0 {
		return nil, nil
	}
	allowed := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		allowed[platform] = struct{}{}
	}
	accounts := make([]Account, 0, len(r.records))
	for _, rec := range r.records {
		if rec.groupID != *groupID || !rec.schedulable {
			continue
		}
		if _, ok := allowed[rec.platform]; !ok {
			continue
		}
		accounts = append(accounts, rec.account)
	}
	return accounts, nil
}

func newOllamaOwnershipResolver(records ...compositeOwnershipScopedRecord) (*CompositeRouteResolver, *compositeOwnershipScopedRepo) {
	repo := &compositeOwnershipScopedRepo{records: records}
	svc := &GatewayService{accountRepo: repo}
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(svc.resolveCompositeModelOwnership)
	return resolver, repo
}

func mappingAccount(id int64, platform, alias, upstreamModel string) Account {
	return Account{
		ID:       id,
		Platform: platform,
		Credentials: map[string]any{
			"model_mapping": map[string]any{alias: upstreamModel},
		},
	}
}

// 回归护栏：组内只有 ollama_cloud 账号时，ownership 候选查询必须把
// ollama_cloud 纳入扫描平台集合（compositeOwnershipQueryPlatforms）；否则仓库
// 层按 platforms 白名单过滤会把 ollama 账号整个丢掉，显式别名再也无法被
// Resolve 识别（detector 刻意无 ollama 分支，只能落 unmatched）。
// 同组 schedulable=false 与其他组的账号不得混入目标平台。
func TestResolveCompositeModelOwnershipFindsOllamaCloudAlias(t *testing.T) {
	const (
		group         = int64(7)
		otherGroup    = int64(8)
		alias         = "private-ollama-alias"
		upstreamModel = "qwen3-coder"
	)
	resolver, repo := newOllamaOwnershipResolver(
		compositeOwnershipScopedRecord{
			groupID: group, platform: PlatformOllamaCloud, schedulable: true,
			account: mappingAccount(166, PlatformOllamaCloud, alias, upstreamModel),
		},
		// 同组但 schedulable=false：配置态查询不得纳入。
		compositeOwnershipScopedRecord{
			groupID: group, platform: PlatformDeepseek, schedulable: false,
			account: mappingAccount(163, PlatformDeepseek, alias, "deepseek-v4-pro"),
		},
		// 其他组的 schedulable 账号：组隔离不得纳入。
		compositeOwnershipScopedRecord{
			groupID: otherGroup, platform: PlatformDeepseek, schedulable: true,
			account: mappingAccount(151, PlatformDeepseek, alias, "deepseek-v4-pro"),
		},
	)

	decision, err := resolver.Resolve(context.Background(), group, alias, CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.Contains(t, repo.lastPlatforms, PlatformOllamaCloud,
		"ownership 候选查询必须显式扫描 ollama_cloud，否则组内 ollama 账号不可见")
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccount, decision.Source)
	require.Equal(t, PlatformOllamaCloud, decision.TargetPlatform)
	require.Empty(t, decision.CandidatePlatforms, "同组不可调度与其他组账号不得构成候选池")
}

// 最高价值反向对照用例：别名本身是 detector 能识别的真实模型名
// （MiniMax-M3 → minimax、deepseek-flash → deepseek、k3 → kimi），组内唯一
// 可调度账号是 ollama_cloud 的显式 mapping。ownership 查询漏掉 ollama_cloud
// 时 Resolve 会回退 detector 并投到上述错误平台；补线后必须走账号目录声明
// （Source=account，TargetPlatform=ollama_cloud），不得被 detector 冒领。
func TestResolveCompositeModelOwnershipPrefersOllamaCloudOverDetector(t *testing.T) {
	const group = int64(7)
	for _, alias := range []string{"MiniMax-M3", "deepseek-flash", "k3"} {
		t.Run(alias, func(t *testing.T) {
			detected, ok := DetectModelPlatform(alias)
			require.True(t, ok, "反向对照别名必须能被 detector 命中")
			require.NotEqual(t, PlatformOllamaCloud, detected, "detector 不得直接命中 ollama_cloud")

			resolver, repo := newOllamaOwnershipResolver(
				compositeOwnershipScopedRecord{
					groupID: group, platform: PlatformOllamaCloud, schedulable: true,
					account: mappingAccount(166, PlatformOllamaCloud, alias, "gpt-oss:120b"),
				},
				// 同组不可调度、以及本可被 detector 投中的平台账号：都不得混入。
				compositeOwnershipScopedRecord{
					groupID: group, platform: detected, schedulable: false,
					account: mappingAccount(163, detected, alias, "unused"),
				},
				compositeOwnershipScopedRecord{
					groupID: 8, platform: detected, schedulable: true,
					account: mappingAccount(151, detected, alias, "unused"),
				},
			)

			decision, err := resolver.Resolve(context.Background(), group, alias, CompositeRouteEndpointResponses)
			require.NoError(t, err)
			require.Contains(t, repo.lastPlatforms, PlatformOllamaCloud)
			require.True(t, decision.Matched)
			require.Equal(t, CompositeRouteSourceAccount, decision.Source,
				"必须由账号目录声明解析，而不是 detector 回退")
			require.NotEqual(t, CompositeRouteSourceDetector, decision.Source)
			require.Equal(t, PlatformOllamaCloud, decision.TargetPlatform)
			require.Empty(t, decision.CandidatePlatforms)
		})
	}
}

// 强声明分层：ollama_cloud 与 anthropic 对同一公开别名都有精确 mapping 时，
// 两者同为强声明，必须构成稳定候选池（不是 detector 单平台、也不是静默丢弃
// ollama）。缺 ollama_cloud 时池退化为 anthropic 单平台，本用例会红。
func TestResolveCompositeModelOwnershipPoolsOllamaCloudWithStrongClaimer(t *testing.T) {
	const (
		group = int64(7)
		alias = "MiniMax-M3"
	)
	resolver, _ := newOllamaOwnershipResolver(
		compositeOwnershipScopedRecord{
			groupID: group, platform: PlatformOllamaCloud, schedulable: true,
			account: mappingAccount(166, PlatformOllamaCloud, alias, "gpt-oss:120b"),
		},
		compositeOwnershipScopedRecord{
			groupID: group, platform: PlatformAnthropic, schedulable: true,
			account: mappingAccount(101, PlatformAnthropic, alias, "claude-sonnet-4-5"),
		},
	)

	decision, err := resolver.Resolve(context.Background(), group, alias, CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccountPool, decision.Source)
	require.Empty(t, decision.TargetPlatform, "多平台强声明不猜单平台")
	require.ElementsMatch(t, []string{PlatformAnthropic, PlatformOllamaCloud}, decision.CandidatePlatforms)
}

// 轻量不变量：ownership 候选扫描范围必须与「全部具体平台」的既有权威清单
// （model.AllPlatforms）一致；新增平台若只加了 AllPlatforms 而漏掉 ownership，
// 本用例直接红灯，不引入任何新的 runtime registry。
func TestCompositeOwnershipQueryPlatformsCoverConcretePlatforms(t *testing.T) {
	require.ElementsMatch(t, model.AllPlatforms(), compositeOwnershipQueryPlatforms())
}

// schedulable=false 的 ollama_cloud 账号不构成候选，且显式别名不得靠 detector
// 兜底；其他组同别名账号同样不得混入（组隔离双向验证）。
func TestResolveCompositeModelOwnershipSkipsUnschedulableOllamaCloud(t *testing.T) {
	const alias = "private-ollama-alias"
	resolver, repo := newOllamaOwnershipResolver(
		compositeOwnershipScopedRecord{
			groupID: 7, platform: PlatformOllamaCloud, schedulable: false,
			account: mappingAccount(163, PlatformOllamaCloud, alias, "qwen3-coder"),
		},
		compositeOwnershipScopedRecord{
			groupID: 8, platform: PlatformOllamaCloud, schedulable: true,
			account: mappingAccount(151, PlatformOllamaCloud, alias, "qwen3-coder"),
		},
	)

	decision, err := resolver.Resolve(context.Background(), 7, alias, CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.Contains(t, repo.lastPlatforms, PlatformOllamaCloud)
	require.False(t, decision.Matched, "不可调度与其他组的 ollama 账号不得被解析为可路由目标")
	require.Empty(t, decision.TargetPlatform)
}
