/**
 * 格式化工具函数
 * 参考 CRS 项目的 format.js 实现
 */

/**
 * 格式化相对时间
 * @param date 日期字符串或 Date 对象
 * @returns 相对时间字符串，如 "5m ago", "2h ago", "3d ago"
 */
export function formatRelativeTime(date: string | Date | null | undefined): string {
  if (!date) return 'Never'

  const now = new Date()
  const past = new Date(date)
  const diffMs = now.getTime() - past.getTime()

  // 处理未来时间或无效日期
  if (diffMs < 0 || isNaN(diffMs)) return 'Never'

  const diffSecs = Math.floor(diffMs / 1000)
  const diffMins = Math.floor(diffSecs / 60)
  const diffHours = Math.floor(diffMins / 60)
  const diffDays = Math.floor(diffHours / 24)

  if (diffDays > 0) return `${diffDays}d ago`
  if (diffHours > 0) return `${diffHours}h ago`
  if (diffMins > 0) return `${diffMins}m ago`
  return 'Just now'
}

/**
 * 格式化数字（支持 K/M/B 单位）
 * @param num 数字
 * @returns 格式化后的字符串，如 "1.2K", "3.5M"
 */
export function formatNumber(num: number | null | undefined): string {
  if (num === null || num === undefined) return '0'

  const absNum = Math.abs(num)

  if (absNum >= 1e9) {
    return (num / 1e9).toFixed(2) + 'B'
  } else if (absNum >= 1e6) {
    return (num / 1e6).toFixed(2) + 'M'
  } else if (absNum >= 1e3) {
    return (num / 1e3).toFixed(1) + 'K'
  }

  return num.toLocaleString()
}

/**
 * 格式化货币金额
 * @param amount 金额
 * @returns 格式化后的字符串，如 "$1.25" 或 "$0.000123"
 */
export function formatCurrency(amount: number | null | undefined): string {
  if (amount === null || amount === undefined) return '$0.00'

  // 小于 0.01 时显示更多小数位
  if (amount > 0 && amount < 0.01) {
    return '$' + amount.toFixed(6)
  }

  return '$' + amount.toFixed(2)
}

/**
 * 格式化字节大小
 * @param bytes 字节数
 * @param decimals 小数位数
 * @returns 格式化后的字符串，如 "1.5 MB"
 */
export function formatBytes(bytes: number, decimals: number = 2): string {
  if (bytes === 0) return '0 Bytes'

  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB']

  const i = Math.floor(Math.log(bytes) / Math.log(k))

  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}

/**
 * 格式化日期
 * @param date 日期字符串或 Date 对象
 * @param format 格式字符串，支持 YYYY, MM, DD, HH, mm, ss
 * @returns 格式化后的日期字符串
 */
export function formatDate(
  date: string | Date | null | undefined,
  format: string = 'YYYY-MM-DD HH:mm:ss'
): string {
  if (!date) return ''

  const d = new Date(date)
  if (isNaN(d.getTime())) return ''

  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  const seconds = String(d.getSeconds()).padStart(2, '0')

  return format
    .replace('YYYY', String(year))
    .replace('MM', month)
    .replace('DD', day)
    .replace('HH', hours)
    .replace('mm', minutes)
    .replace('ss', seconds)
}

/**
 * 格式化日期（只显示日期部分）
 * @param date 日期字符串或 Date 对象
 * @returns 格式化后的日期字符串，格式为 YYYY-MM-DD
 */
export function formatDateOnly(date: string | Date | null | undefined): string {
  return formatDate(date, 'YYYY-MM-DD')
}

/**
 * 格式化日期时间（完整格式）
 * @param date 日期字符串或 Date 对象
 * @returns 格式化后的日期时间字符串，格式为 YYYY-MM-DD HH:mm:ss
 */
export function formatDateTime(date: string | Date | null | undefined): string {
  return formatDate(date, 'YYYY-MM-DD HH:mm:ss')
}

/**
 * 格式化时间（只显示时分）
 * @param date 日期字符串或 Date 对象
 * @returns 格式化后的时间字符串，格式为 HH:mm
 */
export function formatTime(date: string | Date | null | undefined): string {
  return formatDate(date, 'HH:mm')
}
