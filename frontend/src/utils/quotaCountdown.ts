import { i18n } from '@/i18n'

export function quotaCountdown(resetsAt: string | undefined, now: number): string {
  if (!resetsAt || !Number.isFinite(Date.parse(resetsAt))) return '—'
  const seconds = Math.ceil((Date.parse(resetsAt) - now) / 1000)
  if (seconds <= 0) return i18n.global.t('admin.quota.pending')
  const minutes = Math.ceil(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h${minutes % 60 ? ` ${minutes % 60}m` : ''}`
  return `${Math.floor(hours / 24)}d${hours % 24 ? ` ${hours % 24}h` : ''}`
}
