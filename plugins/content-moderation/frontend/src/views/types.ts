import type {
  ContentModerationAPIKeyStatus,
  ContentModerationAPIKeyStatusValue,
  ContentModerationLog,
  ContentModerationModelFilter,
  ContentModerationRuntimeStatus,
  ContentModerationTestAuditResult,
  KeywordBlockingMode,
  ModerationGroup,
  ModerationMode,
  ContentModerationModelFilterType,
} from '../api/riskControl'

export type SettingsTab = 'basic' | 'scope' | 'runtime' | 'response' | 'riskThresholds' | 'retention' | 'keywords'
export type WorkerSlotState = 'active' | 'idle' | 'disabled'
export type APIKeysWriteMode = 'append' | 'replace'
export type OverviewIcon = 'shield' | 'key' | 'users' | 'document'

export interface OverviewItem {
  key: string
  label: string
  value: string
  meta: string
  icon: OverviewIcon
  iconClass: string
  badge?: string
  badgeClass?: string
}

export interface ModerationScoreRow {
  category: string
  score: number
  threshold: number
  hit: boolean
}

export interface RiskThresholdRow {
  category: string
  value: number
  defaultValue: number
}

export interface KeywordNoticeView {
  title: string
  description: string
  icon: 'infoCircle' | 'exclamationTriangle'
  toneClass: string
  iconClass: string
  titleClass: string
}

export interface ConfigForm {
  enabled: boolean
  mode: ModerationMode
  base_url: string
  model: string
  api_keys_text: string
  api_key_configured: boolean
  api_key_masked: string
  api_key_count: number
  api_key_masks: string[]
  api_key_statuses: ContentModerationAPIKeyStatus[]
  api_keys_mode: APIKeysWriteMode
  clear_api_key: boolean
  timeout_ms: number
  retry_count: number
  sample_rate: number
  all_groups: boolean
  group_ids: number[]
  record_non_hits: boolean
  worker_count: number
  queue_size: number
  block_status: number
  block_message: string
  email_on_hit: boolean
  auto_ban_enabled: boolean
  ban_threshold: number
  violation_window_hours: number
  hit_retention_days: number
  non_hit_retention_days: number
  pre_hash_check_enabled: boolean
  thresholds: Record<string, number>
  blocked_keywords_text: string
  keyword_blocking_mode: KeywordBlockingMode
  model_filter_type: ContentModerationModelFilterType
  model_filter_models: string[]
}

export interface PaginationState {
  page: number
  page_size: number
  total: number
  pages: number
}

export interface FiltersState {
  result: string
  group_id: number
  endpoint: string
  search: string
  from: string
  to: string
}

export {
  type ContentModerationAPIKeyStatus,
  type ContentModerationAPIKeyStatusValue,
  type ContentModerationLog,
  type ContentModerationModelFilter,
  type ContentModerationRuntimeStatus,
  type ContentModerationTestAuditResult,
  type KeywordBlockingMode,
  type ModerationGroup,
  type ModerationMode,
  type ContentModerationModelFilterType,
}
