/**
 * Admin Channel Monitor Request Template API.
 *
 * 模板 = 一组可复用的 headers + 可选 body 覆盖配置。
 * 应用到监控 = 拷贝快照；模板后续变动不自动同步，需手动点「应用到关联监控」刷新。
 */

import { apiClient } from '../client'
import type { BodyOverrideMode, Provider } from './channelMonitor'

export interface ChannelMonitorTemplate {
  id: number
  name: string
  provider: Provider
  description: string
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
  created_at: string
  updated_at: string
  /** 关联的监控数量（快照来自此模板，仅 template_id 匹配即可） */
  associated_monitors: number
}

export interface ListParams {
  provider?: Provider
}

export interface ListResponse {
  items: ChannelMonitorTemplate[]
}

export interface CreateParams {
  name: string
  provider: Provider
  description?: string
  extra_headers?: Record<string, string>
  body_override_mode?: BodyOverrideMode
  body_override?: Record<string, unknown> | null
}

export interface UpdateParams {
  name?: string
  description?: string
  extra_headers?: Record<string, string>
  body_override_mode?: BodyOverrideMode
  body_override?: Record<string, unknown> | null
}

export interface ApplyResponse {
  affected: number
}

export async function list(params: ListParams = {}): Promise<ListResponse> {
  const { data } = await apiClient.get<ListResponse>('/admin/channel-monitor-templates', {
    params,
  })
  return data
}

export async function get(id: number): Promise<ChannelMonitorTemplate> {
  const { data } = await apiClient.get<ChannelMonitorTemplate>(
    `/admin/channel-monitor-templates/${id}`,
  )
  return data
}

export async function create(params: CreateParams): Promise<ChannelMonitorTemplate> {
  const { data } = await apiClient.post<ChannelMonitorTemplate>(
    '/admin/channel-monitor-templates',
    params,
  )
  return data
}

export async function update(id: number, params: UpdateParams): Promise<ChannelMonitorTemplate> {
  const { data } = await apiClient.put<ChannelMonitorTemplate>(
    `/admin/channel-monitor-templates/${id}`,
    params,
  )
  return data
}

export async function del(id: number): Promise<void> {
  await apiClient.delete(`/admin/channel-monitor-templates/${id}`)
}

/**
 * Apply the template to all associated monitors (overwrite snapshot fields).
 * Returns count of affected monitors.
 */
export async function apply(id: number): Promise<ApplyResponse> {
  const { data } = await apiClient.post<ApplyResponse>(
    `/admin/channel-monitor-templates/${id}/apply`,
  )
  return data
}

export const channelMonitorTemplateAPI = {
  list,
  get,
  create,
  update,
  del,
  apply,
}

export default channelMonitorTemplateAPI
