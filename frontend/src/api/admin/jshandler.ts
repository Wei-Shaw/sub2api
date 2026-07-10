import { apiClient } from '../client'

export interface JSHandlerConfig {
  enabled: boolean
  timeout: string
}

export interface JSHandlerScriptEntry {
  id: string
  name: string
  filename: string
  created_at: string
  updated_at: string
}

const defaultConfig = (): JSHandlerConfig => ({
  enabled: false,
  timeout: '1s',
})

export async function getJSHandlerConfig(): Promise<JSHandlerConfig> {
  const { data } = await apiClient.get<{ config: string }>('/admin/gateway/jshandler/config')
  const raw = data?.config?.trim()
  if (!raw) {
    return defaultConfig()
  }
  try {
    const parsed = JSON.parse(raw) as JSHandlerConfig
    return {
      enabled: Boolean(parsed.enabled),
      timeout: typeof parsed.timeout === 'string' && parsed.timeout ? parsed.timeout : '1s',
    }
  } catch {
    return defaultConfig()
  }
}

export async function updateJSHandlerConfig(config: Pick<JSHandlerConfig, 'enabled' | 'timeout'>): Promise<JSHandlerConfig> {
  const { data } = await apiClient.put<JSHandlerConfig>('/admin/gateway/jshandler/config', {
    enabled: config.enabled,
    timeout: config.timeout,
  })
  return data
}

export async function listJSHandlerScripts(): Promise<JSHandlerScriptEntry[]> {
  const { data } = await apiClient.get<{ scripts: JSHandlerScriptEntry[] }>('/admin/gateway/jshandler/scripts')
  return Array.isArray(data?.scripts) ? data.scripts : []
}

export async function uploadJSHandlerScript(file: File, name?: string): Promise<JSHandlerScriptEntry> {
  const form = new FormData()
  form.append('file', file)
  if (name?.trim()) {
    form.append('name', name.trim())
  }
  const { data } = await apiClient.post<JSHandlerScriptEntry>('/admin/gateway/jshandler/scripts', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data
}

export async function deleteJSHandlerScript(id: string): Promise<void> {
  await apiClient.delete(`/admin/gateway/jshandler/scripts/${encodeURIComponent(id)}`)
}

export const jshandlerAPI = {
  getJSHandlerConfig,
  updateJSHandlerConfig,
  listJSHandlerScripts,
  uploadJSHandlerScript,
  deleteJSHandlerScript,
}

export default jshandlerAPI