import { config } from '@vue/test-utils'
import { i18n } from '@/i18n'
import quotaZh from '@/i18n/locales/zh/admin/quota'
i18n.global.setLocaleMessage('zh', { admin: { quota: quotaZh } })
i18n.global.locale.value = 'zh'
config.global.plugins = [i18n]
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
const responsive = vi.hoisted(() => ({ width: null as unknown as { value: boolean } }))
vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual<typeof import('@vueuse/core')>('@vueuse/core')
  const { ref } = await import('vue')
  responsive.width = ref(false)
  return { ...actual, useMediaQuery: () => responsive.width }
})
import QuotaPoolTable from '../QuotaPoolTable.vue'
vi.mock('@/composables/useQuotaNow', () => ({
  useQuotaNow: () => ref(new Date('2026-10-29T00:00:00Z'))
}))
const pools = [
  {
    id: 'gemini',
    name: 'Gemini',
    windows: [
      { id: 'weekly', remaining: 0.636, resets_at: '2026-10-31T00:00:00Z' },
      { id: '5h', remaining: 0.966, resets_at: '2026-10-29T03:00:00Z' }
    ]
  },
  {
    id: '3p',
    name: 'Claude / GPT',
    windows: [
      { id: '5h', remaining: 0.421, resets_at: '2026-10-29T01:00:00Z' },
      { id: 'weekly', remaining: 0.789, resets_at: '2026-11-01T00:00:00Z' }
    ]
  }
]
describe('quota matrix', () => {
  it.each([false, true])('keeps model/window values and resets paired, expanded=%s', (expanded) => {
    const wrapper = mount(QuotaPoolTable, { props: { pools, expanded } })
    const rows = wrapper.findAll('.quota-grid')
    expect(rows[0].text()).toContain('GeminiClaude / GPT')
    expect(rows[1].text()).toMatch(/^5h/)
    expect(rows[2].text()).toMatch(/^7d/)
    const five = rows[1].findAll('.quota-metric')
    const week = rows[2].findAll('.quota-metric')
    expect(five[0].text()).toContain('96.6%')
    expect(five[1].text()).toContain('42.1%')
    expect(week[0].text()).toContain('63.6%')
    expect(week[1].text()).toContain('78.9%')
    expect(five[0].attributes('title')).toContain('Gemini · 五小时额度')
    expect(week[1].attributes('title')).toContain('Claude / GPT · 七天额度')
    expect(five[0].find('.quota-reset').text()).not.toBe(five[1].find('.quota-reset').text())
    wrapper.unmount()
  })
  it('shows -- for a missing model/window cell', () => {
    const wrapper = mount(QuotaPoolTable, {
      props: { pools: [pools[0], { ...pools[1], windows: [] }] }
    })
    for (const row of wrapper.findAll('.quota-grid').slice(1))
      expect(row.findAll('.quota-metric')[1].text()).toContain('--')
    wrapper.unmount()
  })
})

// Overview uses the same account data; all requests are replaced with local fixtures.
import { flushPromises } from '@vue/test-utils'
import AccountQuotaOverview from '@/components/admin/account/AccountQuotaOverview.vue'
vi.mock('@/api/admin/accounts', () => ({
  list: vi.fn(async () => ({
    total: 1,
    items: [
      {
        id: 1,
        name: 'Fixture',
        platform: 'antigravity',
        type: 'oauth',
        status: 'active',
        schedulable: true
      }
    ]
  }))
}))
let overviewPools = pools
vi.mock('@/composables/useAccountUsage', () => ({
  useAccountUsage: () => ({
    usage: ref({
      1: {
        quota_pools: overviewPools,
        updated_at: '2026-10-29T00:00:00Z',
        subscription_tier: 'PRO'
      }
    }),
    errors: ref({}),
    loading: ref({}),
    load: vi.fn(async () => {})
  })
}))
it('transposes overview totals while retaining model/window aggregation', async () => {
  const wrapper = mount(AccountQuotaOverview, {
    props: { show: false },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  const matrix = wrapper.find('.card .grid').element.children
  expect(matrix[1].textContent).toBe('Gemini')
  expect(matrix[2].textContent).toBe('Claude / GPT')
  expect(matrix[3].textContent).toBe('5h')
  expect(matrix[6].textContent).toBe('7d')
  expect(matrix[4].textContent).toContain('96.6%')
  expect(matrix[5].textContent).toContain('42.1%')
  expect(matrix[7].textContent).toContain('63.6%')
  expect(matrix[8].textContent).toContain('78.9%')
  expect(matrix[4].textContent).toContain('0.97 / 1.00')
  wrapper.unmount()
})

it('renders only reported periods and omits absent reset columns', () => {
  const wrapper = mount(QuotaPoolTable, {
    props: { pools: [{ id: 'only', name: 'Codex', windows: [{ id: 'weekly', remaining: 0.5 }] }] }
  })
  const rows = wrapper.findAll('.quota-grid')
  expect(rows).toHaveLength(2)
  expect(rows[1].text()).toBe('7d50.0%')
  expect(wrapper.findAll('.quota-reset')).toHaveLength(0)
  expect(rows[0].attributes('style')).toContain('56px')
  wrapper.unmount()
})
it('keeps current two-pool column widths and right-aligned reset slots', () => {
  const wrapper = mount(QuotaPoolTable, { props: { pools } })
  expect(wrapper.find('.quota-grid').attributes('style')).toContain('106px 106px')
  expect(wrapper.findAll('.quota-grid')).toHaveLength(3)
  expect(wrapper.find('.quota-reset').classes()).toContain('text-right')
  wrapper.unmount()
})
it('wraps five models into two-column groups without losing cells', () => {
  const wrapper = mount(QuotaPoolTable, {
    props: {
      pools: Array.from({ length: 5 }, (_, i) => ({
        id: String(i),
        name: 'Model ' + i,
        windows: [{ id: 'monthly', remaining: i / 10 }]
      }))
    }
  })
  expect(wrapper.findAll('.quota-grid')).toHaveLength(6)
  expect(wrapper.findAll('.quota-metric').map((cell) => cell.text())).toEqual([
    '0.0%',
    '10.0%',
    '20.0%',
    '30.0%',
    '40.0%'
  ])
  expect(wrapper.text()).toContain('1mo')
  wrapper.unmount()
})

it('overview groups extra models and keeps only reported periods', async () => {
  overviewPools = Array.from({ length: 3 }, (_, i) => ({
    id: String(i),
    name: 'Pool ' + i,
    windows: [{ id: 'weekly', remaining: (i + 1) / 10, resets_at: '2026-11-01T00:00:00Z' }]
  }))
  const wrapper = mount(AccountQuotaOverview, {
    props: { show: false },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  const cards = wrapper.findAll('.card')
  expect(cards).toHaveLength(2)
  expect(cards[0].text()).toContain('Pool 0')
  expect(cards[0].text()).toContain('Pool 1')
  expect(cards[1].text()).toContain('Pool 2')
  expect(cards[1].text()).toContain('30.0%')
  expect(cards[1].text()).toContain('0.30 / 1.00')
  expect(cards.map((c) => c.text()).join('')).not.toContain('5h')
  wrapper.unmount()
  overviewPools = pools
})
it('overview retains accounts with missing quota reports as --', async () => {
  overviewPools = []
  const wrapper = mount(AccountQuotaOverview, {
    props: { show: false },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  expect(wrapper.text()).toContain('Fixture')
  expect(wrapper.find('.quota-pools').text()).toBe('--')
  expect(wrapper.findAll('.card')).toHaveLength(0)
  wrapper.unmount()
  overviewPools = pools
})

it('reflows the same five-pool component between desktop and mobile', async () => {
  const five = Array.from({ length: 5 }, (_, i) => ({
    id: String(i),
    name: 'Pool ' + i,
    windows: [{ id: 'weekly', remaining: (i + 1) / 10 }]
  }))
  const wrapper = mount(QuotaPoolTable, { props: { pools: five, expanded: true } })
  expect(wrapper.findAll('.quota-grid')).toHaveLength(6)
  responsive.width.value = true
  await wrapper.vm.$nextTick()
  expect(wrapper.findAll('.quota-grid')).toHaveLength(2)
  expect(wrapper.findAll('.quota-metric').map((cell) => cell.text())).toEqual([
    '10.0%',
    '20.0%',
    '30.0%',
    '40.0%',
    '50.0%'
  ])
  expect(wrapper.classes()).toContain('quota-pools-many')
  await wrapper.setProps({ maxColumns: 2 })
  expect(wrapper.findAll('.quota-grid')).toHaveLength(6)
  await wrapper.setProps({ maxColumns: undefined })
  responsive.width.value = false
  await wrapper.vm.$nextTick()
  expect(wrapper.findAll('.quota-grid')).toHaveLength(6)
  wrapper.unmount()
})
it('overview totals use all model columns on desktop and regroup on mobile', async () => {
  overviewPools = Array.from({ length: 3 }, (_, i) => ({
    id: String(i),
    name: 'Pool ' + i,
    windows: [{ id: 'weekly', remaining: 0.5, resets_at: '2026-11-01T00:00:00Z' }]
  }))
  responsive.width.value = true
  const wrapper = mount(AccountQuotaOverview, {
    props: { show: false },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  expect(wrapper.findAll('.card')).toHaveLength(1)
  expect(wrapper.find('.card .grid').attributes('style')).toContain('repeat(3,')
  responsive.width.value = false
  await wrapper.vm.$nextTick()
  expect(wrapper.findAll('.card')).toHaveLength(2)
  wrapper.unmount()
  overviewPools = pools
})

it('switches quota labels and accessible text between Chinese and English', async () => {
  const { default: quotaEn } = await import('@/i18n/locales/en/admin/quota')
  i18n.global.setLocaleMessage('en', { admin: { quota: quotaEn } })
  const wrapper = mount(QuotaPoolTable, { props: { pools } })
  expect(wrapper.text()).toContain('周期')
  i18n.global.locale.value = 'en'
  await wrapper.vm.$nextTick()
  expect(wrapper.text()).toContain('Period')
  expect(wrapper.find('.quota-meter').attributes('aria-label')).toBe('96.6% remaining')
  expect(wrapper.find('.quota-reset').attributes('aria-label')).toBe('Resets in 3h')
  expect(wrapper.find('.quota-metric').attributes('title')).toContain('5-hour quota')
  expect(wrapper.text()).not.toMatch(/[\u3400-\u9fff]/)
  i18n.global.locale.value = 'zh'
  await wrapper.vm.$nextTick()
  expect(wrapper.text()).toContain('周期')
  wrapper.unmount()
})

it('localizes the overview without refetching on a language switch', async () => {
  const { default: quotaEn } = await import('@/i18n/locales/en/admin/quota')
  const { list } = await import('@/api/admin/accounts')
  i18n.global.setLocaleMessage('en', { admin: { quota: quotaEn } })
  const wrapper = mount(AccountQuotaOverview, {
    props: { show: false },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  const calls = vi.mocked(list).mock.calls.length
  i18n.global.locale.value = 'en'
  await wrapper.vm.$nextTick()
  expect(wrapper.text()).toContain('All accounts · Shared pools')
  expect(wrapper.text()).toContain('Shared 5-minute cache')
  expect(wrapper.text()).not.toMatch(/[\u3400-\u9fff]/)
  expect(vi.mocked(list).mock.calls).toHaveLength(calls)
  i18n.global.locale.value = 'zh'
  wrapper.unmount()
})
