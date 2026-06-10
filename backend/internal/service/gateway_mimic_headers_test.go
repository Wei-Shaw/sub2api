package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// Verifies the fill-missing contract of applyClaudeCodeMimicHeaders (v0.1.127+):
// headers already present on the request (e.g. applied from a cached per-account
// fingerprint) must not be overwritten by the hardcoded claude.DefaultHeaders.
func TestApplyClaudeCodeMimicHeaders_DoesNotOverrideExistingFingerprintHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)

	// Simulate ApplyFingerprint having pre-populated req.Header with the
	// real-world cached fingerprint for this account (values from a recent
	// Claude Code 2.1.118 on macOS capture).
	realFP := map[string]string{
		"User-Agent":                  "claude-cli/2.1.118 (external, cli)",
		"X-Stainless-Lang":            "js",
		"X-Stainless-Package-Version": "0.81.0",
		"X-Stainless-OS":              "MacOS",
		"X-Stainless-Arch":            "arm64",
		"X-Stainless-Runtime":         "node",
		"X-Stainless-Runtime-Version": "v24.3.0",
	}
	for k, v := range realFP {
		setHeaderRaw(req.Header, k, v)
	}

	applyClaudeCodeMimicHeaders(req, true)

	// Cached fingerprint values must have been preserved, not reverted to
	// the built-in claude.DefaultHeaders fallback (Linux / arm64 / the
	// pinned default CLI tuple), since a live client capture can be newer.
	for k, want := range realFP {
		require.Equal(t, want, getHeaderRaw(req.Header, k),
			"cached fingerprint header %q must not be overridden by DefaultHeaders", k)
	}

	// Non-fingerprint headers that don't come from the cache are still
	// forced (fill-missing doesn't apply to these).
	require.Equal(t, "application/json", getHeaderRaw(req.Header, "Accept"))
	require.Equal(t, "stream", getHeaderRaw(req.Header, "x-stainless-helper-method"))
}

func TestApplyClaudeCodeMimicHeaders_FillsMissingFromDefaults(t *testing.T) {
	// No fingerprint pre-applied (simulating a cache-miss code path that
	// relies on mimic to fill the blanks). All DefaultHeaders keys must
	// be populated from the hardcoded fallback.
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)

	applyClaudeCodeMimicHeaders(req, false)

	for k, want := range claude.DefaultHeaders {
		if want == "" {
			continue
		}
		require.Equal(t, want, getHeaderRaw(req.Header, resolveWireCasing(k)),
			"missing header %q should have been filled from DefaultHeaders", k)
	}
	// Non-streaming → helper-method must not be set.
	require.Empty(t, getHeaderRaw(req.Header, "x-stainless-helper-method"))
	require.Equal(t, "application/json", getHeaderRaw(req.Header, "Accept"))
}

func TestApplyClaudeCodeMimicHeaders_PartialCacheFillsOnlyGaps(t *testing.T) {
	// Simulate a fingerprint cache that has UA + Package-Version but is
	// missing OS/Arch/Runtime (e.g. legacy cache from an older schema).
	// mimic should leave cached values alone and fill only the gaps.
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)
	setHeaderRaw(req.Header, "User-Agent", "claude-cli/2.1.118 (external, cli)")
	setHeaderRaw(req.Header, "X-Stainless-Package-Version", "0.81.0")

	applyClaudeCodeMimicHeaders(req, false)

	// Preserved
	require.Equal(t, "claude-cli/2.1.118 (external, cli)", getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, "0.81.0", getHeaderRaw(req.Header, "X-Stainless-Package-Version"))
	// Filled from DefaultHeaders
	require.Equal(t, claude.DefaultHeaders["X-Stainless-OS"], getHeaderRaw(req.Header, "X-Stainless-OS"))
	require.Equal(t, claude.DefaultHeaders["X-Stainless-Arch"], getHeaderRaw(req.Header, "X-Stainless-Arch"))
	require.Equal(t, claude.DefaultHeaders["X-Stainless-Runtime"], getHeaderRaw(req.Header, "X-Stainless-Runtime"))
	require.Equal(t, claude.DefaultHeaders["X-Stainless-Runtime-Version"], getHeaderRaw(req.Header, "X-Stainless-Runtime-Version"))
}

// The upstream passthrough policy runtime is disabled, so ForwardClientUA must
// never override the Claude Code mimic UA.
func TestForwardClientUA_DoesNotOverrideMimicUAOnOAuthPath(t *testing.T) {
	applyOverride := func(req *http.Request, clientHeaders http.Header, ctx context.Context) {
		applyClaudeCodeMimicHeaders(req, true)
		if ShouldForwardClientUA(ctx) {
			if clientUA := strings.TrimSpace(clientHeaders.Get("User-Agent")); clientUA != "" {
				setHeaderRaw(req.Header, "User-Agent", clientUA)
			}
		}
	}

	t.Run("no policy in ctx → mimic UA wins (legacy)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)
		clientHeaders := http.Header{"User-Agent": []string{"my-custom-client/1.0"}}

		applyOverride(req, clientHeaders, context.Background())

		require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(req.Header, "User-Agent"))
	})

	t.Run("policy ForwardClientUA=false → mimic UA wins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)
		clientHeaders := http.Header{"User-Agent": []string{"my-custom-client/1.0"}}
		ctx := SetUpstreamPolicyInContext(context.Background(), &EffectiveUpstreamPolicy{ForwardClientUA: false})

		applyOverride(req, clientHeaders, ctx)

		require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(req.Header, "User-Agent"))
	})

	t.Run("policy ForwardClientUA=true → mimic UA still wins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)
		clientHeaders := http.Header{"User-Agent": []string{"my-custom-client/1.0"}}
		ctx := SetUpstreamPolicyInContext(context.Background(), &EffectiveUpstreamPolicy{ForwardClientUA: true})

		applyOverride(req, clientHeaders, ctx)

		require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(req.Header, "User-Agent"))
		// Sibling mimic headers must still be set — the override is narrow to UA only.
		require.Equal(t, claude.DefaultHeaders["X-Stainless-Lang"], getHeaderRaw(req.Header, "X-Stainless-Lang"))
	})

	t.Run("policy ForwardClientUA=true but client UA empty → mimic UA stays", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)
		clientHeaders := http.Header{} // no UA from client
		ctx := SetUpstreamPolicyInContext(context.Background(), &EffectiveUpstreamPolicy{ForwardClientUA: true})

		applyOverride(req, clientHeaders, ctx)

		require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(req.Header, "User-Agent"))
	})

	t.Run("policy ForwardClientUA=true with whitespace-only client UA → mimic UA stays", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", nil)
		clientHeaders := http.Header{"User-Agent": []string{"   "}}
		ctx := SetUpstreamPolicyInContext(context.Background(), &EffectiveUpstreamPolicy{ForwardClientUA: true})

		applyOverride(req, clientHeaders, ctx)

		require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(req.Header, "User-Agent"))
	})
}
