/**
 * Convert between ISO timestamps and datetime-local input values
 * without pulling the i18n-backed format helpers.
 */

export function formatDateTimeLocalInputFromISO(iso: string | null | undefined): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (isNaN(date.getTime())) return ''
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

export function dateTimeLocalInputToISO(value: string): string | undefined {
  if (!value) return undefined
  const match = /^(\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?$/.exec(value)
  if (!match) return undefined

  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const hours = Number(match[4])
  const minutes = Number(match[5])
  const seconds = match[6] ? Number(match[6]) : 0
  const milliseconds = match[7] ? Number(match[7].slice(0, 3).padEnd(3, '0')) : 0
  if (
    !Number.isSafeInteger(year) ||
    year < 1 ||
    month < 1 ||
    month > 12 ||
    day < 1 ||
    day > 31 ||
    hours > 23 ||
    minutes > 59 ||
    seconds > 59
  ) {
    return undefined
  }

  const date = new Date(0)
  date.setFullYear(year, month - 1, day)
  date.setHours(hours, minutes, seconds, milliseconds)
  if (
    date.getFullYear() !== year ||
    date.getMonth() !== month - 1 ||
    date.getDate() !== day
  ) {
    return undefined
  }
  return Number.isFinite(date.getTime()) ? date.toISOString() : undefined
}
