import { videoTaskStatuses, type VideoTaskAdminItem, type VideoTaskStatus } from '@/api/admin/videoTasks'

export function displayStatus(item: VideoTaskAdminItem, now = Date.now()): VideoTaskStatus | 'unknown' {
  if (item.kind === 'submission') return 'submission_unknown'
  const status = item.display_status
  // The backend supplies the status. Keep a pending task safe if the response ages past two hours.
  if (status === 'in_progress' && item.state === 'pending' &&
      now - Date.parse(item.created_at) > 2 * 60 * 60 * 1000) return 'delayed'
  return videoTaskStatuses.includes(status as VideoTaskStatus) ? status as VideoTaskStatus : 'unknown'
}

export function statusBadge(status: string): string {
  if (status === 'completed' || status === 'charged') return 'badge-success'
  if (status === 'failed') return 'badge-danger'
  if (['delayed', 'query_retry', 'billing_pending', 'submission_unknown', 'review_required', 'pending', 'review'].includes(status)) return 'badge-warning'
  return status === 'in_progress' ? 'badge-primary' : 'badge-gray'
}

// Never render unrecognized backend codes or upstream error messages.
const safeReasons = new Set([
  'poll_failed', 'poll_timeout', 'upstream_failed', 'upstream_expired',
  'completion_save_failed', 'billing_failed', 'upstream_cancelled',
  'poll_not_found', 'poll_exhausted', 'account_route_changed'
])
export function reasonKey(code: string): string | null {
  if (!code) return null
  return `admin.videoTasks.reasons.${safeReasons.has(code) ? code : 'unknown'}`
}

export function formatCost(value: number | null | undefined, locale: string): string {
  return value == null || !Number.isFinite(value)
    ? '—'
    : new Intl.NumberFormat(locale, { style: 'currency', currency: 'USD', minimumFractionDigits: 2, maximumFractionDigits: 8 }).format(value)
}

export function formatTaskTime(value: string | null | undefined, locale: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString(locale)
}

export function elapsedParts(item: VideoTaskAdminItem): { hours: number; minutes: number; seconds: number } | null {
  const elapsed = Date.parse(item.completed_at || item.updated_at) - Date.parse(item.created_at)
  if (!Number.isFinite(elapsed) || elapsed < 0) return null
  const seconds = Math.floor(elapsed / 1000)
  return { hours: Math.floor(seconds / 3600), minutes: Math.floor(seconds / 60) % 60, seconds: seconds % 60 }
}
