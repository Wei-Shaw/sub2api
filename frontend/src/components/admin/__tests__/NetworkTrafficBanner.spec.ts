import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import NetworkTrafficBanner from '../NetworkTrafficBanner.vue'
import { getNetworkTraffic } from '@/api/admin/networkTraffic'

vi.mock('@/api/admin/networkTraffic', () => ({ getNetworkTraffic: vi.fn() }))
const getTraffic = vi.mocked(getNetworkTraffic)
const snapshot = { available: true, ready: true, timestamp: 10000, upload_bps: 2048, download_bps: 4096, interfaces: ['eth0'], total_sent: 1000000, total_received: 2000000, samples: [] }
const wrappers: ReturnType<typeof mount>[] = []
function render() {
  const wrapper = mount(NetworkTrafficBanner, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: {} })] } })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => { vi.useFakeTimers(); localStorage.clear(); getTraffic.mockReset(); getTraffic.mockResolvedValue(snapshot); Object.defineProperty(document, 'hidden', { configurable: true, value: false }) })
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers() })

describe('NetworkTrafficBanner', () => {
  it('shows live values, marks failures, and recovers without stale live rates', async () => {
    const wrapper = render(); await flushPromises()
    expect(wrapper.get('[data-testid="download-rate"]').text()).toBe('4.0 KB/s')
    getTraffic.mockRejectedValueOnce(new Error('offline'))
    await vi.advanceTimersByTimeAsync(2000); await flushPromises()
    expect(wrapper.text()).toContain('连接中断')
    expect(wrapper.get('[data-testid="download-rate"]').text()).toBe('—')
    await vi.advanceTimersByTimeAsync(2000); await flushPromises()
    expect(wrapper.get('[data-testid="download-rate"]').text()).toBe('4.0 KB/s')
  })
  it('stops polling in a hidden tab and after unmount', async () => {
    const wrapper = render(); await flushPromises()
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(10000)
    expect(getTraffic).toHaveBeenCalledTimes(1)
    Object.defineProperty(document, 'hidden', { configurable: true, value: false })
    document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    expect(getTraffic).toHaveBeenCalledTimes(2)
    wrapper.unmount(); wrappers.splice(wrappers.indexOf(wrapper), 1)
    await vi.advanceTimersByTimeAsync(10000)
    expect(getTraffic).toHaveBeenCalledTimes(2)
  })
})
