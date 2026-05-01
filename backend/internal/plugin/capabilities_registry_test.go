//go:build unit

package plugin

import (
	"log/slog"
	"sort"
	"testing"

	pluginsdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// TestApproveCapabilities_NewCanonicalNamesAccepted is the host-side
// regression for T3: a manifest declaring only the new canonical
// CapabilitySettingsOwnRead must round-trip through approveCapabilities
// and expandWithLegacyAliases such that the SDK runner — keyed off
// either canonical or legacy via CapabilityGrantedAny — wires the
// settings client. Previously runner.go matched only the legacy alias,
// so a plugin on the new name silently fell back to nilSettingsClient.
func TestApproveCapabilities_NewCanonicalNamesAccepted(t *testing.T) {
	requested := []string{
		pluginsdkroot.CapabilitySettingsOwnRead,
		pluginsdkroot.CapabilityRedisRaw,
		pluginsdkroot.CapabilitySecretsEncrypt,
	}
	approved := approveCapabilities("test-plugin", requested, slog.Default())
	if len(approved) != len(requested) {
		t.Fatalf("approveCapabilities = %v, want all %d preserved", approved, len(requested))
	}
	expanded := expandWithLegacyAliases(approved)

	// Each canonical we asked for must appear; each known legacy alias
	// must also appear so plugins still keyed off the old strings work.
	mustContain(t, expanded, pluginsdkroot.CapabilitySettingsOwnRead)
	mustContain(t, expanded, pluginsdkroot.CapabilitySettingsExtension) // legacy
	mustContain(t, expanded, pluginsdkroot.CapabilityRedisRaw)
	mustContain(t, expanded, pluginsdkroot.CapabilityRedisRawKeys) // legacy
	mustContain(t, expanded, pluginsdkroot.CapabilitySecretsEncrypt)
	mustContain(t, expanded, pluginsdkroot.CapabilitySecretEncryption) // legacy

	// Final sanity: the SDK helper that runner.go uses to wire feature
	// gates must agree the canonical was granted.
	if !pluginsdkroot.CapabilityGrantedAny(expanded, pluginsdkroot.CapabilitySettingsOwnRead) {
		t.Fatal("expanded list should grant SettingsOwnRead — runner.go relies on this")
	}
	if !pluginsdkroot.CapabilityGrantedAny(expanded, pluginsdkroot.CapabilityRedisRaw) {
		t.Fatal("expanded list should grant RedisRaw")
	}
	if !pluginsdkroot.CapabilityGrantedAny(expanded, pluginsdkroot.CapabilitySecretsEncrypt) {
		t.Fatal("expanded list should grant SecretsEncrypt")
	}
}

// TestApproveCapabilities_LegacyNormalisedToCanonical verifies the host
// drops legacy aliases at the manifest boundary and stores only canonical
// names in any internal registry — the alias is re-emitted only when the
// approved list is forwarded to the plugin.
func TestApproveCapabilities_LegacyNormalisedToCanonical(t *testing.T) {
	requested := []string{
		pluginsdkroot.CapabilityRedisRawKeys,      // legacy
		pluginsdkroot.CapabilitySecretEncryption,  // legacy
		pluginsdkroot.CapabilitySettingsExtension, // legacy
	}
	approved := approveCapabilities("test-plugin", requested, slog.Default())

	// All must be canonicalised.
	for _, c := range approved {
		switch c {
		case pluginsdkroot.CapabilityRedisRaw,
			pluginsdkroot.CapabilitySecretsEncrypt,
			pluginsdkroot.CapabilitySettingsOwnRead:
			// ok
		default:
			t.Fatalf("approved list contains non-canonical %q; expected dotted-lowercase forms only", c)
		}
	}
	// And specifically must NOT contain any legacy alias.
	for _, c := range approved {
		switch c {
		case pluginsdkroot.CapabilityRedisRawKeys,
			pluginsdkroot.CapabilitySecretEncryption,
			pluginsdkroot.CapabilitySettingsExtension:
			t.Fatalf("approved list still contains legacy alias %q after normalisation", c)
		}
	}
}

// TestAllowedCapabilities_DerivedFromRegistry locks the data-driven
// invariant: every entry in the SDK registry (canonical + each alias)
// must be reachable through the host's allow-list. Adding a capability
// in the SDK without updating the host should not be possible — the host
// builds its set from pluginsdkroot.CapabilityRegistry directly.
func TestAllowedCapabilities_DerivedFromRegistry(t *testing.T) {
	for _, decl := range pluginsdkroot.CapabilityRegistry {
		if _, ok := allowedPluginCapabilities[decl.Canonical]; !ok {
			t.Errorf("allowedPluginCapabilities missing canonical %q", decl.Canonical)
		}
		for _, alias := range decl.LegacyAliases {
			if _, ok := allowedPluginCapabilities[alias]; !ok {
				t.Errorf("allowedPluginCapabilities missing legacy alias %q for %q", alias, decl.Canonical)
			}
		}
	}
	// Spot-check by listing canonical names so a registry trim shows up
	// in the test output rather than via opaque "missing key" errors.
	canonicals := make([]string, 0, len(pluginsdkroot.CapabilityRegistry))
	for _, decl := range pluginsdkroot.CapabilityRegistry {
		canonicals = append(canonicals, decl.Canonical)
	}
	sort.Strings(canonicals)
	t.Logf("registered canonicals (%d): %v", len(canonicals), canonicals)
}

func mustContain(t *testing.T, haystack []string, needle string) {
	t.Helper()
	for _, s := range haystack {
		if s == needle {
			return
		}
	}
	t.Fatalf("expected %q in %v", needle, haystack)
}
