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
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// pluginSettingsSubscriberBuffer is the per-subscriber channel size. We
// keep it small because the SDK side already caches the latest value;
// a slow subscriber simply drops events and recovers via Get.
const pluginSettingsSubscriberBuffer = 8

// schemaVersionUndeclared is the sentinel stored in
// plugin_settings_schemas.schema_version (and plugin_settings.
// schema_version_at_write) when the plugin did not declare a version.
// See SETTINGS-V2-DESIGN §1.2.
const schemaVersionUndeclared = "0"

// PropertyVisibility constants — values allowed in PropertyMetadata.Visibility.
// See SETTINGS-V2-DESIGN §1.4.
const (
	PropertyVisibilityFrontend = "frontend"
	PropertyVisibilityBackend  = "backend"
	PropertyVisibilitySecret   = "secret"
)

// ErrPluginSettingsSchemaMissing is returned when an admin tries to
// write to a plugin namespace that has not registered a schema yet.
// Bouncing the request loudly is preferable to silently storing
// arbitrary JSON the plugin will never read.
var ErrPluginSettingsSchemaMissing = errors.New("plugin_settings: no schema registered for plugin")

// ErrInvalidSchemaVisibility is returned by RegisterSchema when a property
// declares an x-visibility outside the allowed set (frontend|backend|secret).
// Maps to errcode PLUGIN_SETTINGS_SCHEMA_INVALID_VISIBILITY in
// SETTINGS-V2-DESIGN §7.2.
var ErrInvalidSchemaVisibility = errors.New("plugin_settings: x-visibility must be one of frontend|backend|secret")

// ErrPluginSettingsValidation is returned when a write fails JSON Schema
// validation. The admin handler maps it to HTTP 422.
type ErrPluginSettingsValidation struct {
	Plugin string
	Key    string
	Reason string
}

func (e *ErrPluginSettingsValidation) Error() string {
	return fmt.Sprintf("plugin_settings: validation failed for %s/%s: %s", e.Plugin, e.Key, e.Reason)
}

// ErrPluginSettingsBackendOnly is returned by SetByKey when an admin write
// targets a key whose schema declares x-visibility=backend. Maps to errcode
// PLUGIN_SETTINGS_BACKEND_ONLY (HTTP 403) in SETTINGS-V2-DESIGN §7.2.
type ErrPluginSettingsBackendOnly struct {
	Plugin string
	Key    string
}

func (e *ErrPluginSettingsBackendOnly) Error() string {
	return fmt.Sprintf("plugin_settings: %s/%s is backend-only and not writable via admin API", e.Plugin, e.Key)
}

// SetSource identifies the caller of SetByKey so the service can apply
// visibility rules. Admin sources are subject to the backend-only guard;
// internal sources skip it. See SETTINGS-V2-DESIGN §4.3.
type SetSource int

const (
	// SetSourceUnknown is the zero value. Treated as admin for safety so a
	// caller that forgets to set a source cannot accidentally bypass the
	// backend-only guard.
	SetSourceUnknown SetSource = iota
	// SetSourceAdmin is the admin REST handler. Backend-only writes are
	// rejected with ErrPluginSettingsBackendOnly.
	SetSourceAdmin
	// SetSourceInternal is host-side code (jobs, hooks, plugin manager
	// reload) that may legitimately mutate backend-only keys.
	SetSourceInternal
)

// PropertyMetadata mirrors the per-property marker triple from
// plugin_settings_schemas.properties_meta. Re-defined in this package to
// avoid pulling plugin-sdk into the host service import graph; field shape
// matches plugin-sdk/manifest.go PropertyMetadata one-to-one (visibility /
// deprecated / requires_reload). See SETTINGS-V2-DESIGN §1.4.
type PropertyMetadata struct {
	Visibility     string `json:"visibility"`
	Deprecated     string `json:"deprecated"`
	RequiresReload bool   `json:"requires_reload"`
}

// RegisterSchemaInput is the V5/W6 SETTINGS-V2 envelope for plugin schema
// registration. PluginManager builds it from ManifestResponse fields; the
// fields mirror SettingsSchemaDoc (Schema/Defaults/Version/PropertyMeta)
// plus a PluginName scoping the request. See SETTINGS-V2-DESIGN §4.1.
type RegisterSchemaInput struct {
	// PluginName scopes every other field. Empty is rejected.
	PluginName string
	// SchemaJSON is Manifest.SettingsSchema.Schema bytes. Empty triggers
	// schema deletion (existing behaviour preserved).
	SchemaJSON []byte
	// DefaultsJSON is Manifest.SettingsSchema.Defaults bytes. Empty is
	// normalised to "{}".
	DefaultsJSON []byte
	// SchemaVersion is Manifest.SettingsSchema.Version. Empty is
	// normalised to schemaVersionUndeclared ("0") host-side.
	SchemaVersion string
	// PropertiesMetaJSON is the SDK-authoritative serialisation of
	// SettingsSchemaDoc.PropertyMeta (a JSON object keyed by top-level
	// property name). When non-empty it wins; when empty the host derives
	// the meta from x-visibility / x-deprecated / x-requires-reload vendor
	// extensions inside SchemaJSON. See SETTINGS-V2-DESIGN §4.1 / §4.2.
	PropertiesMetaJSON []byte
}

// PluginSettingsChange is the host-side event the gRPC server fans out
// on its Watch streams. It mirrors the SDK's SettingsChange but lives in
// the service package so both the gRPC server and the admin handler can
// import it without forming a cycle.
type PluginSettingsChange struct {
	Plugin   string
	Key      string
	Value    json.RawMessage
	Revision int64
	// RequiresReload mirrors the schema's x-requires-reload marker for the
	// changed key. PluginManager subscribes to this signal to coalesce a
	// plugin reload after admin saves; plugin SDK clients ignore the field.
	// See SETTINGS-V2-DESIGN §4.3 / §4.4.
	RequiresReload bool
}

// PluginSettingsSchemaInfo is the admin-facing description of a plugin's
// current schema + values. The handler returns this from GET; the UI
// renders the schema with vue-json-schema-form (or the fallback).
//
// V5/W6 SETTINGS-V2 fields (SchemaVersion / PropertiesMeta / SecretKeys)
// expose the per-property marker triple recorded in
// plugin_settings_schemas.properties_meta plus the schema_version stamp
// so the admin UI widget map can pick the correct widget per property.
// See DESIGN §4.6.
type PluginSettingsSchemaInfo struct {
	Plugin    string                     `json:"plugin"`
	Schema    json.RawMessage            `json:"schema"`
	Defaults  json.RawMessage            `json:"defaults"`
	Values    map[string]json.RawMessage `json:"values"` // secret keys → null
	UpdatedAt time.Time                  `json:"updated_at"`

	// SchemaVersion mirrors plugin_settings_schemas.schema_version.
	// "0" (schemaVersionUndeclared) when the plugin omitted the field.
	SchemaVersion string `json:"schema_version"`

	// PropertiesMeta is the marker triple per top-level property. Keys
	// match Schema's top-level properties; absent keys default to
	// {visibility:"frontend", deprecated:"", requires_reload:false}.
	PropertiesMeta map[string]PropertyMetadata `json:"properties_meta"`

	// SecretKeys is the sorted list of properties with visibility=="secret"
	// that have a stored value (so the UI can render "已配置"). Values for
	// these keys in the Values map are masked to JSON null. Empty slice
	// when no secrets are configured.
	SecretKeys []string `json:"secret_keys"`
}

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

	subMu sync.Mutex
	subs  map[string][]*pluginSettingsSubscriber

	subID atomic.Uint64
}

type pluginSettingsSubscriber struct {
	id     uint64
	plugin string
	key    string // empty = whole namespace
	ch     chan PluginSettingsChange
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

// RegisterSchema is the legacy entry point retained for callers that
// have not yet migrated to RegisterSchemaWithInput. It forwards to the
// V2 implementation with empty version + properties_meta — the V2
// implementation derives meta from schema vendor extensions in that
// case (see SETTINGS-V2-DESIGN §4.1).
//
// New callers should build a RegisterSchemaInput so they can carry
// SchemaVersion + PropertiesMetaJSON from the plugin manifest.
func (s *PluginSettingsService) RegisterSchema(
	ctx context.Context, pluginName string, schemaJSON, defaultsJSON []byte,
) error {
	return s.RegisterSchemaWithInput(ctx, RegisterSchemaInput{
		PluginName:   pluginName,
		SchemaJSON:   schemaJSON,
		DefaultsJSON: defaultsJSON,
	})
}

// RegisterSchemaWithInput is the V5/W6 SETTINGS-V2 entry point used by
// PluginManager during plugin start. It compiles the schema, persists
// the raw bytes + schema_version + properties_meta for GET responses,
// and seeds defaults for keys that have not been written yet.
//
// Calling RegisterSchemaWithInput again with the same plugin name
// replaces the cached schema. When schema_version changes the service
// drops existing watch subscribers so SDK clients reconnect and pick
// up the new schema_version (see SETTINGS-V2-DESIGN §4.5; the drop
// is wired in commit 2).
func (s *PluginSettingsService) RegisterSchemaWithInput(
	ctx context.Context, in RegisterSchemaInput,
) error {
	if in.PluginName == "" {
		return errors.New("plugin_settings: empty plugin name")
	}
	if len(in.SchemaJSON) == 0 {
		// Nothing to register — clear any cached entry so a previously
		// schema-bearing plugin can drop the requirement on next restart.
		s.mu.Lock()
		delete(s.compiledSchemas, in.PluginName)
		delete(s.rawSchemas, in.PluginName)
		delete(s.defaults, in.PluginName)
		s.mu.Unlock()
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM plugin_settings_schemas WHERE plugin_name = $1`, in.PluginName)
		return err
	}
	compiled, err := compileSchema(in.SchemaJSON)
	if err != nil {
		return fmt.Errorf("plugin_settings: compile schema for %s: %w", in.PluginName, err)
	}

	// Resolve properties_meta: SDK-authoritative bytes win when present,
	// otherwise derive from x-visibility / x-deprecated / x-requires-reload
	// vendor extensions in the schema. See SETTINGS-V2-DESIGN §4.2.
	meta, err := s.resolvePropertiesMeta(in.SchemaJSON, in.PropertiesMetaJSON)
	if err != nil {
		return err
	}
	if err := validateVisibilities(in.PluginName, meta); err != nil {
		return err
	}

	// Normalise the schema version: empty → "0" sentinel (DESIGN §1.2).
	schemaVersion := in.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = schemaVersionUndeclared
	}

	// Marshal meta back to JSON for persistence. The host always writes
	// `{}` (never SQL NULL) so handlers do not need to nil-check.
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("plugin_settings: marshal properties_meta: %w", err)
	}

	rawSchema := append(json.RawMessage(nil), in.SchemaJSON...)
	rawDefaults := append(json.RawMessage(nil), in.DefaultsJSON...)
	if len(rawDefaults) == 0 {
		rawDefaults = json.RawMessage("{}")
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO plugin_settings_schemas
			(plugin_name, schema_json, defaults_json, schema_version, properties_meta, updated_at)
		VALUES ($1, $2::jsonb, $3::jsonb, $4, $5::jsonb, NOW())
		ON CONFLICT (plugin_name) DO UPDATE
		   SET schema_json     = EXCLUDED.schema_json,
		       defaults_json   = EXCLUDED.defaults_json,
		       schema_version  = EXCLUDED.schema_version,
		       properties_meta = EXCLUDED.properties_meta,
		       updated_at      = NOW()
	`, in.PluginName, string(rawSchema), string(rawDefaults), schemaVersion, string(metaJSON)); err != nil {
		return fmt.Errorf("plugin_settings: persist schema: %w", err)
	}

	s.mu.Lock()
	s.compiledSchemas[in.PluginName] = compiled
	s.rawSchemas[in.PluginName] = rawSchema
	s.defaults[in.PluginName] = rawDefaults
	s.schemaVersions[in.PluginName] = schemaVersion
	s.propertiesMeta[in.PluginName] = meta
	s.mu.Unlock()

	if err := s.seedDefaults(ctx, in.PluginName, rawDefaults); err != nil {
		s.logger.Warn("plugin_settings: seed defaults failed",
			"plugin", in.PluginName, "error", err)
	}
	return nil
}

// seedDefaults writes default values for keys that do not exist yet.
// Existing values (whether plugin- or admin-supplied) are left alone.
func (s *PluginSettingsService) seedDefaults(
	ctx context.Context, pluginName string, defaultsJSON json.RawMessage,
) error {
	var defaults map[string]json.RawMessage
	if err := json.Unmarshal(defaultsJSON, &defaults); err != nil {
		return fmt.Errorf("unmarshal defaults: %w", err)
	}
	for key, val := range defaults {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO plugin_settings (plugin_name, key, value_json, revision, updated_at)
			VALUES ($1, $2, $3::jsonb, 1, NOW())
			ON CONFLICT (plugin_name, key) DO NOTHING
		`, pluginName, key, string(val))
		if err != nil {
			return fmt.Errorf("seed default %s/%s: %w", pluginName, key, err)
		}
	}
	return nil
}

// UnregisterSchema clears in-memory caches for a plugin that has been
// disabled. Stored values are left intact so a re-enable does not lose
// the admin's edits.
func (s *PluginSettingsService) UnregisterSchema(pluginName string) {
	s.mu.Lock()
	delete(s.compiledSchemas, pluginName)
	delete(s.rawSchemas, pluginName)
	delete(s.defaults, pluginName)
	delete(s.schemaVersions, pluginName)
	delete(s.propertiesMeta, pluginName)
	s.mu.Unlock()
}

// GetByKey reads one key. With V5/W6 SETTINGS-V2 read-time fallback:
//
//  1. If a row exists in plugin_settings, return its value + revision.
//     The stored row always wins, even when the plugin has registered a
//     newer schema with a different default — admin intent persists
//     across schema upgrades (DESIGN §6.3).
//  2. Else if the plugin's cached schema defaults contain the key,
//     return the default with revision=0. revision=0 is the sentinel
//     for "synthetic default" so callers can distinguish it from a real
//     write (which always carries revision >= 1).
//  3. Else return (nil, 0, sql.ErrNoRows) so the gRPC server can map
//     it to Exists=false.
//
// This makes startup-time seedDefaults a hot-path optimisation rather
// than a correctness requirement: even if seeding has not run yet (e.g.
// it failed transiently), plugin reads still succeed.
func (s *PluginSettingsService) GetByKey(
	ctx context.Context, pluginName, key string,
) (json.RawMessage, int64, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT value_json::text, revision FROM plugin_settings
		WHERE plugin_name = $1 AND key = $2
	`, pluginName, key)
	var raw string
	var rev int64
	switch err := row.Scan(&raw, &rev); {
	case err == nil:
		return json.RawMessage(raw), rev, nil
	case errors.Is(err, sql.ErrNoRows):
		// Fall through to the schema default lookup below.
	default:
		return nil, 0, err
	}

	if def, ok := s.lookupDefault(pluginName, key); ok {
		return def, 0, nil
	}
	return nil, 0, sql.ErrNoRows
}

// lookupDefault returns the schema default for (plugin, key) from the
// cached defaults map. Returns (nil, false) when the plugin is unknown,
// has no defaults, or has no entry for this specific key.
//
// The cache is populated by RegisterSchemaWithInput (and by SchemaInfo's
// lazy reload after a host restart). Returning the bytes by copy keeps
// callers from mutating the cache.
func (s *PluginSettingsService) lookupDefault(plugin, key string) (json.RawMessage, bool) {
	s.mu.RLock()
	rawDefaults := s.defaults[plugin]
	s.mu.RUnlock()
	if len(rawDefaults) == 0 {
		return nil, false
	}
	var defaults map[string]json.RawMessage
	if err := json.Unmarshal(rawDefaults, &defaults); err != nil {
		return nil, false
	}
	v, ok := defaults[key]
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), v...), true
}

// GetAll returns every key for the plugin (admin GET). Empty when the
// plugin has not registered anything yet.
func (s *PluginSettingsService) GetAll(
	ctx context.Context, pluginName string,
) (map[string]json.RawMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT key, value_json::text FROM plugin_settings
		WHERE plugin_name = $1
	`, pluginName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = json.RawMessage(v)
	}
	return out, rows.Err()
}

// SetByKey validates value against the plugin's schema, then upserts.
// On success the new revision is returned and a PluginSettingsChange is
// fanned out to subscribers.
//
// Backward-compat shim: this signature predates the V5/W6 SETTINGS-V2
// SetSource argument. It now forwards to SetByKeyWithSource with
// SetSourceAdmin so legacy callers (notably the admin handler before
// W3-A migrates) implicitly opt into the backend-only guard. New
// callers should invoke SetByKeyWithSource directly with the explicit
// source value (DESIGN §4.3).
func (s *PluginSettingsService) SetByKey(
	ctx context.Context, pluginName, key string, value json.RawMessage,
) (int64, error) {
	return s.SetByKeyWithSource(ctx, pluginName, key, value, SetSourceAdmin)
}

// SetByKeyWithSource is the V5/W6 SETTINGS-V2 entry point used by the
// admin handler (SetSourceAdmin) and host-internal code (SetSourceInternal).
// Visibility=backend keys reject admin writes with
// ErrPluginSettingsBackendOnly; internal sources skip the guard.
//
// The current schema_version is stamped into
// plugin_settings.schema_version_at_write so a later plugin SDK Get can
// detect stale values via SchemaVersionMismatchError.
func (s *PluginSettingsService) SetByKeyWithSource(
	ctx context.Context, pluginName, key string, value json.RawMessage, source SetSource,
) (int64, error) {
	if pluginName == "" || key == "" {
		return 0, errors.New("plugin_settings: empty plugin or key")
	}

	s.mu.RLock()
	compiled, ok := s.compiledSchemas[pluginName]
	meta := s.propertiesMeta[pluginName]
	schemaVersion := s.schemaVersions[pluginName]
	s.mu.RUnlock()
	if !ok {
		return 0, ErrPluginSettingsSchemaMissing
	}

	// V5/W6 SETTINGS-V2: enforce backend-only visibility against admin
	// sources. SetSourceUnknown is treated as admin so a caller that
	// forgot to set the source cannot bypass the guard.
	if source != SetSourceInternal {
		if propMeta, ok := meta[key]; ok && propMeta.Visibility == PropertyVisibilityBackend {
			return 0, &ErrPluginSettingsBackendOnly{Plugin: pluginName, Key: key}
		}
	}

	if err := validateAgainst(compiled, key, value); err != nil {
		return 0, err
	}
	if schemaVersion == "" {
		schemaVersion = schemaVersionUndeclared
	}

	// Mirror the schema's x-requires-reload marker into the change event
	// so the PluginManager reload watcher (W2-C) can decide whether to
	// restart the plugin process.
	requiresReload := false
	propMeta, hasMeta := meta[key]
	if hasMeta {
		requiresReload = propMeta.RequiresReload
	}

	// SETTINGS-V2-INDUSTRY §3 row 11 + Curator decision 2 (Grafana write
	// semantics): for visibility=secret properties, an empty JSON string
	// ("") means "clear this secret". Delete the row instead of writing
	// the literal "" so SchemaInfo's secret_keys list (derived from row
	// existence) drops the key. Non-secret keys keep the literal "" so
	// admin can still legitimately persist an empty string for ordinary
	// text fields. Other falsy shapes (null / 0 / false / []) keep the
	// UPSERT path — only the JSON string "" triggers the delete branch.
	if hasMeta && propMeta.Visibility == PropertyVisibilitySecret && isEmptyJSONString(value) {
		return s.deleteSecretAndNotify(ctx, pluginName, key, requiresReload)
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO plugin_settings (plugin_name, key, value_json, revision, schema_version_at_write, updated_at)
		VALUES ($1, $2, $3::jsonb, 1, $4, NOW())
		ON CONFLICT (plugin_name, key) DO UPDATE
		   SET value_json              = EXCLUDED.value_json,
		       revision                 = plugin_settings.revision + 1,
		       schema_version_at_write  = EXCLUDED.schema_version_at_write,
		       updated_at               = NOW()
		RETURNING revision
	`, pluginName, key, string(value), schemaVersion)
	var rev int64
	if err := row.Scan(&rev); err != nil {
		return 0, fmt.Errorf("plugin_settings: upsert: %w", err)
	}

	s.notify(PluginSettingsChange{
		Plugin:         pluginName,
		Key:            key,
		Value:          append(json.RawMessage(nil), value...),
		Revision:       rev,
		RequiresReload: requiresReload,
	})
	return rev, nil
}

// isEmptyJSONString reports whether value is the JSON string "" (empty
// string literal). It returns false for JSON null, numbers, booleans,
// arrays, objects, or any non-empty string. Used by the secret-clear
// branch in SetByKeyWithSource so the delete semantic only fires on the
// exact shape admins emit when wiping a secret field.
func isEmptyJSONString(value json.RawMessage) bool {
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return false
	}
	return s == ""
}

// deleteSecretAndNotify removes the (plugin, key) row and fans out a
// change event with Value=null so subscribers (notably W2-C reload
// coordination) observe the clear. The returned revision is the cleared
// row's previous revision + 1 so callers see a monotonically increasing
// number even across a delete; if the row did not exist we return 0
// because there is no prior revision to advance from.
func (s *PluginSettingsService) deleteSecretAndNotify(
	ctx context.Context, pluginName, key string, requiresReload bool,
) (int64, error) {
	row := s.db.QueryRowContext(ctx, `
		DELETE FROM plugin_settings
		WHERE plugin_name = $1 AND key = $2
		RETURNING revision
	`, pluginName, key)
	var prev int64
	switch err := row.Scan(&prev); {
	case err == nil:
		// row existed and was deleted; advance revision.
	case errors.Is(err, sql.ErrNoRows):
		// no prior row; nothing to delete, but we still notify so a
		// transient subscriber sees the clear intent.
		prev = 0
	default:
		return 0, fmt.Errorf("plugin_settings: delete secret: %w", err)
	}
	rev := prev + 1
	s.notify(PluginSettingsChange{
		Plugin:         pluginName,
		Key:            key,
		Value:          json.RawMessage("null"),
		Revision:       rev,
		RequiresReload: requiresReload,
	})
	return rev, nil
}

// SchemaInfo returns the cached schema + current values for the admin UI.
// Returns nil when the plugin has not registered anything.
func (s *PluginSettingsService) SchemaInfo(
	ctx context.Context, pluginName string,
) (*PluginSettingsSchemaInfo, error) {
	s.mu.RLock()
	rawSchema, ok := s.rawSchemas[pluginName]
	rawDefaults := s.defaults[pluginName]
	s.mu.RUnlock()
	if !ok {
		// Maybe the host process restarted; load from DB so the admin UI
		// still works without bouncing the plugin. We pull the V5/W6
		// SETTINGS-V2 columns (schema_version, properties_meta) at the
		// same time so the cache is fully populated for downstream calls.
		var schemaStr, defaultsStr, schemaVersionStr, propertiesMetaStr string
		var updated time.Time
		row := s.db.QueryRowContext(ctx, `
			SELECT schema_json::text, defaults_json::text, schema_version,
			       properties_meta::text, updated_at
			FROM plugin_settings_schemas WHERE plugin_name = $1
		`, pluginName)
		if err := row.Scan(&schemaStr, &defaultsStr, &schemaVersionStr, &propertiesMetaStr, &updated); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		rawSchema = json.RawMessage(schemaStr)
		rawDefaults = json.RawMessage(defaultsStr)
		// Decode properties_meta back into the cache shape; tolerate
		// malformed rows by falling back to the schema-derived map so
		// admin UI still renders even if the column was hand-edited.
		meta := make(map[string]PropertyMetadata)
		if propertiesMetaStr != "" && propertiesMetaStr != "null" {
			if err := json.Unmarshal([]byte(propertiesMetaStr), &meta); err != nil {
				s.logger.Warn("plugin_settings: properties_meta decode failed; deriving from schema",
					"plugin", pluginName, "error", err)
				if derived, derr := s.extractMetaFromSchema(rawSchema); derr == nil {
					meta = derived
				}
			}
		}
		if schemaVersionStr == "" {
			schemaVersionStr = schemaVersionUndeclared
		}
		// Compile lazily so subsequent SetByKey calls work.
		if compiled, err := compileSchema(rawSchema); err == nil {
			s.mu.Lock()
			s.compiledSchemas[pluginName] = compiled
			s.rawSchemas[pluginName] = rawSchema
			s.defaults[pluginName] = rawDefaults
			s.schemaVersions[pluginName] = schemaVersionStr
			s.propertiesMeta[pluginName] = meta
			s.mu.Unlock()
		}
	}
	values, err := s.GetAll(ctx, pluginName)
	if err != nil {
		return nil, err
	}

	// V5/W6 SETTINGS-V2: surface schema_version + properties_meta from the
	// in-memory cache populated by RegisterSchemaWithInput / DB reload above.
	// Mask secret values to JSON null and emit a sorted SecretKeys list so
	// the admin UI can render "已配置" without ever seeing the secret bytes
	// (DESIGN §4.6).
	s.mu.RLock()
	schemaVersion := s.schemaVersions[pluginName]
	cachedMeta := s.propertiesMeta[pluginName]
	s.mu.RUnlock()
	metaCopy := make(map[string]PropertyMetadata, len(cachedMeta))
	for k, v := range cachedMeta {
		metaCopy[k] = v
	}
	if schemaVersion == "" {
		schemaVersion = schemaVersionUndeclared
	}
	secretKeys := make([]string, 0)
	for prop, m := range metaCopy {
		if m.Visibility != PropertyVisibilitySecret {
			continue
		}
		if _, ok := values[prop]; ok {
			secretKeys = append(secretKeys, prop)
		}
	}
	sort.Strings(secretKeys) // deterministic order for tests + UI diffs
	for _, k := range secretKeys {
		values[k] = json.RawMessage("null")
	}

	return &PluginSettingsSchemaInfo{
		Plugin:         pluginName,
		Schema:         rawSchema,
		Defaults:       rawDefaults,
		Values:         values,
		SchemaVersion:  schemaVersion,
		PropertiesMeta: metaCopy,
		SecretKeys:     secretKeys,
	}, nil
}

// ListPlugins returns every plugin that has registered a schema. The
// admin UI uses this to build the namespace tab list.
func (s *PluginSettingsService) ListPlugins(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT plugin_name FROM plugin_settings_schemas ORDER BY plugin_name
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]string, 0)
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Subscribe registers a fan-out channel for changes inside one plugin
// namespace. An empty key matches every change for that plugin.
//
// The returned cleanup must be called when the caller is done; it is
// safe to call multiple times.
func (s *PluginSettingsService) Subscribe(
	pluginName, key string,
) (<-chan PluginSettingsChange, func()) {
	sub := &pluginSettingsSubscriber{
		id:     s.subID.Add(1),
		plugin: pluginName,
		key:    key,
		ch:     make(chan PluginSettingsChange, pluginSettingsSubscriberBuffer),
	}
	s.subMu.Lock()
	s.subs[pluginName] = append(s.subs[pluginName], sub)
	s.subMu.Unlock()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			s.subMu.Lock()
			defer s.subMu.Unlock()
			subs := s.subs[pluginName]
			for i, x := range subs {
				if x.id == sub.id {
					close(x.ch)
					s.subs[pluginName] = append(subs[:i], subs[i+1:]...)
					if len(s.subs[pluginName]) == 0 {
						delete(s.subs, pluginName)
					}
					return
				}
			}
		})
	}
	return sub.ch, cleanup
}

func (s *PluginSettingsService) notify(change PluginSettingsChange) {
	s.subMu.Lock()
	subs := append([]*pluginSettingsSubscriber(nil), s.subs[change.Plugin]...)
	s.subMu.Unlock()
	for _, sub := range subs {
		if sub.key != "" && sub.key != change.Key {
			continue
		}
		select {
		case sub.ch <- change:
		default:
			s.logger.Warn("plugin_settings: subscriber channel full, dropping",
				"plugin", change.Plugin, "key", change.Key)
		}
	}
}

// validateAgainst runs jsonschema validation against the sub-schema for
// the supplied key. Writes are key-scoped (admin saves a single property
// at a time), so we deliberately validate the value against the property's
// own sub-schema instead of wrapping it into `{key: value}` and running
// the whole-object validator — wrapping incorrectly triggers top-level
// `required` arrays for properties that are not part of the partial save.
//
// See SETTINGS-V2-INSPECT §4 (table row for `validateAgainst`, line 114):
// the wrapper approach lets `required` enforce on partial saves only when
// the partial object happens to satisfy them. Symptom seen on test deploy:
// `PUT /api/v1/admin/plugin-settings/channel-management/defaultIntervalSec`
// with body `{"value": 120}` returned HTTP 422 because the schema's
// top-level `required: ["enabled"]` could not be satisfied by the wrapper
// `{"defaultIntervalSec": 120}`.
//
// The sub-schema validates the value's own constraints (type / minimum /
// maximum / pattern / enum / format / etc.) without dragging in object-level
// constraints that do not apply to a single-property write.
func validateAgainst(schema *jsonschema.Schema, key string, value json.RawMessage) error {
	sub, ok := schema.Properties[key]
	if !ok {
		return &ErrPluginSettingsValidation{Key: key, Reason: "unknown property: " + key}
	}
	var doc any
	if err := json.Unmarshal(value, &doc); err != nil {
		return &ErrPluginSettingsValidation{Key: key, Reason: "decode value: " + err.Error()}
	}
	if err := sub.Validate(doc); err != nil {
		return &ErrPluginSettingsValidation{Key: key, Reason: err.Error()}
	}
	return nil
}

// compileSchema turns raw JSON Schema bytes into a compiled validator.
// jsonschema v5 supports Draft-07 + Draft 2020-12; we let the schema
// declare $schema if it wants to opt into a specific draft.
func compileSchema(raw []byte) (*jsonschema.Schema, error) {
	return jsonschema.CompileString("plugin-settings.json", string(raw))
}

// extractMetaFromSchema parses x-visibility / x-deprecated / x-requires-reload
// vendor extensions out of the top-level schema "properties" map. Nested
// properties are intentionally ignored — SETTINGS-V2-DESIGN §1.4 pins the
// marker scope to top-level keys only.
//
// Empty schemas (no top-level "properties") return an empty map without an
// error so callers do not need to special-case schema-less plugins.
func (s *PluginSettingsService) extractMetaFromSchema(schemaJSON []byte) (map[string]PropertyMetadata, error) {
	if len(schemaJSON) == 0 {
		return map[string]PropertyMetadata{}, nil
	}
	var doc struct {
		Properties map[string]map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return nil, fmt.Errorf("plugin_settings: extract meta: unmarshal schema: %w", err)
	}
	out := make(map[string]PropertyMetadata, len(doc.Properties))
	for prop, node := range doc.Properties {
		var m PropertyMetadata
		if raw, ok := node["x-visibility"]; ok {
			_ = json.Unmarshal(raw, &m.Visibility)
		}
		if raw, ok := node["x-deprecated"]; ok {
			_ = json.Unmarshal(raw, &m.Deprecated)
		}
		if raw, ok := node["x-requires-reload"]; ok {
			_ = json.Unmarshal(raw, &m.RequiresReload)
		}
		// Normalise empty visibility to the "frontend" default (§1.4).
		if m.Visibility == "" {
			m.Visibility = PropertyVisibilityFrontend
		}
		out[prop] = m
	}
	return out, nil
}

// resolvePropertiesMeta picks between two sources for the marker triple:
//  1. SDK-authoritative bytes from RegisterSchemaInput.PropertiesMetaJSON
//     (the plugin SDK has serialised SettingsSchemaDoc.PropertyMeta into
//     ManifestResponse.settings_properties_meta_json).
//  2. Fallback: derive from x-visibility / x-deprecated / x-requires-reload
//     vendor extensions inside SchemaJSON.
//
// When source (1) is non-empty it wins (per SETTINGS-V2-DESIGN §3.3.2 / §4.1)
// — the SDK serialisation is canonical because the plugin author can populate
// SettingsSchemaDoc.PropertyMeta even when their schema bytes come from an
// embed.FS and they cannot edit them at runtime.
func (s *PluginSettingsService) resolvePropertiesMeta(schemaJSON, sdkMeta []byte) (map[string]PropertyMetadata, error) {
	if len(sdkMeta) > 0 {
		var meta map[string]PropertyMetadata
		if err := json.Unmarshal(sdkMeta, &meta); err != nil {
			return nil, fmt.Errorf("plugin_settings: parse properties_meta: %w", err)
		}
		// Apply the same "" → "frontend" normalisation the schema-derivation
		// path uses so callers see a single shape regardless of source.
		for k, v := range meta {
			if v.Visibility == "" {
				v.Visibility = PropertyVisibilityFrontend
				meta[k] = v
			}
		}
		if meta == nil {
			meta = map[string]PropertyMetadata{}
		}
		return meta, nil
	}
	return s.extractMetaFromSchema(schemaJSON)
}

// validateVisibilities rejects any property declaring a visibility outside
// the allowed set (frontend|backend|secret). Empty visibility is permitted
// because callers normalise it to "frontend" before reaching this guard.
// The Backstage-style fail-fast (Curator decision 2.1, point 6) means a
// plugin with an invalid x-visibility refuses to register at all.
func validateVisibilities(pluginName string, meta map[string]PropertyMetadata) error {
	for prop, m := range meta {
		switch m.Visibility {
		case "", PropertyVisibilityFrontend, PropertyVisibilityBackend, PropertyVisibilitySecret:
			// ok
		default:
			return fmt.Errorf("%w: plugin=%s property=%s value=%q",
				ErrInvalidSchemaVisibility, pluginName, prop, m.Visibility)
		}
	}
	return nil
}
