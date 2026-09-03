/**
 * User share-revenue (contributor earnings) API.
 */
import { apiClient } from './client'

export interface ShareRevenueSummary {
  total_earned: number
  total_records: number
  enabled: boolean
  user_pct: number
  invite_pct: number
  platform_pct: number
}

export interface ShareRevenueLedgerItem {
  id: number
  request_id: string
  account_id: number
  group_id: number
  revenue_mode: string
  total_cost: number
  user_amount: number
  created_at: string
}

export interface ShareRevenueLedgerList {
  items: ShareRevenueLedgerItem[]
  total: number
  page: number
  page_size: number
  pages: number
}

export async function getSummary(): Promise<ShareRevenueSummary> {
  const { data } = await apiClient.get<ShareRevenueSummary>('/user/share-revenue/summary')
  return data
}

export async function listLedgers(page = 1, pageSize = 20): Promise<ShareRevenueLedgerList> {
  const { data } = await apiClient.get<ShareRevenueLedgerList>('/user/share-revenue/ledgers', {
    params: { page, page_size: pageSize }
  })
  return data
}

export const shareRevenueAPI = {
  getSummary,
  listLedgers
}

export default shareRevenueAPI
