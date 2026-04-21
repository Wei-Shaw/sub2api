/**
 * User Channels API endpoints (non-admin)
 * 用户侧「可用渠道」聚合查询：渠道 + 用户可访问的分组 + 支持模型（含定价）。
 */

import { apiClient REDACTED from './client'
import type { BillingMode REDACTED from '@/constants/channel'

export interface UserAvailableGroup {
  id: number
  name: string
  platform: string
REDACTED

export interface UserPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
REDACTED

export interface UserSupportedModelPricing {
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: UserPricingInterval[]
REDACTED

export interface UserSupportedModel {
  name: string
  platform: string
  pricing: UserSupportedModelPricing | null
REDACTED

export interface UserAvailableChannel {
  name: string
  description: string
  /**
   * 所属平台（anthropic / openai / antigravity / gemini ...）。后端按平台把一个渠道
   * 摊开成多条记录，因此此字段决定整行的配色与图标。
   */
  platform: string
  groups: UserAvailableGroup[]
  supported_models: UserSupportedModel[]
REDACTED

/** 列出当前用户可见的「可用渠道」（与 /groups/available 保持一致，返回平数组）。 */
export async function getAvailable(options?: { signal?: AbortSignal REDACTED): Promise<UserAvailableChannel[]> {
  const { data REDACTED = await apiClient.get<UserAvailableChannel[]>('/channels/available', {
    signal: options?.signal
  REDACTED)
  return data
REDACTED

export const userChannelsAPI = { getAvailable REDACTED

export default userChannelsAPI
