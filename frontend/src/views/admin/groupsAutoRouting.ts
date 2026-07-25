import type { AdminGroup, GroupPlatform, GroupRoutingMode, SubscriptionType } from '@/types'

export type GroupTypeSelection = SubscriptionType

type GroupRoutingDisplaySource = Pick<
  AdminGroup,
  'routing_mode' | 'subscription_type' | 'rate_multiplier'
>

export const groupTypeSelection = (
  group: Pick<GroupRoutingDisplaySource, 'routing_mode' | 'subscription_type'>
): GroupTypeSelection => group.subscription_type

export const groupTypeConfiguration = (
  selection: GroupTypeSelection
): { subscriptionType: SubscriptionType; routingMode: GroupRoutingMode } => ({
  subscriptionType: selection,
  routingMode: 'fixed'
})

export const groupPlatformConfiguration = (
  platform: GroupPlatform
): { subscriptionType: SubscriptionType; routingMode: GroupRoutingMode } => ({
  subscriptionType: 'standard',
  routingMode: platform === 'auto' ? 'auto_lowest_cost' : 'fixed'
})

export const groupBillingTypeDisplay = (group: GroupRoutingDisplaySource): GroupTypeSelection =>
  group.subscription_type

export const groupRateMultiplierDisplay = (group: GroupRoutingDisplaySource): string =>
  group.routing_mode === 'auto_lowest_cost' ? 'auto' : `${group.rate_multiplier}x`

export const filterAutoRoutingCandidates = (
  groups: AdminGroup[],
  currentGroupId?: number
): AdminGroup[] =>
  groups.filter(
    (group) =>
      group.id !== currentGroupId &&
      group.status === 'active' &&
      !group.is_exclusive &&
      group.subscription_type === 'standard' &&
      group.routing_mode !== 'auto_lowest_cost'
  )
