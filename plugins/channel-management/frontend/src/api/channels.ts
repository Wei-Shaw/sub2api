/**
 * Admin Channels API endpoints
 * Handles channel management for administrators
 *
 * Plugin endpoints are exposed under /api/v1/plugin/channel-management/admin/*
 * The host axios instance handles base URL prefixing.
 */

import { getClient } from './client'

export type BillingMode = 'token' | 'per_request' | 'image'

export interface PricingInterval {
  id?: number
  min_tokens: number
  max_tokens: number | null
  tier_label: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  sort_order: number
}

export interface ChannelModelPricing {
  id?: number
  platform: string
  models: string[]
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: PricingInterval[]
}

/**
 * AccountStatsPricingRule mirrors the backend service.AccountStatsPricingRule.
 * Pricing entries use the same shape as ChannelModelPricing but `intervals` is
 * never consulted by the resolver, so the API treats it as a flat list.
 */
export interface AccountStatsPricingRule {
  id?: number
  name?: string
  group_ids: number[]
  account_ids: number[]
  sort_order?: number
  pricing: ChannelModelPricing[]
}

export interface Channel {
  id: number
  name: string
  description: string
  status: string
  billing_model_source: string // "requested" | "upstream" | "channel_mapped"
  restrict_models: boolean
  apply_pricing_to_account_stats?: boolean
  group_ids: number[]
  model_pricing: ChannelModelPricing[]
  model_mapping: Record<string, Record<string, string>> // platform → {src→dst}
  account_stats_pricing_rules?: AccountStatsPricingRule[]
  created_at: string
  updated_at: string
}

export interface CreateChannelRequest {
  name: string
  description?: string
  group_ids?: number[]
  model_pricing?: ChannelModelPricing[]
  model_mapping?: Record<string, Record<string, string>>
  billing_model_source?: string
  restrict_models?: boolean
  apply_pricing_to_account_stats?: boolean
  account_stats_pricing_rules?: AccountStatsPricingRule[]
}

export interface UpdateChannelRequest {
  name?: string
  description?: string
  status?: string
  group_ids?: number[]
  model_pricing?: ChannelModelPricing[]
  model_mapping?: Record<string, Record<string, string>>
  billing_model_source?: string
  restrict_models?: boolean
  apply_pricing_to_account_stats?: boolean
  account_stats_pricing_rules?: AccountStatsPricingRule[]
}

interface PaginatedResponse<T> {
  items: T[]
  total: number
}

const BASE_PATH = '/plugin/channel-management/admin/channels'

/**
 * List channels with pagination
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<Channel>> {
  const { data } = await getClient().get<PaginatedResponse<Channel>>(BASE_PATH, {
    params: {
      page,
      page_size: pageSize,
      ...filters,
    },
    signal: options?.signal,
  })
  return data
}

/**
 * Get channel by ID
 */
export async function getById(id: number): Promise<Channel> {
  const { data } = await getClient().get<Channel>(`${BASE_PATH}/${id}`)
  return data
}

/**
 * Create a new channel
 */
export async function create(req: CreateChannelRequest): Promise<Channel> {
  const { data } = await getClient().post<Channel>(BASE_PATH, req)
  return data
}

/**
 * Update a channel
 */
export async function update(id: number, req: UpdateChannelRequest): Promise<Channel> {
  const { data } = await getClient().put<Channel>(`${BASE_PATH}/${id}`, req)
  return data
}

/**
 * Delete a channel
 */
export async function remove(id: number): Promise<void> {
  await getClient().delete(`${BASE_PATH}/${id}`)
}

export interface ModelDefaultPricing {
  found: boolean
  input_price?: number    // per-token price
  output_price?: number
  cache_write_price?: number
  cache_read_price?: number
  image_output_price?: number
}

export async function getModelDefaultPricing(model: string): Promise<ModelDefaultPricing> {
  const { data } = await getClient().get<ModelDefaultPricing>(`${BASE_PATH}/model-pricing`, {
    params: { model },
  })
  return data
}

const channelsAPI = { list, getById, create, update, remove, getModelDefaultPricing }
export default channelsAPI
