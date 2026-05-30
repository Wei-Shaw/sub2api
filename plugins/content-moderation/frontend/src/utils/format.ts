/**
 * Date formatting helper, self-contained for the plugin bundle.
 *
 * Mirrors the core frontend's formatDateTime (frontend/src/utils/format.ts):
 * full date+time, 24-hour, locale-aware. Locale comes from the host SDK so the
 * plugin follows the host's active language; falls back to the browser locale
 * before the SDK is initialized.
 */
import { getSdk } from '../api/sdk'

function currentLocale(): string {
  try {
    return getSdk().i18n.currentLocale.value
  } catch {
    return typeof navigator !== 'undefined' ? navigator.language : 'en'
  }
}

const DATETIME_OPTIONS: Intl.DateTimeFormatOptions = {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
}

export function formatDateTime(date: string | Date | null | undefined): string {
  if (!date) return ''
  const d = new Date(date)
  if (isNaN(d.getTime())) return ''
  return new Intl.DateTimeFormat(currentLocale(), DATETIME_OPTIONS).format(d)
}
