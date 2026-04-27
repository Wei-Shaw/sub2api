/**
 * 格式化工具函数
 * 参考 CRS 项目的 format.js 实现
 */

import { i18n, getLocale } from '@/i18n'

/**
 * 格式化相对时间
 * @param date 日期字符串或 Date 对象
 * @returns 相对时间字符串，如 "5m ago", "2h ago", "3d ago"
 */
export function formatRelativeTime(date: string | Date | null | undefined): string {
  if (!date) return i18n.global.t('common.time.never')

  const now = new Date()
  const past = new Date(date)
  const diffMs = now.getTime() - past.getTime()

  // 处理未来时间或无效日期
  if (diffMs < 0 || isNaN(diffMs)) return i18n.global.t('common.time.never')

  const diffSecs = Math.floor(diffMs / 1000)
  const diffMins = Math.floor(diffSecs / 60)
  const diffHours = Math.floor(diffMins / 60)
  const diffDays = Math.floor(diffHours / 24)

  if (diffDays > 0) return i18n.global.t('common.time.daysAgo', { n: diffDays })
  if (diffHours > 0) return i18n.global.t('common.time.hoursAgo', { n: diffHours })
  if (diffMins > 0) return i18n.global.t('common.time.minutesAgo', { n: diffMins })
  return i18n.global.t('common.time.justNow')
}

/**
 * 格式化数字（支持 K/M/B 单位）
 * @param num 数字
 * @returns 格式化后的字符串，如 "1.2K", "3.5M"
 */
export function formatNumber(num: number | null | undefined): string {
  if (num === null || num === undefined) return '0'

  const locale = getLocale()
  const absNum = Math.abs(num)

  // Use Intl.NumberFormat for compact notation if supported and needed
  // Note: Compact notation in 'zh' uses '万/亿', which is appropriate for Chinese
  const formatter = new Intl.NumberFormat(locale, {
    notation: absNum >= 10000 ? 'compact' : 'standard',
    maximumFractionDigits: 1
  })

  return formatter.format(num)
}

/**
 * 格式化货币金额
 *
 * 行为：
 * - null / undefined / NaN / 非有限数 → "$0.00"
 * - 正常数值先经 toPrecision(10) 消除 IEEE 754 浮点误差（参考 CLAUDE.md §12 "前端显示规范 - 浮点精度"），
 *   再用 Intl.NumberFormat 按当前 locale 以货币样式输出（默认 USD → "$1.25"）
 * - 极小金额（0 < amount < 0.01）显示 6 位小数，其他情况 2 位小数
 *
 * 合并自原 `formatLimitUsd`，作为货币展示的唯一权威实现。
 *
 * @param amount 金额，可能为 null/undefined
 * @param currency 货币代码，默认 USD
 * @returns 本地化的货币字符串（含货币符号，如 "$1.25"）
 */
export function formatCurrency(amount: number | null | undefined, currency: string = 'USD'): string {
  if (amount === null || amount === undefined) return '$0.00'
  const num = Number(amount)
  if (!Number.isFinite(num)) return '$0.00'

  const locale = getLocale()

  // IEEE 754 误差消除：例如 5e-8 * 1e6 = 0.04999...96 → 0.05
  const normalized = Number(num.toPrecision(10))

  // For very small amounts, show more decimals
  const fractionDigits = normalized > 0 && normalized < 0.01 ? 6 : 2

  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: currency,
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits
  }).format(normalized)
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
 * @param options Intl.DateTimeFormatOptions
 * @param localeOverride 可选 locale 覆盖
 * @returns 格式化后的日期字符串
 */
export function formatDate(
  date: string | Date | null | undefined,
  options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  },
  localeOverride?: string
): string {
  if (!date) return ''

  const d = new Date(date)
  if (isNaN(d.getTime())) return ''

  const locale = localeOverride ?? getLocale()
  return new Intl.DateTimeFormat(locale, options).format(d)
}

/**
 * 格式化日期（只显示日期部分）
 * @param date 日期字符串或 Date 对象
 * @returns 格式化后的日期字符串
 */
export function formatDateOnly(date: string | Date | null | undefined): string {
  return formatDate(date, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  })
}

/**
 * 格式化日期时间（完整格式）
 * @param date 日期字符串或 Date 对象
 * @param options Intl.DateTimeFormatOptions
 * @param localeOverride 可选 locale 覆盖
 * @returns 格式化后的日期时间字符串
 */
export function formatDateTime(
  date: string | Date | null | undefined,
  options?: Intl.DateTimeFormatOptions,
  localeOverride?: string
): string {
  return formatDate(date, options, localeOverride)
}

/**
 * 格式化为 datetime-local 控件值（YYYY-MM-DDTHH:mm，使用本地时间）
 */
export function formatDateTimeLocalInput(timestampSeconds: number | null): string {
  if (!timestampSeconds) return ''
  const date = new Date(timestampSeconds * 1000)
  if (isNaN(date.getTime())) return ''
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

/**
 * 解析 datetime-local 控件值为时间戳（秒，使用本地时间）
 */
export function parseDateTimeLocalInput(value: string): number | null {
  if (!value) return null
  const date = new Date(value)
  if (isNaN(date.getTime())) return null
  return Math.floor(date.getTime() / 1000)
}

/**
 * 格式化 OpenAI reasoning effort（用于使用记录展示）
 * @param effort 原始 effort（如 "low" / "medium" / "high" / "xhigh"）
 * @returns 格式化后的字符串（Low / Medium / High / Xhigh），无值返回 "-"
 */
export function formatReasoningEffort(effort: string | null | undefined): string {
  const raw = (effort ?? '').toString().trim()
  if (!raw) return '-'

  const normalized = raw.toLowerCase().replace(/[-_\s]/g, '')
  switch (normalized) {
    case 'low':
      return 'Low'
    case 'medium':
      return 'Medium'
    case 'high':
      return 'High'
    case 'xhigh':
    case 'extrahigh':
      return 'XHigh'
    case 'max':
      return 'Max'
    case 'none':
    case 'minimal':
      return '-'
    default:
      // best-effort: Title-case first letter
      return raw.length > 1 ? raw[0].toUpperCase() + raw.slice(1) : raw.toUpperCase()
  }
}

/**
 * 格式化时间（显示时分秒）
 * @param date 日期字符串或 Date 对象
 * @returns 格式化后的时间字符串
 */
export function formatTime(date: string | Date | null | undefined): string {
  return formatDate(date, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  })
}

/**
 * 格式化数字（千分位分隔，不使用紧凑单位）
 * @param num 数字
 * @returns 格式化后的字符串，如 "12,345"
 */
export function formatNumberLocaleString(num: number): string {
  return num.toLocaleString()
}

/**
 * 美式千分位逗号格式化（强制 en-US locale，不随浏览器语言变化）。
 *
 * 适用于"始终用 1,234,567 这种风格"的场景——比如限额配置/监控里的限流值，
 * 后端是与货币/单位无关的纯数字，统一千分位逗号让管理员一眼看清量级。
 *
 * 与 formatNumberLocaleString 的区别：后者跟随浏览器 locale，可能渲染成
 * 1.234.567（德语）/ 1 234 567（法语）等；本函数固定 en-US。
 */
export function formatThousands(num: number): string {
  if (!Number.isFinite(num)) return '0'
  return num.toLocaleString('en-US')
}

/**
 * 格式化金额（固定小数位，不带货币符号）
 * @param amount 金额
 * @param fractionDigits 小数位数，默认 4
 * @returns 格式化后的字符串，如 "1.2345"
 */
export function formatCostFixed(amount: number, fractionDigits: number = 4): string {
  return amount.toFixed(fractionDigits)
}

/**
 * service-quota daily_usd 限额展示：美式千分位 + 最多 6 位小数（去尾零）。
 *
 * 与 formatCurrency 的差异：
 *   - 这里的 daily_usd 已经是"美元金额数值"，本身就是 service-quota 限额配置场景，
 *     不走 IEEE 754 误差消除（输入由用户在 NumericInput 里直接录入）
 *   - 不固定 2 位小数（限额可能是 0.05 这种小金额）
 *   - 强制 en-US locale，与 formatThousands 一致（避免随浏览器语言变成
 *     "100.000,00" 等欧式风格，让管理员一眼对齐量级）
 *
 * minimumFractionDigits 默认 0（去尾零）；监控页用量列传 2，让 `0.00 / 50.00` 对齐。
 *
 * 示例：
 *   formatDailyUsd(100000)        → "100,000"
 *   formatDailyUsd(1234.56)       → "1,234.56"
 *   formatDailyUsd(0.05)          → "0.05"
 *   formatDailyUsd(0)             → "0"
 *   formatDailyUsd(100000, 2)     → "100,000.00"
 *   formatDailyUsd(0, 2)          → "0.00"
 *
 * 调用方负责前缀 "$"（监控页 chip 不带符号、配置页 limit chip 带 $）。
 */
export function formatDailyUsd(amount: number | null | undefined, minimumFractionDigits: number = 0): string {
  if (amount === null || amount === undefined || !Number.isFinite(Number(amount))) {
    return (0).toLocaleString('en-US', {
      minimumFractionDigits,
      maximumFractionDigits: Math.max(minimumFractionDigits, 6),
    })
  }
  return Number(amount).toLocaleString('en-US', {
    minimumFractionDigits,
    maximumFractionDigits: Math.max(minimumFractionDigits, 6),
  })
}

/**
 * 格式化 token 数量（>=1M 显示为 M，>=1K 显示为 K，保留 1 位小数）
 * @param tokens token 数量
 * @returns 格式化后的字符串，如 "950", "1.2K", "3.5M"
 */
export function formatTokensK(tokens: number): string {
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}K`
  return tokens.toString()
}

/**
 * 格式化大数字（K/M/B，保留 1 位小数）
 * @param num 数字
 * @param options allowBillions=false 时最高只显示到 M
 */
export function formatCompactNumber(
  num: number | null | undefined,
  options?: { allowBillions?: boolean }
): string {
  if (num === null || num === undefined) return '0'

  const abs = Math.abs(num)
  const allowBillions = options?.allowBillions !== false

  if (allowBillions && abs >= 1_000_000_000) return `${(num / 1_000_000_000).toFixed(1)}B`
  if (abs >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`
  if (abs >= 1_000) return `${(num / 1_000).toFixed(1)}K`
  return num.toString()
}

/**
 * 格式化倒计时（从现在到目标时间的剩余时间）
 * @param targetDate 目标日期字符串或 Date 对象
 * @returns 倒计时字符串，如 "2h 41m", "3d 5h", "15m"
 */
export function formatCountdown(targetDate: string | Date | null | undefined): string | null {
  if (!targetDate) return null

  const now = new Date()
  const target = new Date(targetDate)
  const diffMs = target.getTime() - now.getTime()

  // 如果目标时间已过或无效
  if (diffMs <= 0 || isNaN(diffMs)) return null

  const diffMins = Math.floor(diffMs / (1000 * 60))
  const diffHours = Math.floor(diffMins / 60)
  const diffDays = Math.floor(diffHours / 24)

  const remainingHours = diffHours % 24
  const remainingMins = diffMins % 60

  if (diffDays > 0) {
    // 超过1天：显示 "Xd Yh"
    return i18n.global.t('common.time.countdown.daysHours', { d: diffDays, h: remainingHours })
  }
  if (diffHours > 0) {
    // 小于1天：显示 "Xh Ym"
    return i18n.global.t('common.time.countdown.hoursMinutes', { h: diffHours, m: remainingMins })
  }
  // 小于1小时：显示 "Ym"
  return i18n.global.t('common.time.countdown.minutes', { m: diffMins })
}

/**
 * 格式化倒计时并带后缀（如 "2h 41m 后解除"）
 * @param targetDate 目标日期字符串或 Date 对象
 * @returns 完整的倒计时字符串，如 "2h 41m to lift", "2小时41分钟后解除"
 */
export function formatCountdownWithSuffix(targetDate: string | Date | null | undefined): string | null {
  const countdown = formatCountdown(targetDate)
  if (!countdown) return null
  return i18n.global.t('common.time.countdown.withSuffix', { time: countdown })
}

/**
 * 格式化为相对时间 + 具体时间组合
 * @param date 日期字符串或 Date 对象
 * @returns 组合时间字符串，如 "5 天前 · 2026-01-27 15:25"
 */
export function formatRelativeWithDateTime(date: string | Date | null | undefined): string {
  if (!date) return ''

  const relativeTime = formatRelativeTime(date)
  const dateTime = formatDateTime(date)

  // 如果是 "从未" 或空字符串，只返回相对时间
  if (!dateTime || relativeTime === i18n.global.t('common.time.never')) {
    return relativeTime
  }

  return `${relativeTime} · ${dateTime}`
}
