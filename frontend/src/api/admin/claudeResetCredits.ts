import { apiClient } from '../client'

export interface ClaudeResetCredit {
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

export type ClaudeResetOutcomeKind = 'reset' | 'already_used' | 'not_limited' | 'cooldown' | 'ineligible' | 'unknown'

export interface ClaudeResetOutcome {
  outcome: ClaudeResetOutcomeKind
  reason?: string
  cleared?: string[]
  cooldown_until?: string
  credits?: ClaudeResetCredits
  replayed: boolean
}

// Consumes one reset credit (irreversible). The server picks the grant; the key
// identifies one operator confirmation and must be reused when retrying it.
export async function redeemClaudeResetCredit(id: number, idempotencyKey: string): Promise<ClaudeResetOutcome> {
  const { data } = await apiClient.post<ClaudeResetOutcome>(`/admin/accounts/${id}/claude/reset-credits/redeem`, undefined, {
    headers: { 'Idempotency-Key': idempotencyKey }
  })
  return data
}
