/**
 * Admin Model Catalog API.
 *
 * 模型目录：管理员可在数据库中登记模型（含新发布的模型），无需发版。
 * - 价格单位为 USD / token（null = 不覆盖），前端展示时换算为 $/MTok。
 * - 定价优先级：渠道定价 > 模型目录 > LiteLLM > 内置兜底。
 * - 启用的模型会出现在 /v1/models 兜底列表以及账号白名单/映射候选中。
 */

import { apiClient } from '../client'

export type ModelCatalogPlatform = 'anthropic' | 'openai' | 'gemini' | 'antigravity'

export const MODEL_CATALOG_PLATFORMS: ModelCatalogPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity']

/** 建议的 vendor 值（vendor 为自由文本） */
export const MODEL_CATALOG_VENDOR_SUGGESTIONS = [
  'anthropic',
  'openai',
  'google',
  'xai',
  'moonshot',
  'deepseek',
  'zhipu'
] as const

export interface ModelCatalogEntry {
  id: number
  model_id: string
  platform: ModelCatalogPlatform
  vendor: string
  display_name: string
  description: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  context_window: number
  max_output_tokens: number
  /** YYYY-MM-DD */
  released_at: string | null
  enabled: boolean
  sort_order: number
  created_at: string
  updated_at: string
}

export interface ModelCatalogListParams {
  platform?: ModelCatalogPlatform | ''
  vendor?: string
  search?: string
  enabled?: boolean
}

export interface ModelCatalogListResponse {
  items: ModelCatalogEntry[]
}

/** Create / full-replace update payload */
export interface ModelCatalogRequest {
  model_id: string
  platform: ModelCatalogPlatform
  vendor: string
  display_name: string
  description: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  context_window: number
  max_output_tokens: number
  released_at: string | null
  enabled: boolean
  sort_order: number
}

function cleanParams(params: ModelCatalogListParams): Record<string, string | boolean> {
  const out: Record<string, string | boolean> = {}
  if (params.platform) out.platform = params.platform
  if (params.vendor) out.vendor = params.vendor
  if (params.search) out.search = params.search
  if (typeof params.enabled === 'boolean') out.enabled = params.enabled
  return out
}

export async function list(params: ModelCatalogListParams = {}): Promise<ModelCatalogListResponse> {
  const { data } = await apiClient.get<ModelCatalogListResponse>('/admin/model-catalog', {
    params: cleanParams(params)
  })
  return data
}

export async function get(id: number): Promise<ModelCatalogEntry> {
  const { data } = await apiClient.get<ModelCatalogEntry>(`/admin/model-catalog/${id}`)
  return data
}

export async function create(payload: ModelCatalogRequest): Promise<ModelCatalogEntry> {
  const { data } = await apiClient.post<ModelCatalogEntry>('/admin/model-catalog', payload)
  return data
}

export async function update(id: number, payload: ModelCatalogRequest): Promise<ModelCatalogEntry> {
  const { data } = await apiClient.put<ModelCatalogEntry>(`/admin/model-catalog/${id}`, payload)
  return data
}

export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/model-catalog/${id}`)
}

export const modelCatalogAPI = {
  list,
  get,
  create,
  update,
  remove
}

export default modelCatalogAPI
