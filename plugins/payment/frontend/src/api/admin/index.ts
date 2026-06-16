/**
 * Plugin shim for the host's `@/api/admin` aggregator.
 *
 * The migrated `AdminPaymentPlansView.vue` only consumes `adminAPI.groups.getAll()`
 * to populate the group selector for plan editing. We expose just that method
 * (calling /admin/groups/all on the shared core whitelist) and stub the rest.
 *
 * The plugin manifest must declare CapabilityDBCoreRead — already in place in
 * plugins/payment/manifest_decls.go — so the host gateway permits the read.
 */

import { getClient } from '../client'
import type { AdminGroup, GroupPlatform } from '../../types'

export const groupsAPI = {
  async getAll(platform?: GroupPlatform): Promise<AdminGroup[]> {
    const { data } = await getClient().get<AdminGroup[]>('/admin/groups/all', {
      params: platform ? { platform } : undefined,
    })
    return data
  },
}

const adminAPI = {
  groups: groupsAPI,
}

export default adminAPI
