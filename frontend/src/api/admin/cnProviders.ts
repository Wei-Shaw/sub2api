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
  error?: string
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

/** 智谱重置卡种类：周卡（同步重置 5h 窗口）/ 5 小时卡。 */
export type ZhipuResetType = 'WEEK' | 'FIVE_HOUR'

/** 一张可用的智谱重置卡，对齐后端 service.ZhipuResetCard。 */
export interface ZhipuResetCard {
  record_id: number
  grant_type: string
  /** 上游原样透传（官网北京时间格式，如 2026-10-05 23:59:59）。 */
  expire_time?: string
  available: boolean
}

/** 智谱重置卡列表（只含可用卡，按到期时间升序），对齐后端 service.ZhipuResetCards。 */
export interface ZhipuResetCards {
  week_cards: ZhipuResetCard[]
  five_hour_cards: ZhipuResetCard[]
  last_week_reset_time?: string
  last_five_hour_reset_time?: string
  fetched_at: number
  persisted: boolean
}

export interface ZhipuResetCardUsePayload {
  reset_type: ZhipuResetType
  /** 缺省由服务端选最早到期的可用卡。 */
  record_id?: number
}

/** 手动用卡结果（含用卡后的强制重探与账号状态恢复），对齐后端 service.ZhipuResetCardUseResult。 */
export interface ZhipuResetCardUseResult {
  provider: string
  success: boolean
  reset_type: ZhipuResetType
  window: 'weekly' | '5h'
  record_id?: number
  expire_time?: string
  week_resets_left: number
  five_hour_resets_left: number
  status_code?: number
  error_code?: string
  error?: string
  fetched_at: number
  cards?: ZhipuResetCards | null
  account_state_recovered: boolean
  probe?: CNProviderQuotaProbeResult | null
  warning_code?: string
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

/** 拉取智谱 Coding Plan 账号的重置卡列表（后端同时落 extra 快照）。 */
export async function listResetCards(id: number): Promise<ZhipuResetCards> {
  const { data } = await apiClient.get<ZhipuResetCards>(
    `/admin/cn-providers/accounts/${id}/reset-quota`
  )
  return data
}

/**
 * 消耗一张智谱重置卡。服务端依次 list → use → 刷新 list → 强制重探额度 → 恢复账号状态，
 * 最坏情况要串行四次上游请求，超时给足；请求中途断开时服务端仍会把后处理跑完。
 */
export async function useResetCard(
  id: number,
  payload: ZhipuResetCardUsePayload
): Promise<ZhipuResetCardUseResult> {
  const { data } = await apiClient.post<ZhipuResetCardUseResult>(
    `/admin/cn-providers/accounts/${id}/reset-quota`,
    payload,
    { timeout: 120_000 }
  )
  return data
}

export default {
  queryQuota,
  queryBalance,
  listResetCards,
  useResetCard
}
