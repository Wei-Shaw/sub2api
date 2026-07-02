package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsAnthropicAPIKeyPassthroughEnabled(t *testing.T) {
	t.Run("Anthropic API Key 开启", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_passthrough": true,
		REDACTED,
	REDACTED
		require.True(t, account.IsAnthropicAPIKeyPassthroughEnabled())
REDACTED)

	t.Run("Anthropic API Key 关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_passthrough": false,
		REDACTED,
	REDACTED
		require.False(t, account.IsAnthropicAPIKeyPassthroughEnabled())
REDACTED)

	t.Run("字段类型非法默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_passthrough": "true",
		REDACTED,
	REDACTED
		require.False(t, account.IsAnthropicAPIKeyPassthroughEnabled())
REDACTED)

	t.Run("非 Anthropic API Key 账号始终关闭", func(t *testing.T) {
		oauth := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"anthropic_passthrough": true,
		REDACTED,
	REDACTED
		require.False(t, oauth.IsAnthropicAPIKeyPassthroughEnabled())

		openai := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_passthrough": true,
		REDACTED,
	REDACTED
		require.False(t, openai.IsAnthropicAPIKeyPassthroughEnabled())
REDACTED)
REDACTED

func TestAccount_GetAnthropicAPIKeyAuthScheme(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
REDACTED{
		{
			name: "missing extra defaults to x-api-key",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
		REDACTED,
			want: AnthropicAPIKeyAuthSchemeXAPIKey,
	REDACTED,
		{
			name: "explicit bearer",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"anthropic_apikey_auth_scheme": AnthropicAPIKeyAuthSchemeAuthorizationBearer,
			REDACTED,
		REDACTED,
			want: AnthropicAPIKeyAuthSchemeAuthorizationBearer,
	REDACTED,
		{
			name: "invalid value defaults to x-api-key",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"anthropic_apikey_auth_scheme": "bearer",
			REDACTED,
		REDACTED,
			want: AnthropicAPIKeyAuthSchemeXAPIKey,
	REDACTED,
		{
			name: "non Anthropic API key defaults to x-api-key",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"anthropic_apikey_auth_scheme": AnthropicAPIKeyAuthSchemeAuthorizationBearer,
			REDACTED,
		REDACTED,
			want: AnthropicAPIKeyAuthSchemeXAPIKey,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.GetAnthropicAPIKeyAuthScheme())
	REDACTED)
REDACTED
REDACTED
