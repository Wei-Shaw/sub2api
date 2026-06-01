export type OpenAIQuotaStatusWindow = '5h' | '7d'

export const ACCOUNT_STATUS_FILTER_OPENAI_QUOTA_USED_RANGE_PREFIX = 'openai_quota_used_range:'
export const ACCOUNT_STATUS_FILTER_OPENAI_QUOTA_FULL_PREFIX = 'openai_quota_full:'
export const ACCOUNT_STATUS_FILTER_QUOTA_USED_RANGE_PICKER = '__openai_quota_used_range_picker__'
export const ACCOUNT_STATUS_FILTER_QUOTA_FULL_PICKER = '__openai_quota_full_picker__'

export interface OpenAIQuotaUsedRangeStatusFilter {
  window: OpenAIQuotaStatusWindow
  min: number
  max: number
}

const normalizePercent = (value: number): number => {
  if (!Number.isFinite(value)) return Number.NaN
  return Math.min(100, Math.max(0, value))
}

export const encodeOpenAIQuotaUsedRangeStatus = (
  window: OpenAIQuotaStatusWindow,
  min: number,
  max: number
): string => {
  const safeMin = normalizePercent(min)
  const safeMax = normalizePercent(max)
  if (!Number.isFinite(safeMin) || !Number.isFinite(safeMax) || safeMin > safeMax) {
    return ''
  }
  return `${ACCOUNT_STATUS_FILTER_OPENAI_QUOTA_USED_RANGE_PREFIX}${window}:${safeMin}:${safeMax}`
}

export const encodeOpenAIQuotaFullStatus = (window: OpenAIQuotaStatusWindow): string => {
  return `${ACCOUNT_STATUS_FILTER_OPENAI_QUOTA_FULL_PREFIX}${window}`
}

export const parseOpenAIQuotaUsedRangeStatus = (status: string): OpenAIQuotaUsedRangeStatusFilter | null => {
  if (!status.startsWith(ACCOUNT_STATUS_FILTER_OPENAI_QUOTA_USED_RANGE_PREFIX)) return null
  const parts = status.slice(ACCOUNT_STATUS_FILTER_OPENAI_QUOTA_USED_RANGE_PREFIX.length).split(':')
  if (parts.length !== 3) return null
  const window = parts[0] === '5h' || parts[0] === '7d' ? parts[0] : null
  if (!window) return null
  const min = Number(parts[1])
  const max = Number(parts[2])
  if (!Number.isFinite(min) || !Number.isFinite(max) || min < 0 || max < 0 || min > 100 || max > 100 || min > max) {
    return null
  }
  return { window, min, max }
}

export const parseOpenAIQuotaFullStatus = (status: string): OpenAIQuotaStatusWindow | null => {
  if (!status.startsWith(ACCOUNT_STATUS_FILTER_OPENAI_QUOTA_FULL_PREFIX)) return null
  const window = status.slice(ACCOUNT_STATUS_FILTER_OPENAI_QUOTA_FULL_PREFIX.length)
  return window === '5h' || window === '7d' ? window : null
}

export const isOpenAIQuotaEncodedStatus = (status: string): boolean => {
  return parseOpenAIQuotaUsedRangeStatus(status) !== null || parseOpenAIQuotaFullStatus(status) !== null
}
