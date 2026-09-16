package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 固定账号 manifest 配置必须在管理端 DTO 完整序列化：漏映射会让编辑对话框回显
// 关闭状态，管理员保存无关字段时会把已开启的配置静默关掉。用户侧 DTO 不携带该字段。
func TestGroupMapperRoundTripsCodexModelsManifestConfig(t *testing.T) {
	group := &service.Group{
		ID: 31, Name: "codex-manifest-dto", Platform: service.PlatformOpenAI, Status: service.StatusActive,
		CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{
			Enabled:             true,
			AccountIDs:          []int64{101, 202},
			FallbackToScheduler: true,
		},
	}

	adminJSON, err := json.Marshal(GroupFromServiceAdmin(group))
	require.NoError(t, err)
	var adminEnvelope struct {
		CodexModelsManifestConfig *struct {
			Enabled             bool    `json:"enabled"`
			AccountIDs          []int64 `json:"account_ids"`
			FallbackToScheduler bool    `json:"fallback_to_scheduler"`
		} `json:"codex_models_manifest_config"`
	}
	require.NoError(t, json.Unmarshal(adminJSON, &adminEnvelope))
	require.NotNil(t, adminEnvelope.CodexModelsManifestConfig, "GroupFromServiceAdmin 漏映射 codex_models_manifest_config")
	require.True(t, adminEnvelope.CodexModelsManifestConfig.Enabled)
	require.Equal(t, []int64{101, 202}, adminEnvelope.CodexModelsManifestConfig.AccountIDs)
	require.True(t, adminEnvelope.CodexModelsManifestConfig.FallbackToScheduler)

	userJSON, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "codex_models_manifest_config", "用户侧分组 DTO 不得暴露管理端 manifest 配置")
}
