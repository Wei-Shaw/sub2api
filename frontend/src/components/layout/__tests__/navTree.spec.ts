import { describe, expect, it } from 'vitest'

import {
  applyFeatureFlags,
  buildAdminNavItems,
  buildSelfNavItems,
  finalizeNav,
  tourSelectorFor,
} from '../navTree'
import type { NavContext, NavFlags, NavItem } from '../navTree'
import { navIcons } from '@/components/icons/nav'
import type { CustomMenuItem } from '@/types'

/**
 * The nav tree was untestable while it lived inside AppSidebar.vue: reaching it
 * meant mounting a component that wants a router, four stores, an i18n instance
 * and a DOM. As data it is a pure function of flags and mode, which is what the
 * cases below exercise. Every one of them pins behaviour that predates the
 * extraction — this is a safety net for the move, not a spec for new work.
 */

const ALL_ON: NavFlags = {
  channelMonitor: () => true,
  payment: () => true,
  availableChannels: () => true,
  affiliate: () => true,
  riskControl: () => true,
  opsMonitoring: () => true,
  adminPayment: () => true,
  batchImageAccess: () => true,
}

function ctx(overrides: Partial<NavContext> = {}): NavContext {
  return {
    t: (key: string) => key,
    flags: ALL_ON,
    userCustomMenuItems: [],
    adminCustomMenuItems: [],
    simpleMode: false,
    ...overrides,
  }
}

function paths(items: NavItem[]): string[] {
  return items.map((item) => item.path)
}

function flatten(items: NavItem[]): NavItem[] {
  return items.flatMap((item) => [item, ...(item.children ? flatten(item.children) : [])])
}

const customItem = (over: Partial<CustomMenuItem> = {}): CustomMenuItem => ({
  id: 'a',
  label: 'Custom A',
  icon_svg: '<svg />',
  url: 'https://example.test',
  visibility: 'user',
  sort_order: 1,
  ...over,
})

describe('applyFeatureFlags', () => {
  it('keeps items whose flag has not resolved yet', () => {
    // `undefined` means public settings are still in flight. Hiding on it makes
    // the whole menu blink empty on every cold load.
    const items: NavItem[] = [{ path: '/a', label: 'a', icon: null, featureFlag: () => undefined }]
    expect(paths(applyFeatureFlags(items))).toEqual(['/a'])
  })

  it('drops items whose flag is explicitly false', () => {
    const items: NavItem[] = [
      { path: '/a', label: 'a', icon: null, featureFlag: () => false },
      { path: '/b', label: 'b', icon: null },
    ]
    expect(paths(applyFeatureFlags(items))).toEqual(['/b'])
  })

  it('filters children without dropping their parent', () => {
    const items: NavItem[] = [
      {
        path: '/group',
        label: 'group',
        icon: null,
        children: [
          { path: '/group/on', label: 'on', icon: null },
          { path: '/group/off', label: 'off', icon: null, featureFlag: () => false },
        ],
      },
    ]
    const [group] = applyFeatureFlags(items)
    expect(paths(group.children ?? [])).toEqual(['/group/on'])
  })

  it('does not mutate the input', () => {
    const items: NavItem[] = [
      {
        path: '/group',
        label: 'group',
        icon: null,
        children: [{ path: '/group/off', label: 'off', icon: null, featureFlag: () => false }],
      },
    ]
    applyFeatureFlags(items)
    expect(items[0].children).toHaveLength(1)
  })
})

describe('finalizeNav', () => {
  it('applies the simple-mode cut only in simple mode', () => {
    const items: NavItem[] = [
      { path: '/keep', label: 'keep', icon: null },
      { path: '/hide', label: 'hide', icon: null, hideInSimpleMode: true },
    ]
    expect(paths(finalizeNav(items, false))).toEqual(['/keep', '/hide'])
    expect(paths(finalizeNav(items, true))).toEqual(['/keep'])
  })
})

describe('buildSelfNavItems', () => {
  it('includes the dashboard only when asked', () => {
    expect(paths(buildSelfNavItems(ctx(), true))[0]).toBe('/dashboard')
    expect(paths(buildSelfNavItems(ctx(), false))).not.toContain('/dashboard')
  })

  it('keeps Available Channels directly above Channel Status', () => {
    const list = paths(buildSelfNavItems(ctx(), false))
    expect(list.indexOf('/monitor') - list.indexOf('/available-channels')).toBe(1)
  })

  it('carries the my-keys tour anchor', () => {
    const keys = buildSelfNavItems(ctx(), false).find((item) => item.path === '/keys')
    expect(keys?.tourAnchor).toBe('sidebar-my-keys')
  })

  it('appends only user-visible custom menu items, in sort order', () => {
    const items = buildSelfNavItems(
      ctx({
        userCustomMenuItems: [
          customItem({ id: 'second', sort_order: 2 }),
          customItem({ id: 'first', sort_order: 1 }),
          customItem({ id: 'admin-only', visibility: 'admin', sort_order: 0 }),
        ],
      }),
      false
    )
    const custom = paths(items).filter((path) => path.startsWith('/custom/'))
    expect(custom).toEqual(['/custom/first', '/custom/second'])
  })
})

describe('buildAdminNavItems', () => {
  it('ends with settings followed by admin custom entries', () => {
    const items = buildAdminNavItems(
      ctx({ adminCustomMenuItems: [customItem({ id: 'ops-wiki', visibility: 'admin' })] })
    )
    expect(paths(items).slice(-2)).toEqual(['/admin/settings', '/custom/ops-wiki'])
  })

  it('makes every group expand-only', () => {
    // All four parents toggle rather than navigate. Worth pinning: the parent
    // paths are then pure keys, and `/admin/orders` in particular is BOTH a
    // group key and a real route that its own child points at — so a change
    // here silently turns the parent row into a second way to reach that page.
    const groups = buildAdminNavItems(ctx()).filter((item) => item.children?.length)
    expect(groups.map((item) => item.path).sort()).toEqual([
      '/admin/affiliates',
      '/admin/channels',
      '/admin/orders',
      '/admin/security-audit',
    ])
    expect(groups.every((item) => item.expandOnly)).toBe(true)
  })

  it('hides a whole group when its flag is off', () => {
    const items = buildAdminNavItems(ctx({ flags: { ...ALL_ON, riskControl: () => false } }))
    expect(paths(items)).not.toContain('/admin/security-audit')
  })

  it('adds an API keys entry in simple mode, without a tour anchor', () => {
    const items = buildAdminNavItems(ctx({ simpleMode: true }))
    const keys = items.find((item) => item.path === '/keys')
    expect(keys).toBeDefined()
    // The personal section that normally carries /keys is hidden in simple
    // mode, and the tour does not run there.
    expect(keys?.tourAnchor).toBeUndefined()
    expect(paths(items)).not.toContain('/admin/users')
  })

  it('carries the three id-based tour anchors', () => {
    const byPath = new Map(buildAdminNavItems(ctx()).map((item) => [item.path, item]))
    expect(byPath.get('/admin/accounts')?.domId).toBe('sidebar-channel-manage')
    expect(byPath.get('/admin/groups')?.domId).toBe('sidebar-group-manage')
    expect(byPath.get('/admin/redeem')?.domId).toBe('sidebar-wallet')
  })
})

describe('icon names', () => {
  it('every item resolves to a registered icon or supplies its own SVG', () => {
    const items = [
      ...flatten(buildAdminNavItems(ctx({ adminCustomMenuItems: [customItem({ visibility: 'admin' })] }))),
      ...flatten(buildSelfNavItems(ctx({ userCustomMenuItems: [customItem()] }), true)),
    ]
    const unresolved = items.filter((item) => (item.icon ? !(item.icon in navIcons) : !item.iconSvg))
    expect(unresolved.map((item) => item.path)).toEqual([])
  })
})

describe('tourSelectorFor', () => {
  it('prefers an id, falls back to data-tour, and is null otherwise', () => {
    expect(tourSelectorFor({ path: '/a', label: 'a', icon: null, domId: 'sidebar-wallet' })).toBe('#sidebar-wallet')
    expect(tourSelectorFor({ path: '/b', label: 'b', icon: null, tourAnchor: 'sidebar-my-keys' })).toBe(
      '[data-tour="sidebar-my-keys"]'
    )
    expect(tourSelectorFor({ path: '/c', label: 'c', icon: null })).toBeNull()
  })
})
