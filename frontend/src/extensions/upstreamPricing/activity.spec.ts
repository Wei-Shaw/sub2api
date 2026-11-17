import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountWalletCell from './AccountWalletCell.vue'
import ActivityStatusPanel from './ActivityStatusPanel.vue'
import type { ActivityState } from '.'

const api = vi.hoisted(() => ({ activity: vi.fn() }))
vi.mock('./api', () => ({ upstreamPricingAPI: api }))
const fixture = (): ActivityState => ({ configured: true, last_success: '2026-11-17T02:00:00Z', last_attempt: '2026-11-17T02:00:00Z', next_attempt: '2026-11-17T02:05:00Z', last_error: '', wallet_balance: '0', wallet_checked_at: new Date().toISOString(), wallet_error: '' })
beforeEach(() => { vi.useFakeTimers(); api.activity.mockReset(); api.activity.mockResolvedValue(fixture()) })
afterEach(() => { vi.useRealTimers() })

describe('cached upstream wallet', () => {
  it('preserves zero and negative balances, and explains that a shared station wallet is not local credit', async () => {
    const wrapper = mount(AccountWalletCell, { props: { accountId: 7, enabled: true } }); await flushPromises()
    expect(wrapper.text()).toBe('$0')
    expect(wrapper.attributes('title')).toContain('同一站点钱包可能被多个调用账号共用')
    api.activity.mockResolvedValue({ ...fixture(), wallet_balance: '-2.125' })
    await wrapper.setProps({ refreshToken: 1 }); await flushPromises()
    expect(wrapper.text()).toBe('$-2.125')
    expect(wrapper.get('span').classes()).toContain('text-red-600')
    wrapper.unmount()
  })
  it('does not turn missing wallet data into free credit or query disabled accounts', async () => {
    const wrapper = mount(AccountWalletCell, { props: { accountId: 7, enabled: false } }); await flushPromises()
    expect(api.activity).not.toHaveBeenCalled(); expect(wrapper.text()).toBe('—')
    api.activity.mockResolvedValue({ ...fixture(), wallet_balance: null, wallet_checked_at: null })
    await wrapper.setProps({ enabled: true }); await flushPromises()
    expect(wrapper.text()).toContain('等待首次同步'); expect(wrapper.text()).not.toContain('$0')
    wrapper.unmount()
  })
  it('marks retained data as stale after a failed local snapshot reload and never shows raw errors', async () => {
    const wrapper = mount(AccountWalletCell, { props: { accountId: 7, enabled: true } }); await flushPromises()
    api.activity.mockRejectedValue(new Error('synthetic-private-diagnostic'))
    await vi.advanceTimersByTimeAsync(60000); await flushPromises()
    expect(wrapper.text()).toContain('$0'); expect(wrapper.text()).toContain('旧值')
    expect(wrapper.html()).not.toContain('synthetic-private')
    wrapper.unmount()
  })
  it('discards a delayed response when the selected account changes', async () => {
    let resolve!: (state: ActivityState) => void
    api.activity.mockReturnValueOnce(new Promise(done => { resolve = done }))
    const wrapper = mount(AccountWalletCell, { props: { accountId: 7, enabled: true } })
    api.activity.mockResolvedValue({ ...fixture(), wallet_balance: '12' })
    await wrapper.setProps({ accountId: 8 }); await flushPromises()
    resolve({ ...fixture(), wallet_balance: '999' }); await flushPromises()
    expect(wrapper.text()).toBe('$12')
    wrapper.unmount()
  })
  it('shows schedule and failures separately from price scheduling', async () => {
    api.activity.mockResolvedValue({ ...fixture(), last_error: '明细同步失败', wallet_error: '钱包刷新失败' })
    const wrapper = mount(ActivityStatusPanel, { props: { accountId: 7 } }); await flushPromises()
    expect(wrapper.text()).toContain('每 5 分钟')
    expect(wrapper.text()).toContain('无本地调用则跳过')
    expect(wrapper.text()).toContain('明细同步失败'); expect(wrapper.text()).toContain('钱包刷新失败')
    wrapper.unmount()
  })
})
