//go:build unit

package repository

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// ollama_cloud 空 mapping 的出站白名单（extra.allowed_models）必须穿过调度投影：
// 候选过滤的 Account.IsModelSupported 以它为白名单（无清单即 deny-all）。裁掉它，
// 白名单账号会在选号阶段被整体误判为 model_not_supported（#4936 同款投影缺失）。
func TestFilterSchedulerExtra_KeepsOllamaCloudAllowedModels(t *testing.T) {
	extra := map[string]any{
		service.OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b", "qwen3:32b"},
		"unknown_key":                            "must-be-dropped",
	}

	filtered := filterSchedulerExtra(extra)
	require.Len(t, filtered, 1)

	kept, ok := filtered[service.OllamaCloudAllowedModelsExtraKey]
	require.True(t, ok, "allowed_models 必须保留在调度投影中")
	require.Contains(t, kept, "gpt-oss:120b")

	// 走真实的序列化/反序列化路径：sched:meta 里的 []string 会以 []any 读回，
	// ollamaCloudOutboundModelNames 两种形态都必须能解析。
	payload, err := json.Marshal(filtered)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))

	account := service.Account{ID: 1, Platform: service.PlatformOllamaCloud, Extra: decoded}
	require.True(t, account.IsModelSupported("gpt-oss:120b"))
	require.False(t, account.IsModelSupported("totally-unpriced-model"))
}
