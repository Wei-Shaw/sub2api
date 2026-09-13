//go:build unit

package service

// C15/C14/C22：ollama_cloud 默认模型清单的数据源钉测。
//
// 权威数据：GET https://ollama.com/v1/models 实测（2026-09-12，HTTP 200，
// 20 个模型，含 deepseek-v4-flash:0731 / deepseek-v4-pro:0813 两个带 tag 快照名）。
//
// 前端同源静态表：frontend/src/composables/useModelWhitelist.ts 的
// getModelsByPlatform 'ollama_cloud' case 必须与 DefaultOllamaCloudModelIDs()
// 保持同一集合，两侧各有测试钉住（对侧：useModelWhitelist.spec.ts），
// 改动任一侧须同步另一侧。

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultOllamaCloudModelIDs 钉住目录的形状：数量、去重、无 claude 串入、
// 带 tag 快照名与各家族代表条目不得缺失。
func TestDefaultOllamaCloudModelIDs(t *testing.T) {
	ids := DefaultOllamaCloudModelIDs()

	require.Len(t, ids, 20)

	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		require.NotEmpty(t, id)
		require.NotContains(t, seen, id, "duplicate model id %q", id)
		seen[id] = struct{}{}
		require.NotContains(t, id, "claude", "claude 模型不得串入 ollama_cloud 目录: %q", id)
	}

	for _, key := range []string{
		"deepseek-v4-flash:0731",
		"deepseek-v4-pro:0813",
		"gpt-oss:120b",
		"gpt-oss:20b",
		"glm-5.3",
		"kimi-k3",
		"minimax-m3",
		"qwen3.5:397b",
	} {
		require.Contains(t, ids, key)
	}
}

// C15 验收：defaultModelsListCandidateIDs 的 ollama case 返回目录本身；
// C14/C22 验收：composite 候选包含全部 ollama 条目。
func TestDefaultModelsListCandidateIDs_OllamaCloud(t *testing.T) {
	ollama := defaultModelsListCandidateIDs(PlatformOllamaCloud)
	require.Equal(t, DefaultOllamaCloudModelIDs(), ollama)

	composite := defaultModelsListCandidateIDs(PlatformComposite)
	for _, id := range DefaultOllamaCloudModelIDs() {
		require.Contains(t, composite, id)
	}
}

// B2-② 联动断言：目录内每个模型都必须可计费（两级查价非 nil 且单价为正），
// 防止清单与价目表漂移后出现「默认列表 ∩ 定价表 = 空」与保存期门禁打架。
func TestDefaultOllamaCloudModelIDsAllPriceable(t *testing.T) {
	bs := newTestBillingService()
	for _, id := range DefaultOllamaCloudModelIDs() {
		pricing, err := bs.getModelPricingAt(id, ollamaPricingAt)
		require.NoError(t, err, "model %s", id)
		require.NotNil(t, pricing, "model %s must be priceable", id)
		require.Positive(t, pricing.InputPricePerToken, "model %s", id)
		require.Positive(t, pricing.OutputPricePerToken, "model %s", id)
	}
}
