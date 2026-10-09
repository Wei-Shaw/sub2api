import { apiClient } from './client'
import type { Account, Group } from '@/types'

export interface RelayEarning {
  id: number
  account_id: number
  group_id: number
  request_id: string
  charged_amount: number
  supplier_rate: number
  amount: number
  self_use: boolean
  credited_at: string | null
  created_at: string
}

export interface RelayEarnings {
  items: RelayEarning[]
  total: number
  total_earned: number
  today_earned: number
  pending_earned: number
}

export interface RelayAccountInput {
  name: string
  platform: string
  type: string
  credentials: Record<string, unknown>
  concurrency: number
  status: string
  group_ids: number[]
}

export const relayApi = {
  async groups() {
    return (await apiClient.get<{ private_groups: Group[]; public_groups: Group[] }>('/relay/groups')).data
  },
  async accounts(page = 1) {
    return (await apiClient.get<{ items: Account[]; total: number }>('/relay/accounts', { params: { page, page_size: 20 } })).data
  },
  async earnings(page = 1) {
    return (await apiClient.get<RelayEarnings>('/relay/earnings', { params: { page, page_size: 20 } })).data
  },
  async createAccount(input: RelayAccountInput) {
    return (await apiClient.post<Account>('/relay/accounts', input)).data
  },
  async updateAccount(id: number, input: RelayAccountInput) {
    return (await apiClient.put<Account>(`/relay/accounts/${id}`, input)).data
  },
  async deleteAccount(id: number) {
    await apiClient.delete(`/relay/accounts/${id}`)
  },
  async createGroup(name: string, platform: string) {
    return (await apiClient.post<Group>('/relay/groups', { name, platform })).data
  },
  async updateGroup(group: Group, name: string, description: string, status: string) {
    return (await apiClient.put<Group>(`/relay/groups/${group.id}`, { name, description, status })).data
  },
  async deleteGroup(id: number) {
    await apiClient.delete(`/relay/groups/${id}`)
  }
}
