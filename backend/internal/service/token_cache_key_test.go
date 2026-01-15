//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiTokenCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected string
REDACTED{
		{
			name: "with_project_id",
			account: &Account{
				ID: 100,
		REDACTED
					"project_id": "my-project-123",
			REDACTED,
		REDACTED,
			expected: "my-project-123",
	REDACTED,
		{
			name: "project_id_with_whitespace",
			account: &Account{
				ID: 101,
		REDACTED
					"project_id": "  project-with-spaces  ",
			REDACTED,
		REDACTED,
			expected: "project-with-spaces",
	REDACTED,
		{
			name: "empty_project_id_fallback_to_account_id",
			account: &Account{
				ID: 102,
		REDACTED
					"project_id": "",
			REDACTED,
		REDACTED,
			expected: "account:102",
	REDACTED,
		{
			name: "whitespace_only_project_id_fallback_to_account_id",
			account: &Account{
				ID: 103,
		REDACTED
					"project_id": "   ",
			REDACTED,
		REDACTED,
			expected: "account:103",
	REDACTED,
		{
			name: "no_project_id_key_fallback_to_account_id",
			account: &Account{
				ID:          104,
		REDACTEDREDACTED,
		REDACTED,
			expected: "account:104",
	REDACTED,
		{
			name: "nil_credentials_fallback_to_account_id",
			account: &Account{
				ID:          105,
				Credentials: nil,
		REDACTED,
			expected: "account:105",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GeminiTokenCacheKey(tt.account)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestAntigravityTokenCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected string
REDACTED{
		{
			name: "with_project_id",
			account: &Account{
				ID: 200,
		REDACTED
					"project_id": "ag-project-456",
			REDACTED,
		REDACTED,
			expected: "ag:ag-project-456",
	REDACTED,
		{
			name: "project_id_with_whitespace",
			account: &Account{
				ID: 201,
		REDACTED
					"project_id": "  ag-project-spaces  ",
			REDACTED,
		REDACTED,
			expected: "ag:ag-project-spaces",
	REDACTED,
		{
			name: "empty_project_id_fallback_to_account_id",
			account: &Account{
				ID: 202,
		REDACTED
					"project_id": "",
			REDACTED,
		REDACTED,
			expected: "ag:account:202",
	REDACTED,
		{
			name: "whitespace_only_project_id_fallback_to_account_id",
			account: &Account{
				ID: 203,
		REDACTED
					"project_id": "   ",
			REDACTED,
		REDACTED,
			expected: "ag:account:203",
	REDACTED,
		{
			name: "no_project_id_key_fallback_to_account_id",
			account: &Account{
				ID:          204,
		REDACTEDREDACTED,
		REDACTED,
			expected: "ag:account:204",
	REDACTED,
		{
			name: "nil_credentials_fallback_to_account_id",
			account: &Account{
				ID:          205,
				Credentials: nil,
		REDACTED,
			expected: "ag:account:205",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AntigravityTokenCacheKey(tt.account)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED
