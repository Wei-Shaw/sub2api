import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import RedeemView from '../RedeemView.vue'

const { listRedeemCodes, batchDelete, showSuccess, showError, showInfo } = vi.hoisted(() => ({
  listRedeemCodes: vi.fn(),
  batchDelete: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showInfo: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    redeem: {
      list: listRedeemCodes,
      generate: vi.fn(),
      delete: vi.fn(),
      batchDelete,
      batchUpdate: vi.fn(),
      exportCodes: vi.fn()
    },
    groups: { getAll: vi.fn().mockResolvedValue([]) }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError, showInfo })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: { count?: number }) =>
        params?.count === undefined ? key : `${key}:${params.count}`
    })
  }
})

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: `
    <select :value="modelValue ?? ''" @change="$emit('update:modelValue', $event.target.value); $emit('change', $event.target.value)">
      <option v-for="option in options" :key="String(option.value ?? '')" :value="option.value ?? ''">{{ option.label }}</option>
    </select>
  `
}

const ConfirmDialogStub = {
  props: ['show', 'title'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" :data-test="'dialog-' + title">
      <slot />
      <button data-test="confirm" @click="$emit('confirm')">confirm</button>
      <button data-test="cancel" @click="$emit('cancel')">cancel</button>
    </div>
  `
}

const code = (id: number) => ({
  id, code: `CODE-${id}`, type: 'balance', value: 1, status: 'unused',
  used_by: null, used_at: null, created_at: '2026-01-01T00:00:00Z', expires_at: null
})

// Stateful backend: `remaining` unused codes, at most 1000 per page (like response.go).
let remaining = 0
const pageOfUnused = (size: number) => {
  const n = Math.min(size, remaining)
  return { items: Array.from({ length: n }, (_, i) => code(i + 1)), total: remaining, page: 1, page_size: size, pages: 1 }
}

const openDeleteUnusedDialog = async () => {
  const wrapper = mount(RedeemView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: true,
        Pagination: true,
        ConfirmDialog: ConfirmDialogStub,
        BaseDialog: true,
        Select: SelectStub,
        GroupBadge: true,
        GroupOptionItem: true,
        Icon: true,
        Teleport: true
      }
    }
  })
  await flushPromises()
  const statusSelect = wrapper.findAll('select').find((s) => s.find('option[value="unused"]').exists())!
  await statusSelect.setValue('unused')
  await flushPromises()
  await wrapper.findAll('button').find((b) => b.text() === 'admin.redeem.deleteAllUnused')!.trigger('click')
  return wrapper
}

const dialogSelector = '[data-test="dialog-admin.redeem.deleteAllUnused"]'

describe('admin RedeemView delete all unused', () => {
  beforeEach(() => {
    listRedeemCodes.mockReset()
    batchDelete.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    showInfo.mockReset()
    listRedeemCodes.mockImplementation(async (_page: number, size: number) => pageOfUnused(Math.min(size, 1000)))
    batchDelete.mockImplementation(async (ids: number[]) => {
      remaining -= ids.length
      return { deleted: ids.length, message: 'ok' }
    })
  })

  it('keeps deleting in batches of 1000 until no unused code is left', async () => {
    remaining = 2500
    const wrapper = await openDeleteUnusedDialog()
    await wrapper.get(`${dialogSelector} [data-test="confirm"]`).trigger('click')
    await flushPromises()

    expect(batchDelete.mock.calls.map(([ids]) => ids.length)).toEqual([1000, 1000, 500])
    expect(remaining).toBe(0)
    expect(showSuccess).toHaveBeenCalledWith('admin.redeem.codesDeleted:2500')
    expect(showError).not.toHaveBeenCalled()
    expect(wrapper.find(dialogSelector).exists()).toBe(false)
  })

  it('reports nothing to delete when no unused code exists', async () => {
    remaining = 0
    const wrapper = await openDeleteUnusedDialog()
    await wrapper.get(`${dialogSelector} [data-test="confirm"]`).trigger('click')
    await flushPromises()

    expect(batchDelete).not.toHaveBeenCalled()
    expect(showInfo).toHaveBeenCalledWith('admin.redeem.noUnusedCodes')
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('stops at the round cap and says codes remain', async () => {
    remaining = 1_000_000
    const wrapper = await openDeleteUnusedDialog()
    await wrapper.get(`${dialogSelector} [data-test="confirm"]`).trigger('click')
    await flushPromises()

    expect(batchDelete).toHaveBeenCalledTimes(100)
    expect(showSuccess).toHaveBeenCalledWith('admin.redeem.codesDeleted:100000')
    expect(showError).toHaveBeenCalledWith('admin.redeem.failedToDeleteUnused')
  })

  it('stops when a batch deletes nothing instead of looping forever', async () => {
    remaining = 5
    batchDelete.mockResolvedValue({ deleted: 0, message: 'ok' })
    const wrapper = await openDeleteUnusedDialog()
    await wrapper.get(`${dialogSelector} [data-test="confirm"]`).trigger('click')
    await flushPromises()

    expect(batchDelete).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith('admin.redeem.failedToDeleteUnused')
  })

  it('cancel stops after the in-flight batch and reports what was deleted', async () => {
    remaining = 3000
    let finishBatch!: () => void
    batchDelete.mockImplementationOnce((ids: number[]) => new Promise((resolve) => {
      finishBatch = () => {
        remaining -= ids.length
        resolve({ deleted: ids.length, message: 'ok' })
      }
    }))
    const wrapper = await openDeleteUnusedDialog()
    await wrapper.get(`${dialogSelector} [data-test="confirm"]`).trigger('click')
    await flushPromises()
    await wrapper.get(`${dialogSelector} [data-test="cancel"]`).trigger('click')
    finishBatch()
    await flushPromises()

    expect(batchDelete).toHaveBeenCalledTimes(1)
    expect(showSuccess).toHaveBeenCalledWith('admin.redeem.codesDeleted:1000')
    expect(showError).not.toHaveBeenCalled()
    expect(wrapper.find(dialogSelector).exists()).toBe(false)
  })
})
