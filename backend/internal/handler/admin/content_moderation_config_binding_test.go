package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// 风控配置的每个字段都必须在 contentModerationConfigRequest 上有对应项，否则前端发来的
// 值会在 JSON 绑定阶段被静默丢弃——配置项在 UI 上看着能存，实际永远不生效。
// cyber_policy_exclude_from_ban_count 已经这样坏过一次，分块相关字段坏过第二次，
// 因此用一个覆盖全部可选字段的绑定测试钉住。
func TestContentModerationConfigRequest_BindsChunkAndCaptureFields(t *testing.T) {
	// 与 RiskControlView.vue 提交的 payload 保持一致的字段名。
	payload := `{
		"cyber_policy_exclude_from_ban_count": true,
		"cyber_policy_capture_request": true,
		"chunk_moderation_enabled": true,
		"chunk_tokens": 2500,
		"chunk_concurrency": 4,
		"chunk_max_chunks": 6
	}`

	var req contentModerationConfigRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &req))

	require.NotNil(t, req.CyberPolicyExcludeFromBanCount)
	require.True(t, *req.CyberPolicyExcludeFromBanCount)

	require.NotNil(t, req.CyberPolicyCaptureRequest, "留存请求体开关必须能透传到 service")
	require.True(t, *req.CyberPolicyCaptureRequest)

	require.NotNil(t, req.ChunkModerationEnabled, "分块审核开关必须能透传到 service")
	require.True(t, *req.ChunkModerationEnabled)

	require.NotNil(t, req.ChunkTokens)
	require.Equal(t, 2500, *req.ChunkTokens)

	require.NotNil(t, req.ChunkConcurrency)
	require.Equal(t, 4, *req.ChunkConcurrency)

	require.NotNil(t, req.ChunkMaxChunks)
	require.Equal(t, 6, *req.ChunkMaxChunks)
}

// 未提供的字段必须保持 nil：service 侧靠指针判空来区分「不修改」与「设为零值」。
func TestContentModerationConfigRequest_OmittedFieldsStayNil(t *testing.T) {
	var req contentModerationConfigRequest
	require.NoError(t, json.Unmarshal([]byte(`{}`), &req))

	require.Nil(t, req.CyberPolicyCaptureRequest)
	require.Nil(t, req.ChunkModerationEnabled)
	require.Nil(t, req.ChunkTokens)
	require.Nil(t, req.ChunkConcurrency)
	require.Nil(t, req.ChunkMaxChunks)
}
