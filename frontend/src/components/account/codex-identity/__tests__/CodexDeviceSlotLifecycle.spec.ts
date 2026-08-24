import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { listSlots, finalizeSlots, showSuccess, showInfo, showError } = vi.hoisted(() => ({
  listSlots: vi.fn(),
  finalizeSlots: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      listCodexDeviceSlots: listSlots,
      finalizeCodexDrainingSlots: finalizeSlots,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showInfo, showError }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key, te: () => false }),
}))

import CodexDeviceSlotLifecycle from '../CodexDeviceSlotLifecycle.vue'

const activeSlot = {
  os_class: 'windows',
  canonical_surface: 'desktop',
  architecture: 'x86_64',
  catalog_version: 1,
  epoch: 2,
  slot_index: 0,
  state: 'active',
  proxy_id: 7,
  binding_count: 3,
} as const

const drainingSlot = {
  os_class: 'generic',
  canonical_surface: 'third_party',
  architecture: '',
  catalog_version: 1,
  epoch: 1,
  slot_index: 1,
  state: 'draining',
  binding_count: 0,
} as const

describe('CodexDeviceSlotLifecycle', () => {
  beforeEach(() => {
    listSlots.mockReset().mockResolvedValue([activeSlot, drainingSlot])
    finalizeSlots.mockReset().mockResolvedValue({ deleted: 1 })
    showSuccess.mockReset()
    showInfo.mockReset()
    showError.mockReset()
  })

  it('renders localized slot summaries with mobile-first layout', async () => {
    const wrapper = mount(CodexDeviceSlotLifecycle, {
      props: {
        accountId: 5,
        proxies: [{ id: 7, name: 'Tokyo', status: 'active' }],
      },
      global: { stubs: { Icon: true } },
    })
    await flushPromises()

    expect(listSlots).toHaveBeenCalledWith(5, true)
    expect(wrapper.text()).toContain('Windows / Desktop / x86_64')
    expect(wrapper.text()).toContain('Generic / Third-party')
    expect(wrapper.text()).toContain('Tokyo')
    expect(wrapper.text()).toContain('Direct connection')
    expect(wrapper.text()).toContain('Draining')
    const row = wrapper.get('li')
    expect(row.classes()).toContain('grid-cols-1')
    expect(row.classes()).toContain('sm:grid-cols-[minmax(0,1.4fr)_repeat(3,minmax(5rem,0.7fr))]')
  })

  it('requires confirmation and refreshes after safe finalize', async () => {
    listSlots.mockResolvedValueOnce([activeSlot, drainingSlot]).mockResolvedValueOnce([activeSlot])
    const wrapper = mount(CodexDeviceSlotLifecycle, {
      props: { accountId: 9 },
      global: { stubs: { Icon: true } },
    })
    await flushPromises()

    await wrapper.get('[data-testid="finalize-draining-slots"]').trigger('click')
    expect(wrapper.find('[role="alertdialog"]').exists()).toBe(true)
    await wrapper.get('[data-testid="confirm-finalize-draining"]').trigger('click')
    await flushPromises()

    expect(finalizeSlots).toHaveBeenCalledWith(9)
    expect(listSlots).toHaveBeenCalledTimes(2)
    expect(showSuccess).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="finalize-draining-slots"]').exists()).toBe(false)
  })

  it('moves keyboard focus into confirmation and restores it after cancel or finalize', async () => {
    const wrapper = mount(CodexDeviceSlotLifecycle, {
      attachTo: document.body,
      props: { accountId: 10 },
      global: { stubs: { Icon: true } },
    })
    await flushPromises()

    const trigger = wrapper.get('[data-testid="finalize-draining-slots"]')
    const triggerButton = trigger.element as HTMLButtonElement
    triggerButton.focus()
    await trigger.trigger('click')
    await flushPromises()

    const confirm = wrapper.get('[data-testid="confirm-finalize-draining"]')
    expect(document.activeElement).toBe(confirm.element)
    await wrapper.get('[data-testid="cancel-finalize-draining"]').trigger('click')
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('[data-testid="finalize-draining-slots"]').element)

    await wrapper.get('[data-testid="finalize-draining-slots"]').trigger('click')
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('[data-testid="confirm-finalize-draining"]').element)
    listSlots.mockResolvedValueOnce([activeSlot])
    await wrapper.get('[data-testid="confirm-finalize-draining"]').trigger('click')
    await flushPromises()

    expect(document.activeElement).toBe(wrapper.get('button[aria-label="Refresh device slots"]').element)
    wrapper.unmount()
  })

  it('shows an accessible error state and retry action', async () => {
    listSlots.mockRejectedValueOnce(new Error('slot service unavailable'))
    const wrapper = mount(CodexDeviceSlotLifecycle, {
      props: { accountId: 11 },
      global: { stubs: { Icon: true } },
    })
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('slot service unavailable')
    const retry = wrapper.get('[role="alert"] button')
    listSlots.mockResolvedValueOnce([])
    await retry.trigger('click')
    await flushPromises()

    expect(listSlots).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('No device slots')
  })
})
