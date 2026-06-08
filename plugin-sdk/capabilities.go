package pluginsdk

// CapabilityDecl declares one capability the plugin runtime knows about.
//
// Canonical is the dotted-lowercase form (e.g. "settings.own.read") introduced
// in P12·B-1; new code must reference the matching Capability* constant.
//
// LegacyAliases is the list of deprecated snake_case / older spellings that
// older plugin manifests may still use. The host normalises every alias to
// Canonical before storing it in any internal registry; conversely it expands
// the approved canonical list back into all aliases when shipping
// PluginInitRequest.capabilities to the plugin process, so plugins that hard-
// code a legacy string keep working.
//
// Description is documentation only — it is not used by host or SDK runtime.
type CapabilityDecl struct {
	Canonical     string
	LegacyAliases []string
	Description   string
}

// CapabilityRegistry is the single source of truth for every capability the
// plugin runtime understands. Both the SDK runner (plugin-sdk/runner.go) and
// the host (backend/internal/plugin/manager.go) consume this slice — the host
// builds its allow-list / legacy-rename / canonical-to-legacy maps from it,
// and the SDK derives feature gates by looking the canonical name up here.
//
// Adding a new capability: append one entry below. The host picks it up at
// process start (via buildCapabilityRegistry); no other host wiring needed.
//
// Order is documentation only — host and SDK must not depend on it.
var CapabilityRegistry = []CapabilityDecl{
	// --- Default-grant tier ---------------------------------------------
	{Canonical: CapabilityHTTPRegisterPlugin, Description: "Register HTTP handlers under /plugins/<name>/* (default-grant)."},
	{Canonical: CapabilityJobsRegister, LegacyAliases: []string{CapabilityJobScheduler}, Description: "Declare scheduled jobs (interval/cron/fixed_delay)."},
	{Canonical: CapabilitySettingsOwnRead, LegacyAliases: []string{CapabilitySettingsExtension}, Description: "Read own settings tab; implicit when SettingsSchema is shipped."},
	{Canonical: CapabilitySettingsOwnWrite, LegacyAliases: []string{CapabilitySettingsExtension}, Description: "Write own settings tab; rare — most settings are admin-edited via host UI."},
	{Canonical: CapabilityEventsSubscribeLowfreq, Description: "Subscribe to low-frequency host events (e.g. payment.order.created)."},
	{Canonical: CapabilityRedisOwn, Description: "Read/write own namespaced Redis keys (default-grant)."},
	{Canonical: CapabilityDBOwnRead, Description: "Read own tables (default-grant; SQL gate enforced)."},
	{Canonical: CapabilityDBOwnWrite, Description: "Write own tables (default-grant; SQL gate enforced)."},
	{Canonical: CapabilityMigrationsApply, Description: "Ship migrations the host applies (implicit when Migrations non-empty)."},

	// --- Declare-required tier ------------------------------------------
	{Canonical: CapabilityHTTPRegisterGateway, Description: "Register HTTP handlers on gateway paths (/v1/*)."},
	{Canonical: CapabilityEventsSubscribeGateway, LegacyAliases: []string{CapabilityLegacyEventsGateway}, Description: "Subscribe to high-frequency gateway events."},
	{Canonical: CapabilitySecretsEncrypt, LegacyAliases: []string{CapabilitySecretEncryption}, Description: "Call PluginContext.Secrets().Encrypt/Decrypt with AES-256-GCM."},
	{Canonical: CapabilityOutboundHTTP, LegacyAliases: []string{CapabilityLegacySafeOutboundHTTP}, Description: "Plugin outbound HTTP via SafeOutboundHTTP client."},
	{Canonical: CapabilityRedisRaw, LegacyAliases: []string{CapabilityRedisRawKeys}, Description: "Bypass per-plugin Redis namespace (PluginContext.Redis().Raw())."},
	{Canonical: CapabilityDBCoreRead, Description: "Read host-shared core tables (whitelisted)."},
	{Canonical: CapabilityDBCoreWrite, Description: "Write host-shared core tables (DANGEROUS; Phase 2 admin-approve)."},
	{Canonical: CapabilityEventsPublishPayment, Description: "Publish payment.* HostEvents via EventsExtension.Publish."},
}

// IsKnownCapability reports whether name (canonical OR legacy alias) appears
// anywhere in CapabilityRegistry. Returns true for both spellings of an
// aliased capability so manifest validation accepts either form.
//
// Built lazily inside the function so test fixtures or downstream code can
// modify CapabilityRegistry before the first call (registry is package-level
// so practical mutation is a programming error, but lazy init keeps the API
// future-proof).
func IsKnownCapability(name string) bool {
	for _, decl := range CapabilityRegistry {
		if decl.Canonical == name {
			return true
		}
		for _, alias := range decl.LegacyAliases {
			if alias == name {
				return true
			}
		}
	}
	return false
}

// CanonicalCapability returns the canonical (dotted-lowercase) form of name.
// When name is already canonical or unknown, name is returned unchanged with
// renamed=false. When name is a legacy alias the canonical name is returned
// with renamed=true so callers can emit a deprecation warning.
//
// Aliasing is many-to-one: the legacy "settings_extension" maps to
// CapabilitySettingsOwnRead (the read variant) — a plugin that needs write
// access must declare CapabilitySettingsOwnWrite explicitly. Host-side
// expansion (LegacyAliasesFor) re-emits both legacy aliases when either
// canonical is approved so existing plugins continue to see the old name.
func CanonicalCapability(name string) (canonical string, renamed bool) {
	for _, decl := range CapabilityRegistry {
		for _, alias := range decl.LegacyAliases {
			if alias == name && decl.Canonical != name {
				return decl.Canonical, true
			}
		}
	}
	return name, false
}

// LegacyAliasesFor returns the legacy aliases that should be emitted alongside
// the given canonical capability when the host pushes the approved list to a
// plugin via PluginInitRequest. Returns nil when no aliases exist.
//
// Plugins that still hard-code a legacy capability string (e.g.
// "redis_raw_keys") need to see the alias in the approved list to enable the
// corresponding feature gate; without this expansion, upgrading the SDK would
// silently break them.
func LegacyAliasesFor(canonical string) []string {
	for _, decl := range CapabilityRegistry {
		if decl.Canonical == canonical {
			if len(decl.LegacyAliases) == 0 {
				return nil
			}
			out := make([]string, len(decl.LegacyAliases))
			copy(out, decl.LegacyAliases)
			return out
		}
	}
	return nil
}

// CapabilityMatches reports whether `granted` (a single entry from the
// approved capability list received via PluginInitRequest) authorises the
// `wanted` canonical capability — accepting either the canonical form or any
// of its declared legacy aliases.
//
// Used by the SDK runner to drive feature-gate booleans without spreading
// "if c == X || c == Y" over the call sites. New canonical names added to
// CapabilityRegistry are recognised automatically.
func CapabilityMatches(granted, wanted string) bool {
	if granted == wanted {
		return true
	}
	for _, decl := range CapabilityRegistry {
		if decl.Canonical != wanted {
			continue
		}
		for _, alias := range decl.LegacyAliases {
			if alias == granted {
				return true
			}
		}
		return false
	}
	return false
}

// CapabilityGrantedAny reports whether any entry in `granted` authorises the
// `wanted` canonical capability per CapabilityMatches.
func CapabilityGrantedAny(granted []string, wanted string) bool {
	for _, g := range granted {
		if CapabilityMatches(g, wanted) {
			return true
		}
	}
	return false
}
