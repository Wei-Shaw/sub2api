//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRegistrationEmailSuffixWhitelist(t *testing.T) {
	got, err := NormalizeRegistrationEmailSuffixWhitelist([]string{"example.com", "@EXAMPLE.COM", " @foo.bar "REDACTED)
REDACTED
	require.Equal(t, []string{"@example.com", "@foo.bar"REDACTED, got)
REDACTED

func TestNormalizeRegistrationEmailSuffixWhitelist_Invalid(t *testing.T) {
	_, err := NormalizeRegistrationEmailSuffixWhitelist([]string{"@invalid_domain"REDACTED)
REDACTED
REDACTED

func TestParseRegistrationEmailSuffixWhitelist(t *testing.T) {
	got := ParseRegistrationEmailSuffixWhitelist(`["example.com","@foo.bar","@invalid_domain"]`)
	require.Equal(t, []string{"@example.com", "@foo.bar"REDACTED, got)
REDACTED

func TestIsRegistrationEmailSuffixAllowed(t *testing.T) {
	require.True(t, IsRegistrationEmailSuffixAllowed("user@example.com", []string{"@example.com"REDACTED))
	require.False(t, IsRegistrationEmailSuffixAllowed("user@sub.example.com", []string{"@example.com"REDACTED))
	require.True(t, IsRegistrationEmailSuffixAllowed("user@any.com", []string{REDACTED))
REDACTED
