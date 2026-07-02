package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResolveTLSBindingAccount 验证 spark 影子账号在数据面解析 TLS 路由器/指纹时的账号选择:
// 影子无自绑 → 回落母账号(与 quota 探测路径同源,消除同 token 双 JA3);影子显式自绑 → 尊重其覆盖;
// 非影子 / 无 repo / 母账号解析失败 → 原样返回(向后兼容)。
func TestResolveTLSBindingAccount(t *testing.T) {
	ctx := context.Background()

	parentID := int64(100)
	parent := Account{
		ID:       parentID,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Extra:    map[string]any{"tls_fingerprint_router_id": float64(7)},
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{parent}}
	svc := &OpenAIGatewayService{accountRepo: repo}

	t.Run("nil account passes through", func(t *testing.T) {
		require.Nil(t, svc.resolveTLSBindingAccount(ctx, nil))
	})

	t.Run("non-shadow returns itself", func(t *testing.T) {
		acc := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		require.Same(t, acc, svc.resolveTLSBindingAccount(ctx, acc))
	})

	t.Run("shadow without own binding falls back to parent", func(t *testing.T) {
		shadow := &Account{ID: 200, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID}
		got := svc.resolveTLSBindingAccount(ctx, shadow)
		require.Equal(t, parentID, got.ID)
		require.Equal(t, int64(7), got.GetTLSFingerprintRouterID(), "should carry parent's router binding")
	})

	t.Run("shadow with own router binding is respected", func(t *testing.T) {
		shadow := &Account{
			ID: 201, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID,
			Extra: map[string]any{"tls_fingerprint_router_id": float64(9)},
		}
		got := svc.resolveTLSBindingAccount(ctx, shadow)
		require.Equal(t, int64(201), got.ID, "explicit shadow binding must not be overridden by parent")
	})

	t.Run("shadow with own enabled fixed profile is respected", func(t *testing.T) {
		shadow := &Account{
			ID: 202, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID,
			Extra: map[string]any{"enable_tls_fingerprint": true},
		}
		require.True(t, shadow.IsTLSFingerprintEnabled())
		got := svc.resolveTLSBindingAccount(ctx, shadow)
		require.Equal(t, int64(202), got.ID, "shadow's own enabled fingerprint must not be overridden")
	})

	t.Run("no repo falls back to shadow itself", func(t *testing.T) {
		svcNoRepo := &OpenAIGatewayService{}
		shadow := &Account{ID: 203, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID}
		require.Same(t, shadow, svcNoRepo.resolveTLSBindingAccount(ctx, shadow))
	})

	t.Run("unresolvable parent falls back to shadow itself", func(t *testing.T) {
		missingParent := int64(999)
		shadow := &Account{ID: 204, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &missingParent}
		require.Same(t, shadow, svc.resolveTLSBindingAccount(ctx, shadow))
	})
}
