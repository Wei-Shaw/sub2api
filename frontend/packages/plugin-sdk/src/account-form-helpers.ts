/**
 * Shared helpers for loading account data into form refs (edit mode)
 * and building update payloads. Used by all platform form composables.
 *
 * Canonical location: @sub2api/plugin-sdk
 * Host re-exports from frontend/src/components/account/forms/editHelpers.ts
 */
import type { Ref } from 'vue'
import type { ModelMapping } from './account-form-types'

// ---------------------------------------------------------------------------
// Internal utilities (inlined to avoid host-internal imports)
// ---------------------------------------------------------------------------

/**
 * Build a model_mapping object from whitelist or mapping mode.
 * Inlined from useModelWhitelist.buildModelMappingObject to keep SDK
 * free of host-internal dependencies.
 */
export function buildModelMappingObject(
  mode: 'whitelist' | 'mapping',
  allowedModels: string[],
  modelMappings: ModelMapping[]
): Record<string, string> | null {
  const mapping: Record<string, string> = {}
  if (mode === 'whitelist') {
    for (const model of allowedModels) {
      if (!model.includes('*')) {
        mapping[model] = model
      }
    }
  } else {
    for (const m of modelMappings) {
      const from = m.from.trim()
      const to = m.to.trim()
      if (!from || !to) continue
      if (!isValidWildcardPattern(from)) continue
      if (to.includes('*')) continue
      mapping[from] = to
    }
  }
  return Object.keys(mapping).length > 0 ? mapping : null
}

export function isValidWildcardPattern(pattern: string): boolean {
  const starIndex = pattern.indexOf('*')
  if (starIndex === -1) return true
  return starIndex === pattern.length - 1 && pattern.lastIndexOf('*') === starIndex
}

// ---------- Model mapping ----------


/**
 * Load model_mapping from credentials into restriction mode + refs.
 * Detects whether the stored mapping is whitelist-style (from===to) or true mapping.
 */
export function loadModelMappingFromCredentials(
  credentials: Record<string, unknown> | undefined,
  modeRef: Ref<'whitelist' | 'mapping'>,
  allowedModelsRef: Ref<string[]>,
  modelMappingsRef: Ref<ModelMapping[]>
): void {
  const raw = credentials?.model_mapping as Record<string, string> | undefined
  if (raw && typeof raw === 'object') {
    const entries = Object.entries(raw)
    const isWhitelist = entries.length > 0 && entries.every(([f, t]) => f === t)
    if (isWhitelist) {
      modeRef.value = 'whitelist'
      allowedModelsRef.value = entries.map(([f]) => f)
      modelMappingsRef.value = []
    } else {
      modeRef.value = 'mapping'
      modelMappingsRef.value = entries.map(([from, to]) => ({ from, to }))
      allowedModelsRef.value = []
    }
  } else {
    modeRef.value = 'whitelist'
    modelMappingsRef.value = []
    allowedModelsRef.value = []
  }
}

/**
 * Apply model mapping to credentials object for update payload.
 * When passthrough is enabled, preserves existing mapping unchanged.
 */
export function applyModelMappingToCredentials(
  creds: Record<string, unknown>,
  mode: 'whitelist' | 'mapping',
  allowedModels: string[],
  mappings: ModelMapping[],
  passthroughEnabled?: boolean,
  existingMapping?: unknown
): void {
  if (passthroughEnabled) {
    if (existingMapping) creds.model_mapping = existingMapping
    return
  }
  const mm = buildModelMappingObject(mode, allowedModels, mappings)
  if (mm) {
    creds.model_mapping = mm
  } else {
    delete creds.model_mapping
  }
}

// ---------- Pool mode ----------

const DEFAULT_POOL_MODE_RETRY_COUNT = 3
const MAX_POOL_MODE_RETRY_COUNT = 10

export function normalizePoolModeRetryCount(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_POOL_MODE_RETRY_COUNT
  const n = Math.trunc(value)
  if (n < 0) return 0
  if (n > MAX_POOL_MODE_RETRY_COUNT) return MAX_POOL_MODE_RETRY_COUNT
  return n
}

export function loadPoolModeFromCredentials(
  credentials: Record<string, unknown> | undefined,
  enabledRef: Ref<boolean>,
  retryCountRef: Ref<number>
): void {
  enabledRef.value = credentials?.pool_mode === true
  retryCountRef.value = normalizePoolModeRetryCount(
    Number(credentials?.pool_mode_retry_count ?? DEFAULT_POOL_MODE_RETRY_COUNT)
  )
}

export function applyPoolModeToCredentials(
  creds: Record<string, unknown>,
  enabled: boolean,
  retryCount: number
): void {
  if (enabled) {
    creds.pool_mode = true
    creds.pool_mode_retry_count = normalizePoolModeRetryCount(retryCount)
  } else {
    delete creds.pool_mode
    delete creds.pool_mode_retry_count
  }
}

// ---------- Custom error codes ----------

export function loadCustomErrorCodesFromCredentials(
  credentials: Record<string, unknown> | undefined,
  enabledRef: Ref<boolean>,
  codesRef: Ref<number[]>
): void {
  enabledRef.value = credentials?.custom_error_codes_enabled === true
  const existing = credentials?.custom_error_codes as number[] | undefined
  codesRef.value = Array.isArray(existing) ? [...existing] : []
}

export function applyCustomErrorCodesToCredentials(
  creds: Record<string, unknown>,
  enabled: boolean,
  codes: number[]
): void {
  if (enabled) {
    creds.custom_error_codes_enabled = true
    creds.custom_error_codes = [...codes]
  } else {
    delete creds.custom_error_codes_enabled
    delete creds.custom_error_codes
  }
}

// ---------- Temp unsched ----------

export interface TempUnschedRuleForm {
  error_code: number | null
  keywords: string
  duration_minutes: number | null
  description: string
}

export function loadTempUnschedFromCredentials(
  credentials: Record<string, unknown> | undefined,
  enabledRef: Ref<boolean>,
  rulesRef: Ref<TempUnschedRuleForm[]>
): void {
  enabledRef.value = credentials?.temp_unschedulable_enabled === true
  const raw = credentials?.temp_unschedulable_rules
  if (!Array.isArray(raw)) {
    rulesRef.value = []
    return
  }
  rulesRef.value = raw.map((rule) => {
    const entry = rule as Record<string, unknown>
    return {
      error_code: toPositiveNumber(entry.error_code),
      keywords: formatTempUnschedKeywords(entry.keywords),
      duration_minutes: toPositiveNumber(entry.duration_minutes),
      description: typeof entry.description === 'string' ? entry.description : ''
    }
  })
}

export function applyTempUnschedToCredentials(
  creds: Record<string, unknown>,
  enabled: boolean,
  rules: TempUnschedRuleForm[]
): { valid: boolean; error?: string } {
  if (!enabled) {
    delete creds.temp_unschedulable_enabled
    delete creds.temp_unschedulable_rules
    return { valid: true }
  }
  const built = buildTempUnschedRules(rules)
  if (built.length === 0) {
    return { valid: false, error: 'admin.accounts.tempUnschedulable.rulesInvalid' }
  }
  creds.temp_unschedulable_enabled = true
  creds.temp_unschedulable_rules = built
  return { valid: true }
}

function buildTempUnschedRules(rules: TempUnschedRuleForm[]) {
  const out: Array<{
    error_code: number; keywords: string[]
    duration_minutes: number; description: string
  }> = []
  for (const rule of rules) {
    const errorCode = Number(rule.error_code)
    const duration = Number(rule.duration_minutes)
    const keywords = splitTempUnschedKeywords(rule.keywords)
    if (!Number.isFinite(errorCode) || errorCode < 100 || errorCode > 599) continue
    if (!Number.isFinite(duration) || duration <= 0) continue
    if (keywords.length === 0) continue
    out.push({
      error_code: Math.trunc(errorCode),
      keywords,
      duration_minutes: Math.trunc(duration),
      description: rule.description.trim()
    })
  }
  return out
}

function formatTempUnschedKeywords(value: unknown): string {
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === 'string')
      .map(item => item.trim()).filter(item => item.length > 0).join(', ')
  }
  return typeof value === 'string' ? value : ''
}

function splitTempUnschedKeywords(value: string): string[] {
  return value.split(/[,;]/).map(item => item.trim()).filter(item => item.length > 0)
}

function toPositiveNumber(value: unknown): number | null {
  const num = Number(value)
  if (!Number.isFinite(num) || num <= 0) return null
  return Math.trunc(num)
}

// ---------- Quota (apikey/bedrock) ----------

export function loadQuotaFromExtra(
  extra: Record<string, unknown> | undefined,
  refs: {
    quotaLimit: Ref<number | null>
    quotaDailyLimit: Ref<number | null>
    quotaWeeklyLimit: Ref<number | null>
    dailyResetMode: Ref<'rolling' | 'fixed' | null>
    dailyResetHour: Ref<number | null>
    weeklyResetMode: Ref<'rolling' | 'fixed' | null>
    weeklyResetDay: Ref<number | null>
    weeklyResetHour: Ref<number | null>
    resetTimezone: Ref<string | null>
  }
): void {
  const qv = extra?.quota_limit as number | undefined
  refs.quotaLimit.value = (qv && qv > 0) ? qv : null
  const dv = extra?.quota_daily_limit as number | undefined
  refs.quotaDailyLimit.value = (dv && dv > 0) ? dv : null
  const wv = extra?.quota_weekly_limit as number | undefined
  refs.quotaWeeklyLimit.value = (wv && wv > 0) ? wv : null
  refs.dailyResetMode.value = (extra?.quota_daily_reset_mode as 'rolling' | 'fixed') || null
  refs.dailyResetHour.value = (extra?.quota_daily_reset_hour as number) ?? null
  refs.weeklyResetMode.value = (extra?.quota_weekly_reset_mode as 'rolling' | 'fixed') || null
  refs.weeklyResetDay.value = (extra?.quota_weekly_reset_day as number) ?? null
  refs.weeklyResetHour.value = (extra?.quota_weekly_reset_hour as number) ?? null
  refs.resetTimezone.value = (extra?.quota_reset_timezone as string) || null
}

export function applyQuotaToExtra(
  extra: Record<string, unknown>,
  refs: {
    quotaLimit: number | null
    quotaDailyLimit: number | null
    quotaWeeklyLimit: number | null
    dailyResetMode: 'rolling' | 'fixed' | null
    dailyResetHour: number | null
    weeklyResetMode: 'rolling' | 'fixed' | null
    weeklyResetDay: number | null
    weeklyResetHour: number | null
    resetTimezone: string | null
  }
): void {
  if (refs.quotaLimit != null && refs.quotaLimit > 0) {
    extra.quota_limit = refs.quotaLimit
  } else { delete extra.quota_limit }
  if (refs.quotaDailyLimit != null && refs.quotaDailyLimit > 0) {
    extra.quota_daily_limit = refs.quotaDailyLimit
  } else {
    delete extra.quota_daily_limit
    delete extra.quota_daily_used
    delete extra.quota_daily_start
  }
  if (refs.quotaWeeklyLimit != null && refs.quotaWeeklyLimit > 0) {
    extra.quota_weekly_limit = refs.quotaWeeklyLimit
  } else {
    delete extra.quota_weekly_limit
    delete extra.quota_weekly_used
    delete extra.quota_weekly_start
  }
  if (refs.dailyResetMode === 'fixed') {
    extra.quota_daily_reset_mode = 'fixed'
    extra.quota_daily_reset_hour = refs.dailyResetHour ?? 0
  } else {
    delete extra.quota_daily_reset_mode
    delete extra.quota_daily_reset_hour
  }
  if (refs.weeklyResetMode === 'fixed') {
    extra.quota_weekly_reset_mode = 'fixed'
    extra.quota_weekly_reset_day = refs.weeklyResetDay ?? 1
    extra.quota_weekly_reset_hour = refs.weeklyResetHour ?? 0
  } else {
    delete extra.quota_weekly_reset_mode
    delete extra.quota_weekly_reset_day
    delete extra.quota_weekly_reset_hour
  }
  if (refs.dailyResetMode === 'fixed' || refs.weeklyResetMode === 'fixed') {
    extra.quota_reset_timezone = refs.resetTimezone || 'UTC'
  } else {
    delete extra.quota_reset_timezone
  }
}

// ---------- Intercept warmup ----------

export function loadInterceptWarmupFromCredentials(
  credentials: Record<string, unknown> | undefined,
  ref: Ref<boolean>
): void {
  ref.value = credentials?.intercept_warmup_requests === true
}



// ---------- Compact model mapping (OpenAI) ----------

export function loadCompactModelMappingsFromCredentials(
  credentials: Record<string, unknown> | undefined,
  ref: Ref<ModelMapping[]>
): void {
  const raw = credentials?.compact_model_mapping as Record<string, string> | undefined
  if (raw && typeof raw === 'object') {
    ref.value = Object.entries(raw).map(([from, to]) => ({ from, to }))
  } else {
    ref.value = []
  }
}

// ---------------------------------------------------------------------------
// Intercept warmup (inlined from credentialsBuilder.ts)
// ---------------------------------------------------------------------------

export function applyInterceptWarmup(
  credentials: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    credentials.intercept_warmup_requests = true
  } else if (mode === 'edit') {
    delete credentials.intercept_warmup_requests
  }
}
