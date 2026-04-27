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
	"sync"
	"sync/atomic"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// pluginSettingsSubscriberBuffer is the per-subscriber channel size. We
// keep it small because the SDK side already caches the latest value;
// a slow subscriber simply drops events and recovers via Get.
const pluginSettingsSubscriberBuffer = 8

// ErrPluginSettingsSchemaMissing is returned when an admin tries to
// write to a plugin namespace that has not registered a schema yet.
// Bouncing the request loudly is preferable to silently storing
// arbitrary JSON the plugin will never read.
var ErrPluginSettingsSchemaMissing = errors.New("plugin_settings: no schema registered for plugin")

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

// PluginSettingsChange is the host-side event the gRPC server fans out
// on its Watch streams. It mirrors the SDK's SettingsChange but lives in
// the service package so both the gRPC server and the admin handler can
// import it without forming a cycle.
type PluginSettingsChange struct {
	Plugin   string
	Key      string
	Value    json.RawMessage
	Revision int64
}

// PluginSettingsSchemaInfo is the admin-facing description of a plugin's
// current schema + values. The handler returns this from GET; the UI
// renders the schema with vue-json-schema-form (or the fallback).
type PluginSettingsSchemaInfo struct {
	Plugin    string                     `json:"plugin"`
	Schema    json.RawMessage            `json:"schema"`
	Defaults  json.RawMessage            `json:"defaults"`
	Values    map[string]json.RawMessage `json:"values"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

// PluginSettingsService owns persistence + validation + fan-out.
type PluginSettingsService struct {
	db     *sql.DB
	logger *slog.Logger

	mu              sync.RWMutex
	compiledSchemas map[string]*jsonschema.Schema // by plugin name
	rawSchemas      map[string]json.RawMessage    // last raw schema bytes for GET
	defaults        map[string]json.RawMessage    // last raw defaults for GET

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
		subs:            make(map[string][]*pluginSettingsSubscriber),
	}
}

// RegisterSchema is called by PluginManager during plugin start when the
// manifest declares settings_schema_json. It compiles the schema, stores
// the raw bytes for GET responses, and seeds defaults for keys that have
// not been written yet.
//
// Calling RegisterSchema again with the same plugin name simply replaces
// the cached schema — sufficient for the "restart-on-upgrade" workflow
// the design assumes.
func (s *PluginSettingsService) RegisterSchema(
	ctx context.Context, pluginName string, schemaJSON, defaultsJSON []byte,
) error {
	if pluginName == "" {
		return errors.New("plugin_settings: empty plugin name")
	}
	if len(schemaJSON) == 0 {
		// Nothing to register — clear any cached entry so a previously
		// schema-bearing plugin can drop the requirement on next restart.
		s.mu.Lock()
		delete(s.compiledSchemas, pluginName)
		delete(s.rawSchemas, pluginName)
		delete(s.defaults, pluginName)
		s.mu.Unlock()
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM plugin_settings_schemas WHERE plugin_name = $1`, pluginName)
		return err
	}
	compiled, err := compileSchema(schemaJSON)
	if err != nil {
		return fmt.Errorf("plugin_settings: compile schema for %s: %w", pluginName, err)
	}

	rawSchema := append(json.RawMessage(nil), schemaJSON...)
	rawDefaults := append(json.RawMessage(nil), defaultsJSON...)
	if len(rawDefaults) == 0 {
		rawDefaults = json.RawMessage("{}")
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO plugin_settings_schemas (plugin_name, schema_json, defaults_json, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (plugin_name) DO UPDATE
		   SET schema_json = EXCLUDED.schema_json,
		       defaults_json = EXCLUDED.defaults_json,
		       updated_at = NOW()
	`, pluginName, string(rawSchema), string(rawDefaults)); err != nil {
		return fmt.Errorf("plugin_settings: persist schema: %w", err)
	}

	s.mu.Lock()
	s.compiledSchemas[pluginName] = compiled
	s.rawSchemas[pluginName] = rawSchema
	s.defaults[pluginName] = rawDefaults
	s.mu.Unlock()

	if err := s.seedDefaults(ctx, pluginName, rawDefaults); err != nil {
		s.logger.Warn("plugin_settings: seed defaults failed",
			"plugin", pluginName, "error", err)
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
	s.mu.Unlock()
}

// GetByKey reads one key. Missing keys return (nil, 0, sql.ErrNoRows).
func (s *PluginSettingsService) GetByKey(
	ctx context.Context, pluginName, key string,
) (json.RawMessage, int64, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT value_json::text, revision FROM plugin_settings
		WHERE plugin_name = $1 AND key = $2
	`, pluginName, key)
	var raw string
	var rev int64
	if err := row.Scan(&raw, &rev); err != nil {
		return nil, 0, err
	}
	return json.RawMessage(raw), rev, nil
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
	defer rows.Close()
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
func (s *PluginSettingsService) SetByKey(
	ctx context.Context, pluginName, key string, value json.RawMessage,
) (int64, error) {
	if pluginName == "" || key == "" {
		return 0, errors.New("plugin_settings: empty plugin or key")
	}

	s.mu.RLock()
	compiled, ok := s.compiledSchemas[pluginName]
	s.mu.RUnlock()
	if !ok {
		return 0, ErrPluginSettingsSchemaMissing
	}
	if err := validateAgainst(compiled, key, value); err != nil {
		return 0, err
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO plugin_settings (plugin_name, key, value_json, revision, updated_at)
		VALUES ($1, $2, $3::jsonb, 1, NOW())
		ON CONFLICT (plugin_name, key) DO UPDATE
		   SET value_json = EXCLUDED.value_json,
		       revision   = plugin_settings.revision + 1,
		       updated_at = NOW()
		RETURNING revision
	`, pluginName, key, string(value))
	var rev int64
	if err := row.Scan(&rev); err != nil {
		return 0, fmt.Errorf("plugin_settings: upsert: %w", err)
	}
	s.notify(PluginSettingsChange{
		Plugin:   pluginName,
		Key:      key,
		Value:    append(json.RawMessage(nil), value...),
		Revision: rev,
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
		// still works without bouncing the plugin.
		var schemaStr, defaultsStr string
		var updated time.Time
		row := s.db.QueryRowContext(ctx, `
			SELECT schema_json::text, defaults_json::text, updated_at
			FROM plugin_settings_schemas WHERE plugin_name = $1
		`, pluginName)
		if err := row.Scan(&schemaStr, &defaultsStr, &updated); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		rawSchema = json.RawMessage(schemaStr)
		rawDefaults = json.RawMessage(defaultsStr)
		// Compile lazily so subsequent SetByKey calls work.
		if compiled, err := compileSchema(rawSchema); err == nil {
			s.mu.Lock()
			s.compiledSchemas[pluginName] = compiled
			s.rawSchemas[pluginName] = rawSchema
			s.defaults[pluginName] = rawDefaults
			s.mu.Unlock()
		}
	}
	values, err := s.GetAll(ctx, pluginName)
	if err != nil {
		return nil, err
	}
	return &PluginSettingsSchemaInfo{
		Plugin:   pluginName,
		Schema:   rawSchema,
		Defaults: rawDefaults,
		Values:   values,
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
	defer rows.Close()
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

// validateAgainst runs jsonschema validation by reaching into the
// top-level `properties` map for the supplied key. We validate one key
// at a time because writes are key-scoped, but the schema is whole-object.
func validateAgainst(schema *jsonschema.Schema, key string, value json.RawMessage) error {
	// Compose a single-key object so the existing schema's `properties`
	// constraint applies even when other required fields are missing.
	wrapper := map[string]json.RawMessage{key: value}
	wrapped, err := json.Marshal(wrapper)
	if err != nil {
		return &ErrPluginSettingsValidation{Key: key, Reason: "encode wrapper: " + err.Error()}
	}
	var doc any
	if err := json.Unmarshal(wrapped, &doc); err != nil {
		return &ErrPluginSettingsValidation{Key: key, Reason: "decode wrapper: " + err.Error()}
	}
	if err := schema.Validate(doc); err != nil {
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
