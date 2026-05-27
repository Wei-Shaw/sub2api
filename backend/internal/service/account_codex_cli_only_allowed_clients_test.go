package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_GetCodexCLIOnlyAllowedClients(t *testing.T) {
	t.Run("OAuth 账号读取 []any 字符串列表", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_allowed_clients": []any{"claude_code"REDACTEDREDACTED,
	REDACTED
		require.Equal(t, []string{"claude_code"REDACTED, account.GetCodexCLIOnlyAllowedClients())
REDACTED)

	t.Run("OAuth 账号读取 []string 列表", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_allowed_clients": []string{"claude_code"REDACTEDREDACTED,
	REDACTED
		require.Equal(t, []string{"claude_code"REDACTED, account.GetCodexCLIOnlyAllowedClients())
REDACTED)

	t.Run("[]string 跳过空白元素", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_allowed_clients": []string{"claude_code", "", "  "REDACTEDREDACTED,
	REDACTED
		require.Equal(t, []string{"claude_code"REDACTED, account.GetCodexCLIOnlyAllowedClients())
REDACTED)

	t.Run("跳过非字符串与空白元素", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_allowed_clients": []any{"claude_code", 123, "", "  "REDACTEDREDACTED,
	REDACTED
		require.Equal(t, []string{"claude_code"REDACTED, account.GetCodexCLIOnlyAllowedClients())
REDACTED)

	t.Run("非 OAuth 账号返回空", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"codex_cli_only_allowed_clients": []any{"claude_code"REDACTEDREDACTED,
	REDACTED
		require.Empty(t, account.GetCodexCLIOnlyAllowedClients())
REDACTED)

	t.Run("Extra 为空返回空", func(t *testing.T) {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
		require.Empty(t, account.GetCodexCLIOnlyAllowedClients())
REDACTED)

	t.Run("字段缺失返回空", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{REDACTED,
	REDACTED
		require.Empty(t, account.GetCodexCLIOnlyAllowedClients())
REDACTED)
REDACTED
