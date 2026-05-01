package plugin

import (
	"log/slog"
	"time"

	pluginsdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// =============================================================
// Capability allow-list / legacy-rename helpers
//
// T3 已经把单源真相迁移到 pluginsdkroot.CapabilityRegistry。本文件
// 仅提供从 registry 派生 host 侧三张视图(allow-set / legacy → canonical
// / canonical → legacy aliases)的 builder 与运行时归一化函数。
// =============================================================

// allowedPluginCapabilities is the universe of capabilities the core honours.
// Any value outside this set is rejected even if a plugin requests it.
//
// The set is derived once at process start from pluginsdkroot.CapabilityRegistry
// — that slice is the single source of truth shared by SDK and host. Adding a
// new capability is a one-line append in plugin-sdk/capabilities.go; the host
// picks it up automatically (and the compiler catches typos because legacy
// aliases reference Capability* constants).
var allowedPluginCapabilities = buildAllowedCapabilities()

// legacyCapabilityRenames maps deprecated snake_case capability names to the
// canonical dotted-lowercase names. The host normalises the manifest's
// Capabilities slice via normalizeCapability so the rest of the codebase only
// deals with the new names. Drives the deprecation WARN at register time.
var legacyCapabilityRenames = buildLegacyRenames()

// canonicalToLegacyAliases is the reverse of legacyCapabilityRenames. When
// the host pushes the approved capability list to the plugin via
// PluginInitRequest, we expand each canonical name with every legacy alias
// (if any) so plugin code that still hardcodes the legacy string sees itself
// as granted. Internal host registries (route gate, SQL gate, redis raw key
// gate) only ever see the canonical form.
//
// One canonical may have multiple aliases (e.g. there might be more than one
// historical spelling). The slice preserves declaration order from the SDK
// registry; aliases are emitted in that order.
var canonicalToLegacyAliases = buildCanonicalToLegacy()

// buildAllowedCapabilities reads pluginsdkroot.CapabilityRegistry once and
// flattens both canonical names and every legacy alias into a single set.
// Both spellings are accepted at the manifest boundary; normalizeCapability
// collapses them to canonical before they reach any host registry.
func buildAllowedCapabilities() map[string]struct{} {
	out := make(map[string]struct{}, len(pluginsdkroot.CapabilityRegistry)*2)
	for _, decl := range pluginsdkroot.CapabilityRegistry {
		out[decl.Canonical] = struct{}{}
		for _, alias := range decl.LegacyAliases {
			out[alias] = struct{}{}
		}
	}
	return out
}

// buildLegacyRenames flattens the SDK registry into a legacy → canonical map.
// When multiple canonical entries share an alias (e.g. settings_extension is
// listed under both settings.own.read and settings.own.write to allow
// downstream expansion in both directions), the first canonical encountered
// wins — by convention the registry orders the read variant first so legacy
// callers default to the safer permission.
func buildLegacyRenames() map[string]string {
	out := make(map[string]string)
	for _, decl := range pluginsdkroot.CapabilityRegistry {
		for _, alias := range decl.LegacyAliases {
			if _, ok := out[alias]; ok {
				continue
			}
			out[alias] = decl.Canonical
		}
	}
	return out
}

// buildCanonicalToLegacy flattens the SDK registry into a canonical → []alias
// map. Used only at the PluginInitRequest boundary to re-emit legacy aliases
// alongside canonical names so plugins that still hardcode the old strings
// continue to feature-detect correctly.
func buildCanonicalToLegacy() map[string][]string {
	out := make(map[string][]string, len(pluginsdkroot.CapabilityRegistry))
	for _, decl := range pluginsdkroot.CapabilityRegistry {
		if len(decl.LegacyAliases) == 0 {
			continue
		}
		aliases := make([]string, len(decl.LegacyAliases))
		copy(aliases, decl.LegacyAliases)
		out[decl.Canonical] = aliases
	}
	return out
}

// expandWithLegacyAliases returns the input slice augmented with every legacy
// alias listed in canonicalToLegacyAliases, deduplicated. Order: canonical
// names first in input order, then aliases appended at the end. Used only at
// the PluginInitRequest boundary — registry stores canonical only.
func expandWithLegacyAliases(canonical []string) []string {
	if len(canonical) == 0 {
		return canonical
	}
	out := make([]string, 0, len(canonical)*2)
	seen := make(map[string]struct{}, len(canonical)*2)
	for _, c := range canonical {
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	for _, c := range canonical {
		for _, legacy := range canonicalToLegacyAliases[c] {
			if _, dup := seen[legacy]; dup {
				continue
			}
			seen[legacy] = struct{}{}
			out = append(out, legacy)
		}
	}
	return out
}

// normalizeCapability returns the canonical dotted-lowercase capability name
// for a possibly-legacy input. When the input is a known legacy alias the
// returned `renamed` flag is true and `canonical` carries the new name; the
// caller is expected to log a deprecation WARN. Unknown / already-canonical
// inputs are returned unchanged with renamed=false.
func normalizeCapability(name string) (canonical string, renamed bool) {
	if c, ok := legacyCapabilityRenames[name]; ok {
		return c, true
	}
	return name, false
}

// approveCapabilities filters the capabilities a plugin requested in its
// manifest down to the subset the core is willing to grant. Unknown values
// are dropped with a warning so operators can spot typos in plugin manifests.
//
// The returned slice is what we forward to the plugin via PluginInitRequest;
// it doubles as the authoritative list the SDK server uses to police Do
// requests with raw_key=true.
func approveCapabilities(pluginName string, requested []string, logger *slog.Logger) []string {
	if len(requested) == 0 {
		return nil
	}
	out := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, c := range requested {
		if _, ok := allowedPluginCapabilities[c]; !ok {
			if logger != nil {
				logger.Warn("plugin requested unknown capability — ignored",
					"plugin", pluginName, "capability", c)
			}
			continue
		}
		// Normalise legacy snake_case → dotted-lowercase. The rest of the
		// host (capability registry, route gate, SQL gate) only deals with
		// the canonical form so the legacy aliases stay isolated to the
		// allow-list and this rename map. P12·B-1.
		canonical, renamed := normalizeCapability(c)
		if renamed && logger != nil {
			logger.Warn("plugin uses deprecated capability name — please migrate",
				"plugin", pluginName,
				"deprecated", c,
				"replacement", canonical)
		}
		if _, dup := seen[canonical]; dup {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
	}
	return out
}

// =============================================================
// Outbound defaults pushed at PluginInitRequest time
// =============================================================

// defaultOutboundBlockedCIDRs is the host-side default block list pushed to
// every plugin at Init time. Plugins can layer ExtraBlockedCIDRs on top via
// pluginsdk.OutboundConfig but cannot remove entries. See V5-DESIGN §W4.
var defaultOutboundBlockedCIDRs = []string{
	"127.0.0.0/8",    // IPv4 loopback
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918
	"192.168.0.0/16", // RFC1918
	"169.254.0.0/16", // link-local (cloud metadata 169.254.169.254)
	"100.64.0.0/10",  // CGNAT
	"0.0.0.0/8",      // "this network"
	"224.0.0.0/4",    // multicast
	"::1/128",        // IPv6 loopback
	"fc00::/7",       // IPv6 ULA
	"fe80::/10",      // IPv6 link-local
}

// buildOutboundDefaults produces the OutboundDefaults the core pushes to a
// plugin at Init. V5 keeps this as a pure-default helper — once W3
// SettingsExtension lands, the admin-configured block/allow-lists will be
// merged in here.
func (m *PluginManager) buildOutboundDefaults() *pluginsdk.OutboundDefaults {
	return &pluginsdk.OutboundDefaults{
		BlockedCidrs: defaultOutboundBlockedCIDRs,
		AllowedHosts: nil, // empty = no global host allow-list
		MaxRedirects: 3,
		TimeoutNanos: int64(30 * time.Second),
		MaxBodyBytes: 1 << 20, // 1 MiB
	}
}
