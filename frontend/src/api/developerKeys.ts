import { apiClient } from './client'

export interface DeveloperKey {
  id: number
  name: string
  key_prefix: string
  last_used_at?: string
  created_at: string
  updated_at: string
}

export interface CreateDeveloperKeyResponse {
  key: DeveloperKey
  secret: string
  display_once: boolean
}

export const developerKeysAPI = {
  async list(): Promise<DeveloperKey[]> {
    const { data } = await apiClient.get<{ items: DeveloperKey[] }>('/user/developer-keys')
    return data.items
  },

  async create(name: string): Promise<CreateDeveloperKeyResponse> {
    const { data } = await apiClient.post<CreateDeveloperKeyResponse>('/user/developer-keys', { name })
    return data
  },

  async remove(id: number): Promise<void> {
    await apiClient.delete(`/user/developer-keys/${id}`)
  },
}
