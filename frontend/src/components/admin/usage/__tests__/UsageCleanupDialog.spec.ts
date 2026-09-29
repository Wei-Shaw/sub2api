import { describe, expect, it, vi, beforeEach } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import UsageCleanupDialog from '../UsageCleanupDialog.vue'

const { createCleanupTask, listCleanupTasks } = vi.hoisted(() => ({
  createCleanupTask: vi.fn(),
  listCleanupTasks: vi.fn()
}))

vi.mock('@/api/admin/usage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/admin/usage')>()),
  adminUsageAPI: {
    createCleanupTask,
    listCleanupTasks,
    cancelCleanupTask: vi.fn()
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const UsageFiltersStub = defineComponent({
  name: 'UsageFilters',
  props: ['modelValue', 'mode', 'startDate', 'endDate'],
  template: '<div class="usage-filters-stub" />'
})

const ConfirmDialogStub = defineComponent({
  name: 'ConfirmDialog',
  props: ['show'],
  emits: ['confirm', 'cancel'],
  template: '<div class="confirm-dialog-stub" />'
})

const mountDialog = () =>
  mount(UsageCleanupDialog, {
    props: {
      show: false,
      filters: {
        user_id: 7,
        billing_mode: 'image',
        upstream_model_mismatch: true,
        native_compaction_v2: true
      } as any,
      startDate: '2026-09-01',
      endDate: '2026-09-02'
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        ConfirmDialog: ConfirmDialogStub,
        Pagination: true,
        UsageFilters: UsageFiltersStub
      }
    }
  })

describe('UsageCleanupDialog', () => {
  beforeEach(() => {
    createCleanupTask.mockReset().mockResolvedValue({})
    listCleanupTasks.mockReset().mockResolvedValue({ items: [], total: 0 })
  })

  it('does not show or carry filters the cleanup API cannot apply', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const filters = wrapper.findComponent(UsageFiltersStub)
    expect(filters.props('mode')).toBe('cleanup')
    const model = filters.props('modelValue') as Record<string, unknown>
    expect(model.user_id).toBe(7)
    expect(model).not.toHaveProperty('billing_mode')
    expect(model).not.toHaveProperty('upstream_model_mismatch')
    expect(model).not.toHaveProperty('native_compaction_v2')
  })

  it('submits only the filters that are shown', async () => {
    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })
    await flushPromises()

    wrapper.findAllComponents(ConfirmDialogStub)[0].vm.$emit('confirm')
    await flushPromises()

    expect(createCleanupTask).toHaveBeenCalledTimes(1)
    expect(createCleanupTask.mock.calls[0][0]).toMatchObject({
      start_date: '2026-09-01',
      end_date: '2026-09-02',
      user_id: 7
    })
  })
})
