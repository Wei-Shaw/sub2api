import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexTicketsView from '../CodexTicketsView.vue'
import zhSettings from '@/i18n/locales/zh/admin/settings'

const { getRuntime, getPool, getProvider, updateSettings } = vi.hoisted(() => ({ getRuntime: vi.fn(), getPool: vi.fn(), getProvider: vi.fn(), updateSettings: vi.fn() }))
vi.mock('@/api', () => ({ adminAPI: { settings: { getTicketHarvestStatus: getRuntime, getTicketProxyPool: getPool, getTicketProxyProvider: getProvider, updateSettings } } }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: { value: 'zh-CN' }, t: (key: string, values: Record<string, unknown> = {}) => {
  const messages = zhSettings.settings.gatewayForwarding
  const message = messages[key.split('.').at(-1) as keyof typeof messages]
  return typeof message === 'string' ? message.replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? name)) : key
} }) }))
const runtime = { enabled: true, refresh_before_seconds: 1200, ttl_seconds: 3600, models: ['gpt-6-astra'], harvest_proxy_configured: true }
const pool = { managed: true, count: 1948, removed_count: 252, failure_limit: 1 }
const wrappers: ReturnType<typeof mount>[] = []
function render() {
  const w = mount(CodexTicketsView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true, OnesProxyImport: true, TicketProxyPool: true } } })
  wrappers.push(w); return w
}
beforeEach(() => {
  vi.useFakeTimers(); vi.resetAllMocks()
  getRuntime.mockResolvedValue({ ...runtime }); getPool.mockResolvedValue({ ...pool }); getProvider.mockResolvedValue({ auto_refill: true, configured: true })
  updateSettings.mockResolvedValue({ openai_codex_ticket_enabled: false, openai_codex_ticket_harvest_proxy_configured: true })
})
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers() })

describe('dedicated ticket management', () => {
  it('shows live policy and polls without writing settings', async () => {
    const w = render(); await flushPromises()
    expect(w.get('[data-testid="ticket-refresh-minutes"]').text()).toBe('20 分钟')
    expect(w.get('[data-testid="ticket-pool-count"]').text()).toBe('1,948')
    await vi.advanceTimersByTimeAsync(15000)
    expect(getRuntime).toHaveBeenCalledTimes(2)
    expect(updateSettings).not.toHaveBeenCalled()
    w.unmount(); await vi.advanceTimersByTimeAsync(15000)
    expect(getRuntime).toHaveBeenCalledTimes(2)
  })
  it('changes only the master switch and leaves the previous state on a failed save', async () => {
    const w = render(); await flushPromises()
    updateSettings.mockRejectedValueOnce({ message: '保存失败' })
    await w.get('#ticket-master-switch').trigger('click'); await flushPromises()
    expect(updateSettings).toHaveBeenLastCalledWith({ openai_codex_ticket_enabled: false })
    expect(w.get('#ticket-master-switch').attributes('aria-checked')).toBe('true')
    expect(w.get('[role="alert"]').text()).toContain('保存失败')
    await w.get('#ticket-master-switch').trigger('click'); await flushPromises()
    expect(w.get('#ticket-master-switch').attributes('aria-checked')).toBe('false')
  })
  it('preserves proxy input through polling and clears it after a successful partial save', async () => {
    const w = render(); await flushPromises()
    await w.get('#ticket-single-proxy').setValue('socks5h://user:secret@proxy.example:1080')
    await vi.advanceTimersByTimeAsync(15000)
    expect(w.get<HTMLInputElement>('#ticket-single-proxy').element.value).toContain('secret')
    await w.get('details button').trigger('click'); await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ openai_codex_ticket_harvest_proxy_url: 'socks5h://user:secret@proxy.example:1080' })
    expect(w.get<HTMLInputElement>('#ticket-single-proxy').element.value).toBe('')
  })
  it('does not replace import results with a stale overview request', async () => {
    const w = render(); await flushPromises()
    let resolvePool!: (value: typeof pool) => void
    getPool.mockImplementationOnce(() => new Promise(resolve => { resolvePool = resolve }))
    await vi.advanceTimersByTimeAsync(15000)
    const importer = w.findComponent({ name: 'OnesProxyImport' })
    importer.vm.$emit('busyChange', true)
    importer.vm.$emit('imported', { ...pool, count: 3948 })
    importer.vm.$emit('busyChange', false)
    resolvePool({ ...pool }); await flushPromises()
    expect(w.get('[data-testid="ticket-pool-count"]').text()).toBe('3,948')
  })
})
