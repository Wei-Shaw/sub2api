/**
 * Admin CN providers (Kimi / Zhipu / DeepSeek) API endpoints.
 * Coding-plan rolling-window quota probe + payg balance probe.
 */

import { apiClient } from '../client'

/** 滚动用量窗口档（5 小时 / 每周），对齐后端 service.CNQuotaTier。 */
export interface CNQuotaTier {
  window: '5h' | 'weekly' | 'monthly'
  used_percent: number
  reset_at?: string
}

/** Coding Plan 额度探测结果（kimi / zhipu），对齐后端 CNProviderQuotaProbeResult。 */
export interface CNProviderQuotaProbeResult {
  provider: string
  source?: string
  success: boolean
  credential_valid: boolean
  tiers?: CNQuotaTier[]
  plan_level?: string
  status_code?: number
  fetched_at: number
  persisted: boolean
  /** 智谱：是否存在可用的周额度重置次数（官网「用量重置额度」卡片同源）。 */
  reset_available?: boolean
  /** 智谱：最近一条可用周重置次数的有效期。 */
  week_reset_expire_at?: string
  error?: string
}

/** 智谱周额度重置结果，对齐后端 CNProviderQuotaResetResult。 */
export interface CNProviderQuotaResetResult {
  provider: string
  success: boolean
  window: string
  record_id: number
  week_resets_left: number
  error?: string
  fetched_at: number
  /** 成功时带回刷新后的用量探测快照（含 reset_available）。 */
  probe?: CNProviderQuotaProbeResult
}

/** 单币种余额明细（deepseek 双币种账号含 CNY + USD 两条）。 */
export interface CNProviderBalanceEntry {
  currency: string
  balance: number
}

/** payg 余额探测结果（kimi / deepseek），对齐后端 CNProviderBalanceResult。 */
export interface CNProviderBalanceResult {
  provider: string
  success: boolean
  /** 主币种余额（balances 首条，兼容单币种展示）。 */
  balance: number
  currency?: string
  /** 多币种明细；缺省时按主币种展示。 */
  balances?: CNProviderBalanceEntry[]
  available: boolean
  status_code?: number
  fetched_at: number
  persisted: boolean
  error?: string
}

/** 查询 Coding Plan 滚动窗口用量（5h + weekly）。 */
export async function queryQuota(id: number): Promise<CNProviderQuotaProbeResult> {
  const { data } = await apiClient.get<CNProviderQuotaProbeResult>(
    `/admin/cn-providers/accounts/${id}/quota`
  )
  return data
}

/** 查询 payg 账号余额。 */
export async function queryBalance(id: number): Promise<CNProviderBalanceResult> {
  const { data } = await apiClient.get<CNProviderBalanceResult>(
    `/admin/cn-providers/accounts/${id}/balance`
  )
  return data
}

/** 使用智谱「用量重置额度」周重置次数，恢复账号周用量。 */
export async function resetQuota(id: number): Promise<CNProviderQuotaResetResult> {
  const { data } = await apiClient.post<CNProviderQuotaResetResult>(
    `/admin/cn-providers/accounts/${id}/reset-quota`
  )
  return data
}

export default {
  queryQuota,
  queryBalance,
  resetQuota
}
