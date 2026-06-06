package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ensureCodexReasoningInclude：带 reasoning 时补齐 include，幂等且保留既有项。
func TestEnsureCodexReasoningInclude(t *testing.T) {
	// reasoning 存在、include 缺失 → 注入
	body := map[string]any{"reasoning": map[string]any{"effort": "medium"}}
	require.True(t, ensureCodexReasoningInclude(body))
	require.Equal(t, []any{"reasoning.encrypted_content"}, body["include"])
	// 幂等：再次调用不重复
	require.False(t, ensureCodexReasoningInclude(body))

	// 无 reasoning → 不动
	body2 := map[string]any{}
	require.False(t, ensureCodexReasoningInclude(body2))
	_, ok := body2["include"]
	require.False(t, ok)

	// 既有 include 保留并追加
	body3 := map[string]any{
		"reasoning": map[string]any{"effort": "high"},
		"include":   []any{"foo"},
	}
	require.True(t, ensureCodexReasoningInclude(body3))
	require.Equal(t, []any{"foo", "reasoning.encrypted_content"}, body3["include"])
}

// applyCodexClientMetadata：用账号真实 device_id 注入 installation 标识，幂等、不覆盖既有项、不伪造。
func TestApplyCodexClientMetadata(t *testing.T) {
	// 仅 OpenAI OAuth 账号才有 device_id（GetOpenAIDeviceID 的门控）。
	acc := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_device_id": "dev-xyz"}}

	body := map[string]any{}
	require.True(t, applyCodexClientMetadata(body, acc))
	cm, ok := body["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dev-xyz", cm["x-codex-installation-id"])
	// 幂等
	require.False(t, applyCodexClientMetadata(body, acc))

	// OAuth 账号但无 device_id → 不写入（不伪造）
	body2 := map[string]any{}
	require.False(t, applyCodexClientMetadata(body2, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
	_, ok = body2["client_metadata"]
	require.False(t, ok)

	// 既有 client_metadata（如 turn metadata）保留，仅补 installation 键
	body3 := map[string]any{"client_metadata": map[string]any{"x-codex-turn-metadata": "t"}}
	require.True(t, applyCodexClientMetadata(body3, acc))
	cm3, _ := body3["client_metadata"].(map[string]any)
	require.Equal(t, "t", cm3["x-codex-turn-metadata"])
	require.Equal(t, "dev-xyz", cm3["x-codex-installation-id"])
}

// defaultCodexSynthInstructions：按模型选用真实 Codex base prompt。
func TestDefaultCodexSynthInstructionsModelAware(t *testing.T) {
	require.True(t, strings.Contains(defaultCodexSynthInstructions("gpt-5-codex"), "You are Codex, based on GPT-5"))
	require.True(t, strings.Contains(defaultCodexSynthInstructions("gpt-5.2"), "You are GPT-5.2 running in the Codex CLI"))
	require.True(t, strings.Contains(defaultCodexSynthInstructions("gpt-5.1"), "You are GPT-5.1 running in the Codex CLI"))
}
