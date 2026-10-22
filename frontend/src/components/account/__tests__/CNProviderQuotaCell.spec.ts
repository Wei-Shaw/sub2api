import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CNProviderQuotaCell from '../CNProviderQuotaCell.vue'
import UsageProgressBar from '../UsageProgressBar.vue'
import ZhipuResetCardActions from '../ZhipuResetCardActions.vue'
import type { Account } from '@/types'

const { queryQuota, listResetCards } = vi.hoisted(() => ({
  queryQuota: vi.fn(),
  listResetCards: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    cnProviders: { queryQuota, listResetCards }
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
    listResetCards.mockReset()
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

  describe('zhipu reset cards', () => {
    const cards = {
      week_cards: [{ record_id: 302, grant_type: 'G', expire_time: '2026-10-20 23:59:59', available: true }],
      five_hour_cards: [],
      fetched_at: 1_790_000_000,
      persisted: true
    }

    it('refreshes the quota and the card list together on a manual query', async () => {
      queryQuota.mockResolvedValue({ success: true, tiers: [{ window: 'weekly', used_percent: 40 }] })
      listResetCards.mockResolvedValue(cards)
      const wrapper = mount(CNProviderQuotaCell, { props: { account } })
      await flushPromises()

      const actions = wrapper.getComponent(ZhipuResetCardActions)
      expect(actions.props('cards')).toBeNull()
      expect(wrapper.find('[data-test="zhipu-reset-cards-unknown"]').exists()).toBe(true)

      await wrapper.get('[data-test="cn-provider-quota-probe"]').trigger('click')
      await flushPromises()

      expect(queryQuota).toHaveBeenCalledWith(account.id)
      expect(listResetCards).toHaveBeenCalledWith(account.id)
      expect(actions.props('cards')).toEqual(cards)
      expect(actions.props('tiers')).toEqual([{ window: 'weekly', used_percent: 40 }])
      expect(wrapper.find('[data-test="zhipu-reset-week"]').exists()).toBe(true)
    })

    it('keeps the quota result and reports a failed card refresh', async () => {
      queryQuota.mockResolvedValue({ success: true, tiers: [{ window: 'weekly', used_percent: 40 }] })
      listResetCards.mockRejectedValue({ message: 'CN_QUOTA_RESET_AUTH_FAILED' })
      const wrapper = mount(CNProviderQuotaCell, { props: { account } })
      await flushPromises()

      await wrapper.get('[data-test="cn-provider-quota-probe"]').trigger('click')
      await flushPromises()

      expect(wrapper.text()).toContain('40%')
      expect(wrapper.text()).toContain('admin.accounts.cnProviders.resetCardsFailed')
    })

    it('does not fetch cards on the automatic probe after mount', async () => {
      queryQuota.mockResolvedValue({ success: true, tiers: [] })
      const staleAccount = { ...account, id: 70, extra: {} } as Account
      mount(CNProviderQuotaCell, { props: { account: staleAccount } })
      await flushPromises()

      expect(queryQuota).toHaveBeenCalledWith(70)
      expect(listResetCards).not.toHaveBeenCalled()
    })

    it('applies the probe and card list returned by a used card', async () => {
      const wrapper = mount(CNProviderQuotaCell, { props: { account } })
      await flushPromises()

      const actions = wrapper.getComponent(ZhipuResetCardActions)
      actions.vm.$emit('used', {
        success: true,
        cards,
        probe: {
          success: true,
          tiers: [
            { window: '5h', used_percent: 0 },
            { window: 'weekly', used_percent: 0 }
          ]
        }
      })
      await flushPromises()

      expect(actions.props('cards')).toEqual(cards)
      const bars = wrapper.findAllComponents(UsageProgressBar)
      expect(bars.map((bar) => bar.props('utilization'))).toEqual([0, 0])
    })

    it.each([
      ['team plan', { account_mode: 'coding', zhipu_organization: 'org-1' }],
      ['international site', { account_mode: 'coding', base_url: 'https://api.z.ai/api/coding/paas/v4' }],
      ['custom relay', { account_mode: 'coding', base_url: 'https://relay.example.com/v1' }]
    ])('hides reset cards for a %s account', async (_, credentials) => {
      queryQuota.mockResolvedValue({ success: true, tiers: [] })
      const wrapper = mount(CNProviderQuotaCell, { props: { account: { ...account, credentials } as Account } })
      await flushPromises()

      expect(wrapper.findComponent(ZhipuResetCardActions).exists()).toBe(false)
      await wrapper.get('[data-test="cn-provider-quota-probe"]').trigger('click')
      await flushPromises()
      expect(listResetCards).not.toHaveBeenCalled()
    })
  })
})
