//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRegistrationEmailSuffixWhitelist(t *testing.T) {
	got, err := NormalizeRegistrationEmailSuffixWhitelist([]string{"example.com", "@EXAMPLE.COM", " @foo.bar ", "*.EDU.CN"})
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"}, got)
}

func TestNormalizeRegistrationEmailSuffixWhitelist_Invalid(t *testing.T) {
	for _, item := range []string{"@invalid_domain", "*.", "*", "*.@", "*.foo"} {
		t.Run(item, func(t *testing.T) {
			_, err := NormalizeRegistrationEmailSuffixWhitelist([]string{item})
			require.Error(t, err)
		})
	}
}

func TestParseRegistrationEmailSuffixWhitelist(t *testing.T) {
	got := ParseRegistrationEmailSuffixWhitelist(`["example.com","@foo.bar","*.EDU.CN","@invalid_domain","*.foo"]`)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"}, got)
}

func TestIsRegistrationEmailSuffixAllowed(t *testing.T) {
	require.True(t, IsRegistrationEmailSuffixAllowed("user@example.com", []string{"@example.com"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@example.com.", []string{"@example.com"}))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@sub.example.com", []string{"@example.com"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@qq.com", []string{"@qq.com"}))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@sub.qq.com", []string{"@qq.com"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("student@cs.edu.cn", []string{"*.edu.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("student@edu.cn", []string{"*.edu.cn"}))
	require.False(t, IsRegistrationEmailSuffixAllowed("student@foo.cn", []string{"*.edu.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@a.com", []string{"@a.com", "*.b.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@school.b.cn", []string{"@a.com", "*.b.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@b.cn", []string{"@a.com", "*.b.cn"}))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@c.cn", []string{"@a.com", "*.b.cn"}))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@any.com", []string{}))
}

func TestRegistrationEmailQuotaRejectsMalformedDomainWhenWhitelistConfigured(t *testing.T) {
	repo := &userRepoStub{}
	svc := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["@example.com"]`,
	}, nil, nil)

	_, _, err := svc.Register(context.Background(), "malformed-email", "password")

	require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
	require.Empty(t, repo.created)
}

func TestIsRegistrationEmailSuffixLimited(t *testing.T) {
	require.False(t, IsRegistrationEmailSuffixLimited("user@custom.example", nil))
	require.False(t, IsRegistrationEmailSuffixLimited("user@example.com", []string{"@example.com"}))
	require.True(t, IsRegistrationEmailSuffixLimited("user@custom.example", []string{"@example.com"}))
}

func TestRegistrationEmailDomainUsesRegistrableDomain(t *testing.T) {
	require.Equal(t, "abc.com", RegistrationEmailDomain("user@abc.com"))
	require.Equal(t, "abc.com", RegistrationEmailDomain("user@abcd.abc.com"))
	require.Equal(t, "example.co.uk", RegistrationEmailDomain("user@team.example.co.uk"))
	require.Equal(t, "example.com", RegistrationEmailDomain("user@example.com."))
	require.Equal(t, "example.com", RegistrationEmailDomain("user@team.example.com."))
}
