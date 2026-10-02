import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ZhipuResetCardActions from '../ZhipuResetCardActions.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import type { CNQuotaTier, ZhipuResetCards } from '@/api/admin/cnProviders'
import type { Account } from '@/types'

const { useResetCard } = vi.hoisted(() => ({
  useResetCard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    cnProviders: { useResetCard }
  }
}))

// 按真实 zh 文案渲染（含插值），便于断言用户真正看到的内容。
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const zh = (await vi.importActual<{ default: Record<string, unknown> }>('@/i18n/locales/zh')).default
  const lookup = (key: string): string | undefined => {
    const message = key
      .split('.')
      .reduce<unknown>((node, part) => (node as Record<string, unknown> | undefined)?.[part], zh)
    return typeof message === 'string' ? message : undefined
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const message = lookup(key)
        if (message === undefined) return key
        return message.replace(/\{(\w+)\}/g, (_, name: string) => String(params?.[name] ?? ''))
      }
    })
  }
})

const account = {
  id: 9,
  platform: 'zhipu',
  type: 'apikey',
  credentials: { account_mode: 'coding' },
  extra: {
    zhipu_reset_cards_updated_at: '2026-09-29T08:00:00Z',
    zhipu_week_reset_available: true,
    zhipu_week_reset_count: 2,
    zhipu_week_reset_expire_at: '2026-10-05 23:59:59',
    zhipu_5h_reset_available: true,
    zhipu_5h_reset_count: 1,
    zhipu_5h_reset_expire_at: '2026-10-10 12:00:00'
  }
} as unknown as Account

const busyTiers: CNQuotaTier[] = [
  { window: '5h', used_percent: 100 },
  { window: 'weekly', used_percent: 80 }
]

const mountActions = (props: Record<string, unknown> = {}) =>
  mount(ZhipuResetCardActions, { props: { account, tiers: busyTiers, ...props } })

const confirmUse = async (wrapper: ReturnType<typeof mountActions>, button: 'week' | '5h') => {
  await wrapper.get(`[data-test="zhipu-reset-${button}"]`).trigger('click')
  wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
  await flushPromises()
}

describe('ZhipuResetCardActions', () => {
  beforeEach(() => {
    useResetCard.mockReset()
  })

  it('reads the card counts and earliest expiry from the extra snapshot', () => {
    const wrapper = mountActions()

    expect(wrapper.get('[data-test="zhipu-reset-cards-week"]').text()).toBe('周卡 2')
    expect(wrapper.get('[data-test="zhipu-reset-cards-5h"]').text()).toBe('5h 卡 1')
    expect(wrapper.get('[data-test="zhipu-reset-cards-week"]').attributes('title')).toContain('2026-10-05 23:59')
    expect(wrapper.get('[data-test="zhipu-reset-cards-5h"]').attributes('title')).toContain('2026-10-10 12:00')
    expect(wrapper.get('[data-test="zhipu-reset-cards-expire"]').text()).toBe('最早 10-05 23:59 到期')
    expect(wrapper.find('[data-test="zhipu-reset-week"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="zhipu-reset-5h"]').exists()).toBe(true)
  })

  it('shows a placeholder and no buttons before the cards were ever queried', () => {
    const wrapper = mountActions({ account: { ...account, extra: {} } })

    expect(wrapper.find('[data-test="zhipu-reset-cards-unknown"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="zhipu-reset-cards-chip"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="zhipu-reset-week"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="zhipu-reset-5h"]').exists()).toBe(false)
  })

  it('prefers the latest card list over the snapshot and hides a type without cards', () => {
    const cards: ZhipuResetCards = {
      week_cards: [{ record_id: 302, grant_type: 'G', expire_time: '2026-10-20 23:59:59', available: true }],
      five_hour_cards: [],
      fetched_at: 1_790_000_000,
      persisted: true
    }
    const wrapper = mountActions({ cards })

    expect(wrapper.get('[data-test="zhipu-reset-cards-week"]').text()).toBe('周卡 1')
    expect(wrapper.get('[data-test="zhipu-reset-cards-5h"]').text()).toBe('5h 卡 0')
    expect(wrapper.get('[data-test="zhipu-reset-cards-expire"]').text()).toBe('最早 10-20 23:59 到期')
    expect(wrapper.find('[data-test="zhipu-reset-week"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="zhipu-reset-5h"]').exists()).toBe(false)
  })

  it('disables a card whose window usage is 0, but not when the window is unknown', () => {
    const wrapper = mountActions({
      tiers: [
        { window: '5h', used_percent: 0 },
        { window: 'weekly', used_percent: 35 }
      ]
    })
    const fiveHour = wrapper.get('[data-test="zhipu-reset-5h"]')
    expect(fiveHour.attributes('disabled')).toBeDefined()
    expect(fiveHour.attributes('title')).toBe('当前窗口用量为 0，无需重置')
    const week = wrapper.get('[data-test="zhipu-reset-week"]')
    expect(week.attributes('disabled')).toBeUndefined()
    expect(week.attributes('title')).toContain('消耗 1 张周卡')

    const unknown = mountActions({ tiers: null })
    expect(unknown.get('[data-test="zhipu-reset-week"]').attributes('disabled')).toBeUndefined()
    expect(unknown.get('[data-test="zhipu-reset-5h"]').attributes('disabled')).toBeUndefined()
  })

  it('asks for a danger confirmation that spells out the consequences', async () => {
    const wrapper = mountActions()

    await wrapper.get('[data-test="zhipu-reset-week"]').trigger('click')
    expect(useResetCard).not.toHaveBeenCalled()
    const dialog = wrapper.findComponent(ConfirmDialog)
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('danger')).toBe(true)
    expect(dialog.props('title')).toBe('确认使用周卡')
    const weekMessage = dialog.props('message') as string
    expect(weekMessage).toContain('消耗 1 张周卡')
    expect(weekMessage).toContain('2026-10-05 23:59')
    expect(weekMessage).toContain('同步重置 5 小时窗口')
    expect(weekMessage).toContain('不可撤销')

    dialog.vm.$emit('cancel')
    await flushPromises()
    expect(dialog.props('show')).toBe(false)
    expect(useResetCard).not.toHaveBeenCalled()

    await wrapper.get('[data-test="zhipu-reset-5h"]').trigger('click')
    const fiveHourMessage = dialog.props('message') as string
    expect(fiveHourMessage).toContain('2026-10-10 12:00')
    expect(fiveHourMessage).not.toContain('同步重置 5 小时窗口')
  })

  it('consumes the chosen card after confirmation and emits the result', async () => {
    const result = {
      provider: 'zhipu',
      success: true,
      reset_type: 'FIVE_HOUR',
      window: '5h',
      week_resets_left: 2,
      five_hour_resets_left: 0,
      fetched_at: 1,
      account_state_recovered: true
    }
    useResetCard.mockResolvedValue(result)
    const wrapper = mountActions()

    await confirmUse(wrapper, '5h')

    expect(useResetCard).toHaveBeenCalledTimes(1)
    expect(useResetCard).toHaveBeenCalledWith(account.id, { reset_type: 'FIVE_HOUR' })
    expect(wrapper.emitted('used')).toEqual([[result]])
    expect(wrapper.get('[data-test="zhipu-reset-success"]').text()).toBe('已使用 1 张5h 卡，剩余周卡 2 张、5h 卡 0 张')
  })

  it('shows a warning when the post-processing did not finish', async () => {
    useResetCard.mockResolvedValue({
      success: true,
      reset_type: 'WEEK',
      week_resets_left: 1,
      five_hour_resets_left: 1,
      account_state_recovered: false,
      warning_code: 'account_state_recovery_failed'
    })
    const wrapper = mountActions()

    await confirmUse(wrapper, 'week')

    expect(wrapper.find('[data-test="zhipu-reset-warning"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="zhipu-reset-success"]').exists()).toBe(false)
    expect(wrapper.emitted('used')).toHaveLength(1)
  })

  it('shows the upstream error, does not emit, and re-enables the button', async () => {
    useResetCard.mockRejectedValue({ status: 400, message: 'API error: 指定的重置次数不可用，请刷新后重试' })
    const wrapper = mountActions()

    await confirmUse(wrapper, 'week')

    expect(wrapper.get('[data-test="zhipu-reset-error"]').text()).toContain('指定的重置次数不可用')
    expect(wrapper.emitted('used')).toBeUndefined()
    expect(wrapper.get('[data-test="zhipu-reset-week"]').attributes('disabled')).toBeUndefined()
  })

  it('ignores a second confirmation while a reset is in flight', async () => {
    let resolve: (value: unknown) => void = () => {}
    useResetCard.mockReturnValue(new Promise((r) => { resolve = r }))
    const wrapper = mountActions()

    await confirmUse(wrapper, 'week')
    expect(wrapper.get('[data-test="zhipu-reset-week"]').attributes('disabled')).toBeDefined()
    wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
    await flushPromises()
    expect(useResetCard).toHaveBeenCalledTimes(1)

    resolve({ success: true, reset_type: 'WEEK', week_resets_left: 1, five_hour_resets_left: 1, account_state_recovered: true })
    await flushPromises()
    expect(wrapper.emitted('used')).toHaveLength(1)
  })
})
