import { apiClient } from '../client'

export interface ProviderPriceOverride {
  id: string
  group_name: string
  model_name: string
  input_price: number | null
  output_price?: number | null
  cache_write_price?: number | null
  cache_read_price?: number | null
  image_output_price?: number | null
  cache_input_price?: number | null
  cache_create_price?: number | null
  cache_create_price_1h?: number | null
  enabled: boolean
  note?: string
  sort_order: number
}

interface ProviderPriceOverrideListResponse {
  items: ProviderPriceOverride[]
}

async function listOverrides(): Promise<ProviderPriceOverride[]> {
  const { data } = await apiClient.get<ProviderPriceOverrideListResponse>('/admin/provider-pricing')
  return Array.isArray(data.items) ? data.items : []
}

async function updateOverrides(items: ProviderPriceOverride[]): Promise<ProviderPriceOverride[]> {
  const { data } = await apiClient.put<ProviderPriceOverrideListResponse>('/admin/provider-pricing', { items })
  return Array.isArray(data.items) ? data.items : []
}

export default {
  listOverrides,
  updateOverrides,
}
