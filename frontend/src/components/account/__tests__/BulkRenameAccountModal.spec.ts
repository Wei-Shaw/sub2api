import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BulkRenameAccountModal from '../BulkRenameAccountModal.vue'

const mocks = vi.hoisted(() => ({ previewAccountRenames: vi.fn(), applyAccountRenames: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.resetAllMocks() })
const items = [{ account_id: 2, old_name: 'email@example.test', new_name: 'GPT-Free-01' }]
const open = (accountIds = [2]) => mount(BulkRenameAccountModal, {
  props: { show: true, accountIds },
  global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } }
})
const deferred = () => {
  let resolve!: (value: { items: typeof items }) => void
  const promise = new Promise<{ items: typeof items }>((r) => { resolve = r })
  return { promise, resolve }
}

describe('bulk account naming', () => {
  it('requires preview, sends the exact reviewed plan, and retries with the same operation key', async () => {
    mocks.previewAccountRenames.mockResolvedValue({ items })
    mocks.applyAccountRenames.mockRejectedValueOnce({ message: 'connection lost' }).mockResolvedValueOnce({ applied: 1 })
    const wrapper = open()
    expect(wrapper.get('[data-testid="rename-apply"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="rename-prefix"]').setValue('GPT-Free-')
    await wrapper.get('[data-testid="rename-preview"]').trigger('click')
    await flushPromises()
    expect(mocks.previewAccountRenames).toHaveBeenCalledWith({ account_ids: [2], prefix: 'GPT-Free-', suffix: '', start_number: 1, padding: 2, continue_sequence: false })
    expect(wrapper.text()).toContain('email@example.test')
    expect(wrapper.text()).toContain('GPT-Free-01')
    await wrapper.get('[data-testid="rename-apply"]').trigger('click')
    await flushPromises()
    const key = mocks.applyAccountRenames.mock.calls[0][1]
    expect(wrapper.text()).toContain('connection lost')
    await wrapper.get('[data-testid="rename-apply"]').trigger('click')
    await flushPromises()
    expect(mocks.applyAccountRenames).toHaveBeenLastCalledWith(items, key)
    expect(wrapper.emitted('updated')).toHaveLength(1)
  })

  it('invalidates a preview when rules or targets change', async () => {
    mocks.previewAccountRenames.mockResolvedValue({ items })
    const wrapper = open()
    await wrapper.get('[data-testid="rename-preview"]').trigger('click'); await flushPromises()
    await wrapper.get('[data-testid="rename-prefix"]').setValue('Different-')
    expect(wrapper.get('[data-testid="rename-apply"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="rename-preview"]').trigger('click'); await flushPromises()
    await wrapper.setProps({ accountIds: [3] })
    expect(wrapper.get('[data-testid="rename-apply"]').attributes('disabled')).toBeDefined()
  })

  it('ignores a late preview for an earlier rule', async () => {
    const pending = deferred()
    mocks.previewAccountRenames.mockReturnValueOnce(pending.promise)
    const wrapper = open()
    await wrapper.get('[data-testid="rename-preview"]').trigger('click')
    await wrapper.get('[data-testid="rename-prefix"]').setValue('New-')
    pending.resolve({ items }); await flushPromises()
    expect(wrapper.text()).not.toContain('email@example.test')
    expect(wrapper.get('[data-testid="rename-apply"]').attributes('disabled')).toBeDefined()
  })

  it('does not silently truncate an oversized selection', () => {
    const wrapper = open(Array.from({ length: 1001 }, (_, i) => i + 1))
    expect(wrapper.text()).toContain('admin.accounts.bulkRename.limit')
    expect(wrapper.get('[data-testid="rename-preview"]').attributes('disabled')).toBeDefined()
    expect(mocks.previewAccountRenames).not.toHaveBeenCalled()
  })
})
