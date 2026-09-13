import { apiClient } from '../client'

export interface KeyProtectionConfig {
  enabled: boolean
  user_ids: number[]
  group_ids: number[]
  mode: 'reversible' | 'redact'
  restore_scope: 'text_and_tools' | 'tools_only'
  rules: string[]
  custom_rules: { name: string; pattern: string }[]
  ttl_seconds: number
  max_mappings: number
  max_sessions: number
}

export async function getKeyProtectionConfig(): Promise<KeyProtectionConfig> {
  const { data } = await apiClient.get<KeyProtectionConfig>('/admin/settings/key-protection')
  return data
}

export async function updateKeyProtectionConfig(config: KeyProtectionConfig): Promise<KeyProtectionConfig> {
  const { data } = await apiClient.put<KeyProtectionConfig>('/admin/settings/key-protection', config)
  return data
}

export async function clearKeyProtectionMappings(): Promise<void> {
  await apiClient.delete('/admin/settings/key-protection/mappings')
}
