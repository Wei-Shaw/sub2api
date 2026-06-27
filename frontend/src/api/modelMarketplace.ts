import { apiClient } from './client'

export type ModelMarketplaceBillingMode = 'token' | 'image'
export type ModelMarketplaceServiceTier = 'standard' | 'priority' | 'flex'
export type ModelMarketplaceUnit = '1M' | '1K'

export interface ModelMarketplacePricePart {
  per_token: number
  per_1k: number
  per_1m: number
  display_usd: number
}

export interface ModelMarketplacePerRequestPrice {
  unit_price: number
  display_usd: number
  tiered: boolean
}

export interface ModelMarketplaceLongContext {
  input_token_threshold: number
  input_cost_multiplier: number
  output_cost_multiplier: number
}

export interface ModelMarketplacePricing {
  source: string
  catalog_unit: string
  display_unit: string
  rate_multiplier: number
  group_id?: number
  group_name?: string
  service_tier: ModelMarketplaceServiceTier | string
  input: ModelMarketplacePricePart
  output: ModelMarketplacePricePart
  cache_read: ModelMarketplacePricePart
  cache_write: ModelMarketplacePricePart
  cache_write_5m: ModelMarketplacePricePart
  cache_write_1h: ModelMarketplacePricePart
  image_output: ModelMarketplacePricePart
  per_request: ModelMarketplacePerRequestPrice | null
  long_context?: ModelMarketplaceLongContext
}

export interface ModelMarketplaceFeatures {
  prompt_caching: boolean
  service_tier: boolean
  vision: boolean
  reasoning: boolean
  web_search: boolean
  audio_output: boolean
}

export interface ModelMarketplaceContext {
  max_input_tokens?: number
  max_output_tokens?: number
}

export interface ModelMarketplaceItem {
  model: string
  provider: string
  provider_label: string
  provider_icon: string
  family: string
  mode: string
  billing_mode: ModelMarketplaceBillingMode | string
  supported_endpoints: string[]
  features: ModelMarketplaceFeatures
  context: ModelMarketplaceContext
  pricing: ModelMarketplacePricing
  calculation_note: string
  tags: string[]
}

export interface ModelMarketplaceFacetOption {
  value: string
  label: string
  count: number
  icon?: string
}

export interface ModelMarketplaceGroupFacet {
  id: number
  name: string
  platform: string
  rate_multiplier: number
  subscription_type: string
  count: number
}

export interface ModelMarketplaceFacets {
  providers: ModelMarketplaceFacetOption[]
  modes: ModelMarketplaceFacetOption[]
  billing_modes: ModelMarketplaceFacetOption[]
  endpoints: ModelMarketplaceFacetOption[]
  groups: ModelMarketplaceGroupFacet[]
}

export interface ModelMarketplacePagination {
  page: number
  page_size: number
  total: number
  pages: number
}

export interface ModelMarketplaceCatalogState {
  last_updated?: string
  local_hash?: string
  model_count: number
}

export interface ModelMarketplaceResponse {
  items: ModelMarketplaceItem[]
  facets: ModelMarketplaceFacets
  pagination: ModelMarketplacePagination
  catalog: ModelMarketplaceCatalogState
}

export interface GetModelPricingParams {
  q?: string
  provider?: string
  mode?: string
  billing_mode?: string
  endpoint?: string
  group_id?: number | null
  service_tier?: ModelMarketplaceServiceTier | string
  unit?: ModelMarketplaceUnit | string
  page?: number
  page_size?: number
}

export async function getModelPricing(
  params: GetModelPricingParams = {},
  options?: { signal?: AbortSignal },
): Promise<ModelMarketplaceResponse> {
  const cleaned = Object.fromEntries(
    Object.entries(params).filter(([, value]) => value !== '' && value !== null && value !== undefined),
  )
  const { data } = await apiClient.get<ModelMarketplaceResponse>('/models/pricing', {
    params: cleaned,
    signal: options?.signal,
  })
  return data
}

export const modelMarketplaceAPI = { getModelPricing }

export default modelMarketplaceAPI
