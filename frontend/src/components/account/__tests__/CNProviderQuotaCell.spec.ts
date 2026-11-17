import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import CNProviderQuotaCell from '../CNProviderQuotaCell.vue'
import UsageProgressBar from '../UsageProgressBar.vue'
import type { Account } from '@/types'

const { queryQuota } = vi.hoisted(() => ({
  queryQuota: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    cnProviders: { queryQuota }
  }
}))

// 保留 vue-i18n 真实导出：UsageProgressBar 依赖 @/utils/format → @/i18n，
// 其模块级 createI18n 需要真实 createI18n 存在。
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const account = {
  id: 7,
  platform: 'zhipu',
  type: 'apikey',
  credentials: { account_mode: 'coding' },
  extra: {
    zhipu_5h_used_percent: 0,
    zhipu_weekly_used_percent: 27,
    zhipu_5h_reset_at: '2026-08-18T12:30:00+08:00',
    zhipu_weekly_reset_at: '2026-08-22T00:00:00+08:00',
    zhipu_usage_updated_at: new Date().toISOString()
  }
} as Account

describe('CNProviderQuotaCell', () => {
  beforeEach(() => {
    queryQuota.mockReset()
  })

  it('renders tier rows through the shared UsageProgressBar inside the account table cell', async () => {
    queryQuota.mockResolvedValue({
      success: true,
      tiers: [
        { window: '5h', used_percent: 0, reset_at: '2026-08-18T12:30:00+08:00' },
        { window: 'weekly', used_percent: 27, reset_at: '2026-08-22T00:00:00+08:00' }
      ]
    })
    const wrapper = mount(CNProviderQuotaCell, { props: { account } })

    const root = wrapper.get('[data-test="cn-provider-quota"]')
    expect(root.classes()).toContain('min-w-[220px]')

    // 新鲜快照：挂载即渲染条形图，不触发探测
    await flushPromises()
    expect(queryQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('27%')

    // probe 按钮文案是动词 key（i18n mock 返回 key 本身），点击触发查询
    const probeButton = root.get('[data-test="cn-provider-quota-probe"]')
    expect(probeButton.text()).toBe('admin.accounts.cnProviders.probe')
    await probeButton.trigger('click')
    await flushPromises()
    expect(queryQuota).toHaveBeenCalledWith(account.id)

    // tier 行由 UsageProgressBar 渲染：数量、label/color/utilization/reset 逐行对齐
    expect(root.findAll('[data-test="cn-provider-quota-tier"]')).toHaveLength(2)
    const bars = root.findAllComponents(UsageProgressBar)
    expect(bars).toHaveLength(2)
    expect(bars[0].props('label')).toBe('admin.accounts.cnProviders.window5h')
    expect(bars[0].props('utilization')).toBe(0)
    expect(bars[0].props('color')).toBe('indigo')
    expect(bars[0].props('resetsAt')).toBe('2026-08-18T12:30:00+08:00')
    expect(bars[1].props('label')).toBe('admin.accounts.cnProviders.windowWeekly')
    expect(bars[1].props('utilization')).toBe(27)
    expect(bars[1].props('color')).toBe('emerald')
    expect(bars[1].props('resetsAt')).toBe('2026-08-22T00:00:00+08:00')
  })

  it('labels the refresh control with an explicit action verb, not a data caption', async () => {
    const wrapper = mount(CNProviderQuotaCell, { props: { account } })
    await flushPromises()

    // The snapshot is fresh (usage_updated_at = now): bars render without probing.
    expect(queryQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('27%')

    // The control reads as an action ("query"), unlike the old noun label
    // ("5-hour window/weekly window") which looked like a passive caption.
    // The i18n mock returns the key itself.
    const probeButton = wrapper.get('[data-test="cn-provider-quota-probe"]')
    expect(probeButton.text()).toBe('admin.accounts.cnProviders.probe')

    await probeButton.trigger('click')
    await flushPromises()
    expect(queryQuota).toHaveBeenCalledWith(account.id)
  })

  it('renders replacement quota values and windows for the same account without probing', async () => {
    const wrapper = mount(CNProviderQuotaCell, { props: { account } })
    await wrapper.setProps({ account: {
      ...account,
      extra: {
        ...account.extra,
        zhipu_5h_used_percent: 52,
        zhipu_weekly_used_percent: null,
        zhipu_monthly_used_percent: 81,
        zhipu_monthly_reset_at: '2026-12-01T00:00:00Z'
      }
    } })
    const bars = wrapper.findAllComponents(UsageProgressBar)
    expect(bars.map(bar => bar.props('utilization'))).toEqual([52, 81])
    expect(bars[1].props('label')).toBe('admin.accounts.cnProviders.windowMonthly')
    expect(bars[1].props('resetsAt')).toBe('2026-12-01T00:00:00Z')
    expect(queryQuota).not.toHaveBeenCalled()
  })

  it('reacts to quota changes inside the existing extra object', async () => {
    const row = reactive({ ...account, extra: { ...account.extra } })
    const wrapper = mount(CNProviderQuotaCell, { props: { account: row } })
    row.extra.zhipu_5h_used_percent = 63
    await flushPromises()
    expect(wrapper.findAllComponents(UsageProgressBar)[0].props('utilization')).toBe(63)
    expect(queryQuota).not.toHaveBeenCalled()
  })

  it('removes bars when a refreshed snapshot clears all quota windows', async () => {
    const wrapper = mount(CNProviderQuotaCell, { props: { account } })
    await wrapper.setProps({ account: {
      ...account,
      extra: { ...account.extra, zhipu_5h_used_percent: null, zhipu_weekly_used_percent: null }
    } })
    expect(wrapper.findAllComponents(UsageProgressBar)).toHaveLength(0)
    expect(queryQuota).not.toHaveBeenCalled()
  })

  it('keeps a manual result through unrelated refreshes, then adopts a changed snapshot', async () => {
    queryQuota.mockResolvedValue({ success: true, tiers: [{ window: '5h', used_percent: 44 }] })
    const wrapper = mount(CNProviderQuotaCell, { props: { account } })
    await wrapper.get('[data-test="cn-provider-quota-probe"]').trigger('click')
    await flushPromises()
    await wrapper.setProps({ account: {
      ...account, extra: { ...account.extra, unrelated_field: true }
    } })
    expect(wrapper.findAllComponents(UsageProgressBar).map(bar => bar.props('utilization'))).toEqual([44])
    await wrapper.setProps({ account: {
      ...account, extra: { ...account.extra, zhipu_5h_used_percent: 66 }
    } })
    expect(wrapper.findAllComponents(UsageProgressBar).map(bar => bar.props('utilization'))).toEqual([66, 27])
    expect(queryQuota).toHaveBeenCalledTimes(1)
  })

  it('adopts a new snapshot version even if its quota values equal the old snapshot', async () => {
    queryQuota.mockResolvedValue({ success: true, tiers: [{ window: '5h', used_percent: 44 }] })
    const wrapper = mount(CNProviderQuotaCell, { props: { account } })
    await wrapper.get('[data-test="cn-provider-quota-probe"]').trigger('click')
    await flushPromises()
    await wrapper.setProps({ account: {
      ...account,
      extra: { ...account.extra, zhipu_usage_updated_at: new Date(Date.now() + 1000).toISOString() }
    } })
    expect(wrapper.findAllComponents(UsageProgressBar).map(bar => bar.props('utilization'))).toEqual([0, 27])
  })

  it('keeps the new account snapshot when the previous account probe finishes late', async () => {
    let resolveProbe!: (value: unknown) => void
    queryQuota.mockReturnValue(new Promise(resolve => { resolveProbe = resolve }))
    const wrapper = mount(CNProviderQuotaCell, { props: { account } })
    await wrapper.get('[data-test="cn-provider-quota-probe"]').trigger('click')
    await wrapper.setProps({ account: {
      ...account, id: 8, extra: { ...account.extra, zhipu_5h_used_percent: 91 }
    } })
    expect(wrapper.findAllComponents(UsageProgressBar)[0].props('utilization')).toBe(91)
    resolveProbe({ success: true, tiers: [{ window: '5h', used_percent: 44 }] })
    await flushPromises()
    expect(wrapper.findAllComponents(UsageProgressBar)[0].props('utilization')).toBe(91)
    expect(wrapper.get('[data-test="cn-provider-quota-probe"]').attributes('disabled')).toBeUndefined()
  })
})
