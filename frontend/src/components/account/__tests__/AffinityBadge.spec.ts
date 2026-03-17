import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AffinityBadge from '../AffinityBadge.vue'

vi.mock('@/api/admin/accounts', () => ({
  getAffinityClients: vi.fn(),
  getAffinityDetails: vi.fn()
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

function mountBadge(opts: {
  clientCount: number
  base?: number
  buffer?: number | null
  userCount?: number
  userBase?: number
  userBuffer?: number | null
}) {
  return mount(AffinityBadge, {
    props: {
      accountId: 42,
      clientCount: opts.clientCount,
      base: opts.base ?? 5,
      buffer: opts.buffer ?? 10,
      userCount: opts.userCount ?? 0,
      userBase: opts.userBase ?? 0,
      userBuffer: opts.userBuffer ?? null
    }
  })
}

describe('AffinityBadge', () => {
  // ====== Original tests (client dimension only) ======
  it('renders the correct client count number', () => {
    const wrapper = mountBadge({ clientCount: 5 })
    expect(wrapper.text()).toContain('5')
  })

  it('renders configured limit text', () => {
    const wrapper = mountBadge({ clientCount: 5, base: 5, buffer: 10 })
    expect(wrapper.text()).toContain('15')
  })

  it('renders infinity limit text when base is not configured', () => {
    const wrapper = mountBadge({ clientCount: 5, base: 0, buffer: null })
    expect(wrapper.text()).toContain('\u221E')
  })

  it('applies red badge class when count exceeds base plus buffer', () => {
    const wrapper = mountBadge({ clientCount: 16, base: 5, buffer: 10 })
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-red-100')
    expect(badge.classes()).toContain('text-red-700')
  })

  it('applies yellow badge class when count is in buffer range', () => {
    const wrapper = mountBadge({ clientCount: 6, base: 5, buffer: 10 })
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-yellow-100')
    expect(badge.classes()).toContain('text-yellow-700')
  })

  it('applies yellow badge class when buffer is infinite', () => {
    const wrapper = mountBadge({ clientCount: 6, base: 5, buffer: null })
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-yellow-100')
    expect(badge.classes()).toContain('text-yellow-700')
  })

  it('applies green badge class when count is within base', () => {
    const wrapper = mountBadge({ clientCount: 5, base: 5, buffer: 10 })
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-emerald-100')
    expect(badge.classes()).toContain('text-emerald-700')
  })

  it('applies gray badge class when count is 0', () => {
    const wrapper = mountBadge({ clientCount: 0, base: 5, buffer: 10 })
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-gray-100')
    expect(badge.classes()).toContain('text-gray-600')
  })

  it('does NOT show popover initially', () => {
    const wrapper = mountBadge({ clientCount: 3 })
    expect(wrapper.html()).not.toContain('divide-y')
  })

  it('has mouseenter and mouseleave handlers on the badge', () => {
    const wrapper = mountBadge({ clientCount: 3 })
    const badge = wrapper.find('span')
    expect(badge.exists()).toBe(true)
    badge.trigger('mouseenter')
    badge.trigger('mouseleave')
  })

  // ====== Dual dimension tests ======
  it('dual dimension: user green + client yellow = yellow', () => {
    const wrapper = mountBadge({
      clientCount: 6, base: 5, buffer: 10,
      userCount: 3, userBase: 5, userBuffer: 5
    })
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-yellow-100')
    expect(badge.classes()).toContain('text-yellow-700')
  })

  it('dual dimension: user green + client red = red', () => {
    const wrapper = mountBadge({
      clientCount: 16, base: 5, buffer: 10,
      userCount: 3, userBase: 5, userBuffer: 5
    })
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-red-100')
    expect(badge.classes()).toContain('text-red-700')
  })

  it('dual dimension: user red + client green = red', () => {
    const wrapper = mountBadge({
      clientCount: 3, base: 5, buffer: 10,
      userCount: 11, userBase: 5, userBuffer: 5
    })
    const badge = wrapper.find('span')
    expect(badge.classes()).toContain('bg-red-100')
    expect(badge.classes()).toContain('text-red-700')
  })

  it('user only dimension shows user count text', () => {
    const wrapper = mountBadge({
      clientCount: 0, base: 0, buffer: null,
      userCount: 3, userBase: 5, userBuffer: 5
    })
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('10')
  })

  it('client only dimension shows client count text', () => {
    const wrapper = mountBadge({
      clientCount: 4, base: 5, buffer: 10,
      userCount: 0, userBase: 0, userBuffer: null
    })
    expect(wrapper.text()).toContain('4')
    expect(wrapper.text()).toContain('15')
  })

  it('dual dimension limit text shows both U and C prefixes', () => {
    const wrapper = mountBadge({
      clientCount: 3, base: 5, buffer: 10,
      userCount: 2, userBase: 4, userBuffer: 3
    })
    expect(wrapper.text()).toContain('U2/7')
    expect(wrapper.text()).toContain('C3/15')
  })
})
