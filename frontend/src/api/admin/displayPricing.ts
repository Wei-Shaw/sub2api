import { apiClient } from '../client'
import type { BillingMode } from '@/constants/channel'
import type { DisplayImagePrice, DisplayPriceCurrency } from '@/api/modelPrices'

export interface DisplayPricingSettings {
  global_multiplier: number
  updated_at: string
}

export interface DisplayPricingProvider {
  provider: string
  display_name: string
  provider_note: string
  per_request_note: string
  image_note: string
  currency: DisplayPriceCurrency
  multiplier: number | null
  sort_order: number
  logo_key: string
  logo_url: string
  updated_at: string
}

export type DisplayPricingProviderCreateInput = Omit<DisplayPricingProvider, 'updated_at'>
export type DisplayPricingProviderUpdateInput = Omit<DisplayPricingProviderCreateInput, 'provider'>

export interface DeleteDisplayPricingProviderResult {
  provider: string
  deleted_models: number
}

export interface DisplayPricingModelInput {
  platform: string
  model_name: string
  provider: string
  billing_mode: Extract<BillingMode, 'token' | 'per_request' | 'image'>
  currency: DisplayPriceCurrency
  enabled: boolean
  sort_order: number
  model_note: string
  official_input_per_million: number | null
  official_output_per_million: number | null
  official_cache_write_per_million: number | null
  official_cache_read_per_million: number | null
  official_price_source?: string
  official_price_source_url?: string
  official_price_synced_at?: string | null
  model_multiplier: number | null
  per_request_lte_256k: number | null
  per_request_256k_512k_override: number | null
  per_request_gt_512k_override: number | null
  image_prices: DisplayImagePrice[]
}

export interface DisplayOfficialPrices {
  input_per_million: number | null
  output_per_million: number | null
  cache_write_per_million: number | null
  cache_read_per_million: number | null
}

export interface OfficialPriceSyncCandidate {
  model_id: number
  model_name: string
  provider: string
  currency: DisplayPriceCurrency
  billing_mode?: Extract<BillingMode, 'token' | 'per_request' | 'image'>
  current: DisplayOfficialPrices
  proposed: DisplayOfficialPrices | null
  changed: boolean
  diff?: {
    input_per_million: boolean
    output_per_million: boolean
    cache_write_per_million: boolean
    cache_read_per_million: boolean
    has_changes: boolean
  }
  applicable: boolean
  reason: string
  source: string
  confidence: string
  source_updated_at: string | null
  official_reference_url: string
  expected_updated_at: string
  proposal_hash: string
}

export interface OfficialPriceSyncPreviewResponse {
  items: OfficialPriceSyncCandidate[]
  fetched_at?: string
  warning?: string
}

export interface ApplyOfficialPriceSyncRequest {
  models: Array<{
    model_id: number
    expected_updated_at: string
    proposal_hash: string
  }>
}

export interface ApplyOfficialPriceSyncResponse {
  applied_count: number
  items?: OfficialPriceSyncCandidate[]
}

export interface DisplayPricingModel extends DisplayPricingModelInput {
  id: number
  created_at: string
  updated_at: string
}

export interface DiscoveredDisplayModel {
  platform: string
  model_name: string
  billing_mode: Extract<BillingMode, 'token' | 'per_request' | 'image'>
  provider: string
  configured: boolean
}

export async function getSettings(): Promise<DisplayPricingSettings> {
  const { data } = await apiClient.get<DisplayPricingSettings>('/admin/display-pricing/settings')
  return data
}

export async function updateSettings(globalMultiplier: number): Promise<DisplayPricingSettings> {
  const { data } = await apiClient.put<DisplayPricingSettings>('/admin/display-pricing/settings', {
    global_multiplier: globalMultiplier
  })
  return data
}

export async function listProviders(): Promise<DisplayPricingProvider[]> {
  const { data } = await apiClient.get<{ items: DisplayPricingProvider[] }>('/admin/display-pricing/providers')
  return data.items
}

export async function createProvider(
  payload: DisplayPricingProviderCreateInput
): Promise<DisplayPricingProvider> {
  const { data } = await apiClient.post<DisplayPricingProvider>(
    '/admin/display-pricing/providers',
    payload
  )
  return data
}

export async function updateProvider(
  provider: string,
  payload: DisplayPricingProviderUpdateInput
): Promise<DisplayPricingProvider> {
  const { data } = await apiClient.put<DisplayPricingProvider>(
    `/admin/display-pricing/providers/${encodeURIComponent(provider)}`,
    payload
  )
  return data
}

export async function deleteProvider(provider: string): Promise<DeleteDisplayPricingProviderResult> {
  const { data } = await apiClient.delete<DeleteDisplayPricingProviderResult>(
    `/admin/display-pricing/providers/${encodeURIComponent(provider)}`
  )
  return data
}

export async function listModels(): Promise<DisplayPricingModel[]> {
  const { data } = await apiClient.get<{ items: DisplayPricingModel[] }>('/admin/display-pricing/models')
  return data.items
}

export async function createModel(payload: DisplayPricingModelInput): Promise<DisplayPricingModel> {
  const { data } = await apiClient.post<DisplayPricingModel>('/admin/display-pricing/models', payload)
  return data
}

export async function updateModel(id: number, payload: DisplayPricingModelInput): Promise<DisplayPricingModel> {
  const { data } = await apiClient.put<DisplayPricingModel>(`/admin/display-pricing/models/${id}`, payload)
  return data
}

export async function deleteModel(id: number): Promise<void> {
  await apiClient.delete(`/admin/display-pricing/models/${id}`)
}

export async function listDiscoveredModels(): Promise<DiscoveredDisplayModel[]> {
  const { data } = await apiClient.get<{ items: DiscoveredDisplayModel[] }>('/admin/display-pricing/discovered-models')
  return data.items
}

export async function previewOfficialPriceSync(): Promise<OfficialPriceSyncPreviewResponse> {
  const { data } = await apiClient.post<OfficialPriceSyncPreviewResponse>(
    '/admin/display-pricing/official-sync/preview'
  )
  return data
}

export async function applyOfficialPriceSync(
  payload: ApplyOfficialPriceSyncRequest
): Promise<ApplyOfficialPriceSyncResponse> {
  const { data } = await apiClient.post<ApplyOfficialPriceSyncResponse>(
    '/admin/display-pricing/official-sync/apply',
    payload
  )
  return data
}

const displayPricingAPI = {
  getSettings,
  updateSettings,
  listProviders,
  createProvider,
  updateProvider,
  deleteProvider,
  listModels,
  createModel,
  updateModel,
  deleteModel,
  listDiscoveredModels,
  previewOfficialPriceSync,
  applyOfficialPriceSync
}

export default displayPricingAPI
