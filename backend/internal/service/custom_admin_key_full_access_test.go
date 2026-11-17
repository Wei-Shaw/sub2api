//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fullAccessInput(t *testing.T, confirm bool) ScopedAdminKeyInput {
	t.Helper()
	var input ScopedAdminKeyInput
	data := `{"name":"兼容工具","enabled":true,"policy":{"full_access":true},"confirm_full_access":false}`
	if confirm {
		data = `{"name":"兼容工具","enabled":true,"policy":{"full_access":true},"confirm_full_access":true}`
	}
	require.NoError(t, json.Unmarshal([]byte(data), &input))
	return input
}

func TestAdminFullAccessRequiresExplicitConfirmation(t *testing.T) {
	s := &SettingService{scopedAdminKeyRepo: &scopedKeyMemoryRepo{}}
	ctx := context.Background()
	_, _, err := s.CreateScopedAdminKey(ctx, fullAccessInput(t, false), 9)
	require.Error(t, err, "新建全权密钥必须显式确认")
	_, item, err := s.CreateScopedAdminKey(ctx, ScopedAdminKeyInput{Name: "受限", Enabled: true}, 9)
	require.NoError(t, err)
	_, err = s.UpdateScopedAdminKey(ctx, item.ID, fullAccessInput(t, false))
	require.Error(t, err, "受限密钥升级必须显式确认")
}

func TestAdminFullAccessConfirmationIsNotPersisted(t *testing.T) {
	repo := &scopedKeyMemoryRepo{}
	s := &SettingService{scopedAdminKeyRepo: repo}
	ctx := context.Background()
	key, item, err := s.CreateScopedAdminKey(ctx, fullAccessInput(t, true), 9)
	require.NoError(t, err)
	raw, err := json.Marshal(item)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"full_access":true`)
	require.NotContains(t, string(raw), `"confirm_full_access":true`)
	_, err = s.UpdateScopedAdminKey(ctx, item.ID, fullAccessInput(t, false))
	require.NoError(t, err, "不扩权的编辑不需要再次确认")
	restricted := ScopedAdminKeyInput{Name: "关闭兼容", Enabled: true}
	_, err = s.UpdateScopedAdminKey(ctx, item.ID, restricted)
	require.NoError(t, err)
	_, err = s.UpdateScopedAdminKey(ctx, item.ID, fullAccessInput(t, false))
	require.Error(t, err, "再次开启仍需确认")
	authenticated, err := s.AuthenticateScopedAdminKey(ctx, key, "192.0.2.1")
	require.NoError(t, err)
	raw, err = json.Marshal(authenticated.Policy)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"full_access":true`)
}

func TestAdminFullAccessLifecycleRestrictions(t *testing.T) {
	repo := &scopedKeyMemoryRepo{}
	s := &SettingService{scopedAdminKeyRepo: repo}
	ctx := context.Background()
	in := fullAccessInput(t, true)
	in.AllowedIPs = []string{"192.0.2.0/24"}
	key, item, err := s.CreateScopedAdminKey(ctx, in, 9)
	require.NoError(t, err)
	_, err = s.AuthenticateScopedAdminKey(ctx, key, "198.51.100.1")
	require.Error(t, err)
	next, _, err := s.RotateScopedAdminKey(ctx, item.ID)
	require.NoError(t, err)
	_, err = s.AuthenticateScopedAdminKey(ctx, key, "192.0.2.1")
	require.Error(t, err)
	_, err = s.AuthenticateScopedAdminKey(ctx, next, "192.0.2.1")
	require.NoError(t, err)
	repo.item.Enabled = false
	_, err = s.AuthenticateScopedAdminKey(ctx, next, "192.0.2.1")
	require.Error(t, err)
	repo.item.Enabled = true
	past := time.Now().Add(-time.Hour)
	repo.item.ExpiresAt = &past
	_, err = s.AuthenticateScopedAdminKey(ctx, next, "192.0.2.1")
	require.Error(t, err)
	repo.item.ExpiresAt = nil
	require.NoError(t, s.RevokeScopedAdminKey(ctx, item.ID))
	_, err = s.AuthenticateScopedAdminKey(ctx, next, "192.0.2.1")
	require.Error(t, err)
}
