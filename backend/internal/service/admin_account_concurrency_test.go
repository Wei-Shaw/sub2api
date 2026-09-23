//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAccountConcurrencyDefaultsInvalidGrokOAuthToOne(t *testing.T) {
	require.Equal(t, 1, normalizeAccountConcurrency(PlatformGrok, AccountTypeOAuth, 0))
	require.Equal(t, 1, normalizeAccountConcurrency(PlatformGrok, AccountTypeOAuth, -5))
	require.Equal(t, 1, normalizeAccountConcurrency(PlatformCursor, AccountTypeOAuth, 0))
}

func TestNormalizeAccountConcurrencyPreservesExplicitValues(t *testing.T) {
	require.Equal(t, 50, normalizeAccountConcurrency(PlatformGrok, AccountTypeOAuth, 50))
	require.Equal(t, 50, normalizeAccountConcurrency(PlatformCursor, AccountTypeOAuth, 50))
	require.Equal(t, 2, normalizeAccountConcurrency(PlatformOpenAI, AccountTypeOAuth, 2))
	require.Equal(t, 2, normalizeAccountConcurrency(PlatformGrok, AccountTypeAPIKey, 2))
}
