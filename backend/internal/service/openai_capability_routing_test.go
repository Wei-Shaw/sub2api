//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// capabilityRoutingAccountRepo 把「候选集合」与「忽略临时状态的持久池」分开：
// persistent 非 nil 时 ListModelAvailabilityCandidates 返回它（模拟账号级限流账号不进候选集）。
type capabilityRoutingAccountRepo struct {
	schedulerTestOpenAIAccountRepo
	persistent []Account
}

func (r capabilityRoutingAccountRepo) ListModelAvailabilityCandidates(ctx context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]Account, error) {
	if r.persistent != nil {
		return append([]Account(nil), r.persistent...), nil
	}
	return r.schedulerTestOpenAIAccountRepo.ListModelAvailabilityCandidates(ctx, groupID, platforms, includeGrouped)
}

func capabilityRoutingCaps(t *testing.T, manifest string) CodexAccountCapabilities {
	t.Helper()
	caps, ok := DeriveCodexAccountCapabilities([]byte(manifest))
	require.True(t, ok)
	return caps
}

const capabilityRoutingBlueOnlyManifest = `{"models":[{"slug":"gpt-6-sol","service_tiers":[{"id":"priority"}],"available_access_programs":{"cyber":["standard","daybreak_blue"]}}]}`
const capabilityRoutingUltrafastOnlyManifest = `{"models":[{"slug":"gpt-6.1-sol","service_tiers":[{"id":"priority"},{"id":"ultrafast"}],"available_access_programs":{"cyber":["standard"]}}]}`

func capabilityRoutingAccount(id int64, groupID int64, accountType string, priority int, manifest string) Account {
	account := Account{
		ID: id, Platform: PlatformOpenAI, Type: accountType,
		Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: priority,
		GroupIDs: []int64{groupID},
		Extra:    map[string]any{"openai_oauth_responses_websockets_v2_enabled": true, "openai_apikey_responses_websockets_v2_enabled": true},
	}
	if manifest != "" {
		caps, _ := DeriveCodexAccountCapabilities([]byte(manifest))
		account.Extra[CodexCapabilitiesExtraKey] = caps
	}
	return account
}

func newCapabilityRoutingService(accounts []Account, persistent []Account, loadBatch bool, concurrency schedulerTestConcurrencyCache) *OpenAIGatewayService {
	cfg := newSchedulerTestOpenAIWSV2Config()
	cfg.Gateway.Scheduling.LoadBatchEnabled = loadBatch
	return &OpenAIGatewayService{
		accountRepo:        capabilityRoutingAccountRepo{schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, persistent: persistent},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrency),
	}
}

func selectWithRequirement(t *testing.T, svc *OpenAIGatewayService, ctx context.Context, groupID int64, previousResponseID, sessionHash string, excluded map[int64]struct{}, requirement OpenAIAccountRequirement) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	t.Helper()
	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, previousResponseID, sessionHash, "gpt-6-sol", excluded,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, requirement,
	)
	if selection != nil && selection.ReleaseFunc != nil {
		t.Cleanup(selection.ReleaseFunc)
	}
	return selection, decision, err
}

var (
	capabilityRoutingBlue      = OpenAIAccountRequirement{CyberProgram: "daybreak_blue"}
	capabilityRoutingRed       = OpenAIAccountRequirement{CyberProgram: "daybreak_red"}
	capabilityRoutingUltrafast = OpenAIAccountRequirement{ServiceTier: "ultrafast"}
	capabilityRoutingFast      = OpenAIAccountRequirement{ServiceTier: "priority"}
)

func runCapabilityRoutingModes(t *testing.T, fn func(t *testing.T, loadBatch bool)) {
	t.Helper()
	for _, loadBatch := range []bool{true, false} {
		name := "load_batch"
		if !loadBatch {
			name = "priority_lru"
		}
		t.Run(name, func(t *testing.T) { fn(t, loadBatch) })
	}
}

// 出口 1：previous_response_id 命中的账号不满足需求时释放槽位，继续在满足需求的账号中选择。
func TestCapabilityRouting_PreviousResponseOwnerWithoutCapabilityIsReleased(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	ctx := context.Background()
	groupID := int64(51100)
	responseID := "resp_capability_owner"
	released := make([]int64, 0)
	accounts := []Account{
		capabilityRoutingAccount(51101, groupID, AccountTypeOAuth, 0, codexCapabilitiesBlueAccountManifest),
		capabilityRoutingAccount(51102, groupID, AccountTypeOAuth, 0, codexCapabilitiesStandardAccountManifest),
	}
	svc := newCapabilityRoutingService(accounts, nil, true, schedulerTestConcurrencyCache{releasedIDs: &released})
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, responseID, 51102, time.Hour))

	selection, decision, err := selectWithRequirement(t, svc, ctx, groupID, responseID, "capability-prev-session", nil, capabilityRoutingBlue)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(51101), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickyPreviousHit)
	require.Contains(t, released, int64(51102), "持有 previous_response_id 的账号不满足需求时必须释放已抢槽位")

	// 无需求时仍复用持有响应的账号。
	selection, decision, err = selectWithRequirement(t, svc, ctx, groupID, responseID, "", nil, OpenAIAccountRequirement{})
	require.NoError(t, err)
	require.Equal(t, int64(51102), selection.Account.ID)
	require.True(t, decision.StickyPreviousHit)
}

// 出口 2：guardian 父账号不满足需求时放弃命中，继续后续出口。
func TestCapabilityRouting_GuardianParentWithoutCapabilityFallsBack(t *testing.T) {
	parentID := "44444444-4444-4444-8444-444444444444"
	parentHash := DeriveSessionHashFromSeed(parentID)
	groupID := int64(51200)
	released := make([]int64, 0)
	accounts := []Account{
		capabilityRoutingAccount(51201, groupID, AccountTypeOAuth, 10, codexCapabilitiesStandardAccountManifest),
		capabilityRoutingAccount(51202, groupID, AccountTypeOAuth, 0, codexCapabilitiesBlueAccountManifest),
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + parentHash: 51201}}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("false", ""),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{releasedIDs: &released}),
	}
	ctx := guardianAffinityTestContext(t, codexAutoReviewModel, "guardian", parentID, "")

	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "guardian-capability-child", codexAutoReviewModel, nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, capabilityRoutingBlue,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	if selection.ReleaseFunc != nil {
		defer selection.ReleaseFunc()
	}
	require.Equal(t, int64(51202), selection.Account.ID)
	require.NotEqual(t, openAIAccountScheduleLayerGuardianParent, decision.Layer)
	require.Contains(t, released, int64(51201), "guardian 父账号命中后不满足需求必须释放槽位")
	require.Zero(t, cache.deletedSessions["openai:"+parentHash], "父账号绑定本身保留")
}

// 出口 3 / 4：粘性命中与候选循环只选出具备能力的账号；无需求时候选集不变。
func TestCapabilityRouting_StickyAndCandidatesOnlySelectCapableAccounts(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		ctx := context.Background()
		groupID := int64(51300)
		accounts := []Account{
			capabilityRoutingAccount(51301, groupID, AccountTypeOAuth, 0, codexCapabilitiesStandardAccountManifest),
			capabilityRoutingAccount(51302, groupID, AccountTypeOAuth, 5, codexCapabilitiesBlueAccountManifest),
			capabilityRoutingAccount(51303, groupID, AccountTypeSetupToken, 1, ""),
		}
		svc := newCapabilityRoutingService(accounts, nil, loadBatch, schedulerTestConcurrencyCache{})
		sessionHash := "capability-sticky"
		require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, sessionHash, 51301, time.Hour))

		// 会话绑定到无 Daybreak 账号，Blue 需求放弃绑定并选中 Blue 账号。
		selection, decision, err := selectWithRequirement(t, svc, ctx, groupID, "", sessionHash, nil, capabilityRoutingBlue)
		require.NoError(t, err)
		require.Equal(t, int64(51302), selection.Account.ID)
		require.False(t, decision.StickySessionHit)

		// Ultrafast 需求同样只落到记录含 ultrafast 的账号；未探测账号不参与。
		selection, _, err = selectWithRequirement(t, svc, ctx, groupID, "", "", nil, capabilityRoutingUltrafast)
		require.NoError(t, err)
		require.Equal(t, int64(51302), selection.Account.ID)

		// Fast 需求：标准账号与 Blue 账号都满足，按优先级选标准账号；未探测账号仍被排除。
		selection, _, err = selectWithRequirement(t, svc, ctx, groupID, "", "", nil, capabilityRoutingFast)
		require.NoError(t, err)
		require.Equal(t, int64(51301), selection.Account.ID)

		// 无需求：候选集合与引入能力路由前一致，优先级最高的未探测账号照常被选中；
		// 同一池子带 Fast 需求时未探测账号被排除。
		unprobedFirst := []Account{
			capabilityRoutingAccount(51301, groupID, AccountTypeOAuth, 1, codexCapabilitiesStandardAccountManifest),
			capabilityRoutingAccount(51302, groupID, AccountTypeOAuth, 5, codexCapabilitiesBlueAccountManifest),
			capabilityRoutingAccount(51303, groupID, AccountTypeSetupToken, 0, ""),
		}
		svc = newCapabilityRoutingService(unprobedFirst, nil, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err = selectWithRequirement(t, svc, ctx, groupID, "", "", nil, OpenAIAccountRequirement{})
		require.NoError(t, err)
		require.Equal(t, int64(51303), selection.Account.ID)
		selection, _, err = selectWithRequirement(t, svc, ctx, groupID, "", "", nil, capabilityRoutingFast)
		require.NoError(t, err)
		require.Equal(t, int64(51301), selection.Account.ID)
	})
}

// Red 与 Blue 互不满足：只有 Blue 账号时 Red 需求无可用账号。
func TestCapabilityRouting_RedDoesNotMatchBlue(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		groupID := int64(51400)
		accounts := []Account{capabilityRoutingAccount(51401, groupID, AccountTypeOAuth, 0, codexCapabilitiesBlueAccountManifest)}
		svc := newCapabilityRoutingService(accounts, nil, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, capabilityRoutingRed)
		require.Nil(t, selection)
		var capErr *OpenAICapabilityUnavailableError
		require.ErrorAs(t, err, &capErr)
		require.Equal(t, capabilityRoutingRed, capErr.Requirement)
	})
}

// API Key 账号没有探测来源，不受能力需求过滤。
func TestCapabilityRouting_APIKeyAccountsAreNotFiltered(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		groupID := int64(51500)
		accounts := []Account{
			capabilityRoutingAccount(51501, groupID, AccountTypeOAuth, 0, codexCapabilitiesStandardAccountManifest),
			capabilityRoutingAccount(51502, groupID, AccountTypeAPIKey, 5, ""),
		}
		svc := newCapabilityRoutingService(accounts, nil, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, capabilityRoutingUltrafast)
		require.NoError(t, err)
		require.Equal(t, int64(51502), selection.Account.ID)
	})
}

// 持久池内没有任何具备能力的账号 → 类型化错误（Unwrap 到 ErrNoAvailableAccounts）。
func TestCapabilityRouting_NoCapableAccountInPersistentPoolReturnsTypedError(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		groupID := int64(51600)
		accounts := []Account{
			capabilityRoutingAccount(51601, groupID, AccountTypeOAuth, 0, codexCapabilitiesStandardAccountManifest),
			capabilityRoutingAccount(51602, groupID, AccountTypeOAuth, 1, ""),
		}
		svc := newCapabilityRoutingService(accounts, nil, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, capabilityRoutingBlue)
		require.Nil(t, selection)
		var capErr *OpenAICapabilityUnavailableError
		require.ErrorAs(t, err, &capErr)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Contains(t, err.Error(), "daybreak_blue")
	})
}

// 具备能力的账号处于账号级限流（不在候选集合，但在持久池）→ 既有临时不可用分类，
// 且过滤统计带出能力原因。
func TestCapabilityRouting_RateLimitedCapableAccountKeepsExistingClassification(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		groupID := int64(51700)
		standard := capabilityRoutingAccount(51701, groupID, AccountTypeOAuth, 0, codexCapabilitiesStandardAccountManifest)
		rateLimitedBlue := capabilityRoutingAccount(51702, groupID, AccountTypeOAuth, 0, codexCapabilitiesBlueAccountManifest)
		svc := newCapabilityRoutingService([]Account{standard}, []Account{standard, rateLimitedBlue}, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, capabilityRoutingUltrafast)
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		var capErr *OpenAICapabilityUnavailableError
		require.False(t, errors.As(err, &capErr), "池内存在具备能力账号时沿用既有分类")
		require.Contains(t, err.Error(), "service_tier_not_supported:ultrafast=1", "过滤统计出现带能力名的原因")
	})
}

// failover 排除了唯一的能力账号 → 既有分类（它仍在候选集合中）。
func TestCapabilityRouting_FailoverExcludedCapableAccountKeepsExistingClassification(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		groupID := int64(51800)
		accounts := []Account{
			capabilityRoutingAccount(51801, groupID, AccountTypeOAuth, 0, codexCapabilitiesBlueAccountManifest),
			capabilityRoutingAccount(51802, groupID, AccountTypeOAuth, 0, codexCapabilitiesStandardAccountManifest),
		}
		svc := newCapabilityRoutingService(accounts, nil, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", map[int64]struct{}{51801: {}}, capabilityRoutingBlue)
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		var capErr *OpenAICapabilityUnavailableError
		require.False(t, errors.As(err, &capErr))
	})
}

// 唯一满足需求的 API Key 账号被限流、池内另有无能力 OAuth 账号 → 既有分类。
func TestCapabilityRouting_RateLimitedAPIKeyKeepsExistingClassification(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		groupID := int64(51900)
		unprobed := capabilityRoutingAccount(51901, groupID, AccountTypeOAuth, 0, "")
		rateLimitedAPIKey := capabilityRoutingAccount(51902, groupID, AccountTypeAPIKey, 0, "")
		svc := newCapabilityRoutingService([]Account{unprobed}, []Account{unprobed, rateLimitedAPIKey}, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, capabilityRoutingFast)
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		var capErr *OpenAICapabilityUnavailableError
		require.False(t, errors.As(err, &capErr))
	})
}

// Daybreak 与 Ultrafast 两个需求必须由同一账号满足。
func TestCapabilityRouting_BothRequirementsMustBeSatisfiedBySameAccount(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		groupID := int64(52000)
		blueOnly := capabilityRoutingAccount(52001, groupID, AccountTypeOAuth, 0, capabilityRoutingBlueOnlyManifest)
		ultrafastOnly := capabilityRoutingAccount(52002, groupID, AccountTypeOAuth, 0, capabilityRoutingUltrafastOnlyManifest)
		both := OpenAIAccountRequirement{CyberProgram: "daybreak_blue", ServiceTier: "ultrafast"}

		svc := newCapabilityRoutingService([]Account{blueOnly, ultrafastOnly}, nil, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, both)
		require.Nil(t, selection)
		var capErr *OpenAICapabilityUnavailableError
		require.ErrorAs(t, err, &capErr)
		require.Equal(t, both, capErr.Requirement)

		full := capabilityRoutingAccount(52003, groupID, AccountTypeOAuth, 9, codexCapabilitiesBlueAccountManifest)
		svc = newCapabilityRoutingService([]Account{blueOnly, ultrafastOnly, full}, nil, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err = selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, both)
		require.NoError(t, err)
		require.Equal(t, int64(52003), selection.Account.ID)
	})
}

// 候选快照里的能力账号不服务请求模型时，不能把判定拉回临时不可用分类：
// 判定语义与候选过滤完全一致（模型支持 + 能力）。
func TestCapabilityRouting_CapableCandidateWithoutModelSupportStillReturnsTypedError(t *testing.T) {
	runCapabilityRoutingModes(t, func(t *testing.T, loadBatch bool) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		groupID := int64(52200)
		blueOtherModel := capabilityRoutingAccount(52201, groupID, AccountTypeOAuth, 0, codexCapabilitiesBlueAccountManifest)
		blueOtherModel.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-6.1-sol": "gpt-6.1-sol"}}
		standard := capabilityRoutingAccount(52202, groupID, AccountTypeOAuth, 0, codexCapabilitiesStandardAccountManifest)
		svc := newCapabilityRoutingService([]Account{blueOtherModel, standard}, nil, loadBatch, schedulerTestConcurrencyCache{})
		selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, capabilityRoutingBlue)
		require.Nil(t, selection)
		var capErr *OpenAICapabilityUnavailableError
		require.ErrorAs(t, err, &capErr, "Blue 账号不服务 gpt-6-sol，等价于没有能力账号")
	})
}

// 持久池为空保持既有分类，避免把「分组没有账号」误报为能力缺失。
func TestCapabilityRouting_EmptyPersistentPoolKeepsExistingClassification(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	groupID := int64(52100)
	unprobed := capabilityRoutingAccount(52101, groupID, AccountTypeOAuth, 0, "")
	svc := newCapabilityRoutingService([]Account{unprobed}, []Account{}, true, schedulerTestConcurrencyCache{})
	selection, _, err := selectWithRequirement(t, svc, context.Background(), groupID, "", "", nil, capabilityRoutingBlue)
	require.Nil(t, selection)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	var capErr *OpenAICapabilityUnavailableError
	require.False(t, errors.As(err, &capErr))
}
