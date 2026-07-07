import { apiClient } from './client'

export interface PublicPricingModel {
  model_name: string
  input_cost_per_million: number
  output_cost_per_million: number
  effective_input: number
  effective_output: number
}

export interface PublicPricingGroup {
  id: number
  name: string
  platform: string
  rate_multiplier: number
  models: PublicPricingModel[]
}

export interface PublicPricingResponse {
  groups: PublicPricingGroup[]
}

export async function getPublicPricing(): Promise<PublicPricingResponse> {
  const { data } = await apiClient.get<PublicPricingResponse>('/public/pricing')
  return data
}

export const pricingAPI = { getPublicPricing }
export default pricingAPI
