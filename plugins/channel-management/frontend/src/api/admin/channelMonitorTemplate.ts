/**
 * Admin Channel Monitor Request Template API client (Worker B').
 *
 * 从 host commit 6925ac25c 的 frontend/src/api/admin/channelMonitorTemplate.ts
 * 移植；路径改为 plugin gateway prefix，apiClient 改为 plugin getClient().
 *
 * 模板 = 一组可复用的 headers + 可选 body 覆盖配置。
 * 应用到监控 = 拷贝快照；模板后续变动不自动同步，需手动点「应用到关联监控」刷新。
 *
 * Endpoints proxied through the plugin gateway:
 *   - GET    /api/v1/plugin/channel-management/admin/channel-monitor-templates
 *   - POST   /api/v1/plugin/channel-management/admin/channel-monitor-templates
 *   - GET    /api/v1/plugin/channel-management/admin/channel-monitor-templates/:id
 *   - PUT    /api/v1/plugin/channel-management/admin/channel-monitor-templates/:id
 *   - DELETE /api/v1/plugin/channel-management/admin/channel-monitor-templates/:id
 *   - POST   /api/v1/plugin/channel-management/admin/channel-monitor-templates/:id/apply
 *   - GET    /api/v1/plugin/channel-management/admin/channel-monitor-templates/:id/monitors
 *
 * 注意（Worker B' 2026-05）：plugin backend `template_service` / `template_repo`
 * 已经实现了 ApplyToMonitors / ListAssociatedMonitors，但 plugin.go 里的路由
 * 尚未注册这组 endpoint，manifest_decls.go 也未声明。前端移植先行，后端路由
 * 补齐前调用会 404——这是已知缺口。
 */

import { getClient } from '../client'
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

export interface AssociatedMonitorBrief {
  id: number
  name: string
  provider: Provider
  enabled: boolean
}

export interface AssociatedMonitorsResponse {
  items: AssociatedMonitorBrief[]
}

const BASE = '/plugin/channel-management/admin/channel-monitor-templates'

export async function list(params: ListParams = {}): Promise<ListResponse> {
  const { data } = await getClient().get<ListResponse>(BASE, { params })
  return data
}

export async function get(id: number): Promise<ChannelMonitorTemplate> {
  const { data } = await getClient().get<ChannelMonitorTemplate>(`${BASE}/${id}`)
  return data
}

export async function create(params: CreateParams): Promise<ChannelMonitorTemplate> {
  const { data } = await getClient().post<ChannelMonitorTemplate>(BASE, params)
  return data
}

export async function update(id: number, params: UpdateParams): Promise<ChannelMonitorTemplate> {
  const { data } = await getClient().put<ChannelMonitorTemplate>(`${BASE}/${id}`, params)
  return data
}

export async function del(id: number): Promise<void> {
  await getClient().delete(`${BASE}/${id}`)
}

/**
 * Apply the template to the specified associated monitors (overwrite snapshot fields).
 * monitorIds must be a non-empty subset of the template's associated monitors.
 * Returns count of actually affected monitors.
 */
export async function apply(id: number, monitorIds: number[]): Promise<ApplyResponse> {
  const { data } = await getClient().post<ApplyResponse>(`${BASE}/${id}/apply`, {
    monitor_ids: monitorIds,
  })
  return data
}

/**
 * List monitors currently associated to this template (used by apply picker).
 */
export async function listAssociatedMonitors(id: number): Promise<AssociatedMonitorsResponse> {
  const { data } = await getClient().get<AssociatedMonitorsResponse>(`${BASE}/${id}/monitors`)
  return data
}

export const channelMonitorTemplateAPI = {
  list,
  get,
  create,
  update,
  del,
  apply,
  listAssociatedMonitors,
}

export default channelMonitorTemplateAPI
