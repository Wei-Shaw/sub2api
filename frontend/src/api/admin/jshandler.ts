import { apiClient } from '../client'

export interface JSHandlerConfig {
  enabled: boolean
  script_paths: string[]
  timeout: string
  scripts_dir?: string
}

const defaultConfig = (): JSHandlerConfig => ({
  enabled: false,
  script_paths: [],
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
      script_paths: Array.isArray(parsed.script_paths) ? parsed.script_paths : [],
      timeout: typeof parsed.timeout === 'string' && parsed.timeout ? parsed.timeout : '1s',
      scripts_dir: parsed.scripts_dir,
    }
  } catch {
    return defaultConfig()
  }
}

export async function updateJSHandlerConfig(config: JSHandlerConfig): Promise<JSHandlerConfig> {
  const { data } = await apiClient.put<JSHandlerConfig>('/admin/gateway/jshandler/config', config)
  return data
}

export const jshandlerAPI = {
  getJSHandlerConfig,
  updateJSHandlerConfig,
}

export default jshandlerAPI