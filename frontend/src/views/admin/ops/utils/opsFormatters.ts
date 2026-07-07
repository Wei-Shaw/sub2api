/**
 * Ops 页面共享的格式化/样式工具。
 *
 * 目标：尽量对齐 `docs/sub2api` 备份版本的视觉表现（需求一致部分保持一致），
 * 同时避免引入额外 UI 依赖。
 */

import type { OpsSeverity } from '@/api/admin/ops'
import { formatBytes } from '@/utils/format'

export function getSeverityClass(severity: OpsSeverity): string {
  const classes: Record<string, string> = {
    P0: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
    P1: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400',
    P2: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400',
    P3: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400'
  }
  return classes[String(severity || '')] || classes.P3
}

export function truncateMessage(msg: string, maxLength = 80): string {
  if (!msg) return ''
  return msg.length > maxLength ? msg.substring(0, maxLength) + '...' : msg
}

/**
 * 格式化日期时间（短格式，和旧 Ops 页面一致）。
 * 输出: `MM-DD HH:mm:ss`
 */
export function formatDateTime(dateStr: string): string {
  const d = new Date(dateStr)
  if (Number.isNaN(d.getTime())) return ''
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
}

export function sumNumbers(values: Array<number | null | undefined>): number {
  return values.reduce<number>((acc, v) => {
    const n = typeof v === 'number' && Number.isFinite(v) ? v : 0
    return acc + n
  }, 0)
}

/**
 * 解析 time_range 为分钟数。
 * 支持：`5m/30m/1h/6h/24h`
 */
export function parseTimeRangeMinutes(range: string): number {
  const trimmed = (range || '').trim()
  if (!trimmed) return 60
  if (trimmed.endsWith('m')) {
    const v = Number.parseInt(trimmed.slice(0, -1), 10)
    return Number.isFinite(v) && v > 0 ? v : 60
  }
  if (trimmed.endsWith('h')) {
    const v = Number.parseInt(trimmed.slice(0, -1), 10)
    return Number.isFinite(v) && v > 0 ? v * 60 : 60
  }
  return 60
}

export function formatHistoryLabel(date: string | undefined, timeRange: string): string {
  if (!date) return ''
  const d = new Date(date)
  if (Number.isNaN(d.getTime())) return ''
  const minutes = parseTimeRangeMinutes(timeRange)
  if (minutes >= 24 * 60) {
    return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  }
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

export function formatByteRate(bytes: number, windowMinutes: number): string {
  const seconds = Math.max(1, (windowMinutes || 1) * 60)
  return `${formatBytes(bytes / seconds, 1)}/s`
}
