import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import OpsRenewalCard from '../OpsRenewalCard.vue'
import type { ExpiringAccount } from '@/api/admin/opsTrace'

const mockList = vi.fn()

vi.mock('@/api/admin/opsTrace', () => ({
  listExpiringAccounts: (...args: any[]) => mockList(...args)
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => (params ? `${key}:${JSON.stringify(params)}` : key)
    })
  }
})

function account(partial: Partial<ExpiringAccount>): ExpiringAccount {
  const hours = partial.hours_left ?? 10
  return {
    id: 1,
    name: 'acc',
    platform: 'anthropic',
    type: 'apikey',
    status: 'active',
    schedulable: true,
    expires_at: '2026-11-12T08:00:00Z',
    auto_pause_on_expired: true,
    expired: hours <= 0,
    hours_left: hours,
    ...partial
  }
}

const sample: ExpiringAccount[] = [
  account({ id: 1, name: 'expired-live', hours_left: -50, auto_pause_on_expired: false }),
  account({ id: 2, name: 'expired-paused', hours_left: -3, schedulable: false }),
  account({ id: 3, name: 'soon', hours_left: 5.5 }),
  account({ id: 4, name: 'in-3d', hours_left: 40 }),
  account({ id: 5, name: 'in-7d', hours_left: 120, platform: 'openai' })
]

function mountCard(props: Record<string, unknown> = {}) {
  return mount(OpsRenewalCard, {
    props,
    global: { stubs: { RouterLink: RouterLinkStub } }
  })
}

describe('OpsRenewalCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('有数据：按组渲染、头部计数、告警口径、跳转链接', async () => {
    mockList.mockResolvedValue(sample)
    const wrapper = mountCard()
    await flushPromises()

    expect(mockList).toHaveBeenCalledWith({ days: 7 })

    for (const key of ['expiredActive', 'expiredPaused', 'within24h', 'within3d', 'within7d']) {
      expect(wrapper.find(`[data-testid="renewal-group-${key}"]`).exists()).toBe(true)
      expect(wrapper.find(`[data-testid="renewal-count-${key}"]`).text()).toContain('1')
    }
    // 7 天窗口不显示「7 天以上」计数
    expect(wrapper.find('[data-testid="renewal-count-later"]').exists()).toBe(false)

    // 告警口径：soon + in-3d + expired-live = 3
    expect(wrapper.find('[data-testid="renewal-alert-hint"]').text()).toContain('"n":3')

    const row1 = wrapper.find('[data-testid="renewal-row-1"]')
    expect(row1.text()).toContain('admin.traceRenewal.remaining.expiredDays:{"n":2}')
    expect(row1.text()).toContain('admin.traceRenewal.autoPauseOff')
    expect(row1.text()).toContain('admin.traceRenewal.scheduling.active')

    const row2 = wrapper.find('[data-testid="renewal-row-2"]')
    expect(row2.text()).toContain('admin.traceRenewal.scheduling.paused')

    expect(wrapper.find('[data-testid="renewal-row-3"]').text()).toContain(
      'admin.traceRenewal.remaining.leftHours:{"n":5}'
    )

    const link = wrapper.findAllComponents(RouterLinkStub)[0]
    expect(link.props('to')).toEqual({ path: '/admin/accounts', query: { search: 'expired-live' } })

    // 分组顺序：最紧急在最前
    const groupOrder = wrapper.findAll('tbody').map((el) => el.attributes('data-testid'))
    expect(groupOrder).toEqual([
      'renewal-group-expiredActive',
      'renewal-group-expiredPaused',
      'renewal-group-within24h',
      'renewal-group-within3d',
      'renewal-group-within7d'
    ])
  })

  it('空状态', async () => {
    mockList.mockResolvedValue([])
    const wrapper = mountCard()
    await flushPromises()
    const empty = wrapper.find('[data-testid="renewal-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('"days":7')
  })

  it('出错状态，可重试', async () => {
    mockList.mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.find('[data-testid="renewal-error"]').text()).toContain('boom')
    expect(wrapper.find('[data-testid="renewal-empty"]').exists()).toBe(false)

    mockList.mockResolvedValueOnce(sample)
    await wrapper.find('[data-testid="renewal-error"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="renewal-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="renewal-row-1"]').exists()).toBe(true)
  })

  it('加载中状态', async () => {
    mockList.mockReturnValue(new Promise(() => {}))
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.find('[data-testid="renewal-loading"]').exists()).toBe(true)
  })

  it('切换窗口与全局刷新会重新请求', async () => {
    mockList.mockResolvedValue(sample)
    const wrapper = mountCard({ refreshToken: 1 })
    await flushPromises()
    expect(mockList).toHaveBeenCalledTimes(1)

    await wrapper.find('[data-testid="renewal-window-30"]').trigger('click')
    await flushPromises()
    expect(mockList).toHaveBeenLastCalledWith({ days: 30 })
    expect(wrapper.find('[data-testid="renewal-count-later"]').exists()).toBe(true)

    await wrapper.setProps({ refreshToken: 2 })
    await flushPromises()
    expect(mockList).toHaveBeenCalledTimes(3)
    expect(mockList).toHaveBeenLastCalledWith({ days: 30 })
  })
})
