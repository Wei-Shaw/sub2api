import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UpstreamQuotaSettings from '../UpstreamQuotaSettings.vue'

const api = vi.hoisted(() => ({ settings: vi.fn(), save: vi.fn() }))
vi.mock('@/api/upstreamQuota', () => ({ upstreamQuotaAPI: api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('UpstreamQuotaSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.settings.mockResolvedValue({ enabled: false, account_ids: [1, 2] })
    api.save.mockResolvedValue(undefined)
  })
  it('requires an explicit save before changing sharing', async () => {
    const wrapper = mount(UpstreamQuotaSettings)
    await flushPromises()
    await wrapper.get('input[type="checkbox"]').setValue(true)
    expect(api.save).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith({ enabled: true, account_ids: [1, 2] })
  })
  it('does not overwrite settings with defaults after a failed initial load', async () => {
    api.settings.mockRejectedValue(new Error('unavailable'))
    const wrapper = mount(UpstreamQuotaSettings)
    await flushPromises()
    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    expect(api.save).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('upstreamQuota.admin.error')
  })
  it('rejects malformed IDs rather than silently sharing a different selection', async () => {
    const wrapper = mount(UpstreamQuotaSettings)
    await flushPromises()
    await wrapper.get('input[type="text"]').setValue('1, invalid, 2')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(api.save).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('upstreamQuota.admin.invalid')
  })
})
