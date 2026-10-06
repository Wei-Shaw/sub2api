import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GroupRPMOverridesModal from '../GroupRPMOverridesModal.vue'
import GroupRateMultipliersModal from '../GroupRateMultipliersModal.vue'
import type { AdminGroup } from '@/types'
const mocks = vi.hoisted(() => ({ list: vi.fn().mockResolvedValue({ items: [] }), getGroupRPMOverrides: vi.fn().mockResolvedValue([]), getGroupRateMultipliers: vi.fn().mockResolvedValue([]) }))
vi.mock('@/api/admin', () => ({ adminAPI: { users: mocks, groups: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
beforeEach(() => { vi.clearAllMocks(); vi.useFakeTimers() })
afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })
describe.each([GroupRPMOverridesModal, GroupRateMultipliersModal])('group modal lifecycle', (component) => {
  function open() {
    return mount(component, { props: { show: true, group: { id: 1, name: 'Group', platform: 'openai' } as AdminGroup },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true, PlatformIcon: true, Pagination: true } } })
  }
  it('removes its document click listener on unmount', () => {
    const add = vi.spyOn(document, 'addEventListener'); const remove = vi.spyOn(document, 'removeEventListener')
    const w = open()
    const handlers = add.mock.calls.filter(([event]) => event === 'click').map(([, handler]) => handler)
    expect(handlers.length).toBeGreaterThan(0)
    w.unmount()
    for (const handler of handlers) expect(remove).toHaveBeenCalledWith('click', handler)
  })
  it('cancels a queued search when navigating away', async () => {
    const w = open(); await w.get('input[type="text"]').setValue('alice')
    w.unmount(); await vi.advanceTimersByTimeAsync(300); await flushPromises()
    expect(mocks.list).not.toHaveBeenCalled()
  })
})
