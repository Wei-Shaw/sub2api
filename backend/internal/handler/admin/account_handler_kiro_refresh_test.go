//go:build unit

package admin

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestRefreshSingleAccountRejectsKiroInsteadOfMisusingAnthropicOAuth 是
// Kiro Type 语义修正（跟 Antigravity 对齐，social/builder_id/idc 都是真
// OAuth）的必要配套：Type 变准确后，Kiro OAuth 账号会通过上面的
// account.IsOAuth() 门槛，但下面这条 if/else 链条是照 Anthropic 的凭证
// 形状写的——如果没有这条显式分支，Kiro 账号会落进最后的 else，错误地拿
// h.oauthService（Anthropic）去刷新 Kiro 的 token。这里必须显式拒绝，
// 而不是让它悄悄流进错误的分支。
func TestRefreshSingleAccountRejectsKiroInsteadOfMisusingAnthropicOAuth(t *testing.T) {
	t.Parallel()

	adminSvc := &grokRefreshAdminService{stubAdminService: newStubAdminService()}
	handler := NewAccountHandler(
		adminSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	account := &service.Account{
		ID:       9001,
		Platform: service.PlatformKiro,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_method":   "idc",
			"access_token":  "at",
			"refresh_token": "rt",
		},
	}

	updated, warning, err := handler.refreshSingleAccount(context.Background(), account)
	require.Error(t, err)
	require.Nil(t, updated)
	require.Empty(t, warning)
	appErr, ok := err.(*infraerrors.ApplicationError)
	require.True(t, ok, "must be an ApplicationError so the API returns a structured reason code")
	require.Equal(t, "KIRO_MANUAL_REFRESH_UNSUPPORTED", appErr.Reason)
	require.Equal(t, 400, int(appErr.Code))
	require.Nil(t, adminSvc.updatedCredentials, "不能把 Kiro 凭证交给 UpdateAccount 落库")
}
