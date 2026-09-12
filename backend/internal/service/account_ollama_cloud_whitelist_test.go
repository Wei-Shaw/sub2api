package service

// ollama_cloud 运行时白名单回归测试（钉死独立评审的探针场景）：
//   - 空 model_mapping + extra.allowed_models 清单 → IsModelSupported 只放行
//     清单内模型，清单外（含未定价模型名）一律拒绝、调度不选中；
//   - 空 mapping 且无清单 → deny-all；
//   - 非 ollama 平台空 mapping 仍放行所有（回归护栏）；
//   - /models 列表路径（GetAvailableModels）并入 allowed_models 清单。
//
// 与保存期门禁（account_ollama_cloud_pricing_gate_test.go）合起来形成闭环：
// 白名单决定能发什么，预检决定发出去的东西是否有价。

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func newOllamaCloudWhitelistAccount(credentials map[string]any, extra map[string]any) *Account {
	return &Account{
		ID:          9101,
		Platform:    PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: credentials,
		Extra:       extra,
	}
}

// 评审探针场景：空 mapping + 清单只含 gpt-oss:120b → totally-unpriced-model
// 必须不被支持、不被调度选中（评审当前观测到「仍 supported」）。
func TestOllamaCloudWhitelist_ProbeUnpricedModelNotSupported(t *testing.T) {
	t.Parallel()

	// Extra 走 JSON 反序列化的真实形态（[]any 元素为 string）。
	account := newOllamaCloudWhitelistAccount(
		map[string]any{},
		map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b"}},
	)

	require.False(t, account.IsModelSupported("totally-unpriced-model"),
		"清单外未定价模型名不得放行（评审探针场景）")
	require.False(t, account.IsModelSupported("claude-sonnet-4-6"),
		"其它平台默认模型名同样在清单外")
	require.True(t, account.IsModelSupported("gpt-oss:120b"), "清单内模型必须放行")

	// 程序化构造形态（[]string）语义一致。
	stringListAccount := newOllamaCloudWhitelistAccount(
		nil,
		map[string]any{OllamaCloudAllowedModelsExtraKey: []string{"gpt-oss:120b"}},
	)
	require.False(t, stringListAccount.IsModelSupported("totally-unpriced-model"))
	require.True(t, stringListAccount.IsModelSupported("gpt-oss:120b"))

	// 调度候选过滤同样拒绝：不被选中。
	ctx := context.Background()
	reason := openAICompatibleAccountEligibilityFailureReasonBeforeProfit(
		ctx, account, PlatformOllamaCloud, "totally-unpriced-model", false, OpenAIEndpointCapabilityChatCompletions)
	require.Equal(t, "model_not_supported", reason)
	reason = openAICompatibleAccountEligibilityFailureReasonBeforeProfit(
		ctx, account, PlatformOllamaCloud, "gpt-oss:120b", false, OpenAIEndpointCapabilityChatCompletions)
	require.Empty(t, reason, "清单内模型必须保持可选中")
}

func TestOllamaCloudWhitelist_EmptyMappingNoListDeniesAll(t *testing.T) {
	t.Parallel()

	account := newOllamaCloudWhitelistAccount(map[string]any{}, map[string]any{})
	for _, model := range []string{"gpt-oss:120b", "totally-unpriced-model", "claude-sonnet-4-6", ""} {
		require.False(t, account.IsModelSupported(model),
			"空 mapping 且无清单必须 deny-all: %q", model)
	}

	// 清单存在但全为空白项：视同未提供，仍然 deny-all（与保存期门禁的
	// 「清单缺失即拒绝」语义一致）。
	blankAccount := newOllamaCloudWhitelistAccount(
		nil,
		map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"  ", ""}},
	)
	require.False(t, blankAccount.IsModelSupported("gpt-oss:120b"))
}

func TestOllamaCloudWhitelist_NonEmptyMappingKeepsMappingSemantics(t *testing.T) {
	t.Parallel()

	account := newOllamaCloudWhitelistAccount(
		map[string]any{"model_mapping": map[string]any{"alias:latest": "gpt-oss:120b"}},
		nil,
	)
	require.True(t, account.IsModelSupported("alias:latest"), "mapping 键即公开白名单")
	require.False(t, account.IsModelSupported("totally-unpriced-model"))
	// 既有语义：非 ollama 也一样——mapping 目标值不是公开名，不因本次改动改变。
	require.False(t, account.IsModelSupported("gpt-oss:120b"), "mapping 目标值不是公开名")
}

// 回归护栏：非 ollama 平台的空 mapping 仍放行所有（钉住 4 处风险面语义）。
// 不含 antigravity / grok：这两个平台空 mapping 的既有语义是注入平台默认映射
// （resolveModelMapping），从来不是「放行所有」，与本次改动无关。
func TestOllamaCloudWhitelist_NonOllamaEmptyMappingStillAllowsAll(t *testing.T) {
	t.Parallel()

	for _, platform := range []string{
		"", PlatformAnthropic, PlatformOpenAI, PlatformGemini,
		PlatformDeepseek, PlatformKimi, PlatformMiniMax, PlatformZhipu, PlatformOpenCodeGo,
	} {
		account := &Account{ID: 1, Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{}}
		require.Truef(t, account.IsModelSupported("totally-unpriced-model"),
			"平台 %s 空 mapping 必须仍放行所有模型", platform)
	}
}

// /models 列表路径：GetAvailableModels 并入 ollama_cloud 的 allowed_models 清单。
func TestGetAvailableModels_MergesOllamaCloudAllowedModels(t *testing.T) {
	t.Parallel()

	groupID := int64(9102)
	repo := &modelsListAccountRepoStub{
		byGroup: map[int64][]Account{
			groupID: {
				{
					ID:          1,
					Platform:    PlatformOllamaCloud,
					Credentials: map[string]any{},
					Extra:       map[string]any{OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b", "qwen3:32b"}},
				},
				{
					ID:          2,
					Platform:    PlatformOllamaCloud,
					Credentials: map[string]any{"model_mapping": map[string]any{"alias:latest": "gpt-oss:120b"}},
				},
				{
					ID:          3,
					Platform:    PlatformOllamaCloud,
					Credentials: map[string]any{},
				},
			},
		},
	}
	svc := &GatewayService{accountRepo: repo}

	models := svc.GetAvailableModels(context.Background(), &groupID, PlatformOllamaCloud)
	require.Equal(t, []string{"alias:latest", "gpt-oss:120b", "qwen3:32b"}, models)

	// 全部账号都是 deny-all（无 mapping 无清单）→ 无可用模型列表。
	denyAllGroup := int64(9103)
	repo.byGroup[denyAllGroup] = []Account{
		{ID: 4, Platform: PlatformOllamaCloud, Credentials: map[string]any{}},
	}
	require.Nil(t, svc.GetAvailableModels(context.Background(), &denyAllGroup, PlatformOllamaCloud))

	// 非 ollama 平台的既有语义不变：无任何 mapping → 仍返回 nil（用默认列表）。
	nonOllamaGroup := int64(9104)
	repo.byGroup[nonOllamaGroup] = []Account{
		{ID: 5, Platform: PlatformDeepseek, Credentials: map[string]any{}},
	}
	require.Nil(t, svc.GetAvailableModels(context.Background(), &nonOllamaGroup, PlatformDeepseek))
}
