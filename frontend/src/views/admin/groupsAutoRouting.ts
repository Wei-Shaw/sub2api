import type { AdminGroup, GroupPlatform, GroupRoutingMode, SubscriptionType } from '@/types'

export type GroupTypeSelection = SubscriptionType | 'auto'

type GroupRoutingDisplaySource = Pick<
  AdminGroup,
  'routing_mode' | 'subscription_type' | 'rate_multiplier'
>

export const groupTypeSelection = (
  group: Pick<GroupRoutingDisplaySource, 'routing_mode' | 'subscription_type'>
): GroupTypeSelection =>
  group.routing_mode === 'auto_lowest_cost' ? 'auto' : group.subscription_type

export const groupTypeConfiguration = (
  selection: GroupTypeSelection
): { subscriptionType: SubscriptionType; routingMode: GroupRoutingMode } => ({
  subscriptionType: selection === 'auto' ? 'standard' : selection,
  routingMode: selection === 'auto' ? 'auto_lowest_cost' : 'fixed'
})

export const groupBillingTypeDisplay = (group: GroupRoutingDisplaySource): GroupTypeSelection =>
  groupTypeSelection(group)

export const groupRateMultiplierDisplay = (group: GroupRoutingDisplaySource): string =>
  group.routing_mode === 'auto_lowest_cost' ? 'auto' : `${group.rate_multiplier}x`

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
