import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import TempUnschedStatusModal from '../TempUnschedStatusModal.vue'
import type { Account } from '@/types'
const mocks = vi.hoisted(() => ({ getTempUnschedulableStatus: vi.fn(), recoverState: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('@/utils/format', () => ({ formatDateTime: () => 'date' }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => vi.clearAllMocks())
function deferred() {
  let resolve!: (value: unknown) => void
  let reject!: (value: unknown) => void
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
const active = (message: string) => ({ active: true, state: { until_unix: Date.now() / 1000 + 3600, error_message: message, rule_index: -1 } })
async function open() {
  const w = mount(TempUnschedStatusModal, { props: { show: false, account: { id: 1, name: 'first' } as Account },
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } } })
  await w.setProps({ show: true }); return w
}
describe('temporary unschedulable status requests', () => {
  it('does not let a late response replace the next account status', async () => {
    const old = deferred()
    mocks.getTempUnschedulableStatus.mockReturnValueOnce(old.promise).mockResolvedValueOnce(active('current-error'))
    const w = await open(); await w.setProps({ account: { id: 2, name: 'second' } as Account }); await flushPromises()
    old.resolve(active('old-error')); await flushPromises()
    expect(w.text()).toContain('current-error'); expect(w.text()).not.toContain('old-error')
  })
  it('keeps recovery disabled while loading a different account', async () => {
    const current = deferred()
    mocks.getTempUnschedulableStatus.mockResolvedValueOnce(active('old-error')).mockReturnValueOnce(current.promise)
    const w = await open(); await flushPromises()
    await w.setProps({ account: { id: 2 } as Account })
    expect(w.get('button.btn-primary').attributes('disabled')).toBeDefined()
    current.resolve(active('current-error')); await flushPromises()
    expect(w.get('button.btn-primary').attributes('disabled')).toBeUndefined()
  })
  it('ignores old errors without dismissing the current loading state', async () => {
    const old = deferred(); const current = deferred()
    mocks.getTempUnschedulableStatus.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const w = await open(); await w.setProps({ show: false }); await w.setProps({ show: true })
    old.reject(new Error('obsolete')); await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled(); expect(w.find('.animate-spin').exists()).toBe(true)
    current.resolve({ active: false }); await flushPromises()
    expect(w.text()).toContain('admin.accounts.tempUnschedulable.notActive')
  })
})
