//go:build unit

package service

import (
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
