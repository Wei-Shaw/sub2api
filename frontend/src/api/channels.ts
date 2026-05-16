/**
 * User Channels API endpoints (non-admin)
 * 用户侧「可用渠道」聚合查询：渠道 + 用户可访问的分组 + 支持模型（含定价）。
 */

import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export interface UserAvailableGroup {
  id: number
  name: string
  platform: string
  /** 'standard' | 'subscription' — 订阅分组视觉加深，和 API 密钥页保持一致。 */
  subscription_type: string
  /** 分组默认倍率。用户专属倍率（若有）通过 /groups/rates 获取后在前端 join。 */
  rate_multiplier: number
  /** true 时图片计费使用 image_rate_multiplier，而不是普通分组倍率。 */
  image_rate_independent?: boolean
  /** 图片独立倍率；仅 image_rate_independent=true 时生效。 */
  image_rate_multiplier?: number
  /** 分组图片 1K 基础价格。 */
  image_price_1k?: number | null
  /** 分组图片 2K 基础价格。 */
  image_price_2k?: number | null
  /** 分组图片 4K 基础价格。 */
  image_price_4k?: number | null
  /** true = 专属分组（小范围授权）；false = 公开分组。 */
  is_exclusive: boolean
}

export interface UserPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
}

export interface UserSupportedModelPricing {
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: UserPricingInterval[]
}

export interface UserSupportedModel {
  name: string
  platform: string
  pricing: UserSupportedModelPricing | null
}

export interface UserDefaultModelPricing {
  found: boolean
  billing_mode?: BillingMode
  input_price?: number
  output_price?: number
  cache_write_price?: number
  cache_read_price?: number
  image_output_price?: number
  per_request_price?: number
}

export interface UserModelPricingBatchResponse {
  prices: Record<string, UserDefaultModelPricing>
}

/**
 * 渠道下单个平台的子视图：用户可访问的分组 + 该平台支持的模型。
 * 后端把一个渠道按平台聚合成 sections，前端可以把渠道名作为 row-group
 * 一次渲染，后面按 sections 顺序用 rowspan 铺开。
 */
export interface UserChannelPlatformSection {
  platform: string
  base_url?: string
  groups: UserAvailableGroup[]
  supported_models: UserSupportedModel[]
}

export interface UserAvailableChannel {
  name: string
  description: string
  platforms: UserChannelPlatformSection[]
}

/** 列出可见的「可用渠道」。未登录模型广场使用公开接口，只展示公开分组可见数据。 */
export async function getAvailable(options?: { signal?: AbortSignal, public?: boolean }): Promise<UserAvailableChannel[]> {
  const { data } = await apiClient.get<UserAvailableChannel[]>(
    options?.public ? '/public/channels/available' : '/channels/available',
    {
    signal: options?.signal
    },
  )
  return data
}

export async function getModelPricingBatch(models: string[], options?: { signal?: AbortSignal }): Promise<UserModelPricingBatchResponse> {
  const { data } = await apiClient.post<UserModelPricingBatchResponse>('/channels/model-pricing/batch', {
    models
  }, {
    signal: options?.signal
  })
  return data
}

export const userChannelsAPI = { getAvailable, getModelPricingBatch }

export default userChannelsAPI
