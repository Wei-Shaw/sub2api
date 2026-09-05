import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SubscriptionBatchActionDialog from '../SubscriptionBatchActionDialog.vue'

const { batchAction, showSuccess, showWarning, showError } = vi.hoisted(() => ({
  batchAction: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: { batchAction }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showWarning, showError })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const mountDialog = (selectedIds: number[] = [4, 7]) => mount(SubscriptionBatchActionDialog, {
  props: { show: true, selectedIds },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        emits: ['close'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      Select: {
        props: ['modelValue', 'options', 'searchable', 'id'],
        emits: ['update:modelValue'],
        template: '<select :value="modelValue" data-test="action" @change="$emit(\'update:modelValue\', $event.target.value)"><option value="adjust">adjust</option><option value="reset_quota">reset</option><option value="revoke">revoke</option><option value="restore">restore</option><option value="permanent_delete">delete</option></select>'
      }
    }
  }
})

const successResult = {
  total_count: 2,
  succeeded_count: 2,
  skipped_count: 0,
  failed_count: 0,
  items: [
    { subscription_id: 4, status: 'succeeded' as const },
    { subscription_id: 7, status: 'succeeded' as const }
  ]
}

describe('SubscriptionBatchActionDialog', () => {
  beforeEach(() => {
    batchAction.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()
    showError.mockReset()
    batchAction.mockResolvedValue(successResult)
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(
      '11111111-1111-4111-8111-111111111111'
    )
  })

  it('submits an adjustment for all selected subscriptions', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-test="batch-days"]').setValue('15')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchAction).toHaveBeenCalledWith(
      { subscription_ids: [4, 7], action: 'adjust', days: 15 },
      'subscription-batch-11111111-1111-4111-8111-111111111111'
    )
    expect(wrapper.emitted('success')).toEqual([[successResult]])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('requires explicit confirmation for permanent deletion', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-test="batch-action-select"]').setValue('permanent_delete')

    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-test="danger-confirm"]').setValue(true)
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(batchAction).toHaveBeenCalledWith(
      { subscription_ids: [4, 7], action: 'permanent_delete' },
      expect.stringContaining('subscription-batch-')
    )
  })

  it('reuses the same idempotency key when an unchanged request is retried', async () => {
    batchAction.mockRejectedValueOnce(new Error('network timeout'))
    const wrapper = mountDialog()

    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchAction).toHaveBeenCalledTimes(2)
    expect(batchAction.mock.calls[1][1]).toBe(batchAction.mock.calls[0][1])
    expect(showError).toHaveBeenCalled()
  })

  it('blocks selections over the server limit', () => {
    const wrapper = mountDialog(Array.from({ length: 101 }, (_, index) => index + 1))

    expect(wrapper.text()).toContain('admin.subscriptions.batch.selectionLimit')
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
  })
})
