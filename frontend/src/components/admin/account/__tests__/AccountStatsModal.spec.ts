import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { Account } from '@/types'
import AccountStatsModal from '../AccountStatsModal.vue'

const { getStats } = vi.hoisted(() => ({ getStats: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getStats } } }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

const fixture = (cost: number, tokens: number) => ({
  summary: {
    total_cost: cost, total_user_cost: cost, total_standard_cost: cost,
    total_tokens: tokens, total_requests: 10, avg_daily_cost: 1,
    avg_daily_user_cost: 1, avg_daily_requests: 1, avg_daily_tokens: 10,
    avg_duration_ms: 100, actual_days_used: 10, days: 30,
  },
  history: [], models: [], endpoints: [], upstream_endpoints: [],
})
const createWrapper = () => mount(AccountStatsModal, {
  props: { show: false, account: { id: 3, name: 'stats-test', status: 'active' } as Account },
  global: { stubs: {
    BaseDialog: { template: '<div><slot /></div>' },
    ModelDistributionChart: true, EndpointDistributionChart: true,
  } },
})

describe('active account statistics modal', () => {
  beforeEach(() => getStats.mockReset())

  it('renders separate 30-day and 60-day costs and token totals', async () => {
    getStats.mockImplementation((_id: number, days: number) => Promise.resolve(
      days === 60 ? fixture(456.78, 2500000) : fixture(123.45, 1000000),
    ))
    const wrapper = createWrapper()
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(getStats).toHaveBeenCalledWith(3, 30)
    expect(getStats).toHaveBeenCalledWith(3, 60)
    expect(wrapper.text()).toContain('$123.45')
    expect(wrapper.text()).toContain('$456.78')
    expect(wrapper.text()).toContain('1.00M')
    expect(wrapper.text()).toContain('2.50M')
    expect(wrapper.text()).toContain('admin.accounts.stats.totalTokens60Days')
    wrapper.unmount()
  })

  it('keeps 30-day statistics and does not invent zero when 60-day query fails', async () => {
    getStats.mockImplementation((_id: number, days: number) => days === 60
      ? Promise.reject(new Error('unavailable')) : Promise.resolve(fixture(123.45, 1000000)))
    const wrapper = createWrapper()
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.text()).toContain('$123.45')
    expect(wrapper.text()).toContain('common.error')
    expect(wrapper.text()).toContain('—')
    wrapper.unmount()
  })
  it('shows 30-day results while 60-day query is pending, and ignores stale account results', async () => {
    let finishOld!: (value: ReturnType<typeof fixture>) => void
    getStats.mockImplementation((id: number, days: number) => {
      if (id === 3 && days === 60) return new Promise(resolve => { finishOld = resolve })
      return Promise.resolve(fixture(id === 3 ? 123.45 : 789.12, 1000000))
    })
    const wrapper = createWrapper()
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.text()).toContain('$123.45')
    expect(wrapper.text()).toContain('common.loading')
    expect(wrapper.text()).not.toContain('common.error')
    await wrapper.setProps({ account: { id: 4, name: 'new-account', status: 'active' } as Account })
    await flushPromises()
    finishOld(fixture(456.78, 2500000))
    await flushPromises()
    expect(wrapper.text()).toContain('$789.12')
    expect(wrapper.text()).not.toContain('$456.78')
    wrapper.unmount()
  })

})
