import type { UsageLog } from '@/types'

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

/**
 * 中转开销（毫秒）的分档阈值。
 *
 * 与上面两组**绝对延迟**阈值不同：那两组衡量「这次请求慢不慢」，本组衡量
 * 「中转自己吃掉了多少」——这是**本可以避免**的那部分，所以容忍度低得多。
 *
 * 首字开销（中转在供应商首字基础上多加的毫秒数）：
 *   ≤300ms   无感
 *   300~1s   略可感
 *   1~3s     明显偏重，值得关注
 *   >3s      严重，中转已成瓶颈
 *
 * 总耗时开销的阈值放宽一档：总耗时本身量级更大，且流式响应里几百毫秒的
 * 调度差异属于正常抖动。
 *
 * 负值（本站比上游还快，通常是时钟或采样差异）一律归入 good。
 */
export const FIRST_TOKEN_OVERHEAD_MS_THRESHOLDS = {
  warn: 300,
  slow: 1_000,
  critical: 3_000,
} as const

export const DURATION_OVERHEAD_MS_THRESHOLDS = {
  warn: 1_000,
  slow: 3_000,
  critical: 10_000,
} as const

export const firstTokenOverheadSeverity = (ms: number): LatencySeverity =>
  classify(ms, FIRST_TOKEN_OVERHEAD_MS_THRESHOLDS)

export const durationOverheadSeverity = (ms: number): LatencySeverity =>
  classify(ms, DURATION_OVERHEAD_MS_THRESHOLDS)

/**
 * 计算「中转开销」= 本站延迟 − 上游延迟，单位毫秒。
 *
 * 用差值而不是百分比：分母（上游值）一旦很小，比值就会爆炸成几百上千个百分点，
 * 反而看不出绝对量；排障时真正要回答的是「中转吃掉了多少毫秒」。
 *
 * 任一侧缺失（上游账单尚未对账到）时返回 null —— 返回 0 会被读成
 * 「完全没有中转开销」，与「还不知道」正好相反。
 */
export const upstreamOverheadMs = (
  siteMs: number | null | undefined,
  upstreamMs: number | null | undefined,
): number | null => {
  if (siteMs == null || upstreamMs == null) return null
  return Math.round(siteMs - upstreamMs)
}

export const LATENCY_TEXT_CLASSES: Record<LatencySeverity, string> = {
  good: 'text-emerald-600 dark:text-emerald-400',
  warn: 'text-amber-600 dark:text-amber-400',
  slow: 'text-orange-600 dark:text-orange-400',
  critical: 'text-red-600 dark:text-red-400',
}

/** 无首字数据时的纯色色条（仅按总耗时档着色）。 */
export const LATENCY_BAR_CLASSES: Record<LatencySeverity, string> = {
  good: 'bg-emerald-500',
  warn: 'bg-amber-400',
  slow: 'bg-orange-500',
  critical: 'bg-red-500',
}

/** 渐变色条上端（首字档）；与 LATENCY_BAR_TO_CLASSES 组合成上下渐变，避免两段硬切割裂感。 */
export const LATENCY_BAR_FROM_CLASSES: Record<LatencySeverity, string> = {
  good: 'from-emerald-500',
  warn: 'from-amber-400',
  slow: 'from-orange-500',
  critical: 'from-red-500',
}

/** 渐变色条下端（总耗时档）。 */
export const LATENCY_BAR_TO_CLASSES: Record<LatencySeverity, string> = {
  good: 'to-emerald-500',
  warn: 'to-amber-400',
  slow: 'to-orange-500',
  critical: 'to-red-500',
}

// 与运维 Token 请求统计一致：使用完整耗时，输出可能包含推理 Token。
export const formatUsageOutputRate = (row: Pick<UsageLog,
  'output_tokens' | 'duration_ms' | 'image_count' | 'image_output_tokens' | 'billing_mode' | 'request_type'
>): string => {
  const { output_tokens: outputTokens, duration_ms: durationMs } = row
  if (row.image_count > 0 || row.image_output_tokens > 0 || row.billing_mode === 'image'
    || (row.request_type && !['sync', 'stream', 'ws_v2', 'cyber'].includes(row.request_type))
    || !Number.isFinite(outputTokens) || outputTokens <= 0
    || durationMs == null || !Number.isFinite(durationMs) || durationMs <= 0) {
    return '—'
  }
  return `${(outputTokens * 1000 / durationMs).toFixed(1)} tok/s`
}
