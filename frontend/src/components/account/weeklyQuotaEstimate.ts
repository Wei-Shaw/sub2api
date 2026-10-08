import type { UsageProgress } from '@/types'

// The same account-cost / utilization calculation already used for OpenAI.
// This is a local cost equivalent, not an upstream dollar/token allowance.
export function estimateWindowTotalCost(window?: UsageProgress | null): number | null {
  const utilization = window?.utilization
  const cost = window?.window_stats?.cost
  if (
    typeof utilization !== 'number' || !Number.isFinite(utilization) || utilization <= 0 ||
    typeof cost !== 'number' || !Number.isFinite(cost) || cost <= 0
  ) return null

  const estimate = cost * 100 / utilization
  return Number.isFinite(estimate) && estimate > 0 ? estimate : null
}
