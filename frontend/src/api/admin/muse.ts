import { apiClient } from '../client'

export interface MuseStatus {
  state: 'transport_unqualified' | 'verification_required' | 'ready' | 'session_expired'
  qualified_transport: boolean
  verified: boolean
  owner_user_id: number
  models: string[]
  session_expires_at?: string
  pending_turns?: MuseTurn[]
  capabilities?: Record<string, unknown>
  usage?: { plan?: string; entitlement?: string; observed_at: string; windows?: Array<{ name: string; used?: number; limit?: number; resets_at?: string }> }
}
export async function getMuseStatus(id: number): Promise<MuseStatus> {
  return (await apiClient.get<MuseStatus>(`/admin/muse/accounts/${id}/status`)).data
}
export async function verifyMuse(id: number): Promise<MuseStatus> {
  return (await apiClient.post<MuseStatus>(`/admin/muse/accounts/${id}/verify`)).data
}
export async function renewMuse(id: number) {
  return (await apiClient.post(`/admin/muse/accounts/${id}/renew`)).data
}
export async function resolveMuseTurn(id: string, outcome: 'completed' | 'failed' | 'cancelled') {
  return (await apiClient.post(`/admin/muse/turns/${encodeURIComponent(id)}/resolve`, { outcome, confirm_remote_terminal: true })).data
}

export interface MuseTurn {
 id: string
 state: string
 created_at: string
 actor: { user_id: number; api_key_id: number }
 pricing: { mode: string; unit_price: string; multiplier: string }
}

export async function retryMuseSettlement(id: string) {
 return (await apiClient.post(`/admin/muse/turns/${encodeURIComponent(id)}/settle`)).data
}
