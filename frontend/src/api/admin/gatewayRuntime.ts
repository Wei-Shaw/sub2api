import { apiClient } from '../client'

export type ConnectionPoolIsolationMode = 'proxy' | 'account' | 'account_proxy'

export interface GatewayOutboundPrivacySettings {
  enabled: boolean
  strict_account_isolation: boolean
  preserve_headers: string[]
}

export interface GatewayOpenAIWSBudgetSettings {
  max_conns_per_account: number
  min_idle_per_account: number
  max_idle_per_account: number
}

export interface GatewayRuntimeSettings {
  connection_pool_isolation: ConnectionPoolIsolationMode
  outbound_privacy: GatewayOutboundPrivacySettings
  openai_ws: GatewayOpenAIWSBudgetSettings
}

export async function getSettings(): Promise<GatewayRuntimeSettings> {
  const { data } = await apiClient.get<GatewayRuntimeSettings>('/admin/settings/gateway-runtime')
  return data
}

export async function updateSettings(settings: GatewayRuntimeSettings): Promise<GatewayRuntimeSettings> {
  const { data } = await apiClient.put<GatewayRuntimeSettings>('/admin/settings/gateway-runtime', settings)
  return data
}

export default {
  getSettings,
  updateSettings
}
