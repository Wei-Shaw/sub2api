import { apiClient } from '../client'

export interface ClaudeResetCredit {
  selection_token: string
  label: string
  resets_left: number
  starts_at?: string
  expires_at?: string
  clears: string[]
  percent_used: Record<string, number>
  blocking: string[]
  use_requires_limit: boolean
  redeemable: boolean
}

export interface ClaudeResetCredits {
  eligible: boolean
  available_count: number
  credits: ClaudeResetCredit[]
  cooldown_until?: string
  weekly_resets_at?: string
  fetched_at: string
}

export async function getClaudeResetCredits(id: number): Promise<ClaudeResetCredits> {
  const { data } = await apiClient.get<ClaudeResetCredits>(`/admin/accounts/${id}/claude/reset-credits`)
  return data
}
