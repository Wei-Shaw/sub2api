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

export interface TempUnschedRuleFormInput {
  error_code: number | string | null | undefined
  keywords: string
  duration_minutes: number | string | null | undefined
  description: string
  reset_at_time?: string | null
}

export interface TempUnschedRulePayload {
  error_code: number
  keywords: string[]
  duration_minutes: number
  description: string
  reset_at_time?: string
}

const tempUnschedResetAtTimePattern = /^([01]\d|2[0-3]):[0-5]\d$/

export function isValidTempUnschedResetAtTime(value: string): boolean {
  return tempUnschedResetAtTimePattern.test(value.trim())
}

export function splitTempUnschedKeywords(value: string): string[] {
  return value
    .split(/[,;]/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
}

export function hasInvalidTempUnschedResetAtTime(rules: TempUnschedRuleFormInput[]): boolean {
  return rules.some((rule) => {
    const resetAtTime = rule.reset_at_time?.trim() || ''
    return resetAtTime !== '' && !isValidTempUnschedResetAtTime(resetAtTime)
  })
}

export function buildTempUnschedRules(
  rules: TempUnschedRuleFormInput[]
): TempUnschedRulePayload[] {
  const out: TempUnschedRulePayload[] = []

  for (const rule of rules) {
    const errorCode = Number(rule.error_code)
    const duration = Number(rule.duration_minutes)
    const keywords = splitTempUnschedKeywords(rule.keywords)
    const resetAtTime = rule.reset_at_time?.trim() || ''
    if (!Number.isFinite(errorCode) || errorCode < 100 || errorCode > 599) {
      continue
    }

    const hasValidDuration = Number.isFinite(duration) && duration > 0
    const hasResetTime = resetAtTime !== ''
    const hasValidResetTime = hasResetTime && isValidTempUnschedResetAtTime(resetAtTime)
    if (!hasValidDuration && !hasValidResetTime) {
      continue
    }
    if (hasResetTime && !hasValidResetTime) {
      continue
    }
    if (keywords.length === 0) {
      continue
    }

    const entry: TempUnschedRulePayload = {
      error_code: Math.trunc(errorCode),
      keywords,
      duration_minutes: hasValidDuration ? Math.trunc(duration) : 0,
      description: rule.description.trim()
    }
    if (hasValidResetTime) {
      entry.reset_at_time = resetAtTime
    }
    out.push(entry)
  }

  return out
}
