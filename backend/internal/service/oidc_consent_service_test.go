package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOidcConsent_LoadGrantedScopes_NotFound(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewOidcConsentService(client)

	scopes, found, err := svc.LoadGrantedScopes(context.Background(), 1, "rp_x")
	require.NoError(t, err)
	require.False(t, found)
	require.Nil(t, scopes)
}

func TestOidcConsent_Grant_InsertsRow(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewOidcConsentService(client)

	require.NoError(t, svc.Grant(context.Background(), 42, "rp_alpha", []string{"openid", "profile"}))

	scopes, found, err := svc.LoadGrantedScopes(context.Background(), 42, "rp_alpha")
	require.NoError(t, err)
	require.True(t, found)
	require.ElementsMatch(t, []string{"openid", "profile"}, scopes)
}

func TestOidcConsent_Grant_UnionsExistingScopes(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewOidcConsentService(client)

	require.NoError(t, svc.Grant(context.Background(), 42, "rp_alpha", []string{"openid", "profile"}))
	require.NoError(t, svc.Grant(context.Background(), 42, "rp_alpha", []string{"profile", "email", "offline_access"}))

	scopes, found, err := svc.LoadGrantedScopes(context.Background(), 42, "rp_alpha")
	require.NoError(t, err)
	require.True(t, found)
	require.ElementsMatch(t, []string{"openid", "profile", "email", "offline_access"}, scopes)
}

func TestOidcConsent_Grant_DedupeSameRequest(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewOidcConsentService(client)

	require.NoError(t, svc.Grant(context.Background(), 7, "rp_dup", []string{"openid", "openid", "profile"}))
	scopes, _, err := svc.LoadGrantedScopes(context.Background(), 7, "rp_dup")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"openid", "profile"}, scopes)
}

func TestOidcConsent_Revoke(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewOidcConsentService(client)

	require.NoError(t, svc.Grant(context.Background(), 5, "rp_zeta", []string{"openid"}))
	require.NoError(t, svc.Revoke(context.Background(), 5, "rp_zeta"))

	_, found, err := svc.LoadGrantedScopes(context.Background(), 5, "rp_zeta")
	require.NoError(t, err)
	require.False(t, found)

	// 二次 Revoke → ErrOidcConsentNotFound
	err = svc.Revoke(context.Background(), 5, "rp_zeta")
	require.True(t, errors.Is(err, ErrOidcConsentNotFound))
}

func TestOidcConsent_TouchLastUsed(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := NewOidcConsentService(client)

	require.NoError(t, svc.Grant(context.Background(), 9, "rp_touch", []string{"openid"}))
	require.NoError(t, svc.TouchLastUsed(context.Background(), 9, "rp_touch"))

	// 不存在的行
	err := svc.TouchLastUsed(context.Background(), 9, "rp_missing")
	require.True(t, errors.Is(err, ErrOidcConsentNotFound))
}

func TestOidcConsent_IsCovered(t *testing.T) {
	svc := NewOidcConsentService(nil)

	// 空 requested 永远 true
	require.True(t, svc.IsCovered([]string{}, []string{}))
	require.True(t, svc.IsCovered([]string{"openid"}, nil))

	// granted 完全覆盖 requested
	require.True(t, svc.IsCovered([]string{"openid", "profile", "email"}, []string{"profile"}))
	require.True(t, svc.IsCovered([]string{"openid", "profile"}, []string{"openid", "profile"}))

	// 缺一项 → false
	require.False(t, svc.IsCovered([]string{"openid"}, []string{"openid", "email"}))
	require.False(t, svc.IsCovered(nil, []string{"openid"}))

	// 顺序、重复鲁棒
	require.True(t, svc.IsCovered([]string{"a", "b", "a"}, []string{"b", "b"}))
}

func TestUnionStrings_PreservesOrderAndDedupes(t *testing.T) {
	got := unionStrings([]string{"a", "b"}, []string{"b", "c", "a"})
	require.Equal(t, []string{"a", "b", "c"}, got)
}

func TestUnionStrings_EmptyInputs(t *testing.T) {
	require.Equal(t, []string{}, unionStrings(nil, nil))
	require.Equal(t, []string{"x"}, unionStrings(nil, []string{"x"}))
	require.Equal(t, []string{"x"}, unionStrings([]string{"x"}, nil))
}
