import { apiClient } from '../client'

export interface PluginCapability {
  id: string
  platform: string
  account_type: string
}

export interface PluginRequirements {
  sub2api: string
  recommended_sub2api_version?: string
  tested_sub2api_versions?: string[]
  plugin_protocol: number
  transport_api: number
  ui_bridge: number
}

export interface PluginManifest {
  schema_version: number
  id: string
  name: string
  version: string
  description?: string
  author?: string
  requires: PluginRequirements
  capabilities: PluginCapability[]
  ui: { entrypoint: string }
}

export interface PluginCompatibility {
  compatible: boolean
  tested: boolean
  status: 'compatible' | 'untested' | 'incompatible'
  message: string
  current_sub2api_version: string
  required_sub2api_version: string
  recommended_sub2api_version: string
  plugin_protocol: number
  transport_api: number
  ui_bridge: number
}

export interface PluginBinding {
  id: number
  plugin_id: number
  capability: string
  platform: string
  account_type: string
  enabled: boolean
  rollout_percent: number
}

export interface PluginInstallation {
  id: number
  plugin_key: string
  name: string
  version: string
  description: string
  author: string
  manifest: PluginManifest
  binary_sha256: string
  signature_status: 'trusted' | 'unsigned'
  state: 'disabled' | 'starting' | 'enabled' | 'error' | 'incompatible'
  last_error: string
  installed_at: string
  enabled_at?: string
  updated_at: string
  bindings: PluginBinding[]
  compatibility: PluginCompatibility
  runtime_healthy: boolean
  runtime_message: string
}

export interface PluginTestResult {
  success: boolean
  message: string
  latency_ms: number
}

export interface PluginUISession {
  url: string
  bridge_token: string
  ui_bridge_version: number
  expires_at: string
}

export async function list(): Promise<PluginInstallation[]> {
  const { data } = await apiClient.get<PluginInstallation[]>('/admin/plugins')
  return data
}

export async function upload(file: File): Promise<PluginInstallation> {
  const form = new FormData()
  form.append('plugin', file)
  const { data } = await apiClient.post<PluginInstallation>('/admin/plugins/upload', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 120000
  })
  return data
}

export async function enable(
  id: number,
  rolloutPercent: number,
  acceptUntested: boolean
): Promise<PluginInstallation> {
  const { data } = await apiClient.post<PluginInstallation>(`/admin/plugins/${id}/enable`, {
    rollout_percent: rolloutPercent,
    accept_untested: acceptUntested
  })
  return data
}

export async function disable(id: number): Promise<PluginInstallation> {
  const { data } = await apiClient.post<PluginInstallation>(`/admin/plugins/${id}/disable`)
  return data
}

export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/plugins/${id}`)
}

export async function getConfig(id: number): Promise<Record<string, unknown>> {
  const { data } = await apiClient.get<Record<string, unknown>>(`/admin/plugins/${id}/config`)
  return data
}

export async function saveConfig(
  id: number,
  config: Record<string, unknown>
): Promise<Record<string, unknown>> {
  const { data } = await apiClient.put<Record<string, unknown>>(`/admin/plugins/${id}/config`, config)
  return data
}

export async function test(id: number): Promise<PluginTestResult> {
  const { data } = await apiClient.post<PluginTestResult>(`/admin/plugins/${id}/test`)
  return data
}

export async function createUISession(id: number): Promise<PluginUISession> {
  const { data } = await apiClient.post<PluginUISession>(`/admin/plugins/${id}/ui-session`)
  return data
}

export default {
  list,
  upload,
  enable,
  disable,
  remove,
  getConfig,
  saveConfig,
  test,
  createUISession
}
