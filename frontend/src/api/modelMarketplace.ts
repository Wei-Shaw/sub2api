import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export type MarketplaceAccessState = 'available' | 'subscribed' | 'purchasable'

export interface MarketplacePlan {
  id: number
  group_id: number
  name: string
  description: string
  price: number
  original_price?: number | null
  validity_days: number
  validity_unit: string
  features: string[]
  product_name: string
  sort_order: number
}

export interface MarketplaceSubscription {
  id: number
  status: string
  starts_at: string
  expires_at: string
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
}

export interface MarketplaceGroup {
  id: number
  name: string
  platform: string
  subscription_type: string
  rate_multiplier: number
  user_rate_multiplier?: number | null
  is_exclusive: boolean
  access_state: MarketplaceAccessState
  active_subscription?: MarketplaceSubscription | null
  plans: MarketplacePlan[]
}

export interface MarketplaceChannel {
  name: string
  description: string
}

export interface MarketplaceModelPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
}

export interface MarketplaceModelPricing {
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: MarketplaceModelPricingInterval[]
}

export interface MarketplaceModel {
  name: string
  platform: string
  pricing: MarketplaceModelPricing | null
  channels: MarketplaceChannel[]
  groups: MarketplaceGroup[]
  access_state: MarketplaceAccessState
}

export interface ModelMarketplaceResponse {
  auth: {
    authenticated: boolean
    user_id?: number
  }
  models: MarketplaceModel[]
}

export async function getModelMarketplace(options?: { signal?: AbortSignal }): Promise<ModelMarketplaceResponse> {
  const { data } = await apiClient.get<ModelMarketplaceResponse>('/marketplace/models', {
    signal: options?.signal,
  })
  return data
}

export const modelMarketplaceAPI = {
  getModelMarketplace,
}

export default modelMarketplaceAPI
