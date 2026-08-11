//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRegistrationEmailSuffixWhitelist(t *testing.T) {
	got, err := NormalizeRegistrationEmailSuffixWhitelist([]string{"example.com", "@EXAMPLE.COM", " @foo.bar ", "*.EDU.CN"REDACTED)
REDACTED
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"REDACTED, got)
REDACTED

func TestNormalizeRegistrationEmailSuffixWhitelist_Invalid(t *testing.T) {
	for _, item := range []string{"@invalid_domain", "*.", "*", "*.@", "*.foo"REDACTED {
		t.Run(item, func(t *testing.T) {
			_, err := NormalizeRegistrationEmailSuffixWhitelist([]string{itemREDACTED)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestParseRegistrationEmailSuffixWhitelist(t *testing.T) {
	got := ParseRegistrationEmailSuffixWhitelist(`["example.com","@foo.bar","*.EDU.CN","@invalid_domain","*.foo"]`)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"REDACTED, got)
REDACTED

func TestIsRegistrationEmailSuffixAllowed(t *testing.T) {
	require.True(t, IsRegistrationEmailSuffixAllowed("user@example.com", []string{"@example.com"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@example.com.", []string{"@example.com"REDACTED))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@sub.example.com", []string{"@example.com"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@qq.com", []string{"@qq.com"REDACTED))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@sub.qq.com", []string{"@qq.com"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("student@cs.edu.cn", []string{"*.edu.cn"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("student@edu.cn", []string{"*.edu.cn"REDACTED))
	require.False(t, IsRegistrationEmailSuffixAllowed("student@foo.cn", []string{"*.edu.cn"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@a.com", []string{"@a.com", "*.b.cn"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@school.b.cn", []string{"@a.com", "*.b.cn"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@b.cn", []string{"@a.com", "*.b.cn"REDACTED))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@c.cn", []string{"@a.com", "*.b.cn"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@any.com", []string{REDACTED))
REDACTED

func TestRegistrationEmailQuotaRejectsMalformedDomainWhenWhitelistConfigured(t *testing.T) {
	repo := &userRepoStub{REDACTED
	svc := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:                 "true",
		SettingKeyRegistrationEmailSuffixWhitelist:    `["@example.com"]`,
		SettingKeyRegistrationEmailDomainQuotaEnabled: "true",
REDACTED, nil, nil)

	_, _, err := svc.Register(context.Background(), "malformed-email", "password")

	require.ErrorIs(t, err, ErrEmailSuffixNotAllowed)
	require.Empty(t, repo.created)
REDACTED

func TestIsRegistrationEmailSuffixLimited(t *testing.T) {
	require.False(t, IsRegistrationEmailSuffixLimited("user@custom.example", nil))
	require.False(t, IsRegistrationEmailSuffixLimited("user@example.com", []string{"@example.com"REDACTED))
	require.True(t, IsRegistrationEmailSuffixLimited("user@custom.example", []string{"@example.com"REDACTED))
REDACTED

func TestRegistrationEmailDomainUsesRegistrableDomain(t *testing.T) {
	require.Equal(t, "abc.com", RegistrationEmailDomain("user@abc.com"))
	require.Equal(t, "abc.com", RegistrationEmailDomain("user@abcd.abc.com"))
	require.Equal(t, "example.co.uk", RegistrationEmailDomain("user@team.example.co.uk"))
	require.Equal(t, "example.com", RegistrationEmailDomain("user@example.com."))
	require.Equal(t, "example.com", RegistrationEmailDomain("user@team.example.com."))
REDACTED
