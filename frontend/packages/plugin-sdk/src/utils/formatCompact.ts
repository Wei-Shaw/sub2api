/**
 * Format large numbers with K/M/B suffixes (1 decimal place).
 *
 * Standalone pure function with no external dependencies, safe for
 * plugin-sdk consumption.
 *
 * @param num - Number to format
 * @param options - allowBillions=false caps at M
 */
export function formatCompactNumber(
  num: number | null | undefined,
  options?: { allowBillions?: boolean },
): string {
  if (num === null || num === undefined) return '0'

  const abs = Math.abs(num)
  const allowBillions = options?.allowBillions !== false

  if (allowBillions && abs >= 1_000_000_000) return `${(num / 1_000_000_000).toFixed(1)}B`
  if (abs >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`
  if (abs >= 1_000) return `${(num / 1_000).toFixed(1)}K`
  return num.toString()
}
