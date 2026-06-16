import { apiClient } from '../client'

export interface PlatformDeclaration {
  platform: string
  display_name: string
  icon_svg: string
  theme_color: string
  plugin_name: string
  sort_order: number
  account_types: AccountTypeDeclaration[]
  capacity_display?: CapacityDisplayConfig
  usage_display?: UsageDisplayConfig
  custom_actions?: CustomActionDeclaration[]
  test_config?: TestConnectionConfig
  privacy_states?: PrivacyStateDeclaration[]
  group_config?: GroupConfigDeclaration
  compatible_gateways?: string[]
  frontend_meta?: Record<string, unknown>
}

export interface AccountTypeDeclaration {
  type: string
  display_name: string
  description?: string
  icon_svg?: string
  theme_color?: string
  credential_schema?: Record<string, unknown>
  extra_schema?: Record<string, unknown>
  form_component_path?: string
  sub_types?: { value: string; label: string }[]
  sort_order: number
  badge_label?: string
  frontend_meta?: Record<string, unknown>
}

export interface CapacityDisplayConfig {
  show_concurrency: boolean
  extra_rows?: DisplayRow[]
}

export interface UsageDisplayConfig {
  component_path?: string
  window_label?: string
  show_req_count: boolean
  show_cost: boolean
  extra_rows?: DisplayRow[]
}

export interface DisplayRow {
  label: string
  source: string
  format: string
}

export interface CustomActionDeclaration {
  action_id: string
  icon_svg?: string
  labels: Record<string, string>
  action_type: string
  api_endpoint?: string
  component_path?: string
  sort_order: number
}

export interface TestModeOption {
  value: string
  label: string
}

export interface TestConnectionConfig {
  model_selector: boolean
  test_component_path?: string
  default_test_model?: string
  test_modes?: TestModeOption[]
  image_model_patterns?: string[]
  prioritized_models?: string[]
}

export interface GroupConfigDeclaration {
  group_extra_schema?: Record<string, unknown>
  form_component_path?: string
  frontend_meta?: Record<string, unknown>
}

export interface PrivacyStateDeclaration {
  value: string
  display_name: string
  badge_color?: string
  is_set: boolean
}

export async function listPlatforms(): Promise<PlatformDeclaration[]> {
  const { data } = await apiClient.get<PlatformDeclaration[]>('/admin/platforms')
  return data
}

export async function getPlatform(platform: string): Promise<PlatformDeclaration> {
  const { data } = await apiClient.get<PlatformDeclaration>(`/admin/platforms/${platform}`)
  return data
}

export interface PlatformModelInfo {
  model_id: string
  display_name: string
  available: boolean
}

export async function getPlatformModels(platform: string): Promise<PlatformModelInfo[]> {
  const { data } = await apiClient.get<PlatformModelInfo[]>(`/admin/platforms/${platform}/models`)
  return data
}
