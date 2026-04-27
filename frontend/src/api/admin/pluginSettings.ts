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

export interface PluginSettingsSchemaInfo {
  plugin: string
  schema: JSONSchema
  defaults: Record<string, unknown>
  values: Record<string, unknown>
  updated_at?: string
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
