import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { SubscriptionBulkAction, SubscriptionBulkActionResult } from '@/api/admin/subscriptions'
import type { UserSubscription } from '@/types'
import BulkSubscriptionActionDialog from '../BulkSubscriptionActionDialog.vue'

const bulkAction = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { subscriptions: { bulkAction } } }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => params ? `${key} ${JSON.stringify(params)}` : key
  })
}))

function subscription(id: number): UserSubscription {
  return {
    id,
    user_id: id,
    group_id: 8,
    status: 'active',
    user: { email: `user${id}@example.com` },
    group: { name: 'Subscription group' }
  } as UserSubscription
}

const successResult: SubscriptionBulkActionResult = {
  success_count: 2,
  failed_count: 0,
  results: [
    { subscription_id: 1, success: true },
    { subscription_id: 2, success: true }
  ]
}

function mountDialog(action: SubscriptionBulkAction = 'extend', subscriptions = [subscription(1), subscription(2)]) {
  const wrapper = mount(BulkSubscriptionActionDialog, {
    props: { show: true, action, subscriptions },
    global: {
      stubs: {
        BaseDialog: {
          name: 'BaseDialog',
          props: ['show', 'title', 'closeOnEscape', 'showCloseButton'],
          emits: ['close'],
          template: '<section><h2>{{ title }}</h2><slot /><footer><slot name="footer" /></footer></section>'
        }
      }
    }
  })
  wrappers.push(wrapper)
  return wrapper
}

const wrappers: ReturnType<typeof mountDialog>[] = []
let adminId = 100

beforeEach(() => {
  bulkAction.mockReset()
  bulkAction.mockResolvedValue(successResult)
  localStorage.setItem('auth_user', JSON.stringify({ id: ++adminId }))
  sessionStorage.clear()
})

afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
})

describe('BulkSubscriptionActionDialog', () => {
  it.each([30, -7, 36500, -36500])('submits whole-day adjustments of %s and shows target identities', async days => {
    const wrapper = mountDialog()
    expect(wrapper.text()).toContain('user1@example.com')
    expect(wrapper.text()).toContain('Subscription group')
    await wrapper.get('input[type="number"]').setValue(days)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkAction).toHaveBeenCalledWith({ subscription_ids: [1, 2], action: 'extend', days }, expect.any(String))
    expect(wrapper.emitted('completed')).toEqual([[successResult]])
    expect(wrapper.find('button[type="submit"]').exists()).toBe(false)
  })

  it.each([0, 1.5, 36501, -36501, ''])('rejects invalid day value %s', async days => {
    const wrapper = mountDialog()
    await wrapper.get('input[type="number"]').setValue(days)
    await wrapper.get('form').trigger('submit')
    expect(wrapper.text()).toContain('admin.subscriptions.bulk.invalidDays')
    expect(bulkAction).not.toHaveBeenCalled()
  })

  it('defaults to all quota windows and validates at least one selected window', async () => {
    const wrapper = mountDialog('reset_quota')
    for (const checkbox of wrapper.findAll('input[type="checkbox"]')) {
      expect((checkbox.element as HTMLInputElement).checked).toBe(true)
      await checkbox.setValue(false)
    }
    await wrapper.get('form').trigger('submit')
    expect(bulkAction).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.subscriptions.bulk.selectWindow')

    await wrapper.get('input[name="weekly"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkAction).toHaveBeenCalledWith({
      subscription_ids: [1, 2], action: 'reset_quota', daily: false, weekly: true, monthly: false
    }, expect.any(String))
  })

  it.each(['revoke', 'restore'] as const)('submits %s without unrelated parameters', async action => {
    const wrapper = mountDialog(action)
    expect(wrapper.text()).toContain(`admin.subscriptions.bulk.${action}Hint`)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkAction).toHaveBeenCalledWith({ subscription_ids: [1, 2], action }, expect.any(String))
  })

  it.each([0, 101])('rejects a selection of %s subscriptions', async count => {
    const wrapper = mountDialog('revoke', Array.from({ length: count }, (_, index) => subscription(index + 1)))
    await wrapper.get('form').trigger('submit')
    expect(bulkAction).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  })

  it('prevents duplicate submits, parameter edits, and closing while submitting', async () => {
    let resolve!: (value: SubscriptionBulkActionResult) => void
    bulkAction.mockReturnValue(new Promise<SubscriptionBulkActionResult>(done => { resolve = done }))
    const wrapper = mountDialog()
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    const dialog = wrapper.findComponent({ name: 'BaseDialog' })
    dialog.vm.$emit('close')
    expect(bulkAction).toHaveBeenCalledTimes(1)
    expect(wrapper.get('input[type="number"]').attributes('disabled')).toBeDefined()
    expect(dialog.props('closeOnEscape')).toBe(false)
    expect(dialog.props('showCloseButton')).toBe(false)
    expect(wrapper.emitted('close')).toBeUndefined()
    resolve(successResult)
    await flushPromises()
    expect(dialog.props('closeOnEscape')).toBe(true)
  })

  it('shows partial failures and keeps the original target list after the parent refreshes', async () => {
    const partial: SubscriptionBulkActionResult = {
      success_count: 1, failed_count: 1,
      results: [{ subscription_id: 1, success: true }, { subscription_id: 2, success: false, error: 'Subscription is revoked' }]
    }
    bulkAction.mockResolvedValue(partial)
    const wrapper = mountDialog('reset_quota')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await wrapper.setProps({ subscriptions: [subscription(2)] })
    expect(wrapper.text()).toContain('user1@example.com')
    expect(wrapper.text()).toContain('user2@example.com')
    expect(wrapper.text()).toContain('Subscription is revoked')
    expect(wrapper.emitted('completed')).toEqual([[partial]])
    expect(wrapper.findAll('footer button')).toHaveLength(1)
    await wrapper.get('form').trigger('submit')
    expect(bulkAction).toHaveBeenCalledTimes(1)
    await wrapper.get('footer button').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('retries an unknown outcome with the original payload and key, even when props change', async () => {
    bulkAction.mockRejectedValueOnce({ status: 0, message: 'Connection lost' })
    const wrapper = mountDialog()
    await wrapper.get('input[type="number"]').setValue(-7)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('Connection lost')
    expect(wrapper.text()).toContain('admin.subscriptions.bulk.retryHint')
    expect(wrapper.get('input[type="number"]').attributes('disabled')).toBeDefined()
    expect(wrapper.findComponent({ name: 'BaseDialog' }).props('closeOnEscape')).toBe(true)
    await wrapper.setProps({ action: 'revoke', subscriptions: [subscription(3)] })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkAction.mock.calls[1]).toEqual(bulkAction.mock.calls[0])
    expect(bulkAction.mock.calls[1]![0]).toEqual({ subscription_ids: [1, 2], action: 'extend', days: -7 })
    expect(wrapper.emitted('completed')).toEqual([[successResult]])
  })

  it('retains a pending key across closing and reopening, then releases it on completion', async () => {
    bulkAction.mockRejectedValueOnce({ status: 503, message: 'Unavailable' })
    const first = mountDialog()
    await first.get('form').trigger('submit')
    await flushPromises()
    await first.get('footer button[type="button"]').trigger('click')
    expect(first.emitted('close')).toHaveLength(1)
    first.unmount()

    const retry = mountDialog('extend', [subscription(2), subscription(1)])
    await retry.get('form').trigger('submit')
    await flushPromises()
    expect(bulkAction.mock.calls[1]).toEqual(bulkAction.mock.calls[0])
    retry.unmount()

    const next = mountDialog()
    await next.get('form').trigger('submit')
    await flushPromises()
    expect(bulkAction.mock.calls[2]![1]).not.toBe(bulkAction.mock.calls[0]![1])
  })

  it('uses a new key after a definitive initial validation failure and allows correcting parameters', async () => {
    bulkAction.mockRejectedValueOnce({ status: 400, message: 'Invalid adjustment' })
    const wrapper = mountDialog()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('input[type="number"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('input[type="number"]').setValue(7)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkAction.mock.calls[1]![0].days).toBe(7)
    expect(bulkAction.mock.calls[1]![1]).not.toBe(bulkAction.mock.calls[0]![1])
  })

  it('keeps the pending key when a retry is rate limited after an unknown outcome', async () => {
    bulkAction.mockRejectedValueOnce({ status: 0 }).mockRejectedValueOnce({ status: 429 })
    const wrapper = mountDialog()
    for (let attempt = 0; attempt < 3; attempt++) {
      await wrapper.get('form').trigger('submit')
      await flushPromises()
    }
    expect(bulkAction.mock.calls[1]).toEqual(bulkAction.mock.calls[0])
    expect(bulkAction.mock.calls[2]).toEqual(bulkAction.mock.calls[0])
  })
})
