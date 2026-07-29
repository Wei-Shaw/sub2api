export type ConnectionRiskSeverity = 'low' | 'medium' | 'high' | 'critical' | ''
export type ConnectionRiskStatus = 'open' | 'acknowledged' | 'resolved' | 'suppressed'

export interface ConnectionRiskRuleHit {
  rule_id: string
  severity: string
  confidence: number
  weight: number
  detail: string
}

export interface ConnectionRiskEvent {
  id: number
  created_at: string
  updated_at: string
  subject_type: string
  user_id?: number | null
  api_key_id?: number | null
  api_key_prefix: string
  rules_fired: ConnectionRiskRuleHit[]
  severity: ConnectionRiskSeverity
  score: number
  status: ConnectionRiskStatus
  title: string
  summary: string
  evidence: Record<string, unknown>
  metrics: Record<string, unknown>
  dedupe_key: string
  action_taken: string
  first_seen_at: string
  last_seen_at: string
}

export interface ConnectionRiskEventPage {
  items: ConnectionRiskEvent[]
  total: number
  page: number
  page_size: number
}

export interface ConnectionRiskSettings {
  enabled: boolean
  emit_enabled: boolean
  worker_enabled: boolean
  include_read_only_endpoints: boolean
  emit_sample_rate_evidence: number
  r7_include_admin_actors: boolean
  max_active_members: number
  active_prune_every_n_emits: number
  worker_interval_seconds: number
  phase: string
  notify_email: boolean
  min_notify_severity: string
  retention_days: number
  actions: {
    soft_throttle_enabled: boolean
    throttle_abs_rpm: number
    throttle_concurrency_factor: number
    auto_disable_enabled: boolean
  }
  exempt_user_ids: number[]
  exempt_api_key_ids: number[]
  rules?: Record<string, unknown>
}

export interface ConnectionRiskRuntime {
  yaml_enabled: boolean
  effective_emit?: boolean
  effective_worker?: boolean
  active_keys?: number
  active_users?: number
  settings?: ConnectionRiskSettings
  metrics?: {
    emit_ok: number
    emit_error: number
    emit_timeout: number
    worker_ticks: number
    events_created: number
    subjects_scanned: number
    degraded: boolean
    last_tick_unix: number
  }
}

export interface ConnectionRiskEventFilters {
  status: string
  severity: string
  user_id: string
  api_key_id: string
  rule: string
}
