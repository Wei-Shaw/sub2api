import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import CheckInView from '../CheckInView.vue'
import DashboardView from '../DashboardView.vue'
import zh from '@/i18n/locales/zh/checkin'
import dashboard from '@/i18n/locales/zh/dashboard'

const { getStatus, checkIn, refreshUser, showError, showWarning } = vi.hoisted(() => ({
  getStatus: vi.fn(), checkIn: vi.fn(), refreshUser: vi.fn(), showError: vi.fn(), showWarning: vi.fn()
}))

vi.mock('@/api/checkin', () => ({ default: { getStatus, checkIn } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError, showWarning }) }))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ refreshUser, user: { balance: 0 }, isSimpleMode: false })
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats: vi.fn().mockResolvedValue({}),
    getDashboardTrend: vi.fn().mockResolvedValue({ trend: [] }),
    getDashboardModels: vi.fn().mockResolvedValue({ models: [] }),
    getByDateRange: vi.fn().mockResolvedValue({ items: [] })
  }
}))

enableAutoUnmount(afterEach)

function status() {
  return {
    config: { enabled: true }, checked_today: false, today_reward: 0, total_reward: 0,
    recent_checkins: [], server_date: '2026-09-24', server_timezone: 'Asia/Shanghai'
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  getStatus.mockReset().mockImplementation(async () => status())
  refreshUser.mockReset().mockResolvedValue({ balance: 10 })
  checkIn.mockReset().mockResolvedValue({
    record: { id: 1, date: '2026-09-25', reward: 10, created_at: '2026-09-25T00:00:00+08:00' },
    already_checked: false, new_balance: 10
  })
})

describe.each([
  ['check-in page', CheckInView, '.inline-flex.h-12'],
  ['dashboard', DashboardView, '.dashboard-checkin-pill']
] as const)('%s', (_name, component, button) => {
  async function render() {
    const wrapper = mount(component, {
      global: {
        plugins: [createI18n({
          legacy: false, locale: 'zh', missingWarn: false, fallbackWarn: false,
          messageCompiler: (message) => (context) => String(message).replace(
            /\{(\w+)\}/g, (_match, key) => String(context.named(key))
          ),
          messages: { zh: { ...zh, ...dashboard } }
        })],
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          RouterLink: { template: '<a><slot /></a>' },
          DateRangePicker: true, Select: true, teleport: true
        }
      }
    })
    await flushPromises()
    return wrapper
  }

  it('reveals the confirmed reward and blessing only after check-in', async () => {
    // 即使旧后端仍返回规则，页面也不能展示范围或活动固定金额。
    getStatus.mockResolvedValueOnce({
      ...status(), config: { enabled: true, standard_min: 3, standard_max: 10, campaign_reward: 10 },
      next_reward_min: 3, next_reward_max: 10, campaign_eligible: true
    })
    const wrapper = await render()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('3-10')
    expect(wrapper.text()).not.toContain('10.00')
    if (component === CheckInView) expect(wrapper.text()).toContain('签到后揭晓')

    await wrapper.get(button).trigger('click')
    await flushPromises()
    expect(checkIn).toHaveBeenCalledTimes(1)
    const dialog = wrapper.get('[role="dialog"]')
    expect(dialog.text()).toContain('+$10.00')
    expect(dialog.text()).toContain('中秋节快乐！')
    const blessings = Object.entries(zh.checkIn)
      .filter(([key]) => key.startsWith('midAutumnBlessing'))
      .map(([, text]) => text)
    expect(blessings.some((text) => dialog.text().includes(text))).toBe(true)
    expect(showError).not.toHaveBeenCalled()
  })

  it('keeps the successful reward visible if the following refresh fails', async () => {
    const wrapper = await render()
    getStatus.mockRejectedValueOnce(new Error('refresh unavailable'))
    await wrapper.get(button).trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('中秋节快乐！')
    expect(wrapper.get(button).attributes('disabled')).toBeDefined()
    expect(showError).toHaveBeenCalledWith(zh.checkIn.refreshFailed.replace('{date}', '2026-09-25'))
  })

  it('does not announce another reward for an already completed check-in', async () => {
    checkIn.mockResolvedValueOnce({
      record: { id: 1, date: '2026-09-25', reward: 10, created_at: '' }, already_checked: true, new_balance: 10
    })
    const wrapper = await render()
    await wrapper.get(button).trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(showWarning).toHaveBeenCalledTimes(1)
  })

  it('keeps the reward hidden and reports a failed check-in', async () => {
    checkIn.mockRejectedValueOnce(new Error('签到失败，请稍后重试'))
    const wrapper = await render()
    await wrapper.get(button).trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(showError).toHaveBeenCalledWith('签到失败，请稍后重试')
    expect(wrapper.get(button).attributes('disabled')).toBeUndefined()
  })
})
