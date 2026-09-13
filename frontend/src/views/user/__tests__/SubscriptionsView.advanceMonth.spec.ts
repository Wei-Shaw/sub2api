import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import SubscriptionsView from '../SubscriptionsView.vue'

const api = vi.hoisted(() => ({
  getMySubscriptions: vi.fn(), previewAdvanceMonth: vi.fn(), advanceMonth: vi.fn(),
  previewAdvanceWeek: vi.fn(), advanceWeek: vi.fn(), setAutoAdvanceWeek: vi.fn(),
  fetchActiveSubscriptions: vi.fn(), showError: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/api/subscriptions', () => ({ default: api, ...api }))
vi.mock('@/stores/subscriptions', () => ({ useSubscriptionStore: () => api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => api }))
vi.mock('vue-router', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-router')>(),
  useRouter: () => ({ push: vi.fn() })
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))

const now = new Date('2026-09-13T10:00:00Z')
const sub = {
  id: 42, user_id: 1, group_id: 2, status: 'active', auto_advance_week: false,
  starts_at: '2026-08-16T10:00:00Z', expires_at: '2026-10-15T10:00:00Z',
  monthly_window_start: '2026-08-16T10:00:00Z', weekly_window_start: '2026-09-11T10:00:00Z',
  monthly_usage_usd: 120, weekly_usage_usd: 5,
  group: { name: 'Codex Mini', platform: 'openai', monthly_limit_usd: 120, weekly_limit_usd: 30 }
}
const preview = {
  subscription_id: 42, monthly_window_start: sub.monthly_window_start,
  weekly_window_start: sub.weekly_window_start, expires_at: sub.expires_at,
  new_expires_at: '2026-10-13T10:00:00Z', deduct_seconds: 172800
}
let wrapper: VueWrapper | undefined
const button = (label: string) => wrapper!.findAll('button').find(item => item.text() === label)!
async function render(data: typeof sub & { monthly_reset_at?: string | null } = sub) {
  api.getMySubscriptions.mockResolvedValue([data])
  wrapper = mount(SubscriptionsView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' }, Icon: true,
    BaseDialog: { props: ['show', 'title'], template: '<section v-if="show" role="dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></section>' }
  } } })
  await flushPromises()
}

describe('monthly quota advance confirmation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] })
    vi.setSystemTime(now)
    api.previewAdvanceMonth.mockResolvedValue(preview)
    api.advanceMonth.mockResolvedValue(preview)
    api.fetchActiveSubscriptions.mockResolvedValue(undefined)
  })
  afterEach(() => { wrapper?.unmount(); vi.useRealTimers() })

  it('previews without changing quota and allows cancellation', async () => {
    await render()
    await button('userSubscriptions.advanceMonth.action').trigger('click')
    await flushPromises()
    expect(api.previewAdvanceMonth).toHaveBeenCalledWith(42)
    expect(wrapper!.get('[role="dialog"]').text()).toContain('userSubscriptions.advanceMonth.warning')
    expect(api.advanceMonth).not.toHaveBeenCalled()
    await button('common.cancel').trigger('click')
    expect(wrapper!.find('[role="dialog"]').exists()).toBe(false)
    expect(api.advanceMonth).not.toHaveBeenCalled()
  })

  it('submits the observed window state once and refreshes usage after success', async () => {
    let resolve!: (value: typeof preview) => void
    api.advanceMonth.mockReturnValue(new Promise(r => { resolve = r }))
    await render()
    await button('userSubscriptions.advanceMonth.action').trigger('click')
    await flushPromises()
    await button('userSubscriptions.advanceWeek.confirm').trigger('click')
    expect(api.advanceMonth).toHaveBeenCalledTimes(1)
    expect(api.advanceMonth).toHaveBeenCalledWith(preview)
    expect(button('userSubscriptions.advanceWeek.action').attributes('disabled')).toBeDefined()
    expect(wrapper!.get('[role="switch"]').attributes('disabled')).toBeDefined()
    expect(button('common.processing').attributes('disabled')).toBeDefined()
    resolve(preview)
    await flushPromises()
    expect(api.getMySubscriptions).toHaveBeenCalledTimes(2)
    expect(api.fetchActiveSubscriptions).toHaveBeenCalledWith(true)
    expect(wrapper!.find('[role="dialog"]').exists()).toBe(false)
  })

  it('rejects a stale confirmation visibly and reloads the current subscription', async () => {
    api.advanceMonth.mockRejectedValue({ response: { status: 409 } })
    await render()
    await button('userSubscriptions.advanceMonth.action').trigger('click')
    await flushPromises()
    await button('userSubscriptions.advanceWeek.confirm').trigger('click')
    await flushPromises()
    expect(api.showError).toHaveBeenCalledWith('userSubscriptions.advanceMonth.failed')
    expect(api.getMySubscriptions).toHaveBeenCalledTimes(2)
    expect(wrapper!.find('[role="dialog"]').exists()).toBe(false)
  })

  it.each([
    ['before the corrected reset', '2026-10-01T08:00:00Z', false],
    ['at the corrected reset', '2026-10-01T12:00:00Z', true]
  ])('uses the server reset time for a legacy midnight anchor %s', async (_name, currentTime, disabled) => {
    vi.setSystemTime(new Date(currentTime))
    await render({
      ...sub,
      starts_at: '2026-09-01T12:00:00Z',
      monthly_window_start: '2026-09-01T00:00:00Z',
      monthly_reset_at: '2026-10-01T12:00:00Z',
      expires_at: '2026-10-31T12:00:00Z'
    })
    expect(button('userSubscriptions.advanceMonth.action').attributes('disabled') !== undefined).toBe(disabled)
    if (!disabled) {
      await button('userSubscriptions.advanceMonth.action').trigger('click')
      await flushPromises()
      expect(api.previewAdvanceMonth).toHaveBeenCalledWith(42)
    }
  })

  it.each([
    ['last month', { expires_at: '2026-09-15T10:00:00Z' }],
    ['no usage', { monthly_usage_usd: 0 }],
    ['expired', { expires_at: '2026-09-01T10:00:00Z' }]
  ])('disables reset for %s', async (_name, changes) => {
    await render({ ...sub, ...changes })
    expect(button('userSubscriptions.advanceMonth.action').attributes('disabled')).toBeDefined()
    expect(api.previewAdvanceMonth).not.toHaveBeenCalled()
  })
})
