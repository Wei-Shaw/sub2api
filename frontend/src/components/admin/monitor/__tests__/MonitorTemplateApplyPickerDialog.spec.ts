import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AssociatedMonitorBrief } from '@/api/admin/channelMonitorTemplate'
import MonitorTemplateApplyPickerDialog from '../MonitorTemplateApplyPickerDialog.vue'

const mocks = vi.hoisted(() => ({ listAssociatedMonitors: vi.fn(), apply: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { channelMonitorTemplate: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.clearAllMocks(); mocks.apply.mockResolvedValue({ affected: 1 }) })

function deferred() {
  let resolve!: (value: { items: AssociatedMonitorBrief[] }) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<{ items: AssociatedMonitorBrief[] }>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
const items = (id: number) => ({ items: [{ id, name: `Monitor ${id}`, provider: 'openai', enabled: true } as AssociatedMonitorBrief] })
function mountDialog() {
  return mount(MonitorTemplateApplyPickerDialog, {
    props: { show: true, templateId: 1, templateName: 'Template' },
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } }
  })
}

describe('MonitorTemplateApplyPickerDialog request ordering', () => {
  it('applies the current template only to its own monitors after an older response', async () => {
    const old = deferred()
    mocks.listAssociatedMonitors.mockReturnValueOnce(old.promise).mockResolvedValueOnce(items(20))
    const wrapper = mountDialog()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, templateId: 2 })
    await flushPromises()
    old.resolve(items(10))
    await flushPromises()
    expect(wrapper.text()).toContain('Monitor 20')
    expect(wrapper.text()).not.toContain('Monitor 10')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.apply).toHaveBeenCalledWith(2, [20])
    expect(wrapper.emitted('applied')).toEqual([[1]])
  })

  it('keeps loading the current template when an older request fails', async () => {
    const old = deferred()
    const current = deferred()
    mocks.listAssociatedMonitors.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = mountDialog()
    await wrapper.setProps({ templateId: 2 })
    old.reject({ message: 'Old failure' })
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('common.loading')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    current.resolve(items(20))
    await flushPromises()
    expect(wrapper.text()).toContain('Monitor 20')
  })

  it.each(['close', 'unmount'])('ignores a pending failure after %s', async (action) => {
    const pending = deferred()
    mocks.listAssociatedMonitors.mockReturnValueOnce(pending.promise)
    const wrapper = mountDialog()
    if (action === 'close') await wrapper.setProps({ show: false })
    else wrapper.unmount()
    pending.reject({ message: 'No longer relevant' })
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
  })

  it('still reports errors from the current request', async () => {
    mocks.listAssociatedMonitors.mockRejectedValueOnce({ message: 'Current failure' })
    const wrapper = mountDialog()
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('Current failure')
    expect(wrapper.text()).not.toContain('common.loading')
  })
})
