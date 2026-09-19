import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexAccountTicketSettings from '../CodexAccountTicketSettings.vue'
import type { CodexAccountTicketStatus } from '@/api/admin/codexTickets'

const api = vi.hoisted(() => ({
  getCodexAccountTicket: vi.fn(), saveCodexAccountTicket: vi.fn(), harvestCodexAccountTicket: vi.fn()
}))
vi.mock('@/api/admin/codexTickets', () => api)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => `${key}${params ? JSON.stringify(params) : ''}`, locale: { value: 'zh-CN' } }) }))

function makeStatus(overrides: Partial<CodexAccountTicketStatus> = {}): CodexAccountTicketStatus {
  return { enabled: false, global_enabled: true, model: 'gpt-6-astra', ticket_plan: 'pro', target_length: 292, proxy_configured: false, proxy_display: '', fixed_proxy_configured: true, state: 'disabled', remaining_seconds: 0, last_error: '', attempts: 0, watchdog: { enabled: overrides.enabled === true && overrides.global_enabled !== false, trigger_count: 0 }, ...overrides }
}
const selector = (part: string) => `[data-testid="codex-account-ticket-${part}"]`
const wrappers: ReturnType<typeof mount>[] = []
function mountCard() {
  const wrapper = mount(CodexAccountTicketSettings, { props: { accountId: 4, visible: true } })
  wrappers.push(wrapper)
  return wrapper
}

describe('CodexAccountTicketSettings', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.resetAllMocks()
    api.getCodexAccountTicket.mockResolvedValue(makeStatus())
  })
  afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers() })

  it('opts in an account using the global pool without account proxy controls', async () => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ proxy_configured: true, proxy_display: 'pool.example:3000' }))
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get(selector('enabled')).attributes('aria-checked')).toBe('false')
    expect((wrapper.get(selector('plan')).element as HTMLSelectElement).value).toBe('pro')
    expect(wrapper.find(selector('proxy')).exists()).toBe(false)
    expect(wrapper.get(selector('global-pool')).text()).toContain('pool.example:3000')
    expect(api.harvestCodexAccountTicket).not.toHaveBeenCalled()
    await wrapper.get(selector('enabled')).trigger('click')
    api.saveCodexAccountTicket.mockResolvedValue(makeStatus({ enabled: true, proxy_configured: true, state: 'waiting' }))
    await wrapper.get(selector('save')).trigger('click')
    await flushPromises()
    expect(api.saveCodexAccountTicket).toHaveBeenCalledWith(4, { enabled: true, ticket_plan: 'pro' })
    expect(wrapper.get(selector('harvest')).attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).toContain('admin.accounts.stateTicket.saved')
  })

  it('requires a global pool before enabling but always permits disabling', async () => {
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get(selector('global-pool')).text()).toContain('globalPoolMissing')
    expect(wrapper.find('a[href="/admin/settings?tab=gateway"]').exists()).toBe(true)
    await wrapper.get(selector('enabled')).trigger('click')
    expect(wrapper.get(selector('save')).attributes('disabled')).toBeDefined()
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ enabled: true, state: 'error' }))
    await wrapper.setProps({ accountId: 5 })
    await flushPromises()
    await wrapper.get(selector('enabled')).trigger('click')
    expect(wrapper.get(selector('save')).attributes('disabled')).toBeUndefined()
  })

  it('allows account selection while the master is off but cannot harvest', async () => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ global_enabled: false, proxy_configured: true, proxy_display: 'host:3000' }))
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.find(selector('global-off')).exists()).toBe(true)
    await wrapper.get(selector('enabled')).trigger('click')
    api.saveCodexAccountTicket.mockResolvedValue(makeStatus({ enabled: true, global_enabled: false, proxy_configured: true, state: 'global_disabled' }))
    await wrapper.get(selector('save')).trigger('click')
    await flushPromises()
    expect(api.saveCodexAccountTicket).toHaveBeenCalledWith(4, { enabled: true, ticket_plan: 'pro' })
    expect(wrapper.get(selector('harvest')).attributes('disabled')).toBeDefined()
    expect(api.harvestCodexAccountTicket).not.toHaveBeenCalled()
  })

  it('polls status without replacing edits, stops when hidden and reloads on reopen', async () => {
    const wrapper = mountCard()
    await flushPromises()
    await wrapper.get(selector('enabled')).trigger('click')
    await wrapper.get(selector('plan')).setValue('team')
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ global_enabled: false, watchdog: { enabled: false, trigger_count: 2, last_reason: 'state_312', last_triggered_at: '2026-09-18T12:00:00Z' } }))
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(wrapper.get(selector('enabled')).attributes('aria-checked')).toBe('true')
    expect((wrapper.get(selector('plan')).element as HTMLSelectElement).value).toBe('team')
    expect(wrapper.find(selector('global-off')).exists()).toBe(true)
    expect(wrapper.get(selector('watchdog-event')).text()).toContain('watchdogState312')
    expect(wrapper.get(selector('watchdog-event')).text()).toContain('"count":2')
    await wrapper.setProps({ visible: false })
    const calls = api.getCodexAccountTicket.mock.calls.length
    await vi.advanceTimersByTimeAsync(9000)
    expect(api.getCodexAccountTicket).toHaveBeenCalledTimes(calls)
    await wrapper.setProps({ visible: true })
    await flushPromises()
    expect(api.getCodexAccountTicket).toHaveBeenCalledTimes(calls + 1)
    expect(wrapper.get(selector('enabled')).attributes('aria-checked')).toBe('false')
    expect((wrapper.get(selector('plan')).element as HTMLSelectElement).value).toBe('pro')
  })

  it('requires saving a manual Team selection and sends it without changing the proxy', async () => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ enabled: true, proxy_configured: true, state: 'ready', remaining_seconds: 1800 }))
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get(selector('save')).attributes('disabled')).toBeDefined()
    await wrapper.get(selector('plan')).setValue('team')
    expect(wrapper.get(selector('save')).attributes('disabled')).toBeUndefined()
    expect(wrapper.get(selector('harvest')).attributes('disabled')).toBeDefined()
    expect(api.saveCodexAccountTicket).not.toHaveBeenCalled()
    expect(api.harvestCodexAccountTicket).not.toHaveBeenCalled()
    api.saveCodexAccountTicket.mockResolvedValue(makeStatus({ enabled: true, ticket_plan: 'team', target_length: 332, proxy_configured: true, state: 'waiting' }))
    await wrapper.get(selector('save')).trigger('click')
    await flushPromises()
    expect(api.saveCodexAccountTicket).toHaveBeenCalledWith(4, { enabled: true, ticket_plan: 'team' })
    expect((wrapper.get(selector('plan')).element as HTMLSelectElement).value).toBe('team')
    expect(wrapper.get(selector('save')).attributes('disabled')).toBeDefined()
    expect(wrapper.get(selector('status')).text()).toContain('waiting')
  })

  it('loads the saved plan for each account and discards the previous account draft', async () => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ ticket_plan: 'team', target_length: 332 }))
    const wrapper = mountCard()
    await flushPromises()
    expect((wrapper.get(selector('plan')).element as HTMLSelectElement).value).toBe('team')
    await wrapper.get(selector('plan')).setValue('pro')
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ ticket_plan: 'team', target_length: 332 }))
    await wrapper.setProps({ accountId: 6 })
    await flushPromises()
    expect(api.getCodexAccountTicket).toHaveBeenLastCalledWith(6)
    expect((wrapper.get(selector('plan')).element as HTMLSelectElement).value).toBe('team')
    expect(wrapper.get(selector('save')).attributes('disabled')).toBeDefined()
  })

  it.each([
    { enabled: false, global_enabled: true },
    { enabled: true, global_enabled: false }
  ])('shows the watchdog as disabled without a separate toggle when saved settings are %j', async settings => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus(settings))
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get(selector('watchdog-status')).text()).toContain('watchdogDisabled')
    expect(wrapper.find(selector('watchdog-event')).exists()).toBe(false)
    expect(wrapper.get(selector('watchdog')).find('button, input, select').exists()).toBe(false)
  })

  it.each([
    ['model_mismatch', 'watchdogModelMismatch'],
    ['state_312', 'watchdogState312']
  ] as const)('keeps %s watchdog history visible after the ticket is ready again', async (reason, reasonKey) => {
    const triggeredAt = '2026-09-18T12:00:00Z'
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ enabled: true, state: 'ready', remaining_seconds: 3500, watchdog: { enabled: true, trigger_count: 3, last_reason: reason, last_triggered_at: triggeredAt } }))
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get(selector('watchdog-status')).text()).toContain('watchdogEnabled')
    const event = wrapper.get(selector('watchdog-event')).text()
    expect(event).toContain(reasonKey)
    expect(event).toContain('"count":3')
    expect(event).toContain(new Date(triggeredAt).toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }))
    expect(wrapper.get(selector('status')).text()).toContain('ready')
    expect(api.harvestCodexAccountTicket).not.toHaveBeenCalled()
  })

  it('acquires nonblocking and blocks repeat or stale-configuration requests', async () => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ enabled: true, proxy_configured: true, state: 'waiting' }))
    const wrapper = mountCard()
    await flushPromises()
    api.harvestCodexAccountTicket.mockResolvedValue(makeStatus({ enabled: true, proxy_configured: true, state: 'harvesting', attempts: 1 }))
    await wrapper.get(selector('harvest')).trigger('click')
    await flushPromises()
    expect(api.harvestCodexAccountTicket).toHaveBeenCalledWith(4)
    expect(wrapper.get(selector('harvest')).attributes('disabled')).toBeDefined()
    expect(wrapper.get(selector('status')).text()).toContain('harvesting')
    await wrapper.setProps({ proxyChanged: true })
    await wrapper.get(selector('enabled')).trigger('click')
    expect(wrapper.get(selector('save')).attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('fixedProxyUnsaved')
  })

  it('keeps an old ticket visibly usable while it is being renewed', async () => {
    const capturedAt = '2026-09-18T12:00:00Z'
    const expiresAt = '2026-09-18T13:00:00Z'
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({
      enabled: true,
      proxy_configured: true,
      state: 'harvesting',
      attempts: 1,
      remaining_seconds: 125,
      ticket_usable: true,
      refreshing: true,
      captured_at: capturedAt,
      expires_at: expiresAt
    }))
    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.get(selector('refreshing-usable')).text()).toContain('stateTicket.refreshing')
    expect(wrapper.get(selector('refreshing-usable')).text()).toContain('2m 05s')
    expect(wrapper.get(selector('saved-ticket')).text()).toContain(new Date(capturedAt).toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }))
    expect(wrapper.get(selector('saved-ticket')).text()).toContain(new Date(expiresAt).toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }))
    expect(wrapper.get(selector('saved-ticket-hint')).text()).toContain('stateTicket.savedTicketHint')
  })

  it('reports a renewal failure while continuing to use the saved ticket', async () => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({
      enabled: true,
      proxy_configured: true,
      state: 'ready',
      remaining_seconds: 240,
      ticket_usable: true,
      captured_at: '2026-09-18T12:00:00Z',
      expires_at: '2026-09-18T13:00:00Z',
      last_error: 'renewal failed',
      retry_after: '2026-09-18T12:05:00Z'
    }))
    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.get(selector('usable-error')).text()).toContain('stateTicket.refreshFailedUsable')
    expect(wrapper.get(selector('error')).text()).toContain('renewal failed')
    expect(wrapper.get(selector('retry-after')).text()).toContain('stateTicket.retryAfter')
    expect(wrapper.get(selector('retry-after')).text()).toContain(new Date('2026-09-18T12:05:00Z').toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }))
  })

  it('does not show saved-ticket guarantees after a failure with no usable ticket', async () => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ state: 'error', last_error: 'no ticket captured', retry_after: '2026-09-18T12:10:00Z' }))
    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.find(selector('saved-ticket')).exists()).toBe(false)
    expect(wrapper.find(selector('refreshing-usable')).exists()).toBe(false)
    expect(wrapper.find(selector('usable-error')).exists()).toBe(false)
    expect(wrapper.get(selector('error')).text()).toContain('no ticket captured')
    expect(wrapper.get(selector('retry-after')).text()).toContain('stateTicket.retryAfter')
  })

  it('does not expose internal API errors', async () => {
    api.getCodexAccountTicket.mockResolvedValue(makeStatus({ proxy_configured: true }))
    const wrapper = mountCard()
    await flushPromises()
    await wrapper.get(selector('enabled')).trigger('click')
    api.saveCodexAccountTicket.mockRejectedValue(new Error('http://name:secret@host:3000 is invalid'))
    await wrapper.get(selector('save')).trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('saveFailed')
    expect(wrapper.text()).not.toContain('secret')
  })
})
