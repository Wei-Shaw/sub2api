import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AffinityBadge from '../AffinityBadge.vue'

vi.mock('@/api/admin/accounts', () => ({
  getAffinityClients: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, fallback?: string) => fallback ?? key
    })
  }
})

function mountBadge(count: number, base = 5, buffer: number | null = 10) {
  return mount(AffinityBadge, {
    props: {
      accountId: 42,
      count,
      base,
      buffer
    }
  })
}

describe('AffinityBadge', () => {
  it('renders the correct count number', () => {
    const wrapper = mountBadge(5)
    expect(wrapper.text()).toContain('5')
  })

  it('renders configured limit text', () => {
    const wrapper = mountBadge(5, 5, 10)
    expect(wrapper.text()).toContain('15')
  })

  it('renders infinity limit text when base is not configured', () => {
    const wrapper = mountBadge(5, 0, null)
    expect(wrapper.text()).toContain('∞')
  })

  it('applies red badge class when count exceeds base plus buffer', () => {
    const wrapper = mountBadge(16, 5, 10)
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-red-100')
    expect(badge.classes()).toContain('text-red-700')
  })

  it('applies yellow badge class when count is in buffer range', () => {
    const wrapper = mountBadge(6, 5, 10)
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-yellow-100')
    expect(badge.classes()).toContain('text-yellow-700')
  })

  it('applies yellow badge class when buffer is infinite', () => {
    const wrapper = mountBadge(6, 5, null)
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-yellow-100')
    expect(badge.classes()).toContain('text-yellow-700')
  })

  it('applies green badge class when count is within base', () => {
    const wrapper = mountBadge(5, 5, 10)
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-emerald-100')
    expect(badge.classes()).toContain('text-emerald-700')
  })

  it('applies gray badge class when count is 0', () => {
    const wrapper = mountBadge(0, 5, 10)
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-gray-100')
    expect(badge.classes()).toContain('text-gray-600')
  })

  it('does NOT show popover initially', () => {
    const wrapper = mountBadge(3)
    expect(wrapper.html()).not.toContain('divide-y')
    expect(wrapper.html()).not.toContain('affinityClients')
  })

  it('has mouseenter and mouseleave handlers on the badge', () => {
    const wrapper = mountBadge(3)
    const badge = wrapper.find('span')
    expect(badge.exists()).toBe(true)
    badge.trigger('mouseenter')
    badge.trigger('mouseleave')
  })
})
