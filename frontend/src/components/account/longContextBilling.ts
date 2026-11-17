import type { AdminGroup } from '@/types'

export function allSelectedGroupsEnableLongContextPricing(
  groupIds: number[],
  groups: AdminGroup[]
): boolean {
  if (groupIds.length === 0) return false
  const selectedGroups = groups.filter(group => groupIds.includes(group.id))
  return selectedGroups.length === groupIds.length &&
    selectedGroups.every(group => group.long_context_pricing_enabled === true)
}
