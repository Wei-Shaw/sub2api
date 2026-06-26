package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsCodexCLIOnlyAppServerAllowed(t *testing.T) {
	t.Run("codex_cli_only 开 + allow_app_server=true → true", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true, "codex_cli_only_allow_app_server": trueREDACTED,
	REDACTED
		require.True(t, account.IsCodexCLIOnlyAppServerAllowed())
REDACTED)

	t.Run("codex_cli_only 开 + allow_app_server=false → false", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true, "codex_cli_only_allow_app_server": falseREDACTED,
	REDACTED
		require.False(t, account.IsCodexCLIOnlyAppServerAllowed())
REDACTED)

	t.Run("codex_cli_only 开 + 字段缺失 → false", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": trueREDACTED,
	REDACTED
		require.False(t, account.IsCodexCLIOnlyAppServerAllowed())
REDACTED)

	t.Run("codex_cli_only 关 → 即便 allow_app_server=true 也 false", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_allow_app_server": trueREDACTED,
	REDACTED
		require.False(t, account.IsCodexCLIOnlyAppServerAllowed())
REDACTED)
REDACTED
