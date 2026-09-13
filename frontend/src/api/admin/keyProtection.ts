import { apiClient } from '../client'

export interface KeyProtectionConfig {
  enabled: boolean
  user_ids: number[]
  group_ids: number[]
  rules: string[]
  custom_rules: { name: string; pattern: string }[]
}

export async function getKeyProtectionConfig(): Promise<KeyProtectionConfig> {
  const { data } = await apiClient.get<KeyProtectionConfig>('/admin/settings/key-protection')
  return data
}

export async function updateKeyProtectionConfig(config: KeyProtectionConfig): Promise<KeyProtectionConfig> {
  const { data } = await apiClient.put<KeyProtectionConfig>('/admin/settings/key-protection', config)
  return data
}
