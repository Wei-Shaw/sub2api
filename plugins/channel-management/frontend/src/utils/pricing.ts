/**
 * Plugin-local pricing format helper.
 *
 * V5 W9 内联自 host frontend `src/utils/pricing.ts`. 见 host 注释:
 *   formatScaled(0.000003, 1_000_000) → "$3"        // per 1M tokens
 *   formatScaled(0.5,        1)        → "$0.5"      // per request
 *   formatScaled(null,       1_000_000) → "-"
 *
 * toPrecision(10) 后再剪去尾随 0 是为了避免 IEEE 754 浮点显示噪声.
 */
export function formatScaled(value: number | null, scale: number): string {
  if (value == null) return '-'
  return `$${(value * scale).toPrecision(10).replace(/\.?0+$/, '')}`
}
