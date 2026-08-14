/**
 * 请求延迟健康度分档（用于用量明细"延迟"列的纵向健康扫视）。
 *
 * 首 Token（TTFT）：10s 内正常，10-30s 偏慢，30-60s 缓慢，60s 及以上严重。
 * 总耗时：流式请求整体时长天然更长，阈值放宽为 1min / 3min / 5min。
 */
export type LatencySeverity = 'good' | 'warn' | 'slow' | 'critical'

export const FIRST_TOKEN_THRESHOLDS_MS = {
  warn: 10_000,
  slow: 30_000,
  critical: 60_000,
} as const

export const DURATION_THRESHOLDS_MS = {
  warn: 60_000,
  slow: 180_000,
  critical: 300_000,
} as const

interface Thresholds {
  warn: number
  slow: number
  critical: number
}

const classify = (ms: number, thresholds: Thresholds): LatencySeverity => {
  if (ms >= thresholds.critical) return 'critical'
  if (ms >= thresholds.slow) return 'slow'
  if (ms >= thresholds.warn) return 'warn'
  return 'good'
}

export const firstTokenSeverity = (ms: number): LatencySeverity =>
  classify(ms, FIRST_TOKEN_THRESHOLDS_MS)

export const durationSeverity = (ms: number): LatencySeverity =>
  classify(ms, DURATION_THRESHOLDS_MS)

/*
 * Four severities, three treatments. `good` is deliberately NOT green: a
 * latency that is fine is the normal case, and painting the normal case spends
 * the signal budget on the rows nobody needs to look at. It would also break
 * the rule that colour on a data surface marks exceptions only.
 */
export const LATENCY_TEXT_CLASSES: Record<LatencySeverity, string> = {
  good: 'text-ink-secondary',
  warn: 'text-warn',
  // Burnt orange is its own ramp in the config precisely so a middle step like
  // this one stays distinguishable from amber without becoming red.
  slow: 'text-orange-600 dark:text-orange-400',
  critical: 'text-danger',
}

/** The severity bar beside the two latency numbers. */
export const LATENCY_BAR_CLASSES: Record<LatencySeverity, string> = {
  good: 'bg-status-neutral',
  warn: 'bg-warn',
  slow: 'bg-orange-500',
  critical: 'bg-danger',
}
