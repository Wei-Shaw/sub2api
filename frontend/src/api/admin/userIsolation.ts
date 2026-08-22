import { apiClient } from '../client'

export interface UserIsolationLookupRequest {
  account_id?: number
  isolation_id: string
}

export interface UserIsolationLookupResult {
  account: {
    id: number
    name: string
    platform: string
    type: string
  }
  user: {
    id: number
    email: string
    username: string
    status: string
    last_active_at?: string | null
    last_used_at?: string | null
  }
}

export async function lookup(request: UserIsolationLookupRequest): Promise<UserIsolationLookupResult> {
  const { data } = await apiClient.post<UserIsolationLookupResult>('/admin/user-isolation/lookup', request)
  return data
}

export default { lookup }
