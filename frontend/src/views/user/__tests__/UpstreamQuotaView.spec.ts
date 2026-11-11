import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UpstreamQuotaView from '../UpstreamQuotaView.vue'

const api = vi.hoisted(() => ({ config: vi.fn(), snapshot: vi.fn() }))
vi.mock('@/api/upstreamQuota', () => ({ upstreamQuotaAPI: api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
const row = { id: 1, alias: 'Account #1', status: 'available', quota_state: 'unknown', windows: [], updated_at: null, current_concurrency: null, max_concurrency: 5 }

describe('UpstreamQuotaView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.config.mockResolvedValue({ enabled: true })
    api.snapshot.mockResolvedValue({ accounts: [row], cache_seconds: 180 })
  })
  it('loads only GPT initially and fetches other platforms on selection', async () => {
    const wrapper = mount(UpstreamQuotaView)
    await flushPromises()
    expect(api.snapshot.mock.calls).toEqual([['openai']])
    await wrapper.get('[data-platform="antigravity"]').trigger('click')
    await flushPromises()
    expect(api.snapshot.mock.calls).toEqual([['openai'], ['antigravity']])
    expect(wrapper.text()).toContain('upstreamQuota.unknown')
    expect(wrapper.text()).not.toContain('100%')
    wrapper.unmount()
  })
  it('does not fetch quotas while the feature is disabled', async () => {
    api.config.mockResolvedValue({ enabled: false })
    const wrapper = mount(UpstreamQuotaView)
    await flushPromises()
    expect(api.snapshot).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('upstreamQuota.disabledPage')
  })
  it('clears old quotas and shows a safe error on request failure', async () => {
    const wrapper = mount(UpstreamQuotaView)
    await flushPromises()
    api.snapshot.mockRejectedValue(new Error('secret provider error'))
    await wrapper.get('[data-action="refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('upstreamQuota.loadError')
    expect(wrapper.text()).not.toContain('Account #1')
    expect(wrapper.text()).not.toContain('secret provider error')
  })
  it('does not let a slow previous tab overwrite the selected platform', async () => {
    let finish!: (value: unknown) => void
    api.snapshot.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mount(UpstreamQuotaView)
    await flushPromises()
    await wrapper.get('[data-platform="gemini"]').trigger('click')
    await flushPromises()
    finish({ accounts: [{ ...row, alias: 'Old GPT' }] })
    await flushPromises()
    expect(wrapper.text()).not.toContain('Old GPT')
  })
})
