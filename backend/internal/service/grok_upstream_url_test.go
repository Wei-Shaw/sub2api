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
		Credentials: map[string]any{
			"base_url": "http://grok.example.test/v1",
		},
	}

	t.Run("insecure HTTP enabled with allowlist disabled", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true

		responsesURL, err := buildGrokResponsesURL(account, cfg)
		require.NoError(t, err)
		require.Equal(t, "http://grok.example.test/v1/responses", responsesURL)

		chatURL, err := buildGrokChatCompletionsURL(account, cfg)
		require.NoError(t, err)
		require.Equal(t, "http://grok.example.test/v1/chat/completions", chatURL)

		mediaURL, err := buildGrokMediaURL(account, cfg, GrokMediaEndpointImagesGenerations, "")
		require.NoError(t, err)
		require.Equal(t, "http://grok.example.test/v1/images/generations", mediaURL)
	})

	t.Run("insecure HTTP disabled", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		cfg.Security.URLAllowlist.AllowInsecureHTTP = false

		_, err := buildGrokResponsesURL(account, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
	})

	t.Run("enabled allowlist remains HTTPS only", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"grok.example.test"}

		_, err := buildGrokResponsesURL(account, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
	})
}

func TestGrokAPIKeyURLPolicyAppliesAllowlistAndPrivateHostControls(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://grok.example.test/v1",
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = true
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"grok.example.test"}

	target, err := buildGrokResponsesURL(account, cfg)
	require.NoError(t, err)
	require.Equal(t, "https://grok.example.test/v1/responses", target)

	cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.test"}
	_, err = buildGrokResponsesURL(account, cfg)
	require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")

	account.Credentials["base_url"] = "https://127.0.0.1/v1"
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"127.0.0.1"}
	_, err = buildGrokResponsesURL(account, cfg)
	require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")

	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	target, err = buildGrokResponsesURL(account, cfg)
	require.NoError(t, err)
	require.Equal(t, "https://127.0.0.1/v1/responses", target)
}

func TestGrokAPIKeyURLPolicyRedactsMalformedConfiguredURL(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://%zz:secret@grok.example.test/v1",
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	_, err := buildGrokResponsesURL(account, cfg)
	require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
	require.NotContains(t, err.Error(), "secret")
}

func TestGrokOAuthURLPolicy(t *testing.T) {
	t.Run("default CLI gateway always allowed under restrictive allowlist", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
			Credentials: map[string]any{},
		}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.test"}

		target, err := buildGrokResponsesURL(account, cfg)
		require.NoError(t, err)
		require.Equal(t, xai.DefaultCLIBaseURL+"/responses", target)
	})

	t.Run("stored official API endpoint is honored (manual endpoint switch)", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": xai.DefaultBaseURL,
			},
		}
		cfg := &config.Config{}

		target, err := buildGrokResponsesURL(account, cfg)
		require.NoError(t, err)
		require.Equal(t, xai.DefaultBaseURL+"/responses", target)
	})

	t.Run("stored regional API endpoint is trusted even under restrictive allowlist", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": "https://us-west-2.api.x.ai/v1",
			},
		}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.test"}

		target, err := buildGrokResponsesURL(account, cfg)
		require.NoError(t, err)
		require.Equal(t, "https://us-west-2.api.x.ai/v1/responses", target)
	})

	t.Run("custom forwarding address follows operator policy", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": "https://relay.example.test/v1",
			},
		}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false

		target, err := buildGrokResponsesURL(account, cfg)
		require.NoError(t, err)
		require.Equal(t, "https://relay.example.test/v1/responses", target)
	})

	t.Run("custom path prefix is preserved", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": "https://relay.example.test/xai/v1",
			},
		}
		cfg := &config.Config{}

		target, err := buildGrokResponsesURL(account, cfg)
		require.NoError(t, err)
		require.Equal(t, "https://relay.example.test/xai/v1/responses", target)
	})

	t.Run("custom forwarding address rejected by allowlist", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": "https://relay.example.test/v1",
			},
		}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.test"}

		_, err := buildGrokResponsesURL(account, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
	})

	t.Run("insecure HTTP custom address requires operator opt-in", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": "http://relay.example.test/v1",
			},
		}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		cfg.Security.URLAllowlist.AllowInsecureHTTP = false

		_, err := buildGrokResponsesURL(account, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")

		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		target, err := buildGrokResponsesURL(account, cfg)
		require.NoError(t, err)
		require.Equal(t, "http://relay.example.test/v1/responses", target)
	})

	t.Run("unsafe override switch does not relax the operator allowlist for custom hosts", func(t *testing.T) {
		// XAI_ALLOW_UNSAFE_URL_OVERRIDES relaxes the trusted-host validator to
		// accept-any; a custom OAuth forwarding host must still be governed by
		// the operator allowlist so the bearer token cannot reach arbitrary hosts.
		t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"cli-chat-proxy.grok.com"}

		custom := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": "http://10.0.0.1/v1",
			},
		}
		_, err := buildGrokResponsesURL(custom, cfg)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")

		// The official gateway still resolves even under the restrictive allowlist.
		official := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
			Credentials: map[string]any{},
		}
		target, err := buildGrokResponsesURL(official, cfg)
		require.NoError(t, err)
		require.Equal(t, xai.DefaultCLIBaseURL+"/responses", target)
	})
}

func TestGrokBillingURLFollowsAccountBaseURL(t *testing.T) {
	t.Run("oauth default stays on CLI gateway", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
			Credentials: map[string]any{},
		}

		weeklyURL, err := buildGrokBillingURL(account, nil, true)
		require.NoError(t, err)
		require.Equal(t, xai.DefaultCLIBaseURL+"/billing?format=credits", weeklyURL)

		monthlyURL, err := buildGrokBillingURL(account, nil, false)
		require.NoError(t, err)
		require.Equal(t, xai.DefaultCLIBaseURL+"/billing", monthlyURL)
	})

	t.Run("oauth custom forwarding address carries billing probes", func(t *testing.T) {
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": "https://relay.example.test/v1",
			},
		}

		weeklyURL, err := buildGrokBillingURL(account, nil, true)
		require.NoError(t, err)
		require.Equal(t, "https://relay.example.test/v1/billing?format=credits", weeklyURL)
	})

	t.Run("billing probe honors the operator allowlist like forwarding", func(t *testing.T) {
		// Probe paths must share the forwarding URL policy so a custom host the
		// allowlist rejects cannot receive the OAuth bearer via a billing probe.
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"base_url": "https://relay.example.test/v1",
			},
		}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = true
		cfg.Security.URLAllowlist.UpstreamHosts = []string{"cli-chat-proxy.grok.com"}

		_, err := buildGrokBillingURL(account, cfg, true)
		require.EqualError(t, err, "invalid base url: base URL rejected by URL security policy")
	})
}
