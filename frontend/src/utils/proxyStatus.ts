/**
 * Proxy status display helpers — keep badge/label consistent across
 * ProxiesView, ProxyGroupsView member picker, and ProxySelector.
 */

export type ProxyStatusValue = 'active' | 'inactive' | 'expired' | string

/** Tailwind badge class aligned with admin ProxiesView status column. */
export function proxyStatusBadgeClass(status?: ProxyStatusValue | null): string {
  if (status === 'active') return 'badge badge-success'
  if (status === 'expired') return 'badge badge-danger'
  if (status === 'inactive') return 'badge badge-danger'
  return 'badge badge-gray'
}

/**
 * i18n key for proxy status label.
 * expired uses proxies namespace; others reuse accounts.status (正常/停用).
 */
export function proxyStatusLabelKey(status?: ProxyStatusValue | null): string {
  if (status === 'expired') return 'admin.proxies.expired'
  if (status === 'active' || status === 'inactive') {
    return `admin.accounts.status.${status}`
  }
  if (status) return `admin.accounts.status.${status}`
  return 'admin.accounts.status.inactive'
}

/** Soften non-schedulable rows in pickers without hiding them. */
export function proxyStatusRowClass(status?: ProxyStatusValue | null): string {
  if (status === 'active') return ''
  return 'opacity-70'
}

/** Sort key: active first, then inactive, then expired/unknown. */
export function proxyStatusSortRank(status?: ProxyStatusValue | null): number {
  if (status === 'active') return 0
  if (status === 'inactive') return 1
  if (status === 'expired') return 2
  return 3
}
