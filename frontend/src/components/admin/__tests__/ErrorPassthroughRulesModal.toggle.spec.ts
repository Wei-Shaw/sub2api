import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import ErrorPassthroughRulesModal from '../ErrorPassthroughRulesModal.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), toggleEnabled: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { errorPassthrough: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => vi.restoreAllMocks())
beforeEach(() => vi.resetAllMocks())

function rule(enabled: boolean) {
  return { id: 7, name: 'Rule', enabled, priority: 1, error_codes: [429], keywords: [],
    platforms: [], match_mode: 'any', passthrough_code: true, passthrough_body: true, skip_monitoring: false }
}

async function openRules(enabled = true) {
  mocks.list.mockResolvedValue([rule(enabled)])
  const wrapper = mount(ErrorPassthroughRulesModal, {
    props: { show: false },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      ConfirmDialog: true, Icon: true
    } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('error passthrough enabled state', () => {
  it.each([true, false])('keeps the server state after two pending clicks (enabled=%s)', async (enabled) => {
    const finishes: Array<(value: unknown) => void> = []
    mocks.toggleEnabled.mockImplementation(() => new Promise(resolve => { finishes.push(resolve) }))
    const wrapper = await openRules(enabled)
    const toggle = wrapper.get('tbody button.relative')
    await toggle.trigger('click')
    await toggle.trigger('click')
    expect(mocks.toggleEnabled.mock.calls).toEqual([[7, !enabled], [7, !enabled]])
    for (const finish of finishes) {
      finish(rule(!enabled))
      await flushPromises()
      expect(toggle.classes().includes('bg-primary-600')).toBe(!enabled)
    }
  })

  it('allows sequential toggles in both directions', async () => {
    mocks.toggleEnabled.mockImplementation((_id: number, enabled: boolean) => Promise.resolve(rule(enabled)))
    const wrapper = await openRules()
    const toggle = wrapper.get('tbody button.relative')
    await toggle.trigger('click')
    await flushPromises()
    expect(toggle.classes()).not.toContain('bg-primary-600')
    await toggle.trigger('click')
    await flushPromises()
    expect(toggle.classes()).toContain('bg-primary-600')
    expect(mocks.toggleEnabled.mock.calls).toEqual([[7, false], [7, true]])
  })

  it('keeps the original state when saving fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.toggleEnabled.mockRejectedValue(new Error('offline'))
    const wrapper = await openRules()
    const toggle = wrapper.get('tbody button.relative')
    await toggle.trigger('click')
    await flushPromises()
    expect(toggle.classes()).toContain('bg-primary-600')
    expect(mocks.showError).toHaveBeenCalledWith('admin.errorPassthrough.failedToToggle')
  })
})
