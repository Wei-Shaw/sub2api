// V5/W6 SETTINGS-V2 widget contract.
//
// All types here are pure UI contracts that PluginSettingsForm.vue will
// instantiate from the admin API response (`/admin/plugin-settings/:plugin`).
//
// Files in this package MUST stay in sync with:
//   - backend/internal/service/plugin_settings_service.go (PropertyMetadata,
//     SecretKeys, properties_meta wire shape)
//   - frontend/src/api/admin/pluginSettings.ts (PluginSettingsSchemaInfo —
//     extended in W3-C with `properties_meta`, `secret_keys`,
//     `schema_version`).

import type { Component } from 'vue'

/**
 * PropertyMetadata mirrors the backend `service.PropertyMetadata` wire
 * shape returned in `properties_meta`. Field names use snake_case to match
 * the JSON payload — no client-side mapping is performed.
 */
export interface PropertyMetadata {
  visibility: 'frontend' | 'backend' | 'secret'
  deprecated: string
  requires_reload: boolean
}

/**
 * PropDescriptor is the SETTINGS-V2 form descriptor. One descriptor is
 * built per JSONSchema property by `buildPropDescriptors()` and threaded
 * into every widget through `WidgetProps.prop`.
 *
 * Schema declaration order is preserved (see DESIGN §5.7) — descriptors
 * appear in the order `Object.keys(schema.properties)` returns, which the
 * V8/JS spec guarantees for string keys.
 */
export interface PropDescriptor {
  key: string
  /**
   * Resolved widget type. `secret` overrides the schema-declared type when
   * `visibility === 'secret'`; `enum` overrides when the schema node has a
   * non-empty `enum`.
   */
  type: 'boolean' | 'string' | 'number' | 'integer' | 'enum' | 'json' | 'secret'
  title: string
  description: string

  /**
   * Optional schema constraints. Only `enumValues` reaches the widget
   * layer — every other constraint (`minimum`, `pattern`, ...) is
   * server-validated and surfaced via the save error path.
   */
  enumValues?: Array<{ value: unknown; label: string }>

  // SETTINGS-V2 markers (always present, defaulted in buildPropDescriptors).
  visibility: 'frontend' | 'backend' | 'secret'
  /** Empty string means "not deprecated"; non-empty is the deprecation message. */
  deprecated: string
  requiresReload: boolean

  /**
   * For secret fields only: whether the key currently has a stored value.
   * The admin UI uses this to choose between "configured" and "empty"
   * placeholders.
   */
  isConfigured?: boolean
}

/**
 * Common props every widget receives. Widgets MUST NOT introduce extra
 * props — additional context belongs on `PropDescriptor` so widget swap
 * is a one-line change.
 */
export interface WidgetProps {
  prop: PropDescriptor
  modelValue: unknown
}

/**
 * All widgets emit `update:modelValue` with the typed value (number for
 * numeric widgets, string for string widgets, parsed JSON for the JSON
 * widget, ...). PluginSettingsForm.vue handles dirty tracking + save
 * dispatching — widgets stay stateless.
 */
export type Widget = Component

/** Decorators wrap a widget and inject deprecated / requires-reload UX. */
export type Decorator = Component
