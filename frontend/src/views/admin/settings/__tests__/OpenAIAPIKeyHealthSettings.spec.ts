import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpenAIAPIKeyHealthSettings from '../OpenAIAPIKeyHealthSettings.vue'

const mocks = vi.hoisted(() => ({ load: vi.fn(), save: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))
vi.mock('@/api/admin/settings', () => ({
  getOpenAIAPIKeyHealthBreakerSettings: mocks.load,
  updateOpenAIAPIKeyHealthBreakerSettings: mocks.save,
}))

describe('OpenAIAPIKeyHealthSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.load.mockResolvedValue({ enabled: false, window_minutes: 10, failure_threshold: 3, cooldown_minutes: 2 })
    mocks.save.mockImplementation(async (value) => value)
  })

  it('loads without enabling and saves exactly the four existing fields', async () => {
    const wrapper = mount(OpenAIAPIKeyHealthSettings)
    await flushPromises()
    expect(mocks.save).not.toHaveBeenCalled()
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('false')
    await wrapper.get('[role="switch"]').trigger('click')
    await wrapper.get('#health-failure_threshold').setValue(4)
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith({ enabled: true, window_minutes: 10, failure_threshold: 4, cooldown_minutes: 2 })
    expect(mocks.success).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('blocks invalid ranges and fractional counts', async () => {
    const wrapper = mount(OpenAIAPIKeyHealthSettings)
    await flushPromises()
    for (const value of [0, 10001, 1.5]) {
      await wrapper.get('#health-failure_threshold').setValue(value)
      expect(wrapper.get('.btn-primary').attributes('disabled')).toBeDefined()
    }
    expect(mocks.save).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not expose a default save form after a load error and can retry', async () => {
    mocks.load.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mount(OpenAIAPIKeyHealthSettings)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('loadFailed')
    expect(wrapper.find('.btn-primary').exists()).toBe(false)
    await wrapper.get('.btn-secondary').trigger('click')
    await flushPromises()
    expect(wrapper.find('#health-window_minutes').exists()).toBe(true)
    expect(mocks.save).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps the form on save failure and never reports success', async () => {
    mocks.save.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mount(OpenAIAPIKeyHealthSettings)
    await flushPromises()
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.error).toHaveBeenCalledOnce()
    expect(mocks.success).not.toHaveBeenCalled()
    expect(wrapper.get('.btn-primary').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })
})
