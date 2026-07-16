//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		account  Account
		expected string
REDACTED{
		{
			name: "non-apikey type returns empty",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformAnthropic,
		REDACTED,
			expected: "",
	REDACTED,
		{
			name: "apikey without base_url returns default anthropic",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAnthropic,
		REDACTEDREDACTED,
		REDACTED,
			expected: "https://api.anthropic.com",
	REDACTED,
		{
			name: "apikey with custom base_url",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAnthropic,
		REDACTED"base_url": "https://custom.example.com"REDACTED,
		REDACTED,
			expected: "https://custom.example.com",
	REDACTED,
		{
			name: "antigravity apikey auto-appends /antigravity",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
		REDACTED"base_url": "https://upstream.example.com"REDACTED,
		REDACTED,
			expected: "https://upstream.example.com/antigravity",
	REDACTED,
		{
			name: "antigravity apikey trims trailing slash before appending",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
		REDACTED"base_url": "https://upstream.example.com/"REDACTED,
		REDACTED,
			expected: "https://upstream.example.com/antigravity",
	REDACTED,
		{
			name: "antigravity non-apikey returns empty",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
		REDACTED"base_url": "https://upstream.example.com"REDACTED,
		REDACTED,
			expected: "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetBaseURL()
			if result != tt.expected {
				t.Errorf("GetBaseURL() = %q, want %q", result, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGetGeminiBaseURL(t *testing.T) {
	const defaultGeminiURL = "https://generativelanguage.googleapis.com"

	tests := []struct {
		name     string
		account  Account
		expected string
REDACTED{
		{
			name: "apikey without base_url returns default",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
		REDACTEDREDACTED,
		REDACTED,
			expected: defaultGeminiURL,
	REDACTED,
		{
			name: "apikey with custom base_url",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
		REDACTED"base_url": "https://custom-gemini.example.com"REDACTED,
		REDACTED,
			expected: "https://custom-gemini.example.com",
	REDACTED,
		{
			name: "antigravity apikey auto-appends /antigravity",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
		REDACTED"base_url": "https://upstream.example.com"REDACTED,
		REDACTED,
			expected: "https://upstream.example.com/antigravity",
	REDACTED,
		{
			name: "antigravity apikey trims trailing slash",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
		REDACTED"base_url": "https://upstream.example.com/"REDACTED,
		REDACTED,
			expected: "https://upstream.example.com/antigravity",
	REDACTED,
		{
			name: "antigravity oauth does NOT append /antigravity",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
		REDACTED"base_url": "https://upstream.example.com"REDACTED,
		REDACTED,
			expected: "https://upstream.example.com",
	REDACTED,
		{
			name: "oauth without base_url returns default",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
		REDACTEDREDACTED,
		REDACTED,
			expected: defaultGeminiURL,
	REDACTED,
		{
			name: "nil credentials returns default",
			account: Account{
				Type:     AccountTypeAPIKey,
		REDACTED
		REDACTED,
			expected: defaultGeminiURL,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetGeminiBaseURL(defaultGeminiURL)
			if result != tt.expected {
				t.Errorf("GetGeminiBaseURL() = %q, want %q", result, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGetGrokBaseURLUsesSubscriptionProxyForOAuth(t *testing.T) {
	tests := []struct {
		name     string
		account  Account
		expected string
REDACTED{
		{
			name: "oauth without base_url uses CLI subscription proxy",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformGrok,
		REDACTEDREDACTED,
		REDACTED,
			expected: xai.DefaultCLIBaseURL,
	REDACTED,
		{
			name: "oauth stored official API endpoint is honored (manual endpoint switch)",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": xai.DefaultBaseURL,
			REDACTED,
		REDACTED,
			expected: xai.DefaultBaseURL,
	REDACTED,
		{
			name: "oauth stored regional API endpoint is honored",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": "https://us-west-2.api.x.ai/v1",
			REDACTED,
		REDACTED,
			expected: "https://us-west-2.api.x.ai/v1",
	REDACTED,
		{
			name: "oauth stored CLI proxy is honored verbatim",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": xai.DefaultCLIBaseURL,
			REDACTED,
		REDACTED,
			expected: xai.DefaultCLIBaseURL,
	REDACTED,
		{
			name: "oauth unparseable base_url falls back to CLI proxy",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": "not a url",
			REDACTED,
		REDACTED,
			expected: xai.DefaultCLIBaseURL,
	REDACTED,
		{
			name: "oauth explicit custom base_url redirects forwarding traffic",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": "https://custom.example.com/v1",
			REDACTED,
		REDACTED,
			expected: "https://custom.example.com/v1",
	REDACTED,
		{
			name: "oauth custom base_url with path prefix redirects forwarding traffic",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": "https://relay.example.com/xai/v1",
			REDACTED,
		REDACTED,
			expected: "https://relay.example.com/xai/v1",
	REDACTED,
		{
			name: "API key without base_url uses official credit-backed API",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGrok,
		REDACTEDREDACTED,
		REDACTED,
			expected: xai.DefaultBaseURL,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.GetGrokBaseURL())
	REDACTED)
REDACTED
REDACTED

func TestGetGrokBaseURLHonorsOAuthCustomRegardlessOfUnsafeOverrides(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	account := Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformGrok,
REDACTED
			"base_url": "https://custom.example.com/v1",
	REDACTED,
REDACTED

	require.Equal(t, "https://custom.example.com/v1", account.GetGrokBaseURL())
REDACTED

func TestGetGrokMediaBaseURLFollowsTextTrafficResolution(t *testing.T) {
	tests := []struct {
		name     string
		account  Account
		expected string
REDACTED{
		{
			name: "oauth without base_url uses CLI subscription proxy",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformGrok,
		REDACTEDREDACTED,
		REDACTED,
			expected: xai.DefaultCLIBaseURL,
	REDACTED,
		{
			name: "oauth stored CLI proxy stays on CLI subscription proxy",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": xai.DefaultCLIBaseURL,
			REDACTED,
		REDACTED,
			expected: xai.DefaultCLIBaseURL,
	REDACTED,
		{
			name: "oauth stored official API endpoint is honored (manual endpoint switch)",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": xai.DefaultBaseURL,
			REDACTED,
		REDACTED,
			expected: xai.DefaultBaseURL,
	REDACTED,
		{
			name: "oauth custom base_url redirects media traffic",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
		REDACTED
					"base_url": "https://custom.example.com/v1",
			REDACTED,
		REDACTED,
			expected: "https://custom.example.com/v1",
	REDACTED,
		{
			name: "API key retains its configured media API",
			account: Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformGrok,
		REDACTED
					"base_url": "https://grok.example.com/v1",
			REDACTED,
		REDACTED,
			expected: "https://grok.example.com/v1",
	REDACTED,
		{
			name: "non-Grok account has no Grok media base URL",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformOpenAI,
		REDACTEDREDACTED,
		REDACTED,
			expected: "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.GetGrokMediaBaseURL())
	REDACTED)
REDACTED
REDACTED

func TestGetGrokMediaBaseURLHonorsOAuthCustomRegardlessOfUnsafeOverrides(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	account := Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformGrok,
REDACTED
			"base_url": "https://custom.example.com/v1",
	REDACTED,
REDACTED

	require.Equal(t, "https://custom.example.com/v1", account.GetGrokMediaBaseURL())
REDACTED
