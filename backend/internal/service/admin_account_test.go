package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCreateAccountAssignsFingerprintPoolUserAgentWhenUnset 覆盖 spec User Story 1：
// 新建 OpenAI OAuth 账号且未手填 User-Agent 时，创建流程必须从内置指纹候选池按账号 ID
// 分配一个值并落库，取代此前"完全不写这个键、请求时才回退全局常量"的行为。
func TestCreateAccountAssignsFingerprintPoolUserAgentWhenUnset(t *testing.T) {
	repo := &upstreamBillingProbeAccountRepo{}
	svc := &adminServiceImpl{accountRepo: repo}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "codex-oauth-no-ua",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		SkipDefaultGroupBind: true,
	})
	require.NoError(t, err)

	ua := created.GetOpenAIUserAgent()
	require.NotEmpty(t, ua, "未手填 UA 的 OpenAI OAuth 账号创建后应被指纹池分配一个值")

	candidate, ok := selectCodexFingerprint(codexFingerprintPool, created.ID)
	require.True(t, ok)
	require.Equal(t, buildCodexFingerprintUserAgent(candidate), ua,
		"分配结果应等于按该账号 ID 从候选池选出的候选项拼出的 UA，保证可复现、可稳定")
}

// TestCreateAccountPreservesAdminSuppliedUserAgent 覆盖 spec User Story 1 的验收标准：
// 管理员手填的 user_agent 优先级高于指纹池自动分配，创建流程不得覆盖它。
func TestCreateAccountPreservesAdminSuppliedUserAgent(t *testing.T) {
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
	require.Equal(t, adminUA, created.GetOpenAIUserAgent(),
		"管理员手填的 user_agent 不应被指纹池分配结果覆盖")
}

// TestUpdateAccountUserAgentOverrideAndClear 覆盖 spec User Story 2 的验收标准：编辑账号时
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

// TestUpdateAccountDoesNotBackfillFingerprintPoolForExistingAccount 覆盖分析报告 C2、
// spec FR-007：编辑一个本功能上线前创建的存量账号（Credentials 里没有 user_agent），
// 只改动无关字段时，不应被意外补上指纹池分配结果——指纹池分配只发生在 CreateAccount。
func TestUpdateAccountDoesNotBackfillFingerprintPoolForExistingAccount(t *testing.T) {
	accountID := int64(302)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:          accountID,
			Name:        "legacy-codex-oauth",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Credentials: map[string]any{},
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Name: "legacy-codex-oauth-renamed",
	})
	require.NoError(t, err)
	require.Empty(t, updated.GetOpenAIUserAgent(),
		"编辑存量账号的无关字段不应触发指纹池分配")
}
