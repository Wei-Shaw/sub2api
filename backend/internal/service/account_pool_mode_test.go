//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPoolModeRetryCount(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected int
REDACTED{
		{
			name: "default_when_not_pool_mode",
			account: &Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformOpenAI,
		REDACTEDREDACTED,
		REDACTED,
			expected: defaultPoolModeRetryCount,
	REDACTED,
		{
			name: "default_when_missing_retry_count",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode": true,
			REDACTED,
		REDACTED,
			expected: defaultPoolModeRetryCount,
	REDACTED,
		{
			name: "supports_float64_from_json_credentials",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":             true,
					"pool_mode_retry_count": float64(5),
			REDACTED,
		REDACTED,
			expected: 5,
	REDACTED,
		{
			name: "supports_json_number",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":             true,
					"pool_mode_retry_count": json.Number("4"),
			REDACTED,
		REDACTED,
			expected: 4,
	REDACTED,
		{
			name: "supports_string_value",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":             true,
					"pool_mode_retry_count": "2",
			REDACTED,
		REDACTED,
			expected: 2,
	REDACTED,
		{
			name: "negative_value_is_clamped_to_zero",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":             true,
					"pool_mode_retry_count": -1,
			REDACTED,
		REDACTED,
			expected: 0,
	REDACTED,
		{
			name: "oversized_value_is_clamped_to_max",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":             true,
					"pool_mode_retry_count": 99,
			REDACTED,
		REDACTED,
			expected: maxPoolModeRetryCount,
	REDACTED,
		{
			name: "invalid_value_falls_back_to_default",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED
					"pool_mode":             true,
					"pool_mode_retry_count": "oops",
			REDACTED,
		REDACTED,
			expected: defaultPoolModeRetryCount,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.GetPoolModeRetryCount())
	REDACTED)
REDACTED
REDACTED
