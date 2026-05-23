package pluginsdk

// Authentication kinds accepted by the core gateway. They are mirrored as
// strings on EndpointDecl.AuthType because the proto API uses bare strings.
const (
	AuthTypeAdmin  = "admin"
	AuthTypeUser   = "user"
	AuthTypeAPIKey = "apikey"
	AuthTypeNone   = "none"
)

// Section names used for grouping menu items in the frontend.
const (
	SectionAdmin = "admin"
	SectionUser  = "user"
)

// Capability identifiers a plugin may declare in Manifest.Capabilities.
//
// The SDK server treats these as fine-grained permissions: a plugin lacking
// a capability is rejected when it tries to use the corresponding feature.
// Adding a new capability requires updating both the SDK and the core's
// allow-list (backend/internal/plugin/manager.go::allowedPluginCapabilities).
//
// Naming convention (P12·B-1 onward): "<resource>.<action>" dotted-lowercase.
// The legacy snake_case constants below are kept as deprecated aliases for
// one release cycle; the host normalises them to the canonical form when
// the plugin Init runs and emits a migration WARN.
//
// The constants are grouped into three tiers:
//   - default-grant: declarative-only, host always grants without enforcement
//     (used by manifest tooling / docs).
//   - declare-required: must be listed in the manifest; the host auto-grants
//     when listed and gates the corresponding host-side feature.
//   - admin-approve: future Phase 2; not yet wired but reserved.
const (
	// --- Default-grant tier ----------------------------------------------
	// CapabilityHTTPRegisterPlugin lets a plugin register HTTP handlers under
	// /plugins/<name>/* (its own namespace). Always granted.
	CapabilityHTTPRegisterPlugin = "http.register.plugin"
	// CapabilityJobsRegister grants access to PluginContext.Jobs() for
	// declaring scheduled work (interval / cron / fixed_delay). The host owns
	// the schedule clock and per-(plugin, job) leader lock; the plugin owns
	// the handler. See V5-DESIGN §2 (W2 JobSchedulerCapability).
	CapabilityJobsRegister = "jobs.register"
	// CapabilitySettingsOwnRead lets a plugin read its own settings tab.
	// Implicit when the plugin ships a SettingsSchema.
	CapabilitySettingsOwnRead = "settings.own.read"
	// CapabilitySettingsOwnWrite lets a plugin write to its own settings tab
	// (rare — most settings are admin-edited via the host UI).
	CapabilitySettingsOwnWrite = "settings.own.write"
	// CapabilityEventsSubscribeLowfreq lets a plugin subscribe to
	// low-frequency host events (e.g. payment.order.created). High-frequency
	// gateway events require CapabilityEventsSubscribeGateway separately.
	CapabilityEventsSubscribeLowfreq = "events.subscribe.lowfreq"
	// CapabilityRedisOwn lets a plugin read/write its own namespaced Redis
	// keys (plugin:<name>:*). Always granted.
	CapabilityRedisOwn = "redis.own"
	// CapabilityDBOwnRead lets a plugin read from tables it owns (declared
	// via Manifest.OwnedTables). Always granted; enforced by the SQL gate.
	CapabilityDBOwnRead = "db.own.read"
	// CapabilityDBOwnWrite lets a plugin write to tables it owns. Always
	// granted; enforced by the SQL gate.
	CapabilityDBOwnWrite = "db.own.write"
	// CapabilityMigrationsApply lets a plugin ship migrations the host
	// applies. Implicit when Manifest.Migrations is non-empty.
	CapabilityMigrationsApply = "migrations.apply"

	// --- Declare-required tier -------------------------------------------
	// CapabilityHTTPRegisterGateway lets a plugin register HTTP handlers on
	// gateway paths (/v1/*). Required when GatewayEndpoints are declared.
	CapabilityHTTPRegisterGateway = "http.register.gateway"
	// CapabilityEventsSubscribeGateway gates subscribing to high-frequency
	// gateway events (currently gateway.model.invoked). Replaces the legacy
	// "events.gateway" name.
	CapabilityEventsSubscribeGateway = "events.subscribe.gateway"
	// CapabilitySecretsEncrypt authorises a plugin to call
	// PluginContext.Secrets().Encrypt/Decrypt. The host derives a
	// per-plugin key from its master key on first use and uses AES-256-GCM
	// with the plugin name as AAD. Replaces the legacy "secret_encryption".
	CapabilitySecretsEncrypt = "secrets.encrypt"
	// CapabilityOutboundHTTP authorises plugin outbound HTTP via the SDK's
	// SafeOutboundHTTP client. Replaces the legacy "safe_outbound_http".
	CapabilityOutboundHTTP = "outbound.http"
	// CapabilityRedisRaw lets PluginContext.Redis().Raw() bypass the
	// per-plugin Redis key namespace. Required for plugins that intentionally
	// share keys with other components (e.g. channel-management writes the
	// gateway cache contract documented in GATEWAY_CACHE_SPEC.md). Replaces
	// the legacy "redis_raw_keys".
	CapabilityRedisRaw = "redis.raw"
	// CapabilityDBCoreRead lets a plugin read host-shared core tables (a
	// curated whitelist: users, accounts, payment_orders, …). Phase 2:
	// admin-approve is planned; today the host auto-grants when declared.
	CapabilityDBCoreRead = "db.core.read"
	// CapabilityDBCoreWrite lets a plugin write to host-shared core tables.
	// DANGEROUS — Phase 2: admin-approve is required. Today the host
	// auto-grants when declared but this is intended to change.
	CapabilityDBCoreWrite = "db.core.write"
	// CapabilityEventsPublishPayment lets a plugin emit payment.* HostEvents
	// via EventsExtension.Publish (e.g. payment.order.created). The host
	// validates the capability before fan-out so anonymous / unauthorised
	// plugins cannot inject fake payment events into other subscribers.
	CapabilityEventsPublishPayment = "events.publish.payment"

	// --- Legacy snake_case aliases (deprecated; one-release migration) --
	// CapabilityRedisRawKeys is the legacy alias for CapabilityRedisRaw.
	// Deprecated: migrate to CapabilityRedisRaw.
	CapabilityRedisRawKeys = "redis_raw_keys"
	// CapabilitySecretEncryption is the legacy alias for CapabilitySecretsEncrypt.
	// Deprecated: migrate to CapabilitySecretsEncrypt.
	CapabilitySecretEncryption = "secret_encryption"
	// CapabilityJobScheduler is the legacy alias for CapabilityJobsRegister.
	// Deprecated: migrate to CapabilityJobsRegister.
	CapabilityJobScheduler = "job_scheduler"
	// CapabilitySettingsExtension is the legacy alias for CapabilitySettingsOwnRead.
	// Deprecated: migrate to CapabilitySettingsOwnRead (and OwnWrite if needed).
	CapabilitySettingsExtension = "settings_extension"
	// CapabilityLegacyEventsGateway is the legacy alias for
	// CapabilityEventsSubscribeGateway.
	// Deprecated: migrate to CapabilityEventsSubscribeGateway.
	CapabilityLegacyEventsGateway = "events.gateway"
	// CapabilityLegacySafeOutboundHTTP is the legacy alias for
	// CapabilityOutboundHTTP. Deprecated: migrate to CapabilityOutboundHTTP.
	CapabilityLegacySafeOutboundHTTP = "safe_outbound_http"
)

// Manifest is the Go-level representation of pluginsdk.ManifestResponse.
// Plugins build and return this from Plugin.Manifest(); the SDK converts it
// to the proto type when the core calls GetManifest.
//
// Frontend / settings types (FrontendManifest, MenuItemDecl, RouteDecl,
// SettingsSchemaDoc, PropertyMetadata) live in manifest_frontend.go.
// proto conversion (toProto and helpers) lives in manifest_proto.go — both
// share this package and operate directly on the types defined here.
type Manifest struct {
	Name        string
	DisplayName string
	Version     string
	Description string
	Author      string

	// GatewayEndpoints declare HTTP routes that the core's gateway should
	// forward to this plugin. They are typically user-facing API paths.
	GatewayEndpoints []EndpointDecl
	// PluginEndpoints declare HTTP routes that the plugin serves on its own
	// HTTP server (admin/management paths).
	PluginEndpoints []EndpointDecl

	Frontend *FrontendManifest

	// MigrationFiles is an optional list of SQL migration filenames the
	// plugin ships with. The core handles applying them.
	//
	// Deprecated: prefer Migrations, which carries the SHA-256 pin the host
	// re-verifies against the body fetched via PluginLifecycle.GetMigration.
	// MigrationFiles entries arrive at the host without a checksum, so the
	// host cannot apply them — they are logged for visibility only. New
	// plugins should populate Migrations and implement MigrationProvider.
	MigrationFiles []string

	// Migrations declares the SQL migration files the plugin ships embedded.
	// Each entry pins a SHA-256 checksum the host re-verifies against the
	// body fetched via PluginLifecycle.GetMigration. The plugin must
	// implement MigrationProvider so the SDK runner can serve those bodies.
	//
	// Files are applied in lexicographical order of Filename. Ordering and
	// existing checksums are immutable once shipped: append-only edits keep
	// historical plugin_migrations rows valid.
	Migrations []MigrationDecl

	// Capabilities lists the privileged SDK features this plugin needs. See
	// the Capability* constants. The core checks each entry against an
	// allow-list and only forwards the approved ones to the plugin runtime
	// via PluginInitRequest.capabilities.
	Capabilities []string

	// SettingsSchema is the optional JSON Schema (Draft-07) describing the
	// plugin's admin-tunable runtime settings. When set, the host renders
	// it as a tab on the admin Settings page. Leaving it empty means the
	// plugin contributes no settings. See SettingsSchemaDoc for the
	// preferred construction helper.
	SettingsSchema *SettingsSchemaDoc

	// IconSVG is the complete SVG markup (including the outer <svg> tag)
	// the admin plugin-management page renders next to the plugin display
	// name. Use one of the Icon* constants in this package or supply your
	// own. Empty falls back to the host's generic plugin icon.
	IconSVG string

	// SubscribedEvents declares which HostEvent types this plugin wants to
	// receive via EventsExtension.Subscribe. See plugin-sdk/proto/sdk.proto
	// for the list of available event types.
	SubscribedEvents []string

	// OwnedTables lists the host DB tables this plugin owns / reads / writes.
	// Used by the SQL allow-list at runtime (P12·B-1): queries against tables
	// not listed here (and not in the host-shared whitelist when the plugin
	// holds db.core.read / db.core.write) are rejected with PERMISSION_DENIED.
	//
	// Naming: lowercase, snake_case table names, including the optional schema
	// prefix (e.g. "channel_pricings", "channel_monitors").
	//
	// Plugins SHOULD list every table they create in migrations and any
	// shared host table they read/write (the latter requires db.core.read or
	// db.core.write capability).
	OwnedTables []string

	// PublicFlags declares public-facing settings flags the host should expose
	// alongside its own GetPublicSettings response. Each entry maps a key
	// (which appears in the public_settings JSON envelope shipped to the SSR /
	// browser bootstrap payload) to either a static default or a
	// settings-store lookup. The host queries the value without going through
	// the plugin process, so a degraded / stopped plugin does not break the
	// public bootstrap.
	//
	// Phase 0: Go-side declaration only. The proto wire field
	// (ManifestResponse.public_flags) is reserved for a follow-up commit that
	// regenerates plugin.proto bindings; until then host wiring reads
	// PublicFlags directly from the in-memory Manifest struct returned by
	// the plugin runner.
	PublicFlags []PublicFlagDecl

	// SettingsComponentPath optionally names a Vue component the plugin
	// frontend exports under the same VIEWS map as route components. When
	// set, the host's settings dialog mounts this component instead of
	// rendering the generic JSON-schema form. Empty string = fall back to
	// the schema renderer.
	//
	// The named component receives the current settings values plus a save
	// callback as props; see frontend/src/components/admin/PluginSettingsForm.vue
	// for the host-side wiring.
	SettingsComponentPath string

	// Platforms declares the account platforms this gateway plugin provides.
	// Each platform defines display metadata, account types, and UI
	// customization. The host registers these into PlatformRegistry at
	// plugin load time. See manifest_platform.go for type definitions.
	Platforms []PlatformDecl
}

// PublicFlagSource enumerates where the host should read a public flag's
// value when GetPublicSettings is called.
const (
	// PublicFlagSourceSettings reads the value from the plugin's
	// PluginSettingsService entry. Falls back to Default if missing.
	PublicFlagSourceSettings = "settings"
	// PublicFlagSourceStatic always returns the declared Default. Useful
	// for compile-time constants exposed to the frontend (e.g. plugin
	// version, build channel).
	PublicFlagSourceStatic = "static"
)

// PublicFlagType enumerates the JSON value types a public flag may take.
// Other types are rejected at manifest-validation time so the host can
// safely cast them.
const (
	PublicFlagTypeBool   = "bool"
	PublicFlagTypeString = "string"
	PublicFlagTypeNumber = "number"
)

// PublicFlagDecl is a single entry in Manifest.PublicFlags.
type PublicFlagDecl struct {
	// Key is the field name surfaced in PublicSettings JSON. SHOULD be
	// snake_case to match the dominant convention (e.g.
	// "payment_enabled").
	Key string
	// Source is one of PublicFlagSource* constants.
	Source string
	// SettingsKey is the path inside the plugin's settings document
	// (e.g. "enabled" or "stripe.publishable_key"). Required when
	// Source == PublicFlagSourceSettings.
	SettingsKey string
	// Type is one of PublicFlagType* constants. Drives JSON encoding so
	// the SSR bootstrap renders the correct primitive.
	Type string
	// Default is the value used when SettingsKey is unset / Source is
	// static. Must be JSON-serialisable (bool, string, float64).
	Default any
}

// Placement describes where on a sidebar a plugin's menu item should land.
// Pointing a MenuItemDecl at a Placement opts that item into the V5/W7
// "Placement DSL" merge algorithm; leaving Placement nil keeps the legacy
// SortOrder path (item is appended at the end of its section).
//
// Group is one of the Placement* constants below. Order is the relative
// position *inside* that bucket — lower renders first.
type Placement struct {
	Group string
	Order int
}

// Placement* are the known sidebar buckets the host honours. Plugins should
// always use these constants instead of bare strings so a typo fails to
// compile rather than silently landing in the fallback bucket.
const (
	PlacementAdminMain   = "admin/main"
	PlacementAdminSystem = "admin/system"
	PlacementAdminEnd    = "admin/end"
	PlacementUserMain    = "user/main"
	PlacementUserEnd     = "user/end"
)

// EndpointDecl describes a single HTTP endpoint declaration.
type EndpointDecl struct {
	Path     string
	Methods  []string
	AuthType string // one of AuthType* constants
}

// MigrationDecl describes a single SQL migration the plugin ships embedded.
// The host fetches the body via PluginLifecycle.GetMigration and re-verifies
// the SHA-256 against ChecksumSha256 before applying it. It mirrors the
// pluginsdk.MigrationDecl proto message; see its docstring for the full
// lifecycle.
type MigrationDecl struct {
	// Filename is applied in lexicographical order, e.g. "001_create_x.sql".
	Filename string

	// ChecksumSha256 is the hex-encoded SHA-256 of the SQL body the plugin
	// intends to ship. The host treats a mismatch with the fetched body as
	// a fatal startup error (drift = potential supply-chain tampering).
	ChecksumSha256 string

	// NonTransactional marks migrations that contain statements PostgreSQL
	// refuses to run inside an explicit transaction (e.g.
	// CREATE INDEX CONCURRENTLY). The host applies these outside BEGIN/COMMIT.
	NonTransactional bool

	// DownFilename names the down migration the plugin ships alongside the up
	// body. Empty string means the migration is irreversible — the host skips
	// executing it during plugin Purge and only deletes bookkeeping rows.
	DownFilename string

	// DownChecksumSHA256 is the hex-encoded SHA-256 of the down SQL body. The
	// host re-verifies it against the fetched body to detect supply-chain
	// tampering — same contract as ChecksumSha256 above.
	DownChecksumSHA256 string
}
