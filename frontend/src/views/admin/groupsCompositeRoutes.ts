import type { CompositeModelRouteInput, GroupPlatform } from '@/types'

export type CompositeDestinationMode = 'pool' | 'group' | 'account'

export interface CompositeRouteDestination {
  mode: CompositeDestinationMode
  target_group_id: number | null
  target_account_id: number | null
}

type DestinationIDs = Pick<CompositeModelRouteInput, 'target_group_id' | 'target_account_id'>

export function compositeRouteDestination(route: DestinationIDs = {}): CompositeRouteDestination {
  return {
    mode: route.target_group_id != null ? 'group' : route.target_account_id != null ? 'account' : 'pool',
    target_group_id: route.target_group_id ?? null,
    target_account_id: route.target_account_id ?? null
  }
}

export function changeCompositeDestinationMode(mode: CompositeDestinationMode): CompositeRouteDestination {
  return { mode, target_group_id: null, target_account_id: null }
}

export function validCompositeDestination(
  destination: CompositeRouteDestination,
  sourceGroupId?: number
): boolean {
  const validID = (id: number | null) => id !== null && Number.isSafeInteger(id) && id > 0
  if (destination.mode === 'pool') {
    return destination.target_group_id === null && destination.target_account_id === null
  }
  if (destination.mode === 'group') {
    return validID(destination.target_group_id) && destination.target_group_id !== sourceGroupId &&
      destination.target_account_id === null
  }
  return validID(destination.target_account_id) && destination.target_group_id === null
}

// Explicit nulls also clear an existing pin when updating a route back to pool mode.
export function compositeDestinationInput(destination: CompositeRouteDestination): DestinationIDs {
  return {
    target_group_id: destination.mode === 'group' ? destination.target_group_id : null,
    target_account_id: destination.mode === 'account' ? destination.target_account_id : null
  }
}

export function compositeDestinationLabelKey(route: DestinationIDs): string {
  if (route.target_group_id != null) return 'admin.groups.compositeRoutes.destinationGroup'
  if (route.target_account_id != null) return 'admin.groups.compositeRoutes.destinationAccount'
  return 'admin.groups.compositeRoutes.destinationPool'
}

export function eligibleCompositeTargetGroup(
  group: { id: number; platform: GroupPlatform },
  platform: Exclude<GroupPlatform, 'composite'>,
  sourceGroupId: number
): boolean {
  return group.id !== sourceGroupId && group.platform !== 'composite' && group.platform === platform
}
