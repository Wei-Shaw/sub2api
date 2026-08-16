import { apiClient } from '@/api/client'
import { adminAPI } from '@/api/admin'
import type { Proxy } from '@/types'
import type {
  PromptAuditConfig,
  PromptAuditEvent,
  PromptAuditGroup,
  PromptAuditRuntime,
  PromptAuditUpdateRequest,
  PromptDeletePreview,
  PromptDeleteResult,
  PromptEventFilters,
  PromptEventPage,
  PromptProbeResult,
  PromptAuditEndpointDraft,
  PromptAuditDeleteHashResult,
  PromptAuditClearHashesResult,
} from './types'
import { eventFilterPayload, eventQueryParams } from './viewModel'

const basePath = '/admin/prompt-audit'

export async function getConfig(): Promise<PromptAuditConfig> {
  const { data } = await apiClient.get<PromptAuditConfig>(`${basePath}/config`)
  return data
}

export async function updateConfig(payload: PromptAuditUpdateRequest): Promise<PromptAuditConfig> {
  const { data } = await apiClient.put<PromptAuditConfig>(`${basePath}/config`, payload)
  return data
}

export async function probeEndpoint(endpoint: PromptAuditEndpointDraft, proxyId: number | null): Promise<PromptProbeResult> {
  const { data } = await apiClient.post<PromptProbeResult>(`${basePath}/endpoints/probe`, {
    endpoint: {
      id: endpoint.id,
      name: endpoint.name,
      protocol: endpoint.protocol,
      base_url: endpoint.base_url,
      model: endpoint.model,
      token: endpoint.token || undefined,
      timeout_ms: endpoint.timeout_ms,
      input_limit: endpoint.input_limit,
      enabled: endpoint.enabled,
    },
    // 0 explicitly tests a direct connection; positive values select a managed proxy.
    proxy_id: proxyId ?? 0,
  })
  return data
}

export async function getRuntime(): Promise<PromptAuditRuntime> {
  const { data } = await apiClient.get<PromptAuditRuntime>(`${basePath}/runtime`)
  return data
}

export async function deleteFlaggedHash(promptHash: string): Promise<PromptAuditDeleteHashResult> {
  const { data } = await apiClient.delete<PromptAuditDeleteHashResult>(`${basePath}/hashes`, {
    data: { prompt_hash: promptHash },
  })
  return data
}

export async function clearFlaggedHashes(): Promise<PromptAuditClearHashesResult> {
  const { data } = await apiClient.delete<PromptAuditClearHashesResult>(`${basePath}/hashes/all`)
  return data
}

export async function listEvents(
  filters: PromptEventFilters,
  page: number,
  pageSize: number,
): Promise<PromptEventPage> {
  const { data } = await apiClient.get<PromptEventPage>(`${basePath}/events`, {
    params: { page, page_size: pageSize, ...eventQueryParams(filters) },
  })
  return data
}

export async function getEvent(id: number): Promise<PromptAuditEvent> {
  const { data } = await apiClient.get<PromptAuditEvent>(`${basePath}/events/${id}`)
  return data
}

export async function deleteEvent(id: number): Promise<PromptDeleteResult> {
  const { data } = await apiClient.delete<PromptDeleteResult>(`${basePath}/events/${id}`)
  return data
}

export async function batchDeleteEvents(ids: number[]): Promise<PromptDeleteResult> {
  const { data } = await apiClient.post<PromptDeleteResult>(`${basePath}/events/batch-delete`, { ids })
  return data
}

export async function previewDelete(filters: PromptEventFilters): Promise<PromptDeletePreview> {
  const { data } = await apiClient.post<PromptDeletePreview>(
    `${basePath}/events/delete-preview`,
    eventFilterPayload(filters),
  )
  return data
}

export async function deleteEventsByFilter(
  filters: PromptEventFilters,
  preview: PromptDeletePreview,
): Promise<PromptDeleteResult> {
  const result: PromptDeleteResult = { deleted_events: 0, deleted_jobs: 0 }
  let cursorId = 0
  while (true) {
    const { data } = await apiClient.post<PromptDeleteResult>(`${basePath}/events/delete-by-filter`, {
      filter: eventFilterPayload(filters),
      snapshot_max_id: preview.snapshot_max_id,
      cursor_id: cursorId,
      filter_hash: preview.filter_hash,
      confirmation_token: preview.confirmation_token,
      confirm: true,
    })
    result.deleted_events += data.deleted_events || 0
    result.deleted_jobs += data.deleted_jobs || 0
    if (!data.has_more) return result
    const nextCursorID = Number(data.next_cursor_id || 0)
    if (nextCursorID <= cursorId) {
      throw new Error('Prompt audit deletion did not advance its cursor')
    }
    cursorId = nextCursorID
  }
}

export async function listGroups(): Promise<PromptAuditGroup[]> {
  const { data } = await apiClient.get<PromptAuditGroup[]>('/admin/groups/all', {
    params: { include_inactive: true },
  })
  return data
}

export async function listProxies(): Promise<Proxy[]> {
  return adminAPI.proxies.getAll()
}

export const promptAuditAPI = {
  getConfig,
  updateConfig,
  probeEndpoint,
  getRuntime,
  deleteFlaggedHash,
  clearFlaggedHashes,
  listEvents,
  getEvent,
  deleteEvent,
  batchDeleteEvents,
  previewDelete,
  deleteEventsByFilter,
  listGroups,
  listProxies,
}

export default promptAuditAPI
