package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCreateAccountPreservesSuppliedUserAgent 覆盖账号创建流程不主动生成/篡改
// user_agent：管理员显式提供的值原样落库，未提供则该键不存在（不写入任何默认值，
// 出站阶段回退全局设置 openai_codex_user_agent 或内置常量）。
func TestCreateAccountPreservesSuppliedUserAgent(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{}
	svc := &adminServiceImpl{accountRepo: repo}

	const adminUA = "codex-tui/0.1.0 (Test OS; test) test-term"
	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "codex-oauth-manual-ua",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		SkipDefaultGroupBind: true,
		Credentials:          map[string]any{"user_agent": adminUA},
	})
	require.NoError(t, err)
	require.Equal(t, adminUA, created.GetOpenAIUserAgent())

	withoutUA, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "codex-oauth-no-ua",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		SkipDefaultGroupBind: true,
	})
	require.NoError(t, err)
	require.Empty(t, withoutUA.GetOpenAIUserAgent(),
		"未手填 user_agent 时创建流程不应生成任何默认值，出站阶段统一走全局设置")
}

// TestUpdateAccountUserAgentOverrideAndClear 覆盖账号编辑界面的自定义 User-Agent 覆盖：
// 提交 user_agent 会覆盖现有值；再次提交不含该键、但含其它非敏感字段的完整 credentials
// 会让该键被移除（MergePreservingSensitiveCreds 对非敏感键"完全由 incoming 决定"的既有语义）。
//
// 注意：`UpdateAccount` 对 `len(input.Credentials) == 0` 直接跳过整个凭据合并分支
// （admin_account.go 的 `else if len(input.Credentials) > 0` 守卫），提交空对象不会清空任何
// 凭据。真实账号编辑场景下 credentials 里通常还有 `chatgpt_account_id` 等非敏感字段，所以
// "清空 = 提交不含该键但非空的 credentials"这个约定在实践中成立；这里用一个占位非敏感字段
// 模拟这种真实情况，而不是提交空对象。
func TestUpdateAccountUserAgentOverrideAndClear(t *testing.T) {
	accountID := int64(301)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:          accountID,
			Name:        "codex-oauth-edit",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Credentials: map[string]any{"chatgpt_account_id": "acct-123"},
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	const manualUA = "codex-tui/0.2.0 (Windows 11; x86_64) conhost"
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials: map[string]any{"chatgpt_account_id": "acct-123", "user_agent": manualUA},
	})
	require.NoError(t, err)
	require.Equal(t, manualUA, updated.GetOpenAIUserAgent())

	cleared, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials: map[string]any{"chatgpt_account_id": "acct-123"},
	})
	require.NoError(t, err)
	require.Empty(t, cleared.GetOpenAIUserAgent(),
		"提交不含 user_agent 键、但含其它非敏感字段的完整 credentials 应让该键被移除，回退到未手填状态")
}
