//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestGrokAPIKeyURLPolicyFollowsGlobalSecurityConfig(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED
			"base_url": "http://grok.example.test/v1",
	REDACTED,
REDACTED

	t.Run("insecure HTTP enabled with allowlist disabled", func(t *testing.T) {
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = false
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true

		responsesURL, err := buildGrokResponsesURL(account, cfg)
	REDACTED
		require.Equal(t, "http://grok.example.test/v1/responses", responsesURL)

		chatURL, err := buildGrokChatCompletionsURL(account, cfg)
	REDACTED
		require.Equal(t, "http://grok.example.test/v1/chat/completions", chatURL)

		mediaURL, err := buildGrokMediaURL(account, cfg, GrokMediaEndpointImagesGenerations, "")
	REDACTED
		require.Equal(t, "http://grok.example.test/v1/images/generations", mediaURL)
REDACTED)

	t.Run("insecure HTTP disabled", func(t *testing.T) {
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = false
		cfg.Security.URLAllowlist.AllowInsecureHTTP = false

		_, err := buildGrokResponsesURL(account, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
REDACTED)

	t.Run("enabled allowlist remains HTTPS only", func(t *testing.T) {
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"grok.example.test"REDACTED

		_, err := buildGrokResponsesURL(account, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
REDACTED)
REDACTED

func TestGrokAPIKeyURLPolicyAppliesAllowlistAndPrivateHostControls(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED
			"base_url": "https://grok.example.test/v1",
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = true
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"grok.example.test"REDACTED

	target, err := buildGrokResponsesURL(account, cfg)
REDACTED
	require.Equal(t, "https://grok.example.test/v1/responses", target)

	cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.test"REDACTED
	_, err = buildGrokResponsesURL(account, cfg)
	require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")

	account.Credentials["base_url"] = "https://127.0.0.1/v1"
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"127.0.0.1"REDACTED
	_, err = buildGrokResponsesURL(account, cfg)
	require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")

	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	target, err = buildGrokResponsesURL(account, cfg)
REDACTED
	require.Equal(t, "https://127.0.0.1/v1/responses", target)
REDACTED

func TestGrokAPIKeyURLPolicyRedactsMalformedConfiguredURL(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED
			"base_url": "https://%zz:secret@grok.example.test/v1",
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	_, err := buildGrokResponsesURL(account, cfg)
	require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
	require.NotContains(t, err.Error(), "secret")
REDACTED

func TestGrokOAuthURLPolicyIgnoresAPIKeyOverrides(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
REDACTED
			"base_url": "http://attacker.example.test/v1",
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	target, err := buildGrokResponsesURL(account, cfg)
REDACTED
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", target)
REDACTED
