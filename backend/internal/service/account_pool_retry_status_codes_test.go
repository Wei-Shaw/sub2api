//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPoolModeRetryStatusCodes(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected []int
REDACTED{
		{
			name:     "nil_account_returns_nil",
			account:  nil,
			expected: nil,
	REDACTED,
		{
			name: "nil_credentials_returns_nil",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
		REDACTED,
			expected: nil,
	REDACTED,
		{
			name: "missing_key_returns_nil",
			account: &Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformOpenAI,
		REDACTED"pool_mode": trueREDACTED,
		REDACTED,
			expected: nil,
	REDACTED,
		{
			name: "empty_slice_is_preserved",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{REDACTED,
			REDACTED,
		REDACTED,
			expected: []int{REDACTED,
	REDACTED,
		{
			name: "float64_values_from_json_are_normalized",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{float64(429), float64(401), float64(403)REDACTED,
			REDACTED,
		REDACTED,
			expected: []int{401, 403, 429REDACTED,
	REDACTED,
		{
			name: "json_number_values_supported",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{json.Number("502"), json.Number("503")REDACTED,
			REDACTED,
		REDACTED,
			expected: []int{502, 503REDACTED,
	REDACTED,
		{
			name: "string_values_supported",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{"520", "529"REDACTED,
			REDACTED,
		REDACTED,
			expected: []int{520, 529REDACTED,
	REDACTED,
		{
			name: "duplicates_are_deduped",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{float64(429), float64(429), float64(401)REDACTED,
			REDACTED,
		REDACTED,
			expected: []int{401, 429REDACTED,
	REDACTED,
		{
			name: "out_of_range_values_dropped",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{float64(99), float64(600), float64(429)REDACTED,
			REDACTED,
		REDACTED,
			expected: []int{429REDACTED,
	REDACTED,
		{
			name: "invalid_string_dropped",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{"oops", float64(429)REDACTED,
			REDACTED,
		REDACTED,
			expected: []int{429REDACTED,
	REDACTED,
		{
			name: "non_array_value_returns_nil",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": "not-an-array",
			REDACTED,
		REDACTED,
			expected: nil,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.GetPoolModeRetryStatusCodes())
	REDACTED)
REDACTED
REDACTED

func TestIsPoolModeRetryableStatus_Account(t *testing.T) {
	tests := []struct {
		name       string
		account    *Account
		statusCode int
		expected   bool
REDACTED{
		{
			name:       "nil_account_falls_back_to_default_401",
			account:    nil,
			statusCode: 401,
			expected:   true,
	REDACTED,
		{
			name:       "nil_account_falls_back_to_default_500",
			account:    nil,
			statusCode: 500,
			expected:   false,
	REDACTED,
		{
			name: "unconfigured_uses_default_403",
			account: &Account{
		REDACTED"pool_mode": trueREDACTED,
		REDACTED,
			statusCode: 403,
			expected:   true,
	REDACTED,
		{
			name: "unconfigured_uses_default_502_false",
			account: &Account{
		REDACTED"pool_mode": trueREDACTED,
		REDACTED,
			statusCode: 502,
			expected:   false,
	REDACTED,
		{
			name: "configured_list_overrides_default_401_dropped",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{float64(502), float64(503)REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 401,
			expected:   false,
	REDACTED,
		{
			name: "configured_list_overrides_default_502_added",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{float64(502), float64(503)REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 502,
			expected:   true,
	REDACTED,
		{
			name: "empty_list_disables_all_default_codes",
			account: &Account{
		REDACTED
					"pool_mode_retry_status_codes": []any{REDACTED,
			REDACTED,
		REDACTED,
			statusCode: 429,
			expected:   false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.IsPoolModeRetryableStatus(tt.statusCode))
	REDACTED)
REDACTED
REDACTED
