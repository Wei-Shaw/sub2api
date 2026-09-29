package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalCodexIdentityValidatesBeforeTrimming(t *testing.T) {
	codexCanonicalUAMu.RLock()
	previous := codexCanonicalUAResolver
	codexCanonicalUAMu.RUnlock()
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(previous) })

	for _, raw := range []string{
		"\ncodex-tui/0.200.1 (Windows)",
		"codex-tui/0.200.1 (Windows)\n",
		"codex-tui/0.200.1 bad\x00",
	} {
		SetCodexCanonicalUserAgentResolver(func() string { return raw })
		identity := resolveCodexOutboundIdentity("")
		require.Equal(t, codexCLIUserAgent, identity.userAgent)
		require.Equal(t, codexCLIVersion, identity.version)
	}

	SetCodexCanonicalUserAgentResolver(nil)
	identity := resolveCodexOutboundIdentity("codex_cli_rs/0.145.2 (Windows)\n")
	require.Equal(t, codexCLIUserAgent, identity.userAgent)
}
