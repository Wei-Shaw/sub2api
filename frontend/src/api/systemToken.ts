import { apiClient } from './client'

export async function getSystemTokenStatus(): Promise<{ has_token: boolean }> {
  const { data } = await apiClient.get<{ has_token: boolean }>('/user/system-token')
  return data
}

export async function generateSystemToken(): Promise<{ token: string }> {
  const { data } = await apiClient.post<{ token: string }>('/user/system-token')
  return data
}

export async function revokeSystemToken(): Promise<void> {
  await apiClient.delete('/user/system-token')
}
