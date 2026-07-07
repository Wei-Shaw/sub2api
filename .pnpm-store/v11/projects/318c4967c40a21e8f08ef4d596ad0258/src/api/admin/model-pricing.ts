import { apiClient } from '../client'

export interface ModelPricingEntry {
  id: number
  model: string
  input_cost_per_token: number
  output_cost_per_token: number
  cache_creation_input_token_cost?: number | null
  cache_creation_input_token_cost_above_1hr?: number | null
  cache_read_input_token_cost?: number | null
  input_cost_per_token_priority?: number | null
  output_cost_per_token_priority?: number | null
  cache_read_input_token_cost_priority?: number | null
  output_cost_per_image?: number | null
  output_cost_per_image_token?: number | null
  long_context_input_token_threshold?: number | null
  long_context_input_cost_multiplier?: number | null
  long_context_output_cost_multiplier?: number | null
  supports_service_tier: boolean
  litellm_provider: string
  mode: string
  supports_prompt_caching: boolean
  locked: boolean
  source: string
  created_at: string
  updated_at: string
}

export interface ModelPricingListResult {
  items: ModelPricingEntry[]
  total: number
  page: number
  page_size: number
}

export interface SyncStatus {
  auto_sync_enabled: boolean
  last_synced_at?: string | null
  model_count: number
}

export async function listModelPricing(params: {
  page?: number
  page_size?: number
  search?: string
  source?: string
}): Promise<ModelPricingListResult> {
  const { data } = await apiClient.get<ModelPricingListResult>('/admin/model-pricing', { params })
  return data
}

export async function createModelPricing(entry: Partial<ModelPricingEntry>): Promise<ModelPricingEntry> {
  const { data } = await apiClient.post<ModelPricingEntry>('/admin/model-pricing', entry)
  return data
}

export async function updateModelPricing(id: number, entry: Partial<ModelPricingEntry>): Promise<ModelPricingEntry> {
  const { data } = await apiClient.put<ModelPricingEntry>(`/admin/model-pricing/${id}`, entry)
  return data
}

export async function deleteModelPricing(id: number): Promise<void> {
  await apiClient.delete(`/admin/model-pricing/${id}`)
}

export async function bulkDeleteModelPricing(ids: number[]): Promise<void> {
  await apiClient.post('/admin/model-pricing/bulk-delete', { ids })
}

export async function syncModelPricingFromRemote(): Promise<SyncStatus> {
  const { data } = await apiClient.post<SyncStatus>('/admin/model-pricing/sync')
  return data
}

export async function getModelPricingSyncStatus(): Promise<SyncStatus> {
  const { data } = await apiClient.get<SyncStatus>('/admin/model-pricing/sync-status')
  return data
}

export async function setModelPricingAutoSync(enabled: boolean): Promise<{ auto_sync_enabled: boolean }> {
  const { data } = await apiClient.put<{ auto_sync_enabled: boolean }>('/admin/model-pricing/auto-sync', { enabled })
  return data
}

export const modelPricingAPI = {
  list: listModelPricing,
  create: createModelPricing,
  update: updateModelPricing,
  delete: deleteModelPricing,
  bulkDelete: bulkDeleteModelPricing,
  syncFromRemote: syncModelPricingFromRemote,
  getSyncStatus: getModelPricingSyncStatus,
  setAutoSync: setModelPricingAutoSync,
}

export default modelPricingAPI
