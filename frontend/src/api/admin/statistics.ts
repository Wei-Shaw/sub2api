import { apiClient } from '../client'

export interface OpenAISubscriptionBalanceRow {
  id: number
  name: string
  status: string
  estimated_total_cost?: number
  account_cost?: number
  estimated_balance?: number
  utilization?: number
  estimated: boolean
}

export interface OpenAISubscriptionBalanceSummary {
  accounts: OpenAISubscriptionBalanceRow[]
  summary: {
    accounts: number
    estimated_accounts: number
    unknown_accounts: number
    estimated_balance: number
  }
  generated_at: number
}

export async function getOpenAISubscriptionBalanceSummary(): Promise<OpenAISubscriptionBalanceSummary> {
  const { data } = await apiClient.get<OpenAISubscriptionBalanceSummary>('/admin/statistics/openai-subscriptions')
  return data
}

export default { getOpenAISubscriptionBalanceSummary }
