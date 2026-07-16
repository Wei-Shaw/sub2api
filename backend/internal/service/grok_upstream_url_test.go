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

func TestGrokOAuthURLPolicy(t *testing.T) {
	t.Run("default CLI gateway always allowed under restrictive allowlist", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
	REDACTEDREDACTED,
	REDACTED
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.test"REDACTED

		target, err := buildGrokResponsesURL(account, cfg)
	REDACTED
		require.Equal(t, xai.DefaultCLIBaseURL+"/responses", target)
REDACTED)

	t.Run("stored official API endpoint is honored (manual endpoint switch)", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": xai.DefaultBaseURL,
		REDACTED,
	REDACTED
		cfg := &config.Config{REDACTED

		target, err := buildGrokResponsesURL(account, cfg)
	REDACTED
		require.Equal(t, xai.DefaultBaseURL+"/responses", target)
REDACTED)

	t.Run("stored regional API endpoint is trusted even under restrictive allowlist", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": "https://us-west-2.api.x.ai/v1",
		REDACTED,
	REDACTED
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.test"REDACTED

		target, err := buildGrokResponsesURL(account, cfg)
	REDACTED
		require.Equal(t, "https://us-west-2.api.x.ai/v1/responses", target)
REDACTED)

	t.Run("custom forwarding address follows operator policy", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": "https://relay.example.test/v1",
		REDACTED,
	REDACTED
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = false

		target, err := buildGrokResponsesURL(account, cfg)
	REDACTED
		require.Equal(t, "https://relay.example.test/v1/responses", target)
REDACTED)

	t.Run("custom path prefix is preserved", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": "https://relay.example.test/xai/v1",
		REDACTED,
	REDACTED
		cfg := &config.Config{REDACTED

		target, err := buildGrokResponsesURL(account, cfg)
	REDACTED
		require.Equal(t, "https://relay.example.test/xai/v1/responses", target)
REDACTED)

	t.Run("custom forwarding address rejected by allowlist", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": "https://relay.example.test/v1",
		REDACTED,
	REDACTED
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.test"REDACTED

		_, err := buildGrokResponsesURL(account, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
REDACTED)

	t.Run("insecure HTTP custom address requires operator opt-in", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": "http://relay.example.test/v1",
		REDACTED,
	REDACTED
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = false
		cfg.Security.URLAllowlist.AllowInsecureHTTP = false

		_, err := buildGrokResponsesURL(account, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")

		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		target, err := buildGrokResponsesURL(account, cfg)
	REDACTED
		require.Equal(t, "http://relay.example.test/v1/responses", target)
REDACTED)

	t.Run("unsafe override switch does not relax the operator allowlist for custom hosts", func(t *testing.T) {
		// XAI_ALLOW_UNSAFE_URL_OVERRIDES relaxes the trusted-host validator to
		// accept-any; a custom OAuth forwarding host must still be governed by
		// the operator allowlist so the bearer token cannot reach arbitrary hosts.
		t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"cli-chat-proxy.grok.com"REDACTED

		custom := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": "http://10.0.0.1/v1",
		REDACTED,
	REDACTED
		_, err := buildGrokResponsesURL(custom, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")

		// The official gateway still resolves even under the restrictive allowlist.
		official := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
	REDACTEDREDACTED,
	REDACTED
		target, err := buildGrokResponsesURL(official, cfg)
	REDACTED
		require.Equal(t, xai.DefaultCLIBaseURL+"/responses", target)
REDACTED)
REDACTED

func TestGrokBillingURLFollowsAccountBaseURL(t *testing.T) {
	t.Run("oauth default stays on CLI gateway", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
	REDACTEDREDACTED,
	REDACTED

		weeklyURL, err := buildGrokBillingURL(account, nil, true)
	REDACTED
		require.Equal(t, xai.DefaultCLIBaseURL+"/billing?format=credits", weeklyURL)

		monthlyURL, err := buildGrokBillingURL(account, nil, false)
	REDACTED
		require.Equal(t, xai.DefaultCLIBaseURL+"/billing", monthlyURL)
REDACTED)

	t.Run("oauth custom forwarding address carries billing probes", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": "https://relay.example.test/v1",
		REDACTED,
	REDACTED

		weeklyURL, err := buildGrokBillingURL(account, nil, true)
	REDACTED
		require.Equal(t, "https://relay.example.test/v1/billing?format=credits", weeklyURL)
REDACTED)

	t.Run("billing probe honors the operator allowlist like forwarding", func(t *testing.T) {
		// Probe paths must share the forwarding URL policy so a custom host the
		// allowlist rejects cannot receive the OAuth bearer via a billing probe.
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
	REDACTED
				"base_url": "https://relay.example.test/v1",
		REDACTED,
	REDACTED
		cfg := &config.Config{REDACTED
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"cli-chat-proxy.grok.com"REDACTED

		_, err := buildGrokBillingURL(account, cfg, true)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
REDACTED)
REDACTED
