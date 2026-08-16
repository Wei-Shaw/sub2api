import type {
  PromptAuditConfig,
  PromptAuditDraft,
  PromptAuditEndpointDraft,
  PromptAuditUpdateRequest,
  PromptKeywordBlockingMode,
  PromptEventFilters,
} from './types'

export const DEFAULT_GUARD_MODEL = 'sileader/qwen3guard:0.6b'
export const DEFAULT_MODERATION_MODEL = 'omni-moderation-latest'
export const DEFAULT_NEMOTRON_MODEL = 'nemotron-3.5-content-safety-free'
export const OPENROUTER_NEMOTRON_MODEL = 'nvidia/nemotron-3.5-content-safety:free'
export const NEMOTRON_MODEL_OPTIONS = [
  DEFAULT_NEMOTRON_MODEL,
  OPENROUTER_NEMOTRON_MODEL,
] as const
export const DEFAULT_PROMPT_KEYWORD_MODE: PromptKeywordBlockingMode = 'ai_only'
export const PROMPT_KEYWORD_MAX = 10000
export const PROMPT_KEYWORD_MAX_RUNES = 200

export const SCANNER_CATALOG = [
  { id: 'violent', label: 'Violent' },
  { id: 'non_violent_illegal_acts', label: 'Non-violent Illegal Acts' },
  { id: 'sexual_content_or_sexual_acts', label: 'Sexual Content or Sexual Acts' },
  { id: 'pii', label: 'PII' },
  { id: 'suicide_and_self_harm', label: 'Suicide & Self-Harm' },
  { id: 'unethical_acts', label: 'Unethical Acts' },
  { id: 'politically_sensitive_topics', label: 'Politically Sensitive Topics' },
  { id: 'copyright_violation', label: 'Copyright Violation' },
  { id: 'jailbreak', label: 'Jailbreak' },
] as const

// Vue props/refs are proxies and cannot be passed to structuredClone in every
// browser. Prompt Audit state is JSON-only, so this produces a detached draft
// without retaining reactive proxies or browser storage references.
export function cloneData<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function configToDraft(config: PromptAuditConfig): PromptAuditDraft {
  const legacyBlocking = Boolean(config.blocking_enabled)
  return {
    ...cloneData(config),
    blocking_latest_turn_only: config.blocking_latest_turn_only ?? false,
    keyword_blocking_enabled: config.keyword_blocking_enabled ?? legacyBlocking,
    ai_blocking_enabled: config.ai_blocking_enabled ?? legacyBlocking,
    blocked_keywords: [...(config.blocked_keywords ?? [])],
    pre_hash_check_enabled: config.pre_hash_check_enabled ?? false,
    keyword_blocking_mode: config.keyword_blocking_mode ?? DEFAULT_PROMPT_KEYWORD_MODE,
    proxy_id: config.proxy_id ?? null,
    group_ids: [...(config.group_ids ?? [])],
    scanners: [...(config.scanners ?? [])],
    endpoints: (config.endpoints ?? []).map((endpoint) => ({
      ...endpoint,
      token: '',
      clear_token: false,
    })),
  }
}

export function createDefaultEndpoint(index = 1): PromptAuditEndpointDraft {
  return {
    id: `guard-${Date.now()}-${index}`,
    name: `Guard ${index}`,
    protocol: 'openai_compatible',
    base_url: 'http://127.0.0.1:8000',
    model: DEFAULT_GUARD_MODEL,
    timeout_ms: 3000,
    input_limit: 4000,
    enabled: true,
    has_token: false,
    token_status: 'missing',
    token: '',
    clear_token: false,
  }
}

export function defaultModelForProtocol(protocol: PromptAuditEndpointDraft['protocol'], baseUrl = ''): string {
  if (protocol === 'openai_moderation') return DEFAULT_MODERATION_MODEL
  if (protocol === 'nemotron_content_safety') {
    return isOpenRouterBaseURL(baseUrl) ? OPENROUTER_NEMOTRON_MODEL : DEFAULT_NEMOTRON_MODEL
  }
  return DEFAULT_GUARD_MODEL
}

export function normalizeEndpointModel(endpoint: Pick<PromptAuditEndpointDraft, 'protocol' | 'base_url' | 'model'>): string {
  const model = endpoint.model.trim()
  if (endpoint.protocol === 'nemotron_content_safety' && isOpenRouterBaseURL(endpoint.base_url) && (!model || model === DEFAULT_NEMOTRON_MODEL)) {
    return OPENROUTER_NEMOTRON_MODEL
  }
  return model || defaultModelForProtocol(endpoint.protocol, endpoint.base_url)
}

function isOpenRouterBaseURL(baseUrl: string): boolean {
  try {
    return new URL(baseUrl.trim()).hostname.toLowerCase() === 'openrouter.ai'
  } catch {
    return false
  }
}

export function buildUpdateRequest(draft: PromptAuditDraft): PromptAuditUpdateRequest {
  const keywordBlocking = draft.enabled && Boolean(draft.keyword_blocking_enabled)
  const aiBlocking = draft.enabled && Boolean(draft.ai_blocking_enabled)
  return {
    expected_config_version: draft.config_version,
    enabled: draft.enabled,
    blocking_enabled: keywordBlocking || aiBlocking,
    blocking_latest_turn_only: Boolean(draft.blocking_latest_turn_only),
    keyword_blocking_enabled: keywordBlocking,
    ai_blocking_enabled: aiBlocking,
    store_pass_events: draft.store_pass_events,
    pre_hash_check_enabled: Boolean(draft.pre_hash_check_enabled),
    blocked_keywords: [...(draft.blocked_keywords ?? [])],
    keyword_blocking_mode: draft.keyword_blocking_mode ?? DEFAULT_PROMPT_KEYWORD_MODE,
    strategy: 'priority',
    worker_count: Number(draft.worker_count),
    queue_capacity: Number(draft.queue_capacity),
    scanners: [...draft.scanners],
    all_groups: draft.all_groups,
    group_ids: draft.all_groups ? [] : [...draft.group_ids].sort((a, b) => a - b),
    proxy_id: draft.proxy_id ?? null,
    endpoints: draft.endpoints.map((endpoint) => ({
      id: endpoint.id.trim(),
      name: endpoint.name.trim(),
      protocol: endpoint.protocol,
      base_url: endpoint.base_url.trim(),
      model: normalizeEndpointModel(endpoint),
      token: endpoint.token.trim() || undefined,
      clear_token: endpoint.clear_token,
      timeout_ms: Number(endpoint.timeout_ms),
      input_limit: Number(endpoint.input_limit),
      enabled: endpoint.enabled,
    })),
  }
}

export function draftFingerprint(draft: PromptAuditDraft | null): string {
  if (!draft) return ''
  return JSON.stringify(buildUpdateRequest(draft))
}

export function emptyEventFilters(): PromptEventFilters {
  return {
    decision: '',
    risk_level: '',
    audit_type: '',
    endpoint: '',
    group_id: '',
    user_id: '',
    api_key_id: '',
    request_id: '',
    prompt_hash: '',
    keyword: '',
    start_at: '',
    end_at: '',
  }
}

function toISO(value: string): string | undefined {
  if (!value.trim()) return undefined
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString()
}

export function eventQueryParams(filters: PromptEventFilters): Record<string, string | number> {
  const result: Record<string, string | number> = {}
  for (const key of ['decision', 'risk_level', 'audit_type', 'endpoint', 'request_id', 'prompt_hash', 'keyword'] as const) {
    const value = filters[key].trim()
    if (value) result[key] = value
  }
  for (const key of ['group_id', 'user_id', 'api_key_id'] as const) {
    const value = Number(filters[key])
    if (Number.isInteger(value) && value > 0) result[key] = value
  }
  const start = toISO(filters.start_at)
  const end = toISO(filters.end_at)
  if (start) result.start_at = start
  if (end) result.end_at = end
  return result
}

export function eventFilterPayload(filters: PromptEventFilters): Record<string, unknown> {
  return eventQueryParams(filters)
}

export function hasExplicitDeleteRange(filters: PromptEventFilters): boolean {
  const start = toISO(filters.start_at)
  const end = toISO(filters.end_at)
  return Boolean(start && end && new Date(start).getTime() < new Date(end).getTime())
}

export type DeleteRangePreset = '1d' | '7d' | '30d' | '90d' | 'all' | 'custom'

export const DELETE_RANGE_PRESETS: ReadonlyArray<{ id: DeleteRangePreset; days: number | null }> = [
  { id: '1d', days: 1 },
  { id: '7d', days: 7 },
  { id: '30d', days: 30 },
  { id: '90d', days: 90 },
  { id: 'all', days: null },
  { id: 'custom', days: null },
]

const DAY_MS = 24 * 60 * 60 * 1000

// Presets delete events older than the chosen cutoff: the range always starts
// at the epoch and ends at (now - days) so the backend's explicit-range
// requirement is satisfied without asking the user for a begin date.
export function resolveDeleteRangeFilters(
  filters: PromptEventFilters,
  preset: DeleteRangePreset,
  now: number = Date.now(),
): PromptEventFilters {
  const resolved = cloneData(filters)
  if (preset === 'custom') return resolved
  const days = DELETE_RANGE_PRESETS.find((item) => item.id === preset)?.days ?? null
  resolved.start_at = new Date(0).toISOString()
  resolved.end_at = new Date(days === null ? now : now - days * DAY_MS).toISOString()
  return resolved
}
