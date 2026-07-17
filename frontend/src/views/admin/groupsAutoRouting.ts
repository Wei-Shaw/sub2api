import type { AdminGroup, GroupPlatform } from '@/types'

export const filterAutoRoutingCandidates = (
  groups: AdminGroup[],
  platform: GroupPlatform,
  currentGroupId?: number
): AdminGroup[] =>
  groups.filter(
    (group) =>
      group.id !== currentGroupId &&
      group.platform === platform &&
      group.status === 'active' &&
      group.subscription_type === 'standard' &&
      group.routing_mode !== 'auto_lowest_cost'
  )
