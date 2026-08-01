import { describe, expect, it } from 'vitest'
import {
  proxyStatusBadgeClass,
  proxyStatusLabelKey,
  proxyStatusRowClass,
  proxyStatusSortRank
} from '@/utils/proxyStatus'

describe('proxyStatus helpers', () => {
  it('maps badge classes consistently with ProxiesView', () => {
    expect(proxyStatusBadgeClass('active')).toBe('badge badge-success')
    expect(proxyStatusBadgeClass('inactive')).toBe('badge badge-danger')
    expect(proxyStatusBadgeClass('expired')).toBe('badge badge-danger')
    expect(proxyStatusBadgeClass('unknown')).toBe('badge badge-gray')
  })

  it('returns i18n keys for labels', () => {
    expect(proxyStatusLabelKey('active')).toBe('admin.accounts.status.active')
    expect(proxyStatusLabelKey('inactive')).toBe('admin.accounts.status.inactive')
    expect(proxyStatusLabelKey('expired')).toBe('admin.proxies.expired')
  })

  it('ranks active first for picker sorting', () => {
    expect(proxyStatusSortRank('active')).toBeLessThan(proxyStatusSortRank('inactive'))
    expect(proxyStatusSortRank('inactive')).toBeLessThan(proxyStatusSortRank('expired'))
  })

  it('dims non-active rows', () => {
    expect(proxyStatusRowClass('active')).toBe('')
    expect(proxyStatusRowClass('inactive')).toContain('opacity')
  })
})
