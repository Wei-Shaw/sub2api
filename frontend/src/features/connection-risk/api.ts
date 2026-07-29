import { apiClient } from '@/api/client'
import type {
  ConnectionRiskEvent,
  ConnectionRiskEventFilters,
  ConnectionRiskEventPage,
  ConnectionRiskRuntime,
  ConnectionRiskSettings,
} from './types'

const basePath = '/admin/connection-risk'

export async function getConfig(): Promise<ConnectionRiskSettings> {
  const { data } = await apiClient.get<ConnectionRiskSettings>(`${basePath}/config`)
  return data
}

export async function updateConfig(payload: ConnectionRiskSettings): Promise<ConnectionRiskSettings> {
  const { data } = await apiClient.put<ConnectionRiskSettings>(`${basePath}/config`, payload)
  return data
}

export async function getRuntime(): Promise<ConnectionRiskRuntime> {
  const { data } = await apiClient.get<ConnectionRiskRuntime>(`${basePath}/runtime`)
  return data
}

export async function listEvents(
  filters: ConnectionRiskEventFilters,
  page: number,
  pageSize: number,
): Promise<ConnectionRiskEventPage> {
  const params: Record<string, string | number> = { page, page_size: pageSize }
  if (filters.status) params.status = filters.status
  if (filters.severity) params.severity = filters.severity
  if (filters.user_id) params.user_id = filters.user_id
  if (filters.api_key_id) params.api_key_id = filters.api_key_id
  if (filters.rule) params.rule = filters.rule
  const { data } = await apiClient.get<ConnectionRiskEventPage>(`${basePath}/events`, { params })
  return data
}

export async function getEvent(id: number): Promise<ConnectionRiskEvent> {
  const { data } = await apiClient.get<ConnectionRiskEvent>(`${basePath}/events/${id}`)
  return data
}

export async function ackEvent(id: number): Promise<void> {
  await apiClient.post(`${basePath}/events/${id}/ack`)
}

export async function resolveEvent(id: number): Promise<void> {
  await apiClient.post(`${basePath}/events/${id}/resolve`)
}

export async function suppressEvent(id: number): Promise<void> {
  await apiClient.post(`${basePath}/events/${id}/suppress`)
}

export async function deleteEvent(id: number): Promise<void> {
  await apiClient.delete(`${basePath}/events/${id}`)
}

export async function exemptSubject(scope: 'k' | 'u', id: number, reason = '', ttlSeconds = 86400): Promise<void> {
  await apiClient.post(`${basePath}/actions/exempt`, {
    scope,
    id,
    reason,
    ttl_seconds: ttlSeconds,
  })
}

export async function whitelistIPs(apiKeyId: number, ips: string[]): Promise<void> {
  await apiClient.post(`${basePath}/actions/whitelist-ip`, {
    api_key_id: apiKeyId,
    ips,
  })
}

export async function runRetention(): Promise<{ deleted: number }> {
  const { data } = await apiClient.post<{ deleted: number }>(`${basePath}/actions/run-retention`)
  return data
}
