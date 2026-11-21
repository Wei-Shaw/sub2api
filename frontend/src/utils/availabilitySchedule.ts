export type AvailabilityScheduleKind = 'daily' | 'weekly'
export type AvailabilityScheduleAction = 'enable' | 'disable'

export interface AvailabilityScheduleRuleForm {
  id: string
  kind: AvailabilityScheduleKind
  weekdays: number[]
  start: string
  end: string
  action: AvailabilityScheduleAction
}

export interface AvailabilitySchedulePayload {
  enabled: boolean
  timezone?: string
  rules: Array<{
    id?: string
    kind: AvailabilityScheduleKind
    weekdays?: number[]
    start: string
    end: string
    action: AvailabilityScheduleAction
  }>
}

export const AVAILABILITY_SCHEDULE_EXTRA_KEY = 'availability_schedule'
export const AVAILABILITY_SCHEDULE_MAX_RULES = 20

export const WEEKDAY_OPTIONS = [
  { value: 1, key: 'mon' },
  { value: 2, key: 'tue' },
  { value: 3, key: 'wed' },
  { value: 4, key: 'thu' },
  { value: 5, key: 'fri' },
  { value: 6, key: 'sat' },
  { value: 7, key: 'sun' }
] as const

function newRuleId(): string {
  return `r_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`
}

export function createEmptyAvailabilityRule(
  kind: AvailabilityScheduleKind = 'daily'
): AvailabilityScheduleRuleForm {
  return {
    id: newRuleId(),
    kind,
    weekdays: kind === 'weekly' ? [1, 2, 3, 4, 5] : [],
    start: '00:00',
    end: '08:00',
    action: 'disable'
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

function parseWeekdays(raw: unknown): number[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<number>()
  const out: number[] = []
  for (const item of raw) {
    const n = typeof item === 'number' ? item : Number(item)
    if (!Number.isInteger(n) || n < 1 || n > 7 || seen.has(n)) continue
    seen.add(n)
    out.push(n)
  }
  return out
}

function normalizeHHMM(value: unknown, fallback: string): string {
  if (typeof value !== 'string') return fallback
  const trimmed = value.trim()
  if (!/^\d{1,2}:\d{2}$/.test(trimmed)) return fallback
  const [h, m] = trimmed.split(':').map((part) => Number(part))
  if (!Number.isInteger(h) || !Number.isInteger(m) || h < 0 || h > 23 || m < 0 || m > 59) {
    return fallback
  }
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`
}

export function parseAvailabilityScheduleFromExtra(extra: unknown): {
  enabled: boolean
  rules: AvailabilityScheduleRuleForm[]
} {
  const root = asRecord(extra)
  const schedule = asRecord(root?.[AVAILABILITY_SCHEDULE_EXTRA_KEY])
  if (!schedule) {
    return { enabled: false, rules: [] }
  }
  const rawRules = Array.isArray(schedule.rules) ? schedule.rules : []
  const rules: AvailabilityScheduleRuleForm[] = []
  for (const item of rawRules) {
    const entry = asRecord(item)
    if (!entry) continue
    const kind = entry.kind === 'weekly' ? 'weekly' : 'daily'
    const action = entry.action === 'enable' ? 'enable' : 'disable'
    rules.push({
      id: typeof entry.id === 'string' && entry.id ? entry.id : newRuleId(),
      kind,
      weekdays: kind === 'weekly' ? parseWeekdays(entry.weekdays) : [],
      start: normalizeHHMM(entry.start, '00:00'),
      end: normalizeHHMM(entry.end, '08:00'),
      action
    })
  }
  return {
    enabled: schedule.enabled === true,
    rules
  }
}

export function buildAvailabilitySchedulePayload(
  enabled: boolean,
  rules: AvailabilityScheduleRuleForm[]
): AvailabilitySchedulePayload | null {
  if (!enabled && rules.length === 0) {
    return null
  }
  return {
    enabled,
    rules: rules.slice(0, AVAILABILITY_SCHEDULE_MAX_RULES).map((rule) => {
      const payload: AvailabilitySchedulePayload['rules'][number] = {
        id: rule.id,
        kind: rule.kind,
        start: normalizeHHMM(rule.start, '00:00'),
        end: normalizeHHMM(rule.end, '08:00'),
        action: rule.action
      }
      if (rule.kind === 'weekly') {
        payload.weekdays = parseWeekdays(rule.weekdays)
      }
      return payload
    })
  }
}

export function applyAvailabilityScheduleToExtra(
  extra: Record<string, unknown>,
  enabled: boolean,
  rules: AvailabilityScheduleRuleForm[]
): void {
  const payload = buildAvailabilitySchedulePayload(enabled, rules)
  if (!payload) {
    delete extra[AVAILABILITY_SCHEDULE_EXTRA_KEY]
    return
  }
  extra[AVAILABILITY_SCHEDULE_EXTRA_KEY] = payload
}

export function validateAvailabilityScheduleRules(
  enabled: boolean,
  rules: AvailabilityScheduleRuleForm[]
): string | null {
  if (!enabled) return null
  if (rules.length === 0) {
    return 'empty'
  }
  if (rules.length > AVAILABILITY_SCHEDULE_MAX_RULES) {
    return 'tooMany'
  }
  for (const rule of rules) {
    if (!/^\d{2}:\d{2}$/.test(rule.start) || !/^\d{2}:\d{2}$/.test(rule.end)) {
      return 'time'
    }
    if (rule.kind === 'weekly' && parseWeekdays(rule.weekdays).length === 0) {
      return 'weekdays'
    }
  }
  return null
}

function minutesInWindow(now: number, start: number, end: number): boolean {
  if (start === end) return true
  if (start < end) return now >= start && now < end
  return now >= start || now < end
}

function hhmmToMinutes(value: string): number {
  const [h, m] = value.split(':').map(Number)
  return h * 60 + m
}

/** Preview whether the first matching rule would force disable at `now`. */
export function previewAvailabilityForcedOff(
  enabled: boolean,
  rules: AvailabilityScheduleRuleForm[],
  now: Date = new Date()
): boolean | null {
  if (!enabled || rules.length === 0) return null
  const weekday = now.getDay() === 0 ? 7 : now.getDay()
  const nowMin = now.getHours() * 60 + now.getMinutes()
  for (const rule of rules) {
    if (rule.kind === 'weekly' && !rule.weekdays.includes(weekday)) {
      continue
    }
    if (!minutesInWindow(nowMin, hhmmToMinutes(rule.start), hhmmToMinutes(rule.end))) {
      continue
    }
    return rule.action === 'disable'
  }
  return null
}
