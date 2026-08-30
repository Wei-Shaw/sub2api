/** Customer-facing catalogue prices, fully isolated from real channel billing. */
import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export type DisplayPriceCurrency = 'CNY' | 'USD'

export interface DisplayTokenPrices {
  input_per_million: number | null
  output_per_million: number | null
  cache_write_per_million: number | null
  cache_read_per_million: number | null
}

export interface DisplayPerRequestPrices {
  lte_256k: number | null
  from_256k_to_512k: number | null
  gt_512k: number | null
}

export interface DisplayImagePrice {
  label: string
  price: number
}

export interface DisplayPriceModel {
  id?: number
  platform: string
  model_name: string
  model_note: string
  billing_mode: BillingMode
  provider: string
  currency: DisplayPriceCurrency
  configured: boolean
  enabled: boolean
  official_prices: DisplayTokenPrices | null
  model_multiplier: number | null
  effective_multiplier: number | null
  /** Final catalogue prices per 1M tokens. */
  display_prices: DisplayTokenPrices | null
  /** Final three-tier per-request prices; no multiplier is exposed. */
  per_request: DisplayPerRequestPrices | null
  /** Administrator-managed base prices per image before the display multiplier. */
  image_base_prices: DisplayImagePrice[]
  /** Final customer-facing image prices after the display multiplier. */
  image_prices: DisplayImagePrice[]
}

export interface DisplayPriceProvider {
  provider: string
  display_name: string
  /** Token-metered provider note. */
  provider_note: string
  /** Per-request provider note, independent from token pricing. */
  per_request_note: string
  /** Image-pricing provider note, independent from token pricing. */
  image_note: string
  currency: DisplayPriceCurrency
  logo_key: string
  logo_url: string
  configured_multiplier: number | null
  effective_multiplier: number | null
  models: DisplayPriceModel[]
}

export interface ModelPricesResponse {
  global_multiplier: number
  updated_at: string
  providers: DisplayPriceProvider[]
}

export async function getModelPrices(options?: { signal?: AbortSignal }): Promise<ModelPricesResponse> {
  const { data } = await apiClient.get<ModelPricesResponse>('/model-prices', {
    signal: options?.signal
  })
  return data
}

export const modelPricesAPI = { getModelPrices }
export default modelPricesAPI
