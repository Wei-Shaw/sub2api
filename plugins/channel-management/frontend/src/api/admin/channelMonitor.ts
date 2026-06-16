/**
 * Admin Channel Monitor API client (V5 W7).
 *
 * Endpoints proxied through the plugin gateway:
 *   - GET    /api/v1/plugin/channel-management/admin/monitors
 *   - POST   /api/v1/plugin/channel-management/admin/monitors
 *   - GET    /api/v1/plugin/channel-management/admin/monitors/:id
 *   - PUT    /api/v1/plugin/channel-management/admin/monitors/:id
 *   - DELETE /api/v1/plugin/channel-management/admin/monitors/:id
 *   - POST   /api/v1/plugin/channel-management/admin/monitors/:id/run
 *   - GET    /api/v1/plugin/channel-management/admin/monitors/:id/history
 *
 * 简化说明 (vs 09fd83ab host frontend api/admin/channelMonitor.ts):
 *   - 仍保留 advanced 字段 (extra_headers, body_override*, template_id) 因为
 *     后端 DTO 已在 V5 W6.4 暴露这些字段, 但 plugin 端 UI 暂未实现 advanced
 *     编辑器, 这里只用类型字段保持 round-trip 不丢字段.
 *   - 模板管理 endpoints (channel-monitor-templates) **未** 在 V5 W6 后端注册,
 *     因此 client 端不再导出对应函数.
 */

import { getClient } from '../client'

export type Provider = 'openai' | 'anthropic' | 'gemini'
export type MonitorStatus = 'operational' | 'degraded' | 'failed' | 'error'
export type BodyOverrideMode = 'off' | 'merge' | 'replace'
export type APIMode = 'chat_completions' | 'responses'

export interface ExtraModelStatus {
  model: string
  status: MonitorStatus | ''
  latency_ms: number | null
}

export interface ChannelMonitor {
  id: number
  name: string
  provider: Provider
  api_mode: APIMode
  endpoint: string
  api_key_masked: string
  api_key_decrypt_failed?: boolean
  primary_model: string
  extra_models: string[]
  group_name: string
  enabled: boolean
  interval_seconds: number
  last_checked_at: string | null
  created_by: number
  created_at: string
  updated_at: string
  primary_status: MonitorStatus | ''
  primary_latency_ms: number | null
  availability_7d: number
  extra_models_status: ExtraModelStatus[]
  template_id: number | null
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
}

export interface ListParams {
  page?: number
  page_size?: number
  provider?: Provider
  enabled?: boolean
  search?: string
}

export interface ListResponse {
  items: ChannelMonitor[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface CreateParams {
  name: string
  provider: Provider
  api_mode?: APIMode
  endpoint: string
  api_key: string
  primary_model: string
  extra_models?: string[]
  group_name?: string
  enabled?: boolean
  interval_seconds: number
  template_id?: number | null
  extra_headers?: Record<string, string>
  body_override_mode?: BodyOverrideMode
  body_override?: Record<string, unknown> | null
}

export type UpdateParams = Partial<CreateParams> & {
  clear_template?: boolean
}

export interface CheckResult {
  model: string
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  message: string
  checked_at: string
}

export interface RunNowResponse {
  results: CheckResult[]
}

export interface HistoryItem {
  id: number
  model: string
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  message: string
  checked_at: string
}

export interface HistoryParams {
  model?: string
  limit?: number
}

export interface HistoryResponse {
  items: HistoryItem[]
}

const BASE = '/plugin/channel-management/admin/monitors'

export async function list(
  params: ListParams = {},
  options?: { signal?: AbortSignal },
): Promise<ListResponse> {
  const { data } = await getClient().get<ListResponse>(BASE, {
    params,
    signal: options?.signal,
  })
  return data
}

export async function get(id: number): Promise<ChannelMonitor> {
  const { data } = await getClient().get<ChannelMonitor>(`${BASE}/${id}`)
  return data
}

export async function create(params: CreateParams): Promise<ChannelMonitor> {
  const { data } = await getClient().post<ChannelMonitor>(BASE, params)
  return data
}

export async function update(id: number, params: UpdateParams): Promise<ChannelMonitor> {
  const { data } = await getClient().put<ChannelMonitor>(`${BASE}/${id}`, params)
  return data
}

export async function del(id: number): Promise<void> {
  await getClient().delete(`${BASE}/${id}`)
}

export async function runNow(id: number): Promise<RunNowResponse> {
  const { data } = await getClient().post<RunNowResponse>(`${BASE}/${id}/run`)
  return data
}

export async function listHistory(
  id: number,
  params: HistoryParams = {},
): Promise<HistoryResponse> {
  const { data } = await getClient().get<HistoryResponse>(`${BASE}/${id}/history`, { params })
  return data
}

export const channelMonitorAPI = {
  list,
  get,
  create,
  update,
  del,
  runNow,
  listHistory,
}

export default channelMonitorAPI
