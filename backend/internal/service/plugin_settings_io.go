// Package service — plugin_settings_io.go
//
// Read/write API for the plugin_settings subsystem: GetByKey / GetAll /
// SetByKey / SetByKeyWithSource / SchemaInfo / ListPlugins. See
// plugin_settings_types.go for the shared types and constants.
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// GetByKeyResult bundles the four return values that GetByKey produces so
// callers see one named struct instead of a four-tuple. The fields keep
// the original semantics:
//   - Value: stored row's value, or schema default. nil with sql.ErrNoRows
//     when neither exists.
//   - Revision: stored row's revision (>=1) or 0 when serving a synthetic
//     default.
//   - StoredVersion: schema_version_at_write recorded on the row, or "0"
//     for the schema-default fallback.
//   - CurrentVersion: host's currently-cached schema_version for this
//     plugin, or "0" if the plugin has not registered a schema yet.
type GetByKeyResult struct {
	Value          json.RawMessage
	Revision       int64
	StoredVersion  string
	CurrentVersion string
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
//  3. Else return (nil, sql.ErrNoRows) so the gRPC server can map it to
//     Exists=false.
//
// This makes startup-time seedDefaults a hot-path optimisation rather
// than a correctness requirement: even if seeding has not run yet (e.g.
// it failed transiently), plugin reads still succeed.
//
// StoredVersion / CurrentVersion are normalised to "0" so callers
// (notably the gRPC Get response and SDK SchemaVersionMismatchError) can
// compare them without nil checks. Drift detection is layered on top:
// the SDK constructs SchemaVersionMismatchError when stored != current,
// but only after a type-decoded Get also fails — see plugin-sdk/settings.go
// GetTyped.
func (s *PluginSettingsService) GetByKey(
	ctx context.Context, pluginName, key string,
) (*GetByKeyResult, error) {
	s.mu.RLock()
	currentVersion := normalizeSchemaVersion(s.schemaVersions[pluginName])
	s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx, `
		SELECT value_json::text, revision, schema_version_at_write FROM plugin_settings
		WHERE plugin_name = $1 AND key = $2
	`, pluginName, key)
	var raw, stored string
	var rev int64
	switch err := row.Scan(&raw, &rev, &stored); {
	case err == nil:
		return &GetByKeyResult{
			Value:          json.RawMessage(raw),
			Revision:       rev,
			StoredVersion:  normalizeSchemaVersion(stored),
			CurrentVersion: currentVersion,
		}, nil
	case errors.Is(err, sql.ErrNoRows):
		// Fall through to the schema default lookup below.
	default:
		return nil, err
	}

	if def, ok := s.lookupDefault(pluginName, key); ok {
		// Synthetic default has no recorded write version; report "0".
		return &GetByKeyResult{
			Value:          def,
			Revision:       0,
			StoredVersion:  schemaVersionUndeclared,
			CurrentVersion: currentVersion,
		}, nil
	}
	return nil, sql.ErrNoRows
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
	meta := s.propertiesMeta[plugin]
	s.mu.RUnlock()
	// V5/W6 SETTINGS-V2: never serve schema defaults for secret fields.
	// A secret with no row means "cleared / not configured"; serving the
	// manifest constant would silently undo admin clear and let plugins
	// see a hard-coded fallback secret. Operators must explicitly write.
	if m, ok := meta[key]; ok && m.Visibility == PropertyVisibilitySecret {
		return nil, false
	}
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
	schemaVersion := normalizeSchemaVersion(s.schemaVersions[pluginName])
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
		schemaVersionStr = normalizeSchemaVersion(schemaVersionStr)
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
	schemaVersion := normalizeSchemaVersion(s.schemaVersions[pluginName])
	cachedMeta := s.propertiesMeta[pluginName]
	s.mu.RUnlock()
	metaCopy := make(map[string]PropertyMetadata, len(cachedMeta))
	for k, v := range cachedMeta {
		metaCopy[k] = v
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
