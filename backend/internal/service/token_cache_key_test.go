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
			expected: "gemini:my-project-123",
	REDACTED,
		{
			name: "project_id_with_whitespace",
			account: &Account{
				ID: 101,
		REDACTED
					"project_id": "  project-with-spaces  ",
			REDACTED,
		REDACTED,
			expected: "gemini:project-with-spaces",
	REDACTED,
		{
			name: "empty_project_id_fallback_to_account_id",
			account: &Account{
				ID: 102,
		REDACTED
					"project_id": "",
			REDACTED,
		REDACTED,
			expected: "gemini:account:102",
	REDACTED,
		{
			name: "whitespace_only_project_id_fallback_to_account_id",
			account: &Account{
				ID: 103,
		REDACTED
					"project_id": "   ",
			REDACTED,
		REDACTED,
			expected: "gemini:account:103",
	REDACTED,
		{
			name: "no_project_id_key_fallback_to_account_id",
			account: &Account{
				ID:          104,
		REDACTEDREDACTED,
		REDACTED,
			expected: "gemini:account:104",
	REDACTED,
		{
			name: "nil_credentials_fallback_to_account_id",
			account: &Account{
				ID:          105,
				Credentials: nil,
		REDACTED,
			expected: "gemini:account:105",
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

func TestOpenAITokenCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected string
REDACTED{
		{
			name: "basic_account",
			account: &Account{
				ID: 300,
		REDACTED,
			expected: "openai:account:300",
	REDACTED,
		{
			name: "account_with_credentials",
			account: &Account{
				ID: 301,
		REDACTED
					"access_token": "test-token",
			REDACTED,
		REDACTED,
			expected: "openai:account:301",
	REDACTED,
		{
			name: "account_id_zero",
			account: &Account{
				ID: 0,
		REDACTED,
			expected: "openai:account:0",
	REDACTED,
		{
			name: "large_account_id",
			account: &Account{
				ID: 9999999999,
		REDACTED,
			expected: "openai:account:9999999999",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OpenAITokenCacheKey(tt.account)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestGrokTokenCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected string
REDACTED{
		{
			name: "basic_account",
			account: &Account{
				ID: 350,
		REDACTED,
			expected: "grok:account:350",
	REDACTED,
		{
			name: "account_with_email_uses_account_id",
			account: &Account{
				ID: 351,
		REDACTED
					"email": "same-user@example.com",
			REDACTED,
		REDACTED,
			expected: "grok:account:351",
	REDACTED,
		{
			name: "account_id_zero",
			account: &Account{
				ID: 0,
		REDACTED,
			expected: "grok:account:0",
	REDACTED,
		{
			name:     "nil_account",
			account:  nil,
			expected: "grok:account:0",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GrokTokenCacheKey(tt.account)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestGrokTokenCacheKeySeparatesAccountsWithSameEmail(t *testing.T) {
	first := &Account{
		ID: 351,
REDACTED
			"email": "same-user@example.com",
	REDACTED,
REDACTED
	second := &Account{
		ID: 352,
REDACTED
			"email": "same-user@example.com",
	REDACTED,
REDACTED

	require.NotEqual(t, GrokTokenCacheKey(first), GrokTokenCacheKey(second))
REDACTED

func TestClaudeTokenCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected string
REDACTED{
		{
			name: "basic_account",
			account: &Account{
				ID: 400,
		REDACTED,
			expected: "claude:account:400",
	REDACTED,
		{
			name: "account_with_credentials",
			account: &Account{
				ID: 401,
		REDACTED
					"access_token": "claude-token",
			REDACTED,
		REDACTED,
			expected: "claude:account:401",
	REDACTED,
		{
			name: "account_id_zero",
			account: &Account{
				ID: 0,
		REDACTED,
			expected: "claude:account:0",
	REDACTED,
		{
			name: "large_account_id",
			account: &Account{
				ID: 9999999999,
		REDACTED,
			expected: "claude:account:9999999999",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClaudeTokenCacheKey(tt.account)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestCacheKeyUniqueness(t *testing.T) {
	// 确保不同平台的缓存键不会冲突
	account := &Account{ID: 123REDACTED

	openaiKey := OpenAITokenCacheKey(account)
	claudeKey := ClaudeTokenCacheKey(account)

	require.NotEqual(t, openaiKey, claudeKey, "OpenAI and Claude cache keys should be different")
	require.Contains(t, openaiKey, "openai:")
	require.Contains(t, claudeKey, "claude:")
REDACTED
