//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAntigravityTokenProvider_GetAccessToken_Upstream(t *testing.T) {
	provider := &AntigravityTokenProvider{REDACTED

	t.Run("upstream account with valid api_key", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAntigravity,
			Type:     AccountTypeUpstream,
	REDACTED
				"api_key": "sk-test-key-12345",
		REDACTED,
	REDACTED
		token, err := provider.GetAccessToken(context.Background(), account)
	REDACTED
		require.Equal(t, "sk-test-key-12345", token)
REDACTED)

	t.Run("upstream account missing api_key", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformAntigravity,
			Type:        AccountTypeUpstream,
	REDACTEDREDACTED,
	REDACTED
		token, err := provider.GetAccessToken(context.Background(), account)
	REDACTED
		require.Contains(t, err.Error(), "upstream account missing api_key")
		require.Empty(t, token)
REDACTED)

	t.Run("upstream account with empty api_key", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAntigravity,
			Type:     AccountTypeUpstream,
	REDACTED
				"api_key": "",
		REDACTED,
	REDACTED
		token, err := provider.GetAccessToken(context.Background(), account)
	REDACTED
		require.Contains(t, err.Error(), "upstream account missing api_key")
		require.Empty(t, token)
REDACTED)

	t.Run("upstream account with nil credentials", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAntigravity,
			Type:     AccountTypeUpstream,
	REDACTED
		token, err := provider.GetAccessToken(context.Background(), account)
	REDACTED
		require.Contains(t, err.Error(), "upstream account missing api_key")
		require.Empty(t, token)
REDACTED)
REDACTED

func TestAntigravityTokenProvider_GetAccessToken_Guards(t *testing.T) {
	provider := &AntigravityTokenProvider{REDACTED

	t.Run("nil account", func(t *testing.T) {
		token, err := provider.GetAccessToken(context.Background(), nil)
	REDACTED
		require.Contains(t, err.Error(), "account is nil")
		require.Empty(t, token)
REDACTED)

	t.Run("non-antigravity platform", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
	REDACTED
		token, err := provider.GetAccessToken(context.Background(), account)
	REDACTED
		require.Contains(t, err.Error(), "not an antigravity account")
		require.Empty(t, token)
REDACTED)

	t.Run("unsupported account type", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAntigravity,
			Type:     AccountTypeAPIKey,
	REDACTED
		token, err := provider.GetAccessToken(context.Background(), account)
	REDACTED
		require.Contains(t, err.Error(), "not an antigravity oauth account")
		require.Empty(t, token)
REDACTED)
REDACTED
