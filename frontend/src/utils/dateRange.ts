export interface DateRange {
  start: string
  end: string
}

const pad = (value: number) => String(value).padStart(2, '0')

export const formatLocalDate = (date: Date): string =>
  `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`

export const formatLocalMinute = (date: Date): string =>
  `${formatLocalDate(date)}T${pad(date.getHours())}:${pad(date.getMinutes())}`

export function parseDateBoundary(value: string, end = false): Date {
  if (value.includes('T')) return new Date(value)
  const date = new Date(`${value}T00:00:00`)
  if (end) date.setDate(date.getDate() + 1)
  return date
}

// Reject incomplete inputs and nonexistent local minutes at DST transitions.
export function parseLocalMinute(value: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return null
  const date = new Date(value)
  return Number.isFinite(date.getTime()) && formatLocalMinute(date) === value ? date : null
}

export function getDatePresetRange(preset: string, now = new Date()): DateRange | null {
  const end = new Date(now)
  if (preset === 'last24Hours') {
    // Include requests in the current partial minute; exact minutes stay exact.
    end.setTime(Math.ceil(end.getTime() / 60000) * 60000)
    return { start: new Date(end.getTime() - 86400000).toISOString(), end: end.toISOString() }
  }
  const start = new Date(now)
  switch (preset) {
    case 'today': break
    case 'yesterday':
      start.setDate(start.getDate() - 1)
      end.setDate(end.getDate() - 1)
      break
    case '7days': start.setDate(start.getDate() - 6); break
    case '14days': start.setDate(start.getDate() - 13); break
    case '30days': start.setDate(start.getDate() - 29); break
    case 'thisMonth': start.setDate(1); break
    case 'lastMonth':
      start.setDate(1)
      start.setMonth(start.getMonth() - 1)
      end.setDate(0)
      break
    default: return null
  }
  return { start: formatLocalDate(start), end: formatLocalDate(end) }
}

export const getLast24HourRange = (): DateRange => getDatePresetRange('last24Hours')!

export function getGranularityForRange(start: string, end: string): 'day' | 'hour' {
  return parseDateBoundary(end).getTime() - parseDateBoundary(start).getTime() <= 86400000
    ? 'hour' : 'day'
}
