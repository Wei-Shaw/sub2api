// Plugin settings admin API client.
//
// V5/W3 — surfaces the three REST endpoints exposed by the host:
//   GET  /admin/plugin-settings                — list namespaces
//   GET  /admin/plugin-settings/:plugin        — schema + values for one plugin
//   PUT  /admin/plugin-settings/:plugin/:key   — update one key
//
// Errors are returned as the rejected Promise from apiClient; the caller
// should run them through extractApiErrorMessage so the rendered toast
// contains the backend's human-readable message.
import { apiClient } from '@/api/client'

// JSONSchema is intentionally `unknown` because vue-json-schema-form (or
// the fallback hand-written renderer) only needs the raw value the
// backend supplied. Type-narrowing per-property happens inside the
// component, not here.
export type JSONSchema = Record<string, unknown>

export interface PluginSettingsListResponse {
  items: string[]
}

/**
 * PropertyMetaResponse mirrors backend `service.PropertyMetadata` wire shape
 * returned in `properties_meta`. Field names use snake_case to match the
 * JSON payload — no client-side mapping is performed.
 *
 * V5/W6 SETTINGS-V2: every field is optional because older hosts do not
 * emit this sidecar. Consumers (buildPropDescriptors) default unset values
 * to `{ visibility: 'frontend', deprecated: '', requires_reload: false }`.
 */
export interface PropertyMetaResponse {
  visibility?: 'frontend' | 'backend' | 'secret'
  deprecated?: string
  requires_reload?: boolean
}

export interface PluginSettingsSchemaInfo {
  plugin: string
  schema: JSONSchema
  defaults: Record<string, unknown>
  values: Record<string, unknown>
  updated_at?: string

  // V5/W6 SETTINGS-V2 fields. All optional so the type stays compatible with
  // older host responses that do not emit them. Consumers must default
  // missing values (see plugin-settings-widgets/buildPropDescriptors.ts).

  /**
   * `plugin_settings_schemas.schema_version`. Empty when the plugin did not
   * declare one (the host normalises missing versions to `"0"`, but the
   * pre-W3-A host omits the field entirely).
   */
  schema_version?: string

  /**
   * Per top-level property metadata. Keys match `schema.properties`; absent
   * keys default to `{visibility:'frontend', deprecated:'', requires_reload:false}`.
   */
  properties_meta?: Record<string, PropertyMetaResponse>

  /**
   * Names of properties whose `visibility === 'secret'` AND have a stored
   * value (so the UI can show "configured" placeholder vs "empty"). The
   * matching entries in `values` are JSON `null`. Empty array when no
   * secrets are configured.
   */
  secret_keys?: string[]
}

export interface PluginSettingsUpdateResult {
  plugin: string
  key: string
  value: unknown
  revision: number
}

export const pluginSettingsApi = {
  async list(): Promise<string[]> {
    const resp = await apiClient.get<PluginSettingsListResponse>('/admin/plugin-settings')
    return resp.data?.items ?? []
  },

  async get(plugin: string): Promise<PluginSettingsSchemaInfo> {
    const resp = await apiClient.get<PluginSettingsSchemaInfo>(
      `/admin/plugin-settings/${encodeURIComponent(plugin)}`
    )
    return resp.data
  },

  async update(plugin: string, key: string, value: unknown): Promise<PluginSettingsUpdateResult> {
    const resp = await apiClient.put<PluginSettingsUpdateResult>(
      `/admin/plugin-settings/${encodeURIComponent(plugin)}/${encodeURIComponent(key)}`,
      { value }
    )
    return resp.data
  }
}
