// Package service — plugin_settings_service.go
//
// V5 W3 — host-side service for the SettingsExtension capability.
// The service owns three responsibilities:
//
//  1. Schema lifecycle: each time a plugin starts the host receives a
//     manifest carrying an optional JSON Schema + defaults. The service
//     compiles the schema (santhosh-tekuri/jsonschema), persists it for
//     the admin UI to render, and seeds any missing defaults into
//     plugin_settings so plugins can rely on Settings.Get returning a
//     value immediately after install.
//
//  2. Read/write API: GetByKey / SetByKey are used both by the admin
//     handler (writes) and by the SettingsExtension gRPC server (plugin
//     reads). Writes validate against the cached compiled schema; missing
//     schemas reject writes loudly so silent drift is impossible.
//
//  3. In-process pub/sub: Subscribe returns a channel that receives
//     SettingsChange events as soon as any code path mutates the table.
//     The fan-out is intentionally in-process — sub2api currently runs
//     as a single instance and a future multi-instance deployment can
//     swap the implementation behind a pub/sub abstraction without
//     touching the gRPC server. The doc (§3.6) says "host MAY use
//     LISTEN/NOTIFY"; we keep the option open by routing all writes
//     through notify().
//
// File layout (V5 W3 split — keep each file under the 500-line house
// limit):
//   - plugin_settings_types.go   — shared types/errors/constants
//   - plugin_settings_schema.go  — schema lifecycle (RegisterSchema,
//     compile, extractMeta, validate, seed)
//   - plugin_settings_io.go      — Get/Set + SchemaInfo + ListPlugins
//   - plugin_settings_pubsub.go  — Subscribe / notify / dropAll
//   - plugin_settings_service.go — struct definition + constructor (this file)
package service

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// PluginSettingsService owns persistence + validation + fan-out.
type PluginSettingsService struct {
	db     *sql.DB
	logger *slog.Logger

	mu              sync.RWMutex
	compiledSchemas map[string]*jsonschema.Schema          // by plugin name
	rawSchemas      map[string]json.RawMessage             // last raw schema bytes for GET
	defaults        map[string]json.RawMessage             // last raw defaults for GET
	schemaVersions  map[string]string                      // per-plugin schema version, normalised to "0" when undeclared (V5/W6 SETTINGS-V2)
	propertiesMeta  map[string]map[string]PropertyMetadata // per-plugin {prop -> {visibility,deprecated,requires_reload}} (V5/W6 SETTINGS-V2)

	subMu sync.RWMutex
	subs  map[string][]*pluginSettingsSubscriber

	subID atomic.Uint64
}

// NewPluginSettingsService wires up the service. db must point at the
// shared *sql.DB (same one PluginManager uses); the migration in
// 102_plugin_settings.sql must have run before any call.
func NewPluginSettingsService(db *sql.DB) *PluginSettingsService {
	return &PluginSettingsService{
		db:              db,
		logger:          slog.Default().With("component", "plugin_settings"),
		compiledSchemas: make(map[string]*jsonschema.Schema),
		rawSchemas:      make(map[string]json.RawMessage),
		defaults:        make(map[string]json.RawMessage),
		schemaVersions:  make(map[string]string),
		propertiesMeta:  make(map[string]map[string]PropertyMetadata),
		subs:            make(map[string][]*pluginSettingsSubscriber),
	}
}
