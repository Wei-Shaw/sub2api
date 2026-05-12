/**
 * 格式化缓存 token 数量（1K/1M 缩写）
 */
export function formatCacheTokens(tokens: number): string {
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}K`
  return tokens.toLocaleString()
}

/**
 * 格式化普通 token 数量（1K/1M/1B 缩写）
 */
export function formatTokensCompact(tokens: number | null | undefined): string {
  const value = Number(tokens) || 0
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

/**
 * 缓存读取占可缓存输入 token 的比例。
 */
export function formatTokenCacheRate(inputTokens: number | null | undefined, cacheReadTokens: number | null | undefined): string {
  const input = Math.max(0, Number(inputTokens) || 0)
  const cacheRead = Math.max(0, Number(cacheReadTokens) || 0)
  const total = input + cacheRead
  if (total <= 0) return '0.00%'
  return `${((cacheRead / total) * 100).toFixed(2)}%`
}

/**
 * 自适应精度格式化倍率（确保小数值如 0.001 不被截断）
 */
export function formatMultiplier(val: number): string {
  if (val >= 0.01) return val.toFixed(2)
  if (val >= 0.001) return val.toFixed(3)
  if (val >= 0.0001) return val.toFixed(4)
  return val.toPrecision(2)
}
