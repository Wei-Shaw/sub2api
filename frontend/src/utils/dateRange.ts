/**
 * 日期范围工具
 * 后端 start_date/end_date 同时接受 YYYY-MM-DD(整天)和 RFC3339(精确时刻,半开区间上界)
 */

const pad = (n: number): string => String(n).padStart(2, '0')

/** 本地时区 RFC3339(带偏移),如 2026-07-11T14:03:22+08:00 */
export function formatDateTimeRFC3339(date: Date): string {
  const offsetMinutes = -date.getTimezoneOffset()
  const sign = offsetMinutes >= 0 ? '+' : '-'
  const abs = Math.abs(offsetMinutes)
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}` +
    `${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`
  )
}

/** 滚动 24 小时窗口(RFC3339 边界) */
export function getLast24HourRange(): { start: string; end: string } {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return { start: formatDateTimeRFC3339(start), end: formatDateTimeRFC3339(end) }
}

/** 范围边界值是否带时间部分(RFC3339) */
export function hasTimeComponent(value: string): boolean {
  return value.includes('T')
}

/** 供 <input type="date"> 显示的日期部分 */
export function toDateInputValue(value: string): string {
  return value.slice(0, 10)
}

/** 解析范围边界(纯日期按本地 00:00) */
export function parseRangeBoundary(value: string): Date {
  return hasTimeComponent(value) ? new Date(value) : new Date(`${value}T00:00:00`)
}
