import { dateRangePresets } from '@/utils/dateRangePresets'
import { formatDateLocalInput } from '@/utils/format'

export interface AdminDashboardPreferences {
  startDate: string
  endDate: string
  preset: string | null
  granularity: 'day' | 'hour'
}

const storageKey = (userId: number) => `admin-dashboard-preferences:${userId}`

const isValidDate = (value: unknown): value is string => {
  return typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value) &&
    formatDateLocalInput(new Date(`${value}T00:00:00`)) === value
}

function parsePreferences(value: unknown): AdminDashboardPreferences | null {
  if (!value || typeof value !== 'object') return null
  const data = value as Record<string, unknown>
  if (data.version !== 1 || (data.granularity !== 'day' && data.granularity !== 'hour')) return null

  if (typeof data.preset === 'string') {
    const preset = dateRangePresets.find((preset) => preset.value === data.preset)
    if (!preset) return null
    const range = preset.getRange()
    return {
      startDate: range.start,
      endDate: range.end,
      preset: preset.value,
      granularity: data.granularity
    }
  }

  if (data.preset !== null || !isValidDate(data.startDate) || !isValidDate(data.endDate) ||
    data.startDate > data.endDate) return null

  return {
    startDate: data.startDate,
    endDate: data.endDate,
    preset: null,
    granularity: data.granularity
  }
}

export function loadAdminDashboardPreferences(userId: number | undefined): AdminDashboardPreferences | null {
  if (userId === undefined) return null
  try {
    const saved = localStorage.getItem(storageKey(userId))
    return saved ? parsePreferences(JSON.parse(saved)) : null
  } catch {
    return null
  }
}

export function saveAdminDashboardPreferences(userId: number | undefined, preferences: AdminDashboardPreferences): void {
  if (userId === undefined) return
  const data = {
    version: 1,
    preset: preferences.preset,
    granularity: preferences.granularity,
    ...(preferences.preset === null ? {
      startDate: preferences.startDate,
      endDate: preferences.endDate
    } : {})
  }
  if (!parsePreferences(data)) return
  try {
    localStorage.setItem(storageKey(userId), JSON.stringify(data))
  } catch {
    return
  }
}
