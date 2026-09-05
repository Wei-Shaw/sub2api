import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupCapacityBadge from '../GroupCapacityBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        `${key}:${params?.used ?? ''}/${params?.max ?? ''}`,
    }),
  }
})

function mountBadge(props: Record<string, number>) {
  return mount(GroupCapacityBadge, {
    props: {
      concurrencyUsed: 0,
      concurrencyMax: 0,
      sessionsUsed: 0,
      sessionsMax: 0,
      groupSessionsUsed: 0,
      groupSessionsMax: 0,
      rpmUsed: 0,
      rpmMax: 0,
      ...props,
    },
  })
}

describe('GroupCapacityBadge group session soft limit', () => {
  it('renders the account pool row and the group row separately', () => {
    const wrapper = mountBadge({
      sessionsUsed: 5,
      sessionsMax: 9,
      groupSessionsUsed: 2,
      groupSessionsMax: 4,
    })

    const titles = wrapper.findAll('[title]').map((el) => el.attributes('title'))
    expect(titles).toContain('admin.groups.capacity.accountSessionsTooltip:5/9')
    expect(titles).toContain('admin.groups.capacity.groupSessionsTooltip:2/4')
  })

  it('hides the group row when the group has no session cap configured', () => {
    const wrapper = mountBadge({ sessionsUsed: 5, sessionsMax: 9 })

    const titles = wrapper.findAll('[title]').map((el) => el.attributes('title'))
    expect(titles).toContain('admin.groups.capacity.accountSessionsTooltip:5/9')
    expect(titles.some((title) => title?.includes('groupSessionsTooltip'))).toBe(false)
  })

  it('marks the group row as saturated once used reaches the cap', () => {
    const wrapper = mountBadge({ groupSessionsUsed: 4, groupSessionsMax: 4 })

    const groupRow = wrapper
      .findAll('[title]')
      .find((el) => el.attributes('title')?.includes('groupSessionsTooltip'))

    expect(groupRow?.classes().join(' ')).toContain('text-red-700')
  })
})
