/**
 * Admin Channel Monitor Request Template API.
 *
 * 模板 = 一组可复用的 headers + 可选 body 覆盖配置。
 * 应用到监控 = 拷贝快照；模板后续变动不自动同步，需手动点「应用到关联监控」刷新。
 */

import { apiClient REDACTED from '../client'
import type { APIMode, BodyOverrideMode, Provider REDACTED from './channelMonitor'

export interface ChannelMonitorTemplate {
  id: number
  name: string
  provider: Provider
  api_mode: APIMode
  description: string
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
  created_at: string
  updated_at: string
  /** 关联的监控数量（快照来自此模板，仅 template_id 匹配即可） */
  associated_monitors: number
REDACTED

export interface ListParams {
  provider?: Provider
  api_mode?: APIMode
REDACTED

export interface ListResponse {
  items: ChannelMonitorTemplate[]
REDACTED

export interface CreateParams {
  name: string
  provider: Provider
  api_mode?: APIMode
  description?: string
  extra_headers?: Record<string, string>
  body_override_mode?: BodyOverrideMode
  body_override?: Record<string, unknown> | null
REDACTED

export interface UpdateParams {
  name?: string
  api_mode?: APIMode
  description?: string
  extra_headers?: Record<string, string>
  body_override_mode?: BodyOverrideMode
  body_override?: Record<string, unknown> | null
REDACTED

export interface ApplyResponse {
  affected: number
REDACTED

export interface AssociatedMonitorBrief {
  id: number
  name: string
  provider: Provider
  api_mode: APIMode
  enabled: boolean
REDACTED

export interface AssociatedMonitorsResponse {
  items: AssociatedMonitorBrief[]
REDACTED

export async function list(params: ListParams = {REDACTED): Promise<ListResponse> {
  const { data REDACTED = await apiClient.get<ListResponse>('/admin/channel-monitor-templates', {
    params,
  REDACTED)
  return data
REDACTED

export async function get(id: number): Promise<ChannelMonitorTemplate> {
  const { data REDACTED = await apiClient.get<ChannelMonitorTemplate>(
    `/admin/channel-monitor-templates/${idREDACTED`,
  )
  return data
REDACTED

export async function create(params: CreateParams): Promise<ChannelMonitorTemplate> {
  const { data REDACTED = await apiClient.post<ChannelMonitorTemplate>(
    '/admin/channel-monitor-templates',
    params,
  )
  return data
REDACTED

export async function update(id: number, params: UpdateParams): Promise<ChannelMonitorTemplate> {
  const { data REDACTED = await apiClient.put<ChannelMonitorTemplate>(
    `/admin/channel-monitor-templates/${idREDACTED`,
    params,
  )
  return data
REDACTED

export async function del(id: number): Promise<void> {
  await apiClient.delete(`/admin/channel-monitor-templates/${idREDACTED`)
REDACTED

/**
 * Apply the template to the specified associated monitors (overwrite snapshot fields).
 * monitorIds must be a non-empty subset of the template's associated monitors.
 * Returns count of actually affected monitors.
 */
export async function apply(id: number, monitorIds: number[]): Promise<ApplyResponse> {
  const { data REDACTED = await apiClient.post<ApplyResponse>(
    `/admin/channel-monitor-templates/${idREDACTED/apply`,
    { monitor_ids: monitorIds REDACTED,
  )
  return data
REDACTED

/**
 * List monitors currently associated to this template (used by apply picker).
 */
export async function listAssociatedMonitors(id: number): Promise<AssociatedMonitorsResponse> {
  const { data REDACTED = await apiClient.get<AssociatedMonitorsResponse>(
    `/admin/channel-monitor-templates/${idREDACTED/monitors`,
  )
  return data
REDACTED

export const channelMonitorTemplateAPI = {
  list,
  get,
  create,
  update,
  del,
  apply,
  listAssociatedMonitors,
REDACTED

export default channelMonitorTemplateAPI
