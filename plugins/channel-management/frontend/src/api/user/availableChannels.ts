/**
 * User-facing "Available Channels" API.
 *
 * V5 W9 — 调用 plugin 后端的 `/api/v1/plugin/channel-management/available-channels`
 * (W8 已注册). 该 endpoint 用 V4 X-Plugin-User-* header 识别用户, 返回当前用户
 * 可见的所有渠道 + 平台 sections + 支持模型 + 定价.
 *
 * UserGroupRates `/groups/rates` 是 host 的 user API key handler 提供的,
 * 不属于 plugin —— 通过 sdk.http.apiClient 直接打 host endpoint, host 拦截器
 * 自动挂上 cookie/Authorization.
 */

import { getClient } from '../client'
import { getSdk } from '../sdk'
import type { BillingMode } from '../../utils/channelConstants'

const PLUGIN_BASE = '/plugin/channel-management/available-channels'
const HOST_USER_GROUP_RATES = '/groups/rates'

export interface UserAvailableGroup {
  id: number
  name: string
  platform: string
  subscription_type: string
  rate_multiplier: number
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

export interface UserChannelPlatformSection {
  platform: string
  groups: UserAvailableGroup[]
  supported_models: UserSupportedModel[]
}

export interface UserAvailableChannel {
  name: string
  description: string
  platforms: UserChannelPlatformSection[]
}

/** List all channels visible to the current user (plugin endpoint). */
export async function getAvailable(options?: { signal?: AbortSignal }): Promise<UserAvailableChannel[]> {
  const { data } = await getClient().get<UserAvailableChannel[]>(PLUGIN_BASE, {
    signal: options?.signal,
  })
  return data
}

/** Get current user's custom group rate multipliers from the host user-group endpoint. */
export async function getUserGroupRates(): Promise<Record<number, number>> {
  // host endpoint, plugin 直接通过 sdk.http.apiClient 调用
  const client = getSdk().http.apiClient
  const { data } = await client.get<Record<number, number> | null>(HOST_USER_GROUP_RATES)
  return data || {}
}

export const userChannelsAPI = {
  getAvailable,
  getUserGroupRates,
}

export default userChannelsAPI
