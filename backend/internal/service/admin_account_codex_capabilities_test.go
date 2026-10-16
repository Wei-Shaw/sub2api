//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 管理端编辑账号会用前端提交的 extra 整体替换；能力记录是服务端受管键，
// 必须在改名/改并发等普通编辑后仍然存在。
func TestUpdateAccountPreservesCodexCapabilitiesRecord(t *testing.T) {
	caps, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		901: {
			ID: 901, Name: "before", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
			Credentials: map[string]any{"access_token": "tok"},
			Extra:       map[string]any{CodexCapabilitiesExtraKey: caps, "base_rpm": float64(10)},
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	concurrency := 5
	updated, err := svc.UpdateAccount(context.Background(), 901, &UpdateAccountInput{
		Name:        "after",
		Concurrency: &concurrency,
		Extra:       map[string]any{"base_rpm": float64(20)},
	})
	require.NoError(t, err)
	require.Equal(t, "after", updated.Name)
	require.Equal(t, float64(20), updated.Extra["base_rpm"])
	require.True(t, updated.CodexCapabilities().SupportsCyberProgram("daybreak_blue"), "能力记录不得被整体替换抹掉")
}

// 能力记录只能由上游目录探测写入：四条管理端写路径都必须丢弃客户端提交的值，
// 否则伪造记录可以绕过能力路由，并让部署后补齐把该账号当作已探测而跳过。
func TestAdminAccountWritesDiscardClientSuppliedCodexCapabilities(t *testing.T) {
	caps, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)
	forged := map[string]any{"models": map[string]any{"gpt-6-sol": map[string]any{"service_tiers": []any{"ultrafast"}}}, "cyber_programs": []any{"daybreak_red"}}

	t.Run("create", func(t *testing.T) {
		account, err := buildAccountForCreate(&CreateAccountInput{
			Name: "n", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "tok"},
		}, map[string]any{CodexCapabilitiesExtraKey: forged, "base_rpm": float64(10)})
		require.NoError(t, err)
		require.NotContains(t, account.Extra, CodexCapabilitiesExtraKey)
		require.Equal(t, float64(10), account.Extra["base_rpm"])
	})

	t.Run("update replaces forged value with stored record", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			902: {
				ID: 902, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
				Credentials: map[string]any{"access_token": "tok"},
				Extra:       map[string]any{CodexCapabilitiesExtraKey: caps},
			},
		}}
		updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), 902, &UpdateAccountInput{
			Extra: map[string]any{CodexCapabilitiesExtraKey: forged},
		})
		require.NoError(t, err)
		require.True(t, updated.CodexCapabilities().SupportsCyberProgram("daybreak_blue"))
		require.False(t, updated.CodexCapabilities().SupportsCyberProgram("daybreak_red"))
	})

	t.Run("update without stored record stays unprobed", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			903: {ID: 903, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"access_token": "tok"}},
		}}
		updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), 903, &UpdateAccountInput{
			Extra: map[string]any{CodexCapabilitiesExtraKey: forged},
		})
		require.NoError(t, err)
		require.NotContains(t, updated.Extra, CodexCapabilitiesExtraKey)
	})

	t.Run("update extra", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			904: {ID: 904, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		}}
		err := (&adminServiceImpl{accountRepo: repo}).UpdateAccountExtra(context.Background(), 904, map[string]any{
			CodexCapabilitiesExtraKey: forged,
			"custom":                  "value",
		})
		require.NoError(t, err)
		require.Equal(t, "value", repo.accounts[904].Extra["custom"])
		require.NotContains(t, repo.accounts[904].Extra, CodexCapabilitiesExtraKey)
	})

	t.Run("bulk update", func(t *testing.T) {
		repo := &upstreamBillingProbeAccountRepo{}
		result, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
			AccountIDs: []int64{1},
			Extra:      map[string]any{CodexCapabilitiesExtraKey: forged, "custom": "value"},
		})
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
		require.Len(t, repo.bulkUpdates, 1)
		require.Equal(t, "value", repo.bulkUpdates[0].Extra["custom"])
		require.NotContains(t, repo.bulkUpdates[0].Extra, CodexCapabilitiesExtraKey)
	})

	t.Run("duplicate", func(t *testing.T) {
		extra, err := duplicateAccountExtra(map[string]any{CodexCapabilitiesExtraKey: caps, "base_rpm": float64(10)})
		require.NoError(t, err)
		require.NotContains(t, extra, CodexCapabilitiesExtraKey)
		require.Equal(t, float64(10), extra["base_rpm"])
	})
}
